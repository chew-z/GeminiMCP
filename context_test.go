package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPTransportContext(t *testing.T) {
	base := context.Background()
	assert.False(t, isHTTPTransport(base))

	httpCtx := withHTTPTransport(base)
	assert.True(t, isHTTPTransport(httpCtx))

	otherCtx := context.WithValue(base, transportKey, "stdio")
	assert.False(t, isHTTPTransport(otherCtx))
}
