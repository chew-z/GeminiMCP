package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware(t *testing.T) {
	logger := NewLogger(LevelDebug)
	secret := "test-secret"
	auth := NewAuthMiddleware(secret, true, logger)          // Auth enabled
	disabledAuth := NewAuthMiddleware(secret, false, logger) // Auth disabled

	validToken, err := auth.GenerateToken("123", "testuser", "user", 1)
	require.NoError(t, err)

	expiredToken, err := auth.GenerateToken("123", "testuser", "user", -1) // expired 1 hour ago
	require.NoError(t, err)

	otherAuth := NewAuthMiddleware("different-secret", false, logger)
	invalidSigToken, err := otherAuth.GenerateToken("123", "testuser", "user", 1)
	require.NoError(t, err)

	// Token with a different algorithm

	testCases := []struct {
		name           string
		authMiddleware *AuthMiddleware
		authHeader     string
		expectAuth     bool
		expectErr      string
		expectUserID   string
		expectUsername string
		expectRole     string
	}{
		{
			name:           "valid token",
			authMiddleware: auth,
			authHeader:     "Bearer " + validToken,
			expectAuth:     true,
			expectErr:      "",
			expectUserID:   "123",
			expectUsername: "testuser",
			expectRole:     "user",
		},
		{
			name:           "auth disabled",
			authMiddleware: disabledAuth,
			authHeader:     "",    // No header needed
			expectAuth:     false, // Authenticated is false because middleware is skipped
			expectErr:      "",
			expectUserID:   "", // No user info
		},
		{
			name:           "expired token",
			authMiddleware: auth,
			authHeader:     "Bearer " + expiredToken,
			expectAuth:     false,
			expectErr:      "invalid_token",
		},
		{
			name:           "invalid signature",
			authMiddleware: auth,
			authHeader:     "Bearer " + invalidSigToken,
			expectAuth:     false,
			expectErr:      "invalid_token",
		},

		{
			name:           "missing authorization header",
			authMiddleware: auth,
			authHeader:     "",
			expectAuth:     false,
			expectErr:      "missing_token",
		},
		{
			name:           "malformed header - no bearer prefix",
			authMiddleware: auth,
			authHeader:     validToken,
			expectAuth:     false,
			expectErr:      "invalid_token",
		},
		{
			name:           "malformed header - wrong scheme",
			authMiddleware: auth,
			authHeader:     "Basic " + validToken,
			expectAuth:     false,
			expectErr:      "invalid_token",
		},
		{
			name:           "not a valid jwt token",
			authMiddleware: auth,
			authHeader:     "Bearer not-a-jwt",
			expectAuth:     false,
			expectErr:      "invalid_token",
		},
		{
			name:           "case insensitive bearer",
			authMiddleware: auth,
			authHeader:     "bearer " + validToken,
			expectAuth:     true,
			expectErr:      "",
			expectUserID:   "123",
			expectUsername: "testuser",
			expectRole:     "user",
		},
		{
			name:           "mixed case bearer",
			authMiddleware: auth,
			authHeader:     "Bearer " + validToken,
			expectAuth:     true,
			expectErr:      "",
			expectUserID:   "123",
			expectUsername: "testuser",
			expectRole:     "user",
		},
		{
			name:           "multiple spaces in header",
			authMiddleware: auth,
			authHeader:     "Bearer   " + validToken, // Extra spaces - Fields() handles this correctly
			expectAuth:     true,
			expectErr:      "",
			expectUserID:   "123",
			expectUsername: "testuser",
			expectRole:     "user",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			// Dummy next function that does nothing but return the context it was given.
			next := func(ctx context.Context, r *http.Request) context.Context {
				return ctx
			}

			ctx := tc.authMiddleware.HTTPContextFunc(next)(context.Background(), req)

			assert.Equal(t, tc.expectAuth, isAuthenticated(ctx), "isAuthenticated mismatch")
			assert.Equal(t, tc.expectErr, getAuthError(ctx), "authError mismatch")

			if tc.expectAuth && tc.expectUserID != "" {
				userID, username, role := getUserInfo(ctx)
				assert.Equal(t, tc.expectUserID, userID, "userID mismatch")
				assert.Equal(t, tc.expectUsername, username, "username mismatch")
				assert.Equal(t, tc.expectRole, role, "role mismatch")
			}
		})
	}
}

