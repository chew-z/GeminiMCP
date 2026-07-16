package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Default configuration values
const (
	defaultGeminiModel       = "gemini-3.1-pro-preview"
	defaultDeepSeekBaseURL   = "https://api.deepseek.com"
	defaultGeminiTemperature = 1.0 // Gemini 3 default temperature
	// Pre-qualification defaults
	defaultPrequalify = true
	// GitHub settings defaults
	defaultGitHubAPIBaseURL          = "https://api.github.com"
	defaultMaxGitHubFiles            = 20
	defaultMaxGitHubFileSize         = int64(1 * 1024 * 1024) // 1MB
	defaultMaxGitHubDiffBytes        = int64(500 * 1024)      // 500KB for any single diff payload
	defaultMaxGitHubCommits          = 10                     // max commits per github_commits call
	defaultMaxGitHubPRReviewComments = 50                     // max PR review comments fetched

	// HTTP transport defaults
	defaultEnableHTTP      = false
	defaultHTTPAddress     = ":8080"
	defaultHTTPPath        = "/mcp"
	defaultHTTPStateless   = false
	defaultHTTPHeartbeat   = 20 * time.Second // Keeps nginx proxy_read_timeout and client idle timers quiet on long pro-tier requests.
	defaultHTTPCORSEnabled = true
	// Gemini MCP is typically deployed behind an nginx reverse proxy that forwards
	// the Host header from external clients. mcp-go's localhost-protection check
	// will reject those proxied requests unless disabled for loopback servers.
	defaultHTTPDisableLocalhostProtection = true

	// Progress notification defaults
	defaultProgressInterval = 10 * time.Second // Cadence for notifications/progress; <=0 disables.

	// Task-augmented tool defaults
	defaultMaxConcurrentTasks = 10 // Upper bound on concurrently-executing task tools; <=0 disables.

	// Authentication defaults
	defaultAuthEnabled = false // Authentication disabled by default

)

// Config struct definition moved to structs.go

