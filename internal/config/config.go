package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/CryptoD/blockchain-explorer/internal/export"
)

// Config holds all application configuration derived from environment variables.
type Config struct {
	AppEnv string

	// Core infrastructure
	RedisHost string
	RedisPort int // TCP port; default 6379 (env REDIS_PORT)

	// Default admin (env ADMIN_USERNAME / ADMIN_PASSWORD). Required outside development; validated in Validate().
	AdminUsername string
	AdminPassword string

	// External providers
	GetBlockBaseURL     string
	GetBlockAccessToken string
	SentryDSN           string

	// Email delivery
	EmailProvider  string
	EmailFrom      string
	EmailFromName  string
	AdminEmail     string
	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPPassword   string
	SMTPStartTLS   bool
	SMTPSkipVerify bool
	AppBaseURL     string

	// News provider (contextual / financial news)
	NewsProvider                string
	TheNewsAPIBaseURL           string
	TheNewsAPIToken             string
	TheNewsAPIDefaultSearch     string
	TheNewsAPIDefaultLanguage   string
	TheNewsAPIDefaultLocale     string
	TheNewsAPIDefaultCategories string
	NewsCacheTTLSeconds         int // "fresh" cache TTL; default 300 (5m)
	NewsStaleTTLSeconds         int // "stale fallback" TTL; default 3600 (1h)

	// Security / cookies
	SecureCookies bool
	// HSTS: if HSTSMaxAgeSeconds > 0, handlers may set Strict-Transport-Security for HTTPS requests only.
	HSTSMaxAgeSeconds     int
	HSTSIncludeSubdomains bool

	// Rate limiting
	RateLimitWindowSeconds int
	RateLimitPerIP         int
	RateLimitPerUser       int

	// Export-specific rate limiting (stricter to prevent abuse)
	ExportRateLimitPerIP        int // per window; default 5
	ExportRateLimitPerUser      int // per window when authenticated; default 20
	ExportRateLimitHeavyPerIP   int // for heavy exports (e.g. transactions CSV); default 2
	ExportRateLimitHeavyPerUser int // when authenticated; default 5

	// Request body limits (POST/PUT/PATCH). See docs/INPUT_LIMITS.md.
	MaxRequestBodyBytes int64 // 0 = unlimited; default 1 MiB from env
	MaxJSONDepth        int   // 0 = skip JSON nesting check; default 64
	// CSV export: optional caps at or below export package maxima (0 = use export defaults only)
	ExportMaxBlockCSVRows       int
	ExportMaxTransactionCSVRows int

	// Readiness / health
	ReadyCheckExternal bool

	// Rates (multi-currency): cache TTL and effective update interval in seconds
	RatesCacheTTLSeconds int // Redis key TTL for rate data; default 60

	// Prometheus metrics at GET /metrics
	MetricsEnabled        bool
	MetricsToken          string // optional; if set, require Authorization: Bearer <token> or X-Metrics-Token
	MetricsRateLimitPerIP int    // when MetricsEnabled and MetricsToken is empty: max GET /metrics per IP per window (0 = unlimited)

	// Runtime profiling at /debug/pprof (net/http/pprof). Off by default; enable only for load/perf diagnosis.
	PPROFEnabled bool

	// Redis connection pool (go-redis). Timeouts bound how long commands wait on the wire; pool limits concurrency to Redis.
	// RedisPoolSize 0 = library default (10 × GOMAXPROCS). RedisMaxActiveConns 0 = no hard cap (not recommended at scale).
	RedisPoolSize            int
	RedisMaxActiveConns      int
	RedisDialTimeoutSeconds  int
	RedisReadTimeoutSeconds  int
	RedisWriteTimeoutSeconds int
	RedisPoolTimeoutSeconds  int // wait for a free conn; 0 = library default (ReadTimeout + 1s)
	RedisConnMaxIdleSeconds  int // 0 = library default (~30m idle)
	RedisMinIdleConns        int

	// Shared HTTP client for outbound JSON-RPC, pricing, news. Default timeout matches server JSON-RPC SLA (30s handler budget).
	OutboundHTTPTimeoutSeconds               int
	OutboundHTTPDialTimeoutSeconds           int
	OutboundHTTPResponseHeaderTimeoutSeconds int
	OutboundHTTPMaxConnsPerHost              int
	OutboundHTTPMaxIdleConns                 int
	OutboundHTTPMaxIdleConnsPerHost          int
	OutboundHTTPIdleConnTimeoutSeconds       int

	// Response compression for HTML/JSON/text (skips already-compressed static types).
	// When Brotli is enabled, negotiates br then gzip via Accept-Encoding; otherwise gzip only (less CPU).
	ResponseCompressionEnabled bool
	ResponseCompressionBrotli  bool

	// CDNBaseURL is optional; must match CDN_BASE_URL used when running stamp-frontend-assets (no trailing slash).
	// Expands CSP script-src / style-src / img-src for static assets served from a CDN (roadmap 49).
	CDNBaseURL string
	// StaticAssetCacheMaxAgeSeconds: when > 0, emit Cache-Control public,max-age,immutable for /static,/dist,/images.
	// Use only with versioned asset URLs (npm run build). Env STATIC_ASSET_CACHE_MAX_AGE_SECONDS.
	StaticAssetCacheMaxAgeSeconds int

	// ShutdownGraceSeconds: max time for http.Server.Shutdown to drain in-flight requests on SIGINT/SIGTERM (default 30).
	ShutdownGraceSeconds int
	// RedisCloseTimeoutSeconds: max time to wait for Redis client Close() during shutdown (default 5).
	RedisCloseTimeoutSeconds int

	// Sentry (optional; DSN from SENTRY_DSN)
	SentryEnvironment      string  // SENTRY_ENVIRONMENT; defaults to AppEnv
	SentryRelease          string  // SENTRY_RELEASE (build/version)
	SentryTracesSampleRate float64 // SENTRY_TRACES_SAMPLE_RATE; default 1.0 dev, 0.15 prod
	SentryErrorSampleRate  float64 // SENTRY_SAMPLE_RATE for error events; default 1.0
}

