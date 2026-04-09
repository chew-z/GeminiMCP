package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tokenCall struct {
	secret     string
	userID     string
	username   string
	role       string
	expiration int
}

type mainTestCalls struct {
	tokenCalls       []tokenCall
	startupErrors    []error
	setupCalled      bool
	resolveCalls     []string
	validateCalls    []string
	serveStdioCalled bool
	startHTTPCalled  bool
}

func installMainHooksForTest(t *testing.T, cfg *Config) *mainTestCalls {
	t.Helper()

	origRunMainFn := runMainFn
	origOSExitFn := osExitFn
	origOSArgsFn := osArgsFn
	origCreateTokenCommandFn := createTokenCommandFn
	origNewLoggerFn := newLoggerFn
	origNewConfigFn := newConfigFn
	origHandleStartupErrorFn := handleStartupErrorFn
	origSetupGeminiServerFn := setupGeminiServerFn
	origResolveModelFn := resolveModelFn
	origValidateModelIDFn := validateModelIDFn
	origStartHTTPServerFn := startHTTPServerFn
	origServeStdioFn := serveStdioFn
	origGetEnvFn := getEnvFn
	origNewMCPServerFn := newMCPServerFn

	t.Cleanup(func() {
		runMainFn = origRunMainFn
		osExitFn = origOSExitFn
		osArgsFn = origOSArgsFn
		createTokenCommandFn = origCreateTokenCommandFn
		newLoggerFn = origNewLoggerFn
		newConfigFn = origNewConfigFn
		handleStartupErrorFn = origHandleStartupErrorFn
		setupGeminiServerFn = origSetupGeminiServerFn
		resolveModelFn = origResolveModelFn
		validateModelIDFn = origValidateModelIDFn
		startHTTPServerFn = origStartHTTPServerFn
		serveStdioFn = origServeStdioFn
		getEnvFn = origGetEnvFn
		newMCPServerFn = origNewMCPServerFn
	})

	calls := &mainTestCalls{}

	runMainFn = runMain
	osExitFn = func(int) {}
	osArgsFn = func() []string { return nil }

	createTokenCommandFn = func(secretKey, userID, username, role string, expirationHours int) {
		calls.tokenCalls = append(calls.tokenCalls, tokenCall{
			secret:     secretKey,
			userID:     userID,
			username:   username,
			role:       role,
			expiration: expirationHours,
		})
	}
	newLoggerFn = func(level LogLevel) Logger {
		return NewLogger(LevelError)
	}
	newConfigFn = func(Logger) (*Config, error) {
		return cfg, nil
	}
	handleStartupErrorFn = func(_ context.Context, err error) {
		calls.startupErrors = append(calls.startupErrors, err)
	}
	setupGeminiServerFn = func(context.Context, *server.MCPServer, *Config) error {
		calls.setupCalled = true
		return nil
	}
	resolveModelFn = func(_ context.Context, model string) (string, error) {
		calls.resolveCalls = append(calls.resolveCalls, model)
		return model, nil
	}
	validateModelIDFn = func(model string) (string, bool, error) {
		calls.validateCalls = append(calls.validateCalls, model)
		return model, false, nil
	}
	startHTTPServerFn = func(context.Context, *server.MCPServer, *Config, Logger) error {
		calls.startHTTPCalled = true
		return nil
	}
	serveStdioFn = func(*server.MCPServer, ...server.StdioOption) error {
		calls.serveStdioCalled = true
		return nil
	}
	getEnvFn = func(string) string { return "" }
	newMCPServerFn = func() *server.MCPServer {
		return server.NewMCPServer("gemini", "1.0.0")
	}

	return calls
}

func baseMainTestConfig() *Config {
	return &Config{
		GeminiAPIKey:       "test-key",
		GeminiModel:        "gemini-3.1-pro-preview",
		GeminiSearchModel:  "gemini-3-flash-preview",
		GeminiSystemPrompt: "system",
		ServiceTier:        "standard",
		HTTPAddress:        "127.0.0.1:0",
		HTTPPath:           "/mcp",
	}
}