// Helper function to parse an integer environment variable with a default
func parseEnvVarInt(key string, defaultValue int, logger Logger) int {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.Atoi(str); err == nil {
			return val
		}
		logger.Warnf("Invalid integer value for %s: %q. Using default: %d", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a float64 environment variable with a default
func parseEnvVarFloat(key string, defaultValue float64, logger Logger) float64 {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseFloat(str, 64); err == nil {
			return val
		}
		logger.Warnf("Invalid float value for %s: %q. Using default: %f", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a duration environment variable with a default
func parseEnvVarDuration(key string, defaultValue time.Duration, logger Logger) time.Duration {
	if str := os.Getenv(key); str != "" {
		if val, err := time.ParseDuration(str); err == nil {
			return val
		}
		logger.Warnf("Invalid duration value for %s: %q. Using default: %s", key, str, defaultValue.String())
	}
	return defaultValue
}

// Helper function to parse a boolean environment variable with a default
func parseEnvVarBool(key string, defaultValue bool, logger Logger) bool {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseBool(str); err == nil {
			return val
		}
		logger.Warnf("Invalid boolean value for %s: %q. Using default: %t", key, str, defaultValue)
	}
	return defaultValue
}

// isLoopbackHost reports whether host (with or without a port) resolves to a
// loopback identifier per the RFC 9728 startup-validation rule. The accepted
// set is intentionally narrow: "localhost", "127.0.0.1", and "::1". Used
// at startup (parseHTTPPublicURL) to validate the rule.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(strings.ToLower(h), "[]")
	switch h {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// parseHTTPPublicURL validates GEMINI_HTTP_PUBLIC_URL. Empty input is allowed
// and signals "derive from request". Non-empty input must be an absolute URL
// with https scheme (or http when the host is loopback), no query, and no
// fragment. A trailing slash is trimmed from the stored path so registration
// and comparison are deterministic.
func parseHTTPPublicURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL is not a valid URL: %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL must use http or https scheme: got %q", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL must include a host: got %q", raw)
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Host) {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL must use https (or http for loopback): got %q", raw)
	}
	if u.RawQuery != "" {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL must not include a query: got %q", raw)
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("GEMINI_HTTP_PUBLIC_URL must not include a fragment: got %q", raw)
	}

	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// httpTransportConfig captures HTTP-transport env values.
type httpTransportConfig struct {
	enableHTTP                 bool
	address                    string
	path                       string
	stateless                  bool
	heartbeat                  time.Duration
	corsEnabled                bool
	corsOrigins                []string
	progressInterval           time.Duration
	publicURL                  string
	disableLocalhostProtection bool
}

func loadHTTPConfig(logger Logger) (httpTransportConfig, error) {
	enableHTTP := parseEnvVarBool("GEMINI_ENABLE_HTTP", defaultEnableHTTP, logger)
	address := os.Getenv("GEMINI_HTTP_ADDRESS")
	if address == "" {
		address = defaultHTTPAddress
	}
	path := os.Getenv("GEMINI_HTTP_PATH")
	if path == "" {
		path = defaultHTTPPath
	}
	stateless := parseEnvVarBool("GEMINI_HTTP_STATELESS", defaultHTTPStateless, logger)
	heartbeat := parseEnvVarDuration("GEMINI_HTTP_HEARTBEAT", defaultHTTPHeartbeat, logger)
	if heartbeat < 0 {
		logger.Warnf("GEMINI_HTTP_HEARTBEAT must be non-negative. Using default: %s", defaultHTTPHeartbeat.String())
		heartbeat = defaultHTTPHeartbeat
	}
	corsEnabled := parseEnvVarBool("GEMINI_HTTP_CORS_ENABLED", defaultHTTPCORSEnabled, logger)
	disableLocalhostProtection := parseEnvVarBool(
		"GEMINI_HTTP_DISABLE_LOCALHOST_PROTECTION",
		defaultHTTPDisableLocalhostProtection,
		logger,
	)

	var corsOrigins []string
	if originsStr := os.Getenv("GEMINI_HTTP_CORS_ORIGINS"); originsStr != "" {
		for _, p := range strings.Split(originsStr, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
	}
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}

	publicURL, err := parseHTTPPublicURL(os.Getenv("GEMINI_HTTP_PUBLIC_URL"))
	if err != nil {
		return httpTransportConfig{}, err
	}

	return httpTransportConfig{
		enableHTTP:                 enableHTTP,
		address:                    address,
		path:                       path,
		stateless:                  stateless,
		heartbeat:                  heartbeat,
		corsEnabled:                corsEnabled,
		corsOrigins:                corsOrigins,
		progressInterval:           parseEnvVarDuration("GEMINI_PROGRESS_INTERVAL", defaultProgressInterval, logger),
		publicURL:                  publicURL,
		disableLocalhostProtection: disableLocalhostProtection,
	}, nil
}

// authConfig captures authentication env values.
type authConfig struct {
	enabled   bool
	secretKey string
}

func loadAuthConfig(logger Logger) (authConfig, error) {
	enabled := parseEnvVarBool("GEMINI_AUTH_ENABLED", defaultAuthEnabled, logger)
	secretKey := os.Getenv("GEMINI_AUTH_SECRET_KEY")

	if enabled && secretKey == "" {
		return authConfig{}, fmt.Errorf("GEMINI_AUTH_SECRET_KEY is required when GEMINI_AUTH_ENABLED=true")
	}
	if enabled && len(secretKey) < 32 {
		return authConfig{}, fmt.Errorf("GEMINI_AUTH_SECRET_KEY must be at least 32 bytes for HS256 (got %d)", len(secretKey))
	}
	return authConfig{enabled: enabled, secretKey: secretKey}, nil
}

// taskExecConfig captures task-augmented execution env values (concurrency +
// pre-qualification classifier).
type taskExecConfig struct {
	maxConcurrentTasks int
	prequalify         bool
}

func loadTaskConfig(logger Logger) taskExecConfig {
	return taskExecConfig{
		maxConcurrentTasks: parseEnvVarInt("GEMINI_MAX_CONCURRENT_TASKS", defaultMaxConcurrentTasks, logger),
		prequalify:         parseEnvVarBool("GEMINI_PREQUALIFY", defaultPrequalify, logger),
	}
}

// githubSettings captures GitHub-integration env values.
type githubSettings struct {
	token                     string
	apiBaseURL                string
	maxGitHubFiles            int
	maxGitHubFileSize         int64
	maxGitHubDiffBytes        int64
	maxGitHubCommits          int
	maxGitHubPRReviewComments int
}

func loadGitHubConfig(logger Logger) githubSettings {
	apiBaseURL := os.Getenv("GEMINI_GITHUB_API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = defaultGitHubAPIBaseURL
	}

	maxFiles := parseEnvVarInt("GEMINI_MAX_GITHUB_FILES", defaultMaxGitHubFiles, logger)
	if maxFiles <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_FILES must be positive. Using default: %d", defaultMaxGitHubFiles)
		maxFiles = defaultMaxGitHubFiles
	}
	maxFileSize := int64(parseEnvVarInt("GEMINI_MAX_GITHUB_FILE_SIZE", int(defaultMaxGitHubFileSize), logger))
	if maxFileSize <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_FILE_SIZE must be positive. Using default: %d", defaultMaxGitHubFileSize)
		maxFileSize = defaultMaxGitHubFileSize
	}
	maxDiffBytes := int64(parseEnvVarInt("GEMINI_MAX_GITHUB_DIFF_BYTES", int(defaultMaxGitHubDiffBytes), logger))
	if maxDiffBytes <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_DIFF_BYTES must be positive. Using default: %d", defaultMaxGitHubDiffBytes)
		maxDiffBytes = defaultMaxGitHubDiffBytes
	}
	maxCommits := parseEnvVarInt("GEMINI_MAX_GITHUB_COMMITS", defaultMaxGitHubCommits, logger)
	if maxCommits <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_COMMITS must be positive. Using default: %d", defaultMaxGitHubCommits)
		maxCommits = defaultMaxGitHubCommits
	}
	maxPRReviewComments := parseEnvVarInt("GEMINI_MAX_GITHUB_PR_REVIEW_COMMENTS", defaultMaxGitHubPRReviewComments, logger)
	if maxPRReviewComments < 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_PR_REVIEW_COMMENTS must be non-negative. Using default: %d", defaultMaxGitHubPRReviewComments)
		maxPRReviewComments = defaultMaxGitHubPRReviewComments
	}

	return githubSettings{
		token:                     os.Getenv("GEMINI_GITHUB_TOKEN"),
		apiBaseURL:                apiBaseURL,
		maxGitHubFiles:            maxFiles,
		maxGitHubFileSize:         maxFileSize,
		maxGitHubDiffBytes:        maxDiffBytes,
		maxGitHubCommits:          maxCommits,
		maxGitHubPRReviewComments: maxPRReviewComments,
	}
}

