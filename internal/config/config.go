package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration derived from environment variables.
type Config struct {
	AppEnv string

	// Core infrastructure
	RedisHost string

	// External providers
	GetBlockBaseURL    string
	GetBlockAccessToken string
	SentryDSN          string

	// News provider (contextual / financial news)
	NewsProvider string
	TheNewsAPIBaseURL          string
	TheNewsAPIToken            string
	TheNewsAPIDefaultSearch    string
	TheNewsAPIDefaultLanguage  string
	TheNewsAPIDefaultLocale    string
	TheNewsAPIDefaultCategories string
	NewsCacheTTLSeconds        int // "fresh" cache TTL; default 300 (5m)
	NewsStaleTTLSeconds        int // "stale fallback" TTL; default 3600 (1h)

	// Security / cookies
	SecureCookies bool

	// Rate limiting
	RateLimitWindowSeconds int
	RateLimitPerIP         int
	RateLimitPerUser       int

	// Export-specific rate limiting (stricter to prevent abuse)
	ExportRateLimitPerIP   int // per window; default 5
	ExportRateLimitPerUser int // per window when authenticated; default 20
	ExportRateLimitHeavyPerIP   int // for heavy exports (e.g. transactions CSV); default 2
	ExportRateLimitHeavyPerUser int // when authenticated; default 5

	// Readiness / health
	ReadyCheckExternal bool

	// Rates (multi-currency): cache TTL and effective update interval in seconds
	RatesCacheTTLSeconds int // Redis key TTL for rate data; default 60
}

// Load parses environment variables into a Config struct and validates
// required settings. It is intended to be called once at startup.
func Load() (*Config, error) {
	appEnv := GetAppEnv()

	cfg := &Config{
		AppEnv:                appEnv,
		RedisHost:             GetEnvWithDefault("REDIS_HOST", "localhost"),
		GetBlockBaseURL:       strings.TrimSpace(os.Getenv("GETBLOCK_BASE_URL")),
		GetBlockAccessToken:   strings.TrimSpace(os.Getenv("GETBLOCK_ACCESS_TOKEN")),
		SentryDSN:             strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		NewsProvider:          strings.ToLower(strings.TrimSpace(os.Getenv("NEWS_PROVIDER"))),
		TheNewsAPIBaseURL:     strings.TrimSpace(os.Getenv("THENEWSAPI_BASE_URL")),
		TheNewsAPIToken:       strings.TrimSpace(os.Getenv("THENEWSAPI_API_TOKEN")),
		TheNewsAPIDefaultSearch: strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_SEARCH")),
		TheNewsAPIDefaultLanguage: strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_LANGUAGE")),
		TheNewsAPIDefaultLocale: strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_LOCALE")),
		TheNewsAPIDefaultCategories: strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_CATEGORIES")),
		NewsCacheTTLSeconds:   GetEnvIntWithDefault("NEWS_CACHE_TTL_SECONDS", 300),
		NewsStaleTTLSeconds:   GetEnvIntWithDefault("NEWS_STALE_TTL_SECONDS", 3600),
		SecureCookies:         UseSecureCookies(),
		RateLimitWindowSeconds:     GetEnvIntWithDefault("RATE_LIMIT_WINDOW_SECONDS", 60),
		RateLimitPerIP:             GetEnvIntWithDefault("RATE_LIMIT_PER_IP", 10),
		RateLimitPerUser:           GetEnvIntWithDefault("RATE_LIMIT_PER_USER", 10),
		ExportRateLimitPerIP:       GetEnvIntWithDefault("EXPORT_RATE_LIMIT_PER_IP", 5),
		ExportRateLimitPerUser:     GetEnvIntWithDefault("EXPORT_RATE_LIMIT_PER_USER", 20),
		ExportRateLimitHeavyPerIP:  GetEnvIntWithDefault("EXPORT_RATE_LIMIT_HEAVY_PER_IP", 2),
		ExportRateLimitHeavyPerUser: GetEnvIntWithDefault("EXPORT_RATE_LIMIT_HEAVY_PER_USER", 5),
		ReadyCheckExternal:         strings.ToLower(os.Getenv("READY_CHECK_EXTERNAL")) == "true",
		RatesCacheTTLSeconds:       GetEnvIntWithDefault("RATES_CACHE_TTL_SECONDS", 60),
	}

	// Required in all environments: GetBlock configuration for core blockchain operations.
	if cfg.GetBlockBaseURL == "" || cfg.GetBlockAccessToken == "" {
		return nil, fmt.Errorf("missing required environment variables GETBLOCK_BASE_URL and GETBLOCK_ACCESS_TOKEN")
	}

	return cfg, nil
}

// GetEnvWithDefault returns the value of the environment variable named by key,
// or defaultValue if the variable is not set or empty.
func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetEnvIntWithDefault reads an environment variable and parses it as int,
// returning defaultValue if unset or invalid.
func GetEnvIntWithDefault(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	if v, err := strconv.Atoi(valStr); err == nil {
		return v
	}
	return defaultValue
}

// GetAppEnv returns the current application environment, defaulting to "development".
func GetAppEnv() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		return "development"
	}
	return strings.ToLower(env)
}

// UseSecureCookies determines whether cookies should be marked Secure.
// Priority:
// - If SECURE_COOKIES is set to a truthy value, always use secure cookies.
// - Otherwise, use secure cookies for any non-development APP_ENV.
func UseSecureCookies() bool {
	if val := strings.ToLower(os.Getenv("SECURE_COOKIES")); val != "" {
		return val == "1" || val == "true" || val == "yes"
	}
	return GetAppEnv() != "development"
}

