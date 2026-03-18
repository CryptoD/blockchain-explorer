package news

import (
	"context"
	"time"
)

// Cache stores fresh and stale results for a given key.
type Cache interface {
	GetFresh(ctx context.Context, key string) ([]Article, bool)
	GetStale(ctx context.Context, key string) ([]Article, bool)
	SetFresh(ctx context.Context, key string, articles []Article, ttl time.Duration) error
	SetStale(ctx context.Context, key string, articles []Article, ttl time.Duration) error
}