func TestMainWrapper(t *testing.T) {
	t.Run("does not call exit when runMain returns zero", func(t *testing.T) {
		cfg := baseMainTestConfig()
		_ = installMainHooksForTest(t, cfg)

		exitCalled := false
		calledArgs := []string{}
		runMainFn = func(args []string) int {
			calledArgs = append([]string(nil), args...)
			return 0
		}
		osArgsFn = func() []string { return []string{"-transport=stdio"} }
		osExitFn = func(code int) {
			exitCalled = true
			_ = code
		}

		main()

		assert.False(t, exitCalled)
		assert.Equal(t, []string{"-transport=stdio"}, calledArgs)
	})

	t.Run("calls exit when runMain returns non-zero", func(t *testing.T) {
		cfg := baseMainTestConfig()
		_ = installMainHooksForTest(t, cfg)

		exitCode := 0
		exitCalled := false
		runMainFn = func(args []string) int {
			_ = args
			return 2
		}
		osExitFn = func(code int) {
			exitCalled = true
			exitCode = code
		}

		main()

		assert.True(t, exitCalled)
		assert.Equal(t, 2, exitCode)
	})
}

func TestGetFeatureStatusStr(t *testing.T) {
	assert.Equal(t, "enabled", getFeatureStatusStr(true))
	assert.Equal(t, "disabled", getFeatureStatusStr(false))
}

func TestRunMainGenerateToken(t *testing.T) {
	cfg := baseMainTestConfig()
	calls := installMainHooksForTest(t, cfg)
	getEnvFn = func(key string) string {
		if key == "GEMINI_AUTH_SECRET_KEY" {
			return "secret-123"
		}
		return ""
	}

	code := runMain([]string{
		"-generate-token",
		"-token-user-id=u1",
		"-token-username=alice",
		"-token-role=developer",
		"-token-expiration=24",
	})

	assert.Equal(t, 0, code)
	require.Len(t, calls.tokenCalls, 1)
	assert.Equal(t, tokenCall{
		secret:     "secret-123",
		userID:     "u1",
		username:   "alice",
		role:       "developer",
		expiration: 24,
	}, calls.tokenCalls[0])
	assert.False(t, calls.setupCalled)
}

func TestRunMainStartupErrorPaths(t *testing.T) {
	t.Run("config creation error uses startup handler", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		newConfigFn = func(Logger) (*Config, error) {
			return nil, errors.New("missing api key")
		}

		code := runMain(nil)

		assert.Equal(t, 0, code)
		require.Len(t, calls.startupErrors, 1)
		assert.Contains(t, calls.startupErrors[0].Error(), "missing api key")
		assert.False(t, calls.setupCalled)
	})

	t.Run("invalid temperature uses startup handler", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)

		code := runMain([]string{"-gemini-temperature=1.1"})

		assert.Equal(t, 0, code)
		require.Len(t, calls.startupErrors, 1)
		assert.Contains(t, calls.startupErrors[0].Error(), "invalid temperature")
		assert.False(t, calls.setupCalled)
	})

	t.Run("setup server error uses startup handler", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		setupGeminiServerFn = func(context.Context, *server.MCPServer, *Config) error {
			calls.setupCalled = true
			return errors.New("setup failed")
		}

		code := runMain(nil)

		assert.Equal(t, 0, code)
		assert.True(t, calls.setupCalled)
		require.Len(t, calls.startupErrors, 1)
		assert.Equal(t, "setup failed", calls.startupErrors[0].Error())
	})

	t.Run("resolve model error uses startup handler", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		resolveModelFn = func(_ context.Context, model string) (string, error) {
			calls.resolveCalls = append(calls.resolveCalls, model)
			return "", errors.New("resolve failed")
		}

		code := runMain(nil)

		assert.Equal(t, 0, code)
		require.Len(t, calls.startupErrors, 1)
		assert.Equal(t, "resolve failed", calls.startupErrors[0].Error())
		assert.False(t, calls.serveStdioCalled)
		assert.False(t, calls.startHTTPCalled)
	})
}

