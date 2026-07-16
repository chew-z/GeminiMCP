package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateTaskInstructionsEscapesInput(t *testing.T) {
	got := createTaskInstructions("<script>")
	assert.Contains(t, got, "&lt;script&gt;")
	assert.Contains(t, got, "gemini_ask")
}
