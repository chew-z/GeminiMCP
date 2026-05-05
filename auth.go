package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Bounded dedup of auth WARN logs. A misconfigured client can otherwise emit
// dozens of identical lines in seconds, drowning out real auth incidents. The
// state is bounded by maxAuthLogKeys + authLogTTL so a public-facing server
// cannot be turned into a memory-pressure vector by high-cardinality input.
const (
	maxAuthLogKeys = 1024
	authLogTTL     = 5 * time.Minute
	authLogWindow  = 60 * time.Second
)

type authLogState struct {
	firstSeen   time.Time
	lastEmitted time.Time
	suppressed  int
}

// AuthMiddleware handles JWT-based authentication for HTTP transport
type AuthMiddleware struct {
	secretKey []byte
	enabled   bool
	logger    Logger

	// Injectable clock so tests can step time deterministically.
	nowFn func() time.Time

	logMu    sync.Mutex
	logState map[string]*authLogState
}

// Claims represents JWT token claims
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(secretKey string, enabled bool, logger Logger) *AuthMiddleware {
	return &AuthMiddleware{
		secretKey: []byte(secretKey),
		enabled:   enabled,
		logger:    logger,
		logState:  make(map[string]*authLogState),
	}
}

// now returns the current time, honoring the injectable clock if set.
func (a *AuthMiddleware) now() time.Time {
	if a.nowFn != nil {
		return a.nowFn()
	}
	return time.Now()
}

// remoteHost returns the host portion of a "host:port" address, or the input
// as-is if it can't be split. Used as a stable dedup key so transient ephemeral
// ports don't fragment the per-client rate limit.
func remoteHost(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// logAuthWarn emits a WARN log deduplicated by (host, errClass) within
// authLogWindow. State is bounded by maxAuthLogKeys with TTL eviction; on
// overflow the entry with the oldest lastEmitted is dropped.
func (a *AuthMiddleware) logAuthWarn(remoteAddr, errClass, format string, args ...any) {
	key := remoteHost(remoteAddr) + "|" + errClass
	now := a.now()

	a.logMu.Lock()

	// Opportunistic TTL sweep — cheap because we already hold the lock and the
	// map is hard-capped at maxAuthLogKeys.
	for k, st := range a.logState {
		if now.Sub(st.lastEmitted) >= authLogTTL {
			delete(a.logState, k)
		}
	}

	if st, ok := a.logState[key]; ok {
		if now.Sub(st.lastEmitted) < authLogWindow {
			st.suppressed++
			a.logMu.Unlock()
			return
		}
		suppressed := st.suppressed
		st.lastEmitted = now
		st.suppressed = 0
		a.logMu.Unlock()

		if suppressed > 0 {
			a.logger.Warn(format+" (×%d suppressed in last %s)", append(args, suppressed, authLogWindow)...)
		} else {
			a.logger.Warn(format, args...)
		}
		return
	}

	// New key — enforce the cap by evicting the oldest entry by lastEmitted.
	if len(a.logState) >= maxAuthLogKeys {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, st := range a.logState {
			if first || st.lastEmitted.Before(oldestTime) {
				oldestKey = k
				oldestTime = st.lastEmitted
				first = false
			}
		}
		delete(a.logState, oldestKey)
	}

	a.logState[key] = &authLogState{firstSeen: now, lastEmitted: now}
	a.logMu.Unlock()

	a.logger.Warn(format, args...)
}

// extractTokenFromHeader extracts the JWT token from Authorization header
// Handles case-insensitivity and multiple spaces robustly
func extractTokenFromHeader(authHeader string) string {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// HTTPContextFunc returns a middleware function compatible with mcp-go
func (a *AuthMiddleware) HTTPContextFunc(
	next func(ctx context.Context, r *http.Request) context.Context,
) func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		// If authentication is disabled, just call the next middleware
		if !a.enabled {
			return next(ctx, r)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			a.logAuthWarn(r.RemoteAddr, "missing_header", "Missing authorization header from %s", r.RemoteAddr)
			ctx = context.WithValue(ctx, authErrorKey, "missing_token")
			return next(ctx, r)
		}

		tokenString := extractTokenFromHeader(authHeader)
		if tokenString == "" {
			a.logAuthWarn(r.RemoteAddr, "invalid_header", "Invalid authorization header from %s", r.RemoteAddr)
			// Set authentication error in context instead of failing the request
			ctx = context.WithValue(ctx, authErrorKey, "invalid_token")
			return next(ctx, r)
		}

		// Validate JWT token
		claims, err := a.validateJWT(tokenString)
		if err != nil {
			var authErr string
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				authErr = "expired_token"
			case errors.Is(err, jwt.ErrTokenNotValidYet):
				authErr = "token_not_valid_yet"
			case errors.Is(err, jwt.ErrTokenMalformed):
				authErr = "malformed_token"
			case errors.Is(err, jwt.ErrTokenSignatureInvalid):
				authErr = "invalid_signature"
			default:
				authErr = "invalid_token"
			}
			a.logAuthWarn(r.RemoteAddr, authErr, "Invalid token from %s: %v", r.RemoteAddr, err)
			ctx = context.WithValue(ctx, authErrorKey, authErr)
			return next(ctx, r)
		}

		a.logger.Info("Authenticated user %s (%s) from %s", claims.Username, claims.Role, r.RemoteAddr)
		exp := "none"
		if claims.ExpiresAt != nil {
			exp = claims.ExpiresAt.Time.Format(time.RFC3339)
		}
		a.logger.Debug("auth ok: subject=%s username=%s role=%s exp=%s from=%s",
			claims.UserID, claims.Username, claims.Role, exp, r.RemoteAddr)

		// Add user to request context
		ctx = context.WithValue(ctx, authenticatedKey, true)
		ctx = context.WithValue(ctx, userIDKey, claims.UserID)
		ctx = context.WithValue(ctx, usernameKey, claims.Username)
		ctx = context.WithValue(ctx, userRoleKey, claims.Role)

		return next(ctx, r)
	}
}