// timeoutAndRetryConfig captures HTTP timeout + retry env values.
type timeoutAndRetryConfig struct {
	timeout          time.Duration
	httpWriteTimeout time.Duration
	maxRetries       int
	initialBackoff   time.Duration
	maxBackoff       time.Duration
}

func loadTimeoutAndRetryConfig(logger Logger) timeoutAndRetryConfig {
	timeout := parseEnvVarDuration("GEMINI_TIMEOUT", 300*time.Second, logger)
	// HTTPWriteTimeout must outlive the outbound per-call budget so the
	// inbound connection can still write a response that finishes near the
	// deadline. Default = HTTPTimeout + 60s slack.
	return timeoutAndRetryConfig{
		timeout:          timeout,
		httpWriteTimeout: parseEnvVarDuration("GEMINI_HTTP_WRITE_TIMEOUT", timeout+60*time.Second, logger),
		maxRetries:       parseEnvVarInt("GEMINI_MAX_RETRIES", 2, logger),
		initialBackoff:   parseEnvVarDuration("GEMINI_INITIAL_BACKOFF", 1*time.Second, logger),
		maxBackoff:       parseEnvVarDuration("GEMINI_MAX_BACKOFF", 10*time.Second, logger),
	}
}

// validateAuthInterop enforces cross-section invariants between the auth and
// HTTP transport sub-configs. Currently: when auth is on, HTTPPublicURL must
// be set so RFC 9728 metadata can advertise a stable resource identifier.
func validateAuthInterop(auth authConfig, httpCfg httpTransportConfig) error {
	if auth.enabled && httpCfg.publicURL == "" {
		return fmt.Errorf(
			"GEMINI_HTTP_PUBLIC_URL is required when authentication is enabled: " +
				"set it to the externally-facing resource URL (e.g. https://mcp.example.com/mcp) " +
				"so RFC 9728 metadata can be served")
	}
	return nil
}

