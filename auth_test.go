package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
