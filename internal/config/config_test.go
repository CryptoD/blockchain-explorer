package config

import (
	"strings"
	"testing"
)

func minimalValidBase() *Config {
	return &Config{
		AppEnv:              "development",
		RedisHost:           "localhost",
		RedisPort:           6379,
		GetBlockBaseURL:     "https://example.test",
		GetBlockAccessToken: "token",
	}
}

func TestValidate_GetBlockRequired(t *testing.T) {
	c := minimalValidBase()
	c.GetBlockBaseURL = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "GETBLOCK") {
		t.Fatalf("expected GETBLOCK error, got %v", err)
	}
}

func TestValidate_ProductionRequiresAdminPassword(t *testing.T) {
	c := minimalValidBase()
	c.AppEnv = "production"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ADMIN_USERNAME") {
		t.Fatalf("expected admin env error, got %v", err)
	}

	c.AdminUsername = "admin"
	c.AdminPassword = "short"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("expected weak password error, got %v", err)
	}

	c.AdminPassword = "Str0ngEnough"
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_SentrySampleRates(t *testing.T) {
	c := minimalValidBase()
	c.SentryTracesSampleRate = 1.5
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for traces sample rate")
	}
	c.SentryTracesSampleRate = 0.1
	c.SentryErrorSampleRate = -0.1
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for error sample rate")
	}
}

func TestValidate_SMTPPort(t *testing.T) {
	c := minimalValidBase()
	c.SMTPPort = 70000
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for SMTP port")
	}
}

func TestValidate_SMTPSkipVerifyOnlyInDevelopment(t *testing.T) {
	c := minimalValidBase()
	c.SMTPSkipVerify = true
	if err := c.Validate(); err != nil {
		t.Fatalf("development should allow SMTP_SKIP_VERIFY: %v", err)
	}

	c.AppEnv = "production"
	c.AdminUsername = "admin"
	c.AdminPassword = "Str0ngEnough"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "SMTP_SKIP_VERIFY") {
		t.Fatalf("expected SMTP_SKIP_VERIFY error for production, got %v", err)
	}
}

func TestValidate_RedisPort(t *testing.T) {
	c := minimalValidBase()
	c.RedisPort = 0
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_PORT") {
		t.Fatalf("expected REDIS_PORT error, got %v", err)
	}
}

func TestValidate_ExportCSVRowCaps(t *testing.T) {
	c := minimalValidBase()
	c.ExportMaxBlockCSVRows = 2001
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "EXPORT_MAX_BLOCK_CSV_ROWS") {
		t.Fatalf("expected EXPORT_MAX_BLOCK_CSV_ROWS error, got %v", err)
	}
	c.ExportMaxBlockCSVRows = 0
	c.ExportMaxTransactionCSVRows = 6000
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "EXPORT_MAX_TRANSACTION_CSV_ROWS") {
		t.Fatalf("expected EXPORT_MAX_TRANSACTION_CSV_ROWS error, got %v", err)
	}
	c.ExportMaxTransactionCSVRows = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_MaxRequestBodyBytes(t *testing.T) {
	c := minimalValidBase()
	c.MaxRequestBodyBytes = 500
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "MAX_REQUEST_BODY_BYTES") {
		t.Fatalf("expected MAX_REQUEST_BODY_BYTES error, got %v", err)
	}
	c.MaxRequestBodyBytes = 1024 * 1024
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_MetricsRateLimitPerIP(t *testing.T) {
	c := minimalValidBase()
	c.MetricsRateLimitPerIP = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "METRICS_RATE_LIMIT_PER_IP") {
		t.Fatalf("expected METRICS_RATE_LIMIT_PER_IP error, got %v", err)
	}
	c.MetricsRateLimitPerIP = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_HSTSMaxAge(t *testing.T) {
	c := minimalValidBase()
	c.HSTSMaxAgeSeconds = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "HSTS_MAX_AGE_SECONDS") {
		t.Fatalf("expected HSTS_MAX_AGE_SECONDS error, got %v", err)
	}
	c.HSTSMaxAgeSeconds = 63072001
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "HSTS_MAX_AGE_SECONDS") {
		t.Fatalf("expected HSTS_MAX_AGE_SECONDS error, got %v", err)
	}
	c.HSTSMaxAgeSeconds = 31536000
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_ConnectionPools(t *testing.T) {
	c := minimalValidBase()
	c.RedisPoolSize = 100
	c.RedisMaxActiveConns = 50
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_MAX_ACTIVE_CONNS") {
		t.Fatalf("expected pool vs max-active error, got %v", err)
	}
	c.RedisMaxActiveConns = 128
	c.OutboundHTTPTimeoutSeconds = 15
	c.OutboundHTTPResponseHeaderTimeoutSeconds = 30
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_HTTP_RESPONSE_HEADER_TIMEOUT") {
		t.Fatalf("expected response header timeout error, got %v", err)
	}
	c.OutboundHTTPResponseHeaderTimeoutSeconds = 10
	c.OutboundHTTPMaxIdleConns = 10
	c.OutboundHTTPMaxIdleConnsPerHost = 40
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_HTTP_MAX_IDLE_CONNS") {
		t.Fatalf("expected max idle conns error, got %v", err)
	}
	c.OutboundHTTPMaxIdleConns = 128
	c.OutboundHTTPMaxIdleConnsPerHost = 32
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidate_CDNBaseURL(t *testing.T) {
	c := minimalValidBase()
	c.CDNBaseURL = "https://d111111abcdef8.cloudfront.net"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.CDNBaseURL = "https://cdn.example.com/prefix"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "CDN_BASE_URL") {
		t.Fatalf("expected CDN_BASE_URL error, got %v", err)
	}
}

