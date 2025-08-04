package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// AuthMiddleware handles JWT-based authentication for HTTP transport
type AuthMiddleware struct {
	secretKey []byte
	enabled   bool
	logger    Logger
}

// Claims represents JWT token claims
type Claims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(secretKey string, enabled bool, logger Logger) *AuthMiddleware {
	return &AuthMiddleware{
		secretKey: []byte(secretKey),
		enabled:   enabled,
		logger:    logger,
	}
}

// HTTPContextFunc returns a middleware function compatible with mcp-go
func (a *AuthMiddleware) HTTPContextFunc(next func(ctx context.Context, r *http.Request) context.Context) func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		// If authentication is disabled, just call the next middleware
		if !a.enabled {
			return next(ctx, r)
		}

		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			a.logger.Warn("Missing or invalid authorization header from %s", r.RemoteAddr)
			// Set authentication error in context instead of failing the request
			ctx = context.WithValue(ctx, authErrorKey, "missing_token")
			return next(ctx, r)
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate JWT token
		claims, err := a.validateJWT(token)
		if err != nil {
			a.logger.Warn("Invalid token from %s: %v", r.RemoteAddr, err)
			ctx = context.WithValue(ctx, authErrorKey, "invalid_token")
			return next(ctx, r)
		}

		// Check if token is expired
		if time.Now().Unix() > claims.ExpiresAt {
			a.logger.Warn("Expired token from %s", r.RemoteAddr)
			ctx = context.WithValue(ctx, authErrorKey, "expired_token")
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
	// Split the token into header, payload, and signature
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode and parse header
	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerData, &header); err != nil {
		return nil, fmt.Errorf("invalid header format: %w", err)
	}

	// Verify algorithm
	if header.Alg != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	// Decode and parse payload
	payloadData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadData, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload format: %w", err)
	}

	// Verify signature
	expectedSignature := a.generateSignature(parts[0] + "." + parts[1])
	actualSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !hmac.Equal(expectedSignature, actualSignature) {
		return nil, fmt.Errorf("invalid signature")
	}

	return &claims, nil
}

// generateSignature generates HMAC-SHA256 signature for the given data
func (a *AuthMiddleware) generateSignature(data string) []byte {
	h := hmac.New(sha256.New, a.secretKey)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// GenerateToken generates a JWT token for a user (utility function for testing/setup)
func (a *AuthMiddleware) GenerateToken(userID, username, role string, expirationHours int) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Duration(expirationHours) * time.Hour).Unix(),
	}

	// Create header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encode header and payload
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Generate signature
	signatureData := headerEncoded + "." + payloadEncoded
	signature := a.generateSignature(signatureData)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	token := headerEncoded + "." + payloadEncoded + "." + signatureEncoded
	return token, nil
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

// RequireAuth is a utility function to check authentication and return error if not authenticated
func RequireAuth(ctx context.Context) error {
	if !isAuthenticated(ctx) {
		if authError := getAuthError(ctx); authError != "" {
			switch authError {
			case "missing_token":
				return fmt.Errorf("authentication required: missing or invalid authorization header")
			case "invalid_token":
				return fmt.Errorf("authentication required: invalid token")
			case "expired_token":
				return fmt.Errorf("authentication required: token expired")
			default:
				return fmt.Errorf("authentication required: %s", authError)
			}
		}
		return fmt.Errorf("authentication required")
	}
	return nil
}

// CreateTokenCommand creates a command-line utility to generate tokens
func CreateTokenCommand(secretKey, userID, username, role string, expirationHours int) {
	if secretKey == "" {
		fmt.Fprintln(os.Stderr, "Error: SECRET_KEY environment variable is required")
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
