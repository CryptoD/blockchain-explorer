// Package redisstore documents how the application uses Redis.
//
// The server depends on redis.Cmdable (implemented by *redis.Client from go-redis).
// Unit tests use the same client type backed by in-process miniredis (see internal/redistest).
// Optional integration tests can use a real Redis via BLOCKCHAIN_EXPLORER_TEST_REDIS=integration.
//
// See internal/repos/redis_integration_test.go for miniredis-backed (and optional real-Redis) repo tests.
package redisstore

import "github.com/redis/go-redis/v9"

// Client is the Redis command surface used by the explorer (strings, hashes, sorted sets, etc.).
type Client = redis.Cmdable