func TestRunMainFlagAndTransportPaths(t *testing.T) {
	t.Run("unknown flag returns error code", func(t *testing.T) {
		cfg := baseMainTestConfig()
		_ = installMainHooksForTest(t, cfg)

		code := runMain([]string{"-unknown-flag"})
		assert.Equal(t, 1, code)
	})

	t.Run("auth enabled with missing secret returns error code", func(t *testing.T) {
		cfg := baseMainTestConfig()
		cfg.AuthSecretKey = ""
		calls := installMainHooksForTest(t, cfg)

		code := runMain([]string{"-auth-enabled"})

		assert.Equal(t, 1, code)
		assert.False(t, calls.setupCalled)
	})

	t.Run("invalid transport returns error code", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)

		code := runMain([]string{"-transport=invalid"})

		assert.Equal(t, 1, code)
		assert.True(t, calls.setupCalled)
		assert.False(t, calls.serveStdioCalled)
		assert.False(t, calls.startHTTPCalled)
	})

	t.Run("stdio path succeeds", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)

		code := runMain(nil)

		assert.Equal(t, 0, code)
		assert.True(t, calls.serveStdioCalled)
		assert.False(t, calls.startHTTPCalled)
	})

	t.Run("stdio path failure returns error code", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		serveStdioFn = func(*server.MCPServer, ...server.StdioOption) error {
			calls.serveStdioCalled = true
			return errors.New("stdio failed")
		}

		code := runMain(nil)

		assert.Equal(t, 1, code)
		assert.True(t, calls.serveStdioCalled)
	})

	t.Run("explicit http path succeeds", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)

		code := runMain([]string{"-transport=http"})

		assert.Equal(t, 0, code)
		assert.True(t, calls.startHTTPCalled)
		assert.False(t, calls.serveStdioCalled)
	})

	t.Run("implicit http path when enableHTTP is true", func(t *testing.T) {
		cfg := baseMainTestConfig()
		cfg.EnableHTTP = true
		calls := installMainHooksForTest(t, cfg)

		code := runMain(nil)

		assert.Equal(t, 0, code)
		assert.True(t, calls.startHTTPCalled)
		assert.False(t, calls.serveStdioCalled)
	})

	t.Run("http path failure returns error code", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		startHTTPServerFn = func(context.Context, *server.MCPServer, *Config, Logger) error {
			calls.startHTTPCalled = true
			return errors.New("http failed")
		}

		code := runMain([]string{"-transport=http"})

		assert.Equal(t, 1, code)
		assert.True(t, calls.startHTTPCalled)
	})
}

func TestRunMainModelFlagValidation(t *testing.T) {
	t.Run("invalid gemini-model flag uses startup handler", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		validateModelIDFn = func(model string) (string, bool, error) {
			calls.validateCalls = append(calls.validateCalls, model)
			return "", false, errors.New("bad model")
		}

		code := runMain([]string{"-gemini-model=bad-model"})

		assert.Equal(t, 0, code)
		require.Len(t, calls.validateCalls, 1)
		require.Len(t, calls.startupErrors, 1)
		assert.Equal(t, "bad model", calls.startupErrors[0].Error())
		assert.False(t, calls.serveStdioCalled)
		assert.False(t, calls.startHTTPCalled)
	})

	t.Run("valid gemini-model flag overrides config model", func(t *testing.T) {
		cfg := baseMainTestConfig()
		calls := installMainHooksForTest(t, cfg)
		validateModelIDFn = func(model string) (string, bool, error) {
			calls.validateCalls = append(calls.validateCalls, model)
			return "gemini-validated", true, nil
		}

		code := runMain([]string{"-gemini-model=gemini-custom"})

		assert.Equal(t, 0, code)
		assert.Equal(t, "gemini-validated", cfg.GeminiModel)
		require.Len(t, calls.validateCalls, 1)
		assert.True(t, calls.serveStdioCalled)
	})

	t.Run("applies non-model flag overrides and known model branch", func(t *testing.T) {
		cfg := baseMainTestConfig()
		cfg.AuthSecretKey = "secret-present"
		calls := installMainHooksForTest(t, cfg)
		validateModelIDFn = func(model string) (string, bool, error) {
			calls.validateCalls = append(calls.validateCalls, model)
			return "gemini-known", false, nil
		}

		code := runMain([]string{
			"-gemini-system-prompt=override",
			"-gemini-temperature=0.6",
			"-enable-thinking=false",
			"-service-tier=priority",
			"-auth-enabled",
			"-gemini-model=gemini-known",
		})

		assert.Equal(t, 0, code)
		assert.Equal(t, "override", cfg.GeminiSystemPrompt)
		assert.Equal(t, 0.6, cfg.GeminiTemperature)
		assert.False(t, cfg.EnableThinking)
		assert.Equal(t, "priority", cfg.ServiceTier)
		assert.True(t, cfg.AuthEnabled)
		assert.Equal(t, "gemini-known", cfg.GeminiModel)
		require.Len(t, calls.validateCalls, 1)
		assert.True(t, calls.serveStdioCalled)
	})
}
