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

func TestValidate_RedisPort(t *testing.T) {
	c := minimalValidBase()
	c.RedisPort = 0
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_PORT") {
		t.Fatalf("expected REDIS_PORT error, got %v", err)
	}
}