// Load parses environment variables into a Config struct and validates
// required settings. It is intended to be called once at startup.
func Load() (*Config, error) {
	appEnv := GetAppEnv()

	cfg := &Config{
		AppEnv:                                   appEnv,
		RedisHost:                                GetEnvWithDefault("REDIS_HOST", "localhost"),
		RedisPort:                                GetEnvIntWithDefault("REDIS_PORT", 6379),
		AdminUsername:                            strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		AdminPassword:                            os.Getenv("ADMIN_PASSWORD"),
		GetBlockBaseURL:                          strings.TrimSpace(os.Getenv("GETBLOCK_BASE_URL")),
		GetBlockAccessToken:                      strings.TrimSpace(os.Getenv("GETBLOCK_ACCESS_TOKEN")),
		SentryDSN:                                strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		EmailProvider:                            strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_PROVIDER"))),
		EmailFrom:                                strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		EmailFromName:                            strings.TrimSpace(os.Getenv("EMAIL_FROM_NAME")),
		AdminEmail:                               strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		SMTPHost:                                 strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:                                 GetEnvIntWithDefault("SMTP_PORT", 587),
		SMTPUsername:                             strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:                             strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPStartTLS:                             strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_STARTTLS"))) == "true",
		SMTPSkipVerify:                           strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_SKIP_VERIFY"))) == "true",
		AppBaseURL:                               strings.TrimSpace(os.Getenv("APP_BASE_URL")),
		NewsProvider:                             strings.ToLower(strings.TrimSpace(os.Getenv("NEWS_PROVIDER"))),
		TheNewsAPIBaseURL:                        strings.TrimSpace(os.Getenv("THENEWSAPI_BASE_URL")),
		TheNewsAPIToken:                          strings.TrimSpace(os.Getenv("THENEWSAPI_API_TOKEN")),
		TheNewsAPIDefaultSearch:                  strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_SEARCH")),
		TheNewsAPIDefaultLanguage:                strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_LANGUAGE")),
		TheNewsAPIDefaultLocale:                  strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_LOCALE")),
		TheNewsAPIDefaultCategories:              strings.TrimSpace(os.Getenv("THENEWSAPI_DEFAULT_CATEGORIES")),
		NewsCacheTTLSeconds:                      GetEnvIntWithDefault("NEWS_CACHE_TTL_SECONDS", 300),
		NewsStaleTTLSeconds:                      GetEnvIntWithDefault("NEWS_STALE_TTL_SECONDS", 3600),
		SecureCookies:                            UseSecureCookies(),
		HSTSMaxAgeSeconds:                        GetEnvIntWithDefault("HSTS_MAX_AGE_SECONDS", 0),
		HSTSIncludeSubdomains:                    strings.EqualFold(strings.TrimSpace(os.Getenv("HSTS_INCLUDE_SUBDOMAINS")), "true"),
		RateLimitWindowSeconds:                   GetEnvIntWithDefault("RATE_LIMIT_WINDOW_SECONDS", 60),
		RateLimitPerIP:                           GetEnvIntWithDefault("RATE_LIMIT_PER_IP", 10),
		RateLimitPerUser:                         GetEnvIntWithDefault("RATE_LIMIT_PER_USER", 10),
		ExportRateLimitPerIP:                     GetEnvIntWithDefault("EXPORT_RATE_LIMIT_PER_IP", 5),
		ExportRateLimitPerUser:                   GetEnvIntWithDefault("EXPORT_RATE_LIMIT_PER_USER", 20),
		ExportRateLimitHeavyPerIP:                GetEnvIntWithDefault("EXPORT_RATE_LIMIT_HEAVY_PER_IP", 2),
		ExportRateLimitHeavyPerUser:              GetEnvIntWithDefault("EXPORT_RATE_LIMIT_HEAVY_PER_USER", 5),
		MaxRequestBodyBytes:                      GetEnvInt64WithDefault("MAX_REQUEST_BODY_BYTES", 1024*1024),
		MaxJSONDepth:                             GetEnvIntWithDefault("MAX_JSON_DEPTH", 64),
		ExportMaxBlockCSVRows:                    GetEnvIntWithDefault("EXPORT_MAX_BLOCK_CSV_ROWS", 0),
		ExportMaxTransactionCSVRows:              GetEnvIntWithDefault("EXPORT_MAX_TRANSACTION_CSV_ROWS", 0),
		ReadyCheckExternal:                       strings.ToLower(os.Getenv("READY_CHECK_EXTERNAL")) == "true",
		RatesCacheTTLSeconds:                     GetEnvIntWithDefault("RATES_CACHE_TTL_SECONDS", 60),
		MetricsEnabled:                           metricsEnabledFromEnv(),
		MetricsToken:                             strings.TrimSpace(os.Getenv("METRICS_TOKEN")),
		MetricsRateLimitPerIP:                    GetEnvIntWithDefault("METRICS_RATE_LIMIT_PER_IP", 120),
		PPROFEnabled:                             strings.EqualFold(strings.TrimSpace(os.Getenv("PPROF_ENABLED")), "true"),
		RedisPoolSize:                            GetEnvIntWithDefault("REDIS_POOL_SIZE", 0),
		RedisMaxActiveConns:                      GetEnvIntWithDefault("REDIS_MAX_ACTIVE_CONNS", 128),
		RedisDialTimeoutSeconds:                  GetEnvIntWithDefault("REDIS_DIAL_TIMEOUT_SECONDS", 5),
		RedisReadTimeoutSeconds:                  GetEnvIntWithDefault("REDIS_READ_TIMEOUT_SECONDS", 5),
		RedisWriteTimeoutSeconds:                 GetEnvIntWithDefault("REDIS_WRITE_TIMEOUT_SECONDS", 5),
		RedisPoolTimeoutSeconds:                  GetEnvIntWithDefault("REDIS_POOL_TIMEOUT_SECONDS", 0),
		RedisConnMaxIdleSeconds:                  GetEnvIntWithDefault("REDIS_CONN_MAX_IDLE_SECONDS", 300),
		RedisMinIdleConns:                        GetEnvIntWithDefault("REDIS_MIN_IDLE_CONNS", 2),
		OutboundHTTPTimeoutSeconds:               GetEnvIntWithDefault("OUTBOUND_HTTP_TIMEOUT_SECONDS", 30),
		OutboundHTTPDialTimeoutSeconds:           GetEnvIntWithDefault("OUTBOUND_HTTP_DIAL_TIMEOUT_SECONDS", 10),
		OutboundHTTPResponseHeaderTimeoutSeconds: GetEnvIntWithDefault("OUTBOUND_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS", 30),
		OutboundHTTPMaxConnsPerHost:              GetEnvIntWithDefault("OUTBOUND_HTTP_MAX_CONNS_PER_HOST", 64),
		OutboundHTTPMaxIdleConns:                 GetEnvIntWithDefault("OUTBOUND_HTTP_MAX_IDLE_CONNS", 128),
		OutboundHTTPMaxIdleConnsPerHost:          GetEnvIntWithDefault("OUTBOUND_HTTP_MAX_IDLE_CONNS_PER_HOST", 32),
		OutboundHTTPIdleConnTimeoutSeconds:       GetEnvIntWithDefault("OUTBOUND_HTTP_IDLE_CONN_TIMEOUT_SECONDS", 90),
		ResponseCompressionEnabled:               responseCompressionEnabledFromEnv(),
		ResponseCompressionBrotli:                responseCompressionBrotliFromEnv(),
		CDNBaseURL:                               strings.TrimRight(strings.TrimSpace(os.Getenv("CDN_BASE_URL")), "/"),
		StaticAssetCacheMaxAgeSeconds:            GetEnvIntWithDefault("STATIC_ASSET_CACHE_MAX_AGE_SECONDS", 0),
		ShutdownGraceSeconds:                     GetEnvIntWithDefault("SHUTDOWN_GRACE_SECONDS", 30),
		RedisCloseTimeoutSeconds:                 GetEnvIntWithDefault("REDIS_CLOSE_TIMEOUT_SECONDS", 5),
		SentryEnvironment:                        strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT")),
		SentryRelease:                            strings.TrimSpace(os.Getenv("SENTRY_RELEASE")),
		SentryTracesSampleRate:                   sentryTracesSampleRateForEnv(appEnv),
		SentryErrorSampleRate:                    sentryErrorSampleRateFromEnv(),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks configuration for invalid combinations and required fields. Call after Load populates the struct.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

	c.ensureConnectionPoolDefaults()
	c.ensureShutdownDefaults()

	if strings.TrimSpace(c.GetBlockBaseURL) == "" || strings.TrimSpace(c.GetBlockAccessToken) == "" {
		return fmt.Errorf("GETBLOCK_BASE_URL and GETBLOCK_ACCESS_TOKEN are required")
	}

	if c.RedisPort < 1 || c.RedisPort > 65535 {
		return fmt.Errorf("REDIS_PORT must be between 1 and 65535")
	}

	if c.HSTSMaxAgeSeconds < 0 || c.HSTSMaxAgeSeconds > 63072000 {
		return fmt.Errorf("HSTS_MAX_AGE_SECONDS must be between 0 and 63072000")
	}

	if c.RateLimitWindowSeconds < 0 {
		return fmt.Errorf("RATE_LIMIT_WINDOW_SECONDS must be >= 0")
	}
	if c.RateLimitPerIP < 0 || c.RateLimitPerUser < 0 {
		return fmt.Errorf("rate limit counts must be >= 0")
	}
	if c.MetricsRateLimitPerIP < 0 {
		return fmt.Errorf("METRICS_RATE_LIMIT_PER_IP must be >= 0")
	}
	if c.ExportRateLimitPerIP < 0 || c.ExportRateLimitPerUser < 0 || c.ExportRateLimitHeavyPerIP < 0 || c.ExportRateLimitHeavyPerUser < 0 {
		return fmt.Errorf("export rate limit counts must be >= 0")
	}
	if c.MaxRequestBodyBytes < 0 {
		return fmt.Errorf("MAX_REQUEST_BODY_BYTES must be >= 0")
	}
	if c.MaxRequestBodyBytes > 0 && c.MaxRequestBodyBytes < 1024 {
		return fmt.Errorf("MAX_REQUEST_BODY_BYTES must be 0 (unlimited) or at least 1024")
	}
	const maxBodyBytes = 100 << 20 // 100 MiB
	if c.MaxRequestBodyBytes > maxBodyBytes {
		return fmt.Errorf("MAX_REQUEST_BODY_BYTES cannot exceed %d", maxBodyBytes)
	}
	if c.MaxJSONDepth < 0 {
		return fmt.Errorf("MAX_JSON_DEPTH must be >= 0")
	}
	if c.ExportMaxBlockCSVRows < 0 || c.ExportMaxTransactionCSVRows < 0 {
		return fmt.Errorf("EXPORT_MAX_*_CSV_ROWS must be >= 0")
	}
	if c.ExportMaxBlockCSVRows > 0 && c.ExportMaxBlockCSVRows > export.MaxBlockRows {
		return fmt.Errorf("EXPORT_MAX_BLOCK_CSV_ROWS cannot exceed %d", export.MaxBlockRows)
	}
	if c.ExportMaxTransactionCSVRows > 0 && c.ExportMaxTransactionCSVRows > export.MaxTxRows {
		return fmt.Errorf("EXPORT_MAX_TRANSACTION_CSV_ROWS cannot exceed %d", export.MaxTxRows)
	}
	if c.RatesCacheTTLSeconds < 0 {
		return fmt.Errorf("RATES_CACHE_TTL_SECONDS must be >= 0")
	}
	if c.NewsCacheTTLSeconds < 0 || c.NewsStaleTTLSeconds < 0 {
		return fmt.Errorf("NEWS_CACHE_TTL_SECONDS and NEWS_STALE_TTL_SECONDS must be >= 0")
	}
	if c.SMTPPort < 0 || c.SMTPPort > 65535 {
		return fmt.Errorf("SMTP_PORT must be between 0 and 65535")
	}
	if c.SMTPSkipVerify && !strings.EqualFold(c.AppEnv, "development") {
		return fmt.Errorf("SMTP_SKIP_VERIFY=true is only allowed when APP_ENV=development (got APP_ENV=%q); production and staging must verify SMTP TLS certificates", c.AppEnv)
	}
	if c.SentryTracesSampleRate < 0 || c.SentryTracesSampleRate > 1 {
		return fmt.Errorf("SENTRY_TRACES_SAMPLE_RATE must be between 0 and 1")
	}
	if c.SentryErrorSampleRate < 0 || c.SentryErrorSampleRate > 1 {
		return fmt.Errorf("SENTRY_SAMPLE_RATE must be between 0 and 1")
	}

	// Non-development: require explicit admin credentials (same rules as initializeDefaultAdmin).
	if !strings.EqualFold(c.AppEnv, "development") {
		if strings.TrimSpace(c.AdminUsername) == "" || c.AdminPassword == "" {
			return fmt.Errorf("ADMIN_USERNAME and ADMIN_PASSWORD must be set when APP_ENV is not development (got APP_ENV=%q)", c.AppEnv)
		}
		if !isStrongAdminPassword(c.AdminPassword) {
			return fmt.Errorf("ADMIN_PASSWORD must be 8-128 characters and include at least one letter and one digit when APP_ENV is not development")
		}
	}

	if err := c.validateConnectionPools(); err != nil {
		return err
	}

	if c.StaticAssetCacheMaxAgeSeconds < 0 || c.StaticAssetCacheMaxAgeSeconds > 63072000 {
		return fmt.Errorf("STATIC_ASSET_CACHE_MAX_AGE_SECONDS must be between 0 and 63072000 (0 disables long-cache headers)")
	}
	if strings.TrimSpace(c.CDNBaseURL) != "" {
		u, err := url.Parse(c.CDNBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" && u.Path != "/" {
			return fmt.Errorf("CDN_BASE_URL must be a valid absolute URL with scheme and host only (no path), got %q", c.CDNBaseURL)
		}
	}

	if c.ShutdownGraceSeconds < 1 || c.ShutdownGraceSeconds > 600 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be between 1 and 600")
	}
	if c.RedisCloseTimeoutSeconds < 1 || c.RedisCloseTimeoutSeconds > 120 {
		return fmt.Errorf("REDIS_CLOSE_TIMEOUT_SECONDS must be between 1 and 120")
	}

	return nil
}