func TestValidate_StaticAssetCacheMaxAge(t *testing.T) {
	c := minimalValidBase()
	c.StaticAssetCacheMaxAgeSeconds = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "STATIC_ASSET_CACHE_MAX_AGE_SECONDS") {
		t.Fatalf("expected static cache error, got %v", err)
	}
	c.StaticAssetCacheMaxAgeSeconds = 31536000
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_FeatureFlagsEnv(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("REDIS_HOST", "localhost")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("GETBLOCK_BASE_URL", "https://example.test")
	t.Setenv("GETBLOCK_ACCESS_TOKEN", "token")
	t.Setenv("FEATURE_NEWS_ENABLED", "false")
	t.Setenv("FEATURE_PRICE_ALERTS_ENABLED", "0")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FeatureNewsEnabled {
		t.Fatal("expected FEATURE_NEWS_ENABLED=false")
	}
	if cfg.FeaturePriceAlertsEnabled {
		t.Fatal("expected FEATURE_PRICE_ALERTS_ENABLED=0 to be false")
	}
}

func TestValidate_Idempotency(t *testing.T) {
	c := minimalValidBase()
	c.IdempotencyTTLSeconds = 30
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_TTL_SECONDS") {
		t.Fatalf("expected idempotency TTL error, got %v", err)
	}
}

func TestValidate_OutboundRetryBudget(t *testing.T) {
	c := minimalValidBase()
	c.OutboundHTTPRetryCount = 11
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_HTTP_RETRY_COUNT") {
		t.Fatalf("expected retry count error, got %v", err)
	}
	c.OutboundHTTPRetryCount = 3
	c.OutboundHTTPInboundAttemptBudget = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_HTTP_INBOUND_ATTEMPT_BUDGET") {
		t.Fatalf("expected inbound budget error, got %v", err)
	}
}

func TestValidate_OutboundCircuitBreaker(t *testing.T) {
	c := minimalValidBase()
	c.OutboundCircuitBreakerOpenTimeoutSeconds = 700
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_CIRCUIT_BREAKER_OPEN_TIMEOUT_SECONDS") {
		t.Fatalf("expected open timeout error, got %v", err)
	}
	c.OutboundCircuitBreakerOpenTimeoutSeconds = 60
	c.OutboundCircuitBreakerTripAfterConsecutiveFailures = 101
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_CIRCUIT_BREAKER_TRIP_AFTER_CONSECUTIVE_FAILURES") {
		t.Fatalf("expected trip threshold error, got %v", err)
	}
}

func TestValidate_ShutdownTimeouts(t *testing.T) {
	c := minimalValidBase()
	c.ShutdownGraceSeconds = 601
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "SHUTDOWN_GRACE_SECONDS") {
		t.Fatalf("expected SHUTDOWN_GRACE_SECONDS error, got %v", err)
	}
	c.ShutdownGraceSeconds = 30
	c.RedisCloseTimeoutSeconds = 200
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_CLOSE_TIMEOUT_SECONDS") {
		t.Fatalf("expected REDIS_CLOSE_TIMEOUT_SECONDS error, got %v", err)
	}
}
