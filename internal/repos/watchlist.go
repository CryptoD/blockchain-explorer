package repos

import (
	"context"
	"encoding/json"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/redis/go-redis/v9"
)

// WatchlistRepo persists watchlist JSON under watchlist:{user}:{id}.
type WatchlistRepo struct {
	RDB redisstore.Client
}

func (r *WatchlistRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// ListKeys returns Redis keys for all watchlists of a user.
func (r *WatchlistRepo) ListKeys(ctx context.Context, username string) ([]string, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	return r.RDB.Keys(ctx, WatchlistKey(username, "*")).Result()
}

// Count returns the number of watchlists for quota checks.
func (r *WatchlistRepo) Count(ctx context.Context, username string) (int, error) {
	keys, err := r.ListKeys(ctx, username)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// ListJSON returns raw JSON blobs for each watchlist.
func (r *WatchlistRepo) ListJSON(ctx context.Context, username string) ([][]byte, error) {
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

// Get returns raw JSON for one watchlist.
func (r *WatchlistRepo) Get(ctx context.Context, username, id string) ([]byte, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	key := WatchlistKey(username, id)
	data, err := r.RDB.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, redis.Nil
	}
	return data, err
}

// Save marshals v to JSON and stores under watchlist:{username}:{id}.
func (r *WatchlistRepo) Save(ctx context.Context, username, id string, v interface{}) error {
	if err := r.check(); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, WatchlistKey(username, id), data, 0).Err()
}

// Delete removes one watchlist key.
func (r *WatchlistRepo) Delete(ctx context.Context, username, id string) error {
	if err := r.check(); err != nil {
		return err
	}
	return r.RDB.Del(ctx, WatchlistKey(username, id)).Err()
}
