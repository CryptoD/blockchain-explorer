package news

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache stores fresh and stale payloads as JSON.
// Keys are written as:
// - fresh: <key>
// - stale: <key>:stale
type RedisCache struct {
	RDB redis.Cmdable
}

func (c *RedisCache) GetFresh(ctx context.Context, key string) ([]Article, bool) {
	return c.get(ctx, key)
}

func (c *RedisCache) GetStale(ctx context.Context, key string) ([]Article, bool) {
	return c.get(ctx, key+":stale")
}

func (c *RedisCache) SetFresh(ctx context.Context, key string, articles []Article, ttl time.Duration) error {
	return c.set(ctx, key, articles, ttl)
}

func (c *RedisCache) SetStale(ctx context.Context, key string, articles []Article, ttl time.Duration) error {
	return c.set(ctx, key+":stale", articles, ttl)
}

func (c *RedisCache) get(ctx context.Context, key string) ([]Article, bool) {
	if c == nil || c.RDB == nil {
		return nil, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}
	val, err := c.RDB.Get(ctx, key).Result()
	if err != nil || val == "" {
		return nil, false
	}
	var out []Article
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func (c *RedisCache) set(ctx context.Context, key string, articles []Article, ttl time.Duration) error {
	if c == nil || c.RDB == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("news cache key is empty")
	}
	if ttl < 0 {
		ttl = 0
	}
	b, err := json.Marshal(articles)
	if err != nil {
		return err
	}
	return c.RDB.Set(ctx, key, b, ttl).Err()
}
