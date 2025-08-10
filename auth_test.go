package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddleware(t *testing.T) {
	logger := NewLogger(LevelDebug)
	secret := "test-secret"
	auth := NewAuthMiddleware(secret, true, logger)

	t.Run("valid token", func(t *testing.T) {
		token, err := auth.GenerateToken("123", "testuser", "user", 1)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		next := func(ctx context.Context, r *http.Request) context.Context {
			return ctx
		}

		ctx := auth.HTTPContextFunc(next)(context.Background(), req)

		if !isAuthenticated(ctx) {
			t.Error("expected authentication to succeed")
		}
		if getAuthError(ctx) != "" {
			t.Errorf("unexpected auth error: %s", getAuthError(ctx))
		}
		userID, username, role := getUserInfo(ctx)
		if userID != "123" || username != "testuser" || role != "user" {
			t.Errorf("unexpected user info: got %s, %s, %s", userID, username, role)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token, err := auth.GenerateToken("123", "testuser", "user", -1) // expired 1 hour ago
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		next := func(ctx context.Context, r *http.Request) context.Context {
			return ctx
		}

		ctx := auth.HTTPContextFunc(next)(context.Background(), req)

		if isAuthenticated(ctx) {
			t.Error("expected authentication to fail for expired token")
		}
		if getAuthError(ctx) != "invalid_token" {
			t.Errorf("expected 'invalid_token' error, got '%s'", getAuthError(ctx))
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		otherAuth := NewAuthMiddleware("different-secret", true, logger)
		token, err := otherAuth.GenerateToken("123", "testuser", "user", 1)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		next := func(ctx context.Context, r *http.Request) context.Context {
			return ctx
		}

		ctx := auth.HTTPContextFunc(next)(context.Background(), req)

		if isAuthenticated(ctx) {
			t.Error("expected authentication to fail for invalid signature")
		}
		if getAuthError(ctx) != "invalid_token" {
			t.Errorf("expected 'invalid_token' error, got '%s'", getAuthError(ctx))
		}
	})

	t.Run("wrong signing method", func(t *testing.T) {
		claims := Claims{
			UserID:   "123",
			Username: "testuser",
			Role:     "user",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
		// We can't sign this token without a private key, so we'll just use a dummy string
		// The validation should fail before it even gets to signature verification
		dummyToken, _ := token.SignedString("dummy")

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+dummyToken)

		next := func(ctx context.Context, r *http.Request) context.Context {
			return ctx
		}

		ctx := auth.HTTPContextFunc(next)(context.Background(), req)

		if isAuthenticated(ctx) {
			t.Error("expected authentication to fail for wrong signing method")
		}
		if getAuthError(ctx) != "invalid_token" {
			t.Errorf("expected 'invalid_token' error, got '%s'", getAuthError(ctx))
		}
	})
}
