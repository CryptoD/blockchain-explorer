package repos

import "errors"

var (
	// ErrNotConfigured is returned when the Redis client was not wired.
	ErrNotConfigured = errors.New("redis not configured")
)