// validateJWT validates a JWT token and returns the claims
func (a *AuthMiddleware) validateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// Pin to specific HS256 algorithm for enhanced security
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.secretKey, nil
	},
		jwt.WithIssuer("gemini-mcp"),
		jwt.WithAudience("gemini-mcp-user"),
		jwt.WithLeeway(60*time.Second),
	)

	if err != nil {
		return nil, err // The library handles various parsing/validation errors
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// GenerateToken generates a JWT token for a user (utility function for testing/setup)
func (a *AuthMiddleware) GenerateToken(userID, username, role string, expirationHours int) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "gemini-mcp",
			Audience:  jwt.ClaimStrings{"gemini-mcp-user"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expirationHours) * time.Hour)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secretKey)
}

// isAuthenticated checks if the request context contains valid authentication
func isAuthenticated(ctx context.Context) bool {
	if auth, ok := ctx.Value(authenticatedKey).(bool); ok && auth {
		return true
	}
	return false
}

// getAuthError returns any authentication error from the context
func getAuthError(ctx context.Context) string {
	if err, ok := ctx.Value(authErrorKey).(string); ok {
		return err
	}
	return ""
}

// getUserInfo extracts user information from the authenticated context
func getUserInfo(ctx context.Context) (userID, username, role string) {
	if userID, ok := ctx.Value(userIDKey).(string); ok {
		if username, ok := ctx.Value(usernameKey).(string); ok {
			if role, ok := ctx.Value(userRoleKey).(string); ok {
				return userID, username, role
			}
		}
	}
	return "", "", ""
}

// CreateTokenCommand creates a command-line utility to generate tokens
func CreateTokenCommand(secretKey, userID, username, role string, expirationHours int) {
	if secretKey == "" {
		fmt.Fprintln(os.Stderr, "Error: GEMINI_AUTH_SECRET_KEY environment variable is required")
		return
	}

	logger := NewLogger(LevelInfo)
	auth := NewAuthMiddleware(secretKey, true, logger)

	token, err := auth.GenerateToken(userID, username, role, expirationHours)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Generated JWT token:\n%s\n\n", token)
	fmt.Fprintf(os.Stderr, "Token details:\n")
	fmt.Fprintf(os.Stderr, "  User ID: %s\n", userID)
	fmt.Fprintf(os.Stderr, "  Username: %s\n", username)
	fmt.Fprintf(os.Stderr, "  Role: %s\n", role)
	fmt.Fprintf(os.Stderr, "  Expires: %s\n", time.Now().Add(time.Duration(expirationHours)*time.Hour).Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "\nTo use this token, include it in HTTP requests:\n")
	fmt.Fprintf(os.Stderr, "  Authorization: Bearer %s\n", token)
}