// NewConfig creates a new configuration from environment variables
func NewConfig(logger Logger) (*Config, error) {
	provider, err := loadProviderConfig(logger)
	if err != nil {
		return nil, err
	}
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	geminiModel := os.Getenv("GEMINI_MODEL")
	if geminiModel == "" {
		geminiModel = defaultGeminiModel
	}

	geminiTemperature := parseEnvVarFloat("GEMINI_TEMPERATURE", defaultGeminiTemperature, logger)
	if geminiTemperature < 0.0 || geminiTemperature > 1.0 {
		return nil, fmt.Errorf("GEMINI_TEMPERATURE must be between 0.0 and 1.0, got %v", geminiTemperature)
	}

	tr := loadTimeoutAndRetryConfig(logger)
	github := loadGitHubConfig(logger)
	task := loadTaskConfig(logger)
	providerMaxTokens := parseEnvVarInt("PROVIDER_MAX_TOKENS", 0, logger)
	if providerMaxTokens < 0 {
		logger.Warnf("PROVIDER_MAX_TOKENS must be non-negative. Using default: 0")
		providerMaxTokens = 0
	}
	httpCfg, err := loadHTTPConfig(logger)
	if err != nil {
		return nil, err
	}
	auth, err := loadAuthConfig(logger)
	if err != nil {
		return nil, err
	}
	if err := validateAuthInterop(auth, httpCfg); err != nil {
		return nil, err
	}
	return assembleConfig(provider, geminiAPIKey, geminiModel, geminiTemperature, int32(providerMaxTokens), tr, github, task, httpCfg, auth), nil
}

// loadProviderConfig parses and validates the provider-specific environment.
func loadProviderConfig(logger Logger) (ProviderConfig, error) {
	vendor := strings.ToLower(strings.TrimSpace(os.Getenv("PROVIDER")))
	if vendor == "" {
		vendor = "gemini"
	}
	provider := ProviderConfig{Vendor: vendor}
	providerAPIKey := os.Getenv("PROVIDER_API_KEY")
	providerBaseURL := os.Getenv("PROVIDER_BASE_URL")
	providerModel := os.Getenv("PROVIDER_MODEL")

	switch vendor {
	case "gemini":
		if os.Getenv("GEMINI_API_KEY") == "" {
			return ProviderConfig{}, errors.New("GEMINI_API_KEY environment variable is required")
		}
		warnIgnoredProviderVars(logger, providerAPIKey, providerBaseURL, providerModel)
		return provider, nil
	case "deepseek":
		return loadDeepSeekProviderConfig(providerAPIKey, providerBaseURL, providerModel)
	default:
		return ProviderConfig{}, fmt.Errorf("unsupported PROVIDER value %q; valid values: gemini, deepseek", vendor)
	}
}