// ensureConnectionPoolDefaults fills zero values so hand-built Config (tests) and partial env still get safe timeouts.
func (c *Config) ensureConnectionPoolDefaults() {
	if c.RedisDialTimeoutSeconds <= 0 {
		c.RedisDialTimeoutSeconds = 5
	}
	if c.RedisReadTimeoutSeconds <= 0 {
		c.RedisReadTimeoutSeconds = 5
	}
	if c.RedisWriteTimeoutSeconds <= 0 {
		c.RedisWriteTimeoutSeconds = 5
	}
	if c.OutboundHTTPTimeoutSeconds <= 0 {
		c.OutboundHTTPTimeoutSeconds = 30
	}
	if c.OutboundHTTPDialTimeoutSeconds <= 0 {
		c.OutboundHTTPDialTimeoutSeconds = 10
	}
	if c.OutboundHTTPResponseHeaderTimeoutSeconds <= 0 {
		c.OutboundHTTPResponseHeaderTimeoutSeconds = c.OutboundHTTPTimeoutSeconds
	}
	if c.OutboundHTTPMaxConnsPerHost <= 0 {
		c.OutboundHTTPMaxConnsPerHost = 64
	}
	if c.OutboundHTTPMaxIdleConns <= 0 {
		c.OutboundHTTPMaxIdleConns = 128
	}
	if c.OutboundHTTPMaxIdleConnsPerHost <= 0 {
		c.OutboundHTTPMaxIdleConnsPerHost = 32
	}
	if c.OutboundHTTPIdleConnTimeoutSeconds <= 0 {
		c.OutboundHTTPIdleConnTimeoutSeconds = 90
	}
	if c.RedisMinIdleConns < 0 {
		c.RedisMinIdleConns = 0
	}
}

