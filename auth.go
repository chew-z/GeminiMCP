package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware handles JWT-based authentication for HTTP transport
type AuthMiddleware struct {
	secretKey []byte
	enabled   bool
	logger    Logger
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
	}
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
func (a *AuthMiddleware) HTTPContextFunc(next func(ctx context.Context, r *http.Request) context.Context) func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		// If authentication is disabled, just call the next middleware
		if !a.enabled {
			return next(ctx, r)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			a.logger.Warn("Missing authorization header from %s", r.RemoteAddr)
			ctx = context.WithValue(ctx, authErrorKey, "missing_token")
			return next(ctx, r)
		}

		tokenString := extractTokenFromHeader(authHeader)
		if tokenString == "" {
			a.logger.Warn("Invalid authorization header from %s", r.RemoteAddr)
			// Set authentication error in context instead of failing the request
			ctx = context.WithValue(ctx, authErrorKey, "invalid_token")
			return next(ctx, r)
		}

		// Validate JWT token
		claims, err := a.validateJWT(tokenString)
		if err != nil {
			a.logger.Warn("Invalid token from %s: %v", r.RemoteAddr, err)
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
			ctx = context.WithValue(ctx, authErrorKey, authErr)
			return next(ctx, r)
		}

		a.logger.Info("Authenticated user %s (%s) from %s", claims.Username, claims.Role, r.RemoteAddr)

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
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
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
