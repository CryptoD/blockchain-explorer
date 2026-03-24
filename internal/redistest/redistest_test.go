package redistest

import "testing"

func TestIntegrationAddr_TEST_REDIS_ADDR(t *testing.T) {
	t.Setenv("TEST_REDIS_ADDR", "10.0.0.1:6380")
	t.Setenv("REDIS_HOST", "ignored")
	if got := IntegrationAddr(); got != "10.0.0.1:6380" {
		t.Fatalf("IntegrationAddr() = %q", got)
	}
}

func TestIntegrationAddr_REDIS_HOST(t *testing.T) {
	t.Setenv("TEST_REDIS_ADDR", "")
	t.Setenv("REDIS_HOST", "redis.example")
	if got := IntegrationAddr(); got != "redis.example:6379" {
		t.Fatalf("IntegrationAddr() = %q", got)
	}
}

func TestUseIntegration(t *testing.T) {
	t.Setenv(EnvUseRedis, "")
	if UseIntegration() {
		t.Fatal("expected false")
	}
	t.Setenv(EnvUseRedis, ModeIntegration)
	if !UseIntegration() {
		t.Fatal("expected true")
	}
	t.Setenv(EnvUseRedis, "INTEGRATION")
	if !UseIntegration() {
		t.Fatal("expected true for case-insensitive")
	}
}