func TestValidateJWT(t *testing.T) {
	logger := NewLogger(LevelDebug)
	secret := "test-secret-for-validation"
	auth := NewAuthMiddleware(secret, true, logger)

	// Generate tokens for testing
	validToken, err := auth.GenerateToken("123", "testuser", "user", 1)
	require.NoError(t, err)

	expiredToken, err := auth.GenerateToken("123", "testuser", "user", -1) // expired 1 hour ago
	require.NoError(t, err)

	// Create token with wrong secret for signature validation
	wrongSecretAuth := NewAuthMiddleware("different-secret", true, logger)
	wrongSigToken, err := wrongSecretAuth.GenerateToken("123", "testuser", "user", 1)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		token         string
		expectedError error
		shouldSucceed bool
	}{
		{
			name:          "valid token",
			token:         validToken,
			expectedError: nil,
			shouldSucceed: true,
		},
		{
			name:          "expired token",
			token:         expiredToken,
			expectedError: jwt.ErrTokenExpired,
			shouldSucceed: false,
		},
		{
			name:          "invalid signature",
			token:         wrongSigToken,
			expectedError: jwt.ErrSignatureInvalid,
			shouldSucceed: false,
		},
		{
			name:          "malformed token",
			token:         "not-a-valid-jwt",
			expectedError: jwt.ErrTokenMalformed,
			shouldSucceed: false,
		},
		{
			name:          "empty token",
			token:         "",
			expectedError: jwt.ErrTokenMalformed,
			shouldSucceed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := auth.validateJWT(tc.token)

			if tc.shouldSucceed {
				assert.NoError(t, err, "Expected no error for valid token")
				assert.NotNil(t, claims, "Expected claims to be returned")
				if claims != nil {
					assert.Equal(t, "123", claims.UserID)
					assert.Equal(t, "testuser", claims.Username)
					assert.Equal(t, "user", claims.Role)
					assert.Equal(t, "gemini-mcp", claims.Issuer)
					assert.Contains(t, claims.Audience, "gemini-mcp-user")
				}
			} else {
				assert.Error(t, err, "Expected error for invalid token")
				assert.Nil(t, claims, "Expected no claims for invalid token")
				if tc.expectedError != nil {
					assert.True(t, errors.Is(err, tc.expectedError),
						"Expected error %v, but got %v", tc.expectedError, err)
				}
			}
		})
	}

	// Test algorithm pinning with manually crafted token using wrong algorithm
	t.Run("wrong_algorithm_rejected", func(t *testing.T) {
		// Create a token with HS512 instead of HS256 to test algorithm pinning
		wrongAlgToken := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{
			UserID:   "123",
			Username: "testuser",
			Role:     "user",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:   "gemini-mcp",
				Audience: jwt.ClaimStrings{"gemini-mcp-user"},
			},
		})
		wrongAlgTokenString, err := wrongAlgToken.SignedString([]byte(secret))
		require.NoError(t, err)

		claims, err := auth.validateJWT(wrongAlgTokenString)
		assert.Error(t, err, "Expected error for wrong algorithm")
		assert.Nil(t, claims, "Expected no claims for wrong algorithm")
		assert.Contains(t, err.Error(), "unexpected signing method", "Error should mention unexpected signing method")
	})
}

func TestExtractTokenFromHeader(t *testing.T) {
	testCases := []struct {
		name          string
		authHeader    string
		expectedToken string
	}{
		{
			name:          "valid bearer token",
			authHeader:    "Bearer abc123",
			expectedToken: "abc123",
		},
		{
			name:          "case insensitive bearer",
			authHeader:    "bearer abc123",
			expectedToken: "abc123",
		},
		{
			name:          "mixed case",
			authHeader:    "BEARER abc123",
			expectedToken: "abc123",
		},
		{
			name:          "multiple spaces between bearer and token",
			authHeader:    "Bearer   abc123",
			expectedToken: "abc123", // Fields() correctly handles multiple spaces
		},
		{
			name:          "wrong scheme",
			authHeader:    "Basic abc123",
			expectedToken: "",
		},
		{
			name:          "missing token",
			authHeader:    "Bearer",
			expectedToken: "",
		},
		{
			name:          "empty header",
			authHeader:    "",
			expectedToken: "",
		},
		{
			name:          "only token no scheme",
			authHeader:    "abc123",
			expectedToken: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractTokenFromHeader(tc.authHeader)
			assert.Equal(t, tc.expectedToken, result)
		})
	}
}
