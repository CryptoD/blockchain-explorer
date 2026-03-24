// Package redistest configures Redis for tests: in-process miniredis by default,
// or a real Redis when BLOCKCHAIN_EXPLORER_TEST_REDIS=integration.
package redistest

import (
	"os"
	"strings"
)

// EnvUseRedis selects how tests obtain Redis:
//   - unset (default): in-process miniredis (no network, no Docker).
//   - "integration": real Redis; set TEST_REDIS_ADDR or REDIS_HOST.
const EnvUseRedis = "BLOCKCHAIN_EXPLORER_TEST_REDIS"

// ModeIntegration uses a real Redis server (see IntegrationAddr).
const ModeIntegration = "integration"

// IntegrationAddr returns host:port for integration tests.
func IntegrationAddr() string {
	if a := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR")); a != "" {
		return a
	}
	host := strings.TrimSpace(os.Getenv("REDIS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	return host + ":6379"
}

// UseIntegration returns true when tests should dial a real Redis.
func UseIntegration() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvUseRedis)), ModeIntegration)
}