func (c *Config) ensureShutdownDefaults() {
	if c.ShutdownGraceSeconds <= 0 {
		c.ShutdownGraceSeconds = 30
	}
	if c.RedisCloseTimeoutSeconds <= 0 {
		c.RedisCloseTimeoutSeconds = 5
	}
}

func (c *Config) validateConnectionPools() error {
	if c.RedisPoolSize < 0 || c.RedisPoolSize > 2000 {
		return fmt.Errorf("REDIS_POOL_SIZE must be between 0 and 2000")
	}
	if c.RedisMaxActiveConns < 0 {
		return fmt.Errorf("REDIS_MAX_ACTIVE_CONNS must be >= 0")
	}
	if c.RedisPoolSize > 0 && c.RedisMaxActiveConns > 0 && c.RedisMaxActiveConns < c.RedisPoolSize {
		return fmt.Errorf("REDIS_MAX_ACTIVE_CONNS (%d) must be >= REDIS_POOL_SIZE (%d) when both are non-zero", c.RedisMaxActiveConns, c.RedisPoolSize)
	}
	if c.RedisMinIdleConns > 500 {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be <= 500")
	}
	if c.RedisPoolSize > 0 && c.RedisMinIdleConns > c.RedisPoolSize {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be <= REDIS_POOL_SIZE when pool size is set")
	}
	if c.RedisDialTimeoutSeconds < 1 || c.RedisDialTimeoutSeconds > 120 {
		return fmt.Errorf("REDIS_DIAL_TIMEOUT_SECONDS must be between 1 and 120")
	}
	if c.RedisReadTimeoutSeconds < 1 || c.RedisReadTimeoutSeconds > 120 {
		return fmt.Errorf("REDIS_READ_TIMEOUT_SECONDS must be between 1 and 120")
	}
	if c.RedisWriteTimeoutSeconds < 1 || c.RedisWriteTimeoutSeconds > 120 {
		return fmt.Errorf("REDIS_WRITE_TIMEOUT_SECONDS must be between 1 and 120")
	}
	if c.RedisPoolTimeoutSeconds < 0 || c.RedisPoolTimeoutSeconds > 120 {
		return fmt.Errorf("REDIS_POOL_TIMEOUT_SECONDS must be between 0 and 120 (0 = library default)")
	}
	if c.RedisConnMaxIdleSeconds < 0 || c.RedisConnMaxIdleSeconds > 86400 {
		return fmt.Errorf("REDIS_CONN_MAX_IDLE_SECONDS must be between 0 and 86400 (0 = library default)")
	}
	if c.OutboundHTTPTimeoutSeconds < 1 || c.OutboundHTTPTimeoutSeconds > 600 {
		return fmt.Errorf("OUTBOUND_HTTP_TIMEOUT_SECONDS must be between 1 and 600")
	}
	if c.OutboundHTTPDialTimeoutSeconds < 1 || c.OutboundHTTPDialTimeoutSeconds > c.OutboundHTTPTimeoutSeconds {
		return fmt.Errorf("OUTBOUND_HTTP_DIAL_TIMEOUT_SECONDS must be between 1 and OUTBOUND_HTTP_TIMEOUT_SECONDS (%d)", c.OutboundHTTPTimeoutSeconds)
	}
	if c.OutboundHTTPResponseHeaderTimeoutSeconds < 1 || c.OutboundHTTPResponseHeaderTimeoutSeconds > c.OutboundHTTPTimeoutSeconds {
		return fmt.Errorf("OUTBOUND_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS must be between 1 and OUTBOUND_HTTP_TIMEOUT_SECONDS (%d)", c.OutboundHTTPTimeoutSeconds)
	}
	if c.OutboundHTTPMaxConnsPerHost < 1 || c.OutboundHTTPMaxConnsPerHost > 4096 {
		return fmt.Errorf("OUTBOUND_HTTP_MAX_CONNS_PER_HOST must be between 1 and 4096")
	}
	if c.OutboundHTTPMaxIdleConns < c.OutboundHTTPMaxIdleConnsPerHost {
		return fmt.Errorf("OUTBOUND_HTTP_MAX_IDLE_CONNS must be >= OUTBOUND_HTTP_MAX_IDLE_CONNS_PER_HOST")
	}
	if c.OutboundHTTPIdleConnTimeoutSeconds < 1 || c.OutboundHTTPIdleConnTimeoutSeconds > 600 {
		return fmt.Errorf("OUTBOUND_HTTP_IDLE_CONN_TIMEOUT_SECONDS must be between 1 and 600")
	}
	return nil
}