// warnIgnoredProviderVars reports provider-specific settings that do not apply
// while Gemini remains the selected backend.
func warnIgnoredProviderVars(logger Logger, apiKey, baseURL, model string) {
	if apiKey != "" || baseURL != "" || model != "" {
		logger.Warnf("PROVIDER_API_KEY, PROVIDER_BASE_URL, and PROVIDER_MODEL are ignored when PROVIDER=gemini")
	}
}

// loadDeepSeekProviderConfig validates DeepSeek settings and applies its base
// URL default.
func loadDeepSeekProviderConfig(apiKey, baseURL, model string) (ProviderConfig, error) {
	if apiKey == "" {
		return ProviderConfig{}, errors.New("PROVIDER_API_KEY environment variable is required when PROVIDER=deepseek")
	}
	if model == "" {
		return ProviderConfig{}, errors.New("PROVIDER_MODEL environment variable is required when PROVIDER=deepseek")
	}
	if !slices.Contains(deepseekModels, model) {
		return ProviderConfig{}, fmt.Errorf("PROVIDER_MODEL %q is unsupported for deepseek; allowed values: %s", model,
			strings.Join(deepseekModels, ", "))
	}
	if baseURL == "" {
		baseURL = defaultDeepSeekBaseURL
	}
	return ProviderConfig{Vendor: "deepseek", APIKey: apiKey, BaseURL: baseURL, Model: model}, nil
}

// assembleConfig builds the public Config struct from the sub-section values
// loaded by NewConfig. Factored out so NewConfig stays focused on env parsing
// and cross-section validation.
func assembleConfig(
	provider ProviderConfig,
	geminiAPIKey, geminiModel string,
	geminiTemperature float64,
	providerMaxTokens int32,
	tr timeoutAndRetryConfig,
	github githubSettings,
	task taskExecConfig,
	httpCfg httpTransportConfig,
	auth authConfig,
) *Config {
	return &Config{
		Provider:                       provider,
		GeminiAPIKey:                   geminiAPIKey,
		GeminiModel:                    geminiModel,
		GeminiTemperature:              geminiTemperature,
		ProviderMaxTokens:              providerMaxTokens,
		HTTPTimeout:                    tr.timeout,
		HTTPWriteTimeout:               tr.httpWriteTimeout,
		EnableHTTP:                     httpCfg.enableHTTP,
		HTTPAddress:                    httpCfg.address,
		HTTPPath:                       httpCfg.path,
		HTTPStateless:                  httpCfg.stateless,
		HTTPHeartbeat:                  httpCfg.heartbeat,
		HTTPCORSEnabled:                httpCfg.corsEnabled,
		HTTPCORSOrigins:                httpCfg.corsOrigins,
		ProgressInterval:               httpCfg.progressInterval,
		HTTPPublicURL:                  httpCfg.publicURL,
		HTTPDisableLocalhostProtection: httpCfg.disableLocalhostProtection,
		MaxConcurrentTasks:             task.maxConcurrentTasks,

		AuthEnabled:    auth.enabled,
		AuthSecretKey:  auth.secretKey,
		MaxRetries:     tr.maxRetries,
		InitialBackoff: tr.initialBackoff,
		MaxBackoff:     tr.maxBackoff,

		GitHubToken:               github.token,
		GitHubAPIBaseURL:          github.apiBaseURL,
		MaxGitHubFiles:            github.maxGitHubFiles,
		MaxGitHubFileSize:         github.maxGitHubFileSize,
		MaxGitHubDiffBytes:        github.maxGitHubDiffBytes,
		MaxGitHubCommits:          github.maxGitHubCommits,
		MaxGitHubPRReviewComments: github.maxGitHubPRReviewComments,

		Prequalify: task.prequalify,
	}
}
