package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestApplyCLIOverrides(t *testing.T) {
	cfg := &Config{GeminiTemperature: 1}
	require := applyCLIOverrides(&cliFlags{geminiTemperature: 0.4}, cfg, NewLogger(LevelError))
	assert.NoError(t, require)
	assert.Equal(t, 0.4, cfg.GeminiTemperature)
}
