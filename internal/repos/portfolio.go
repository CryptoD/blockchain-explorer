package repos

import (
	"context"
	"encoding/json"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/redis/go-redis/v9"
)

// PortfolioRepo persists portfolio JSON blobs under portfolio:{user}:{id}.
type PortfolioRepo struct {
	RDB redisstore.Client
}

func (r *PortfolioRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// ListKeys returns Redis keys for all portfolios of a user.
func (r *PortfolioRepo) ListKeys(ctx context.Context, username string) ([]string, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	return r.RDB.Keys(ctx, PortfolioKey(username, "*")).Result()
}

// ListJSON returns raw JSON blobs for each portfolio key (same order as keys iteration).
func (r *PortfolioRepo) ListJSON(ctx context.Context, username string) ([][]byte, error) {
	keys, err := r.ListKeys(ctx, username)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(keys))
	for _, key := range keys {
		data, err := r.RDB.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		out = append(out, data)
	}
	return out, nil
}

// Get returns raw JSON for one portfolio.
func (r *PortfolioRepo) Get(ctx context.Context, username, id string) ([]byte, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	key := PortfolioKey(username, id)
	data, err := r.RDB.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, redis.Nil
	}
	return data, err
}

// Save marshals v to JSON and stores under portfolio:{username}:{id} (no TTL).
func (r *PortfolioRepo) Save(ctx context.Context, username, id string, v interface{}) error {
	if err := r.check(); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, PortfolioKey(username, id), data, 0).Err()
}

// Delete removes one portfolio key.
func (r *PortfolioRepo) Delete(ctx context.Context, username, id string) error {
	if err := r.check(); err != nil {
		return err
	}
	return r.RDB.Del(ctx, PortfolioKey(username, id)).Err()
}
