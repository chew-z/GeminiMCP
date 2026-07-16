package main

import (
	"errors"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestGeminiProviderIsRetryable(t *testing.T) {
	p := &GeminiProvider{}
	for _, tt := range []struct {
		code int
		want bool
	}{{429, true}, {500, true}, {400, false}, {403, false}} {
		assert := p.IsRetryable(&googleapi.Error{Code: tt.code})
		if assert != tt.want {
			t.Fatalf("code %d: got %v want %v", tt.code, assert, tt.want)
		}
	}
	if p.IsRetryable(errors.New("bad request")) {
		t.Fatal("unexpected retry")
	}
}
