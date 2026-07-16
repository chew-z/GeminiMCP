package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDegradedToolClearsExecution(t *testing.T) {
	assert.Nil(t, degradedTool(GeminiAskTool).Execution)
}
