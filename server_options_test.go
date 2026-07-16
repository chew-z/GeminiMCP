package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildMCPServerOptions(t *testing.T) {
	assert.NotEmpty(t, buildMCPServerOptions(&Config{}, NewLogger(LevelError)))
}