// RedisAddr returns host:port for the Redis server.
func (c *Config) RedisAddr() string {
	if c == nil {
		return "localhost:6379"
	}
	return fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort)
}

func isStrongAdminPassword(pw string) bool {
	if len(pw) < 8 || len(pw) > 128 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range pw {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

func sentryTracesSampleRateForEnv(appEnv string) float64 {
	s := strings.TrimSpace(os.Getenv("SENTRY_TRACES_SAMPLE_RATE"))
	if s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err == nil && v >= 0 && v <= 1 {
			return v
		}
	}
	if appEnv == "development" {
		return 1.0
	}
	return 0.15
}

func sentryErrorSampleRateFromEnv() float64 {
	s := strings.TrimSpace(os.Getenv("SENTRY_SAMPLE_RATE"))
	if s == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || v > 1 {
		return 1.0
	}
	return v
}

func metricsEnabledFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("METRICS_ENABLED")))
	switch v {
	case "", "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return true
	}
}

func responseCompressionEnabledFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RESPONSE_COMPRESSION_ENABLED")))
	switch v {
	case "0", "false", "no":
		return false
	default:
		return true
	}
}

func responseCompressionBrotliFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RESPONSE_COMPRESSION_BROTLI")))
	switch v {
	case "0", "false", "no":
		return false
	default:
		return true
	}
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

// GetEnvInt64WithDefault reads an environment variable and parses it as int64,
// returning defaultValue if unset or invalid.
func GetEnvInt64WithDefault(key string, defaultValue int64) int64 {
	valStr := strings.TrimSpace(os.Getenv(key))
	if valStr == "" {
		return defaultValue
	}
	if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
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
