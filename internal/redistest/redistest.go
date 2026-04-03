// Package redistest configures Redis for tests: in-process miniredis by default,
// or a real Redis when BLOCKCHAIN_EXPLORER_TEST_REDIS=integration.
//
// Repository integration tests (TTL, keys, session/CSRF) live in internal/repos/redis_integration_test.go.
// The HTTP server test harness uses the same env variable in internal/server TestMain.
package redistest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnvUseRedis selects how tests obtain Redis:
//   - unset (default): in-process miniredis (no network, no Docker).
//   - "integration": real Redis; set TEST_REDIS_ADDR or REDIS_HOST and optional REDIS_PORT (default 6379).
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
	port := redisPortFromEnv()
	return fmt.Sprintf("%s:%d", host, port)
}

func redisPortFromEnv() int {
	s := strings.TrimSpace(os.Getenv("REDIS_PORT"))
	if s == "" {
		return 6379
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 6379
	}
	return n
}

// UseIntegration returns true when tests should dial a real Redis.
func UseIntegration() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvUseRedis)), ModeIntegration)
}
