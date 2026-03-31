package repos

import (
	"context"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/redis/go-redis/v9"
)

// UserRepo persists user JSON under user:{username}.
type UserRepo struct {
	RDB redisstore.Client
}

func (r *UserRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// ListUserKeys returns all keys matching user:*.
func (r *UserRepo) ListUserKeys(ctx context.Context) ([]string, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	return r.RDB.Keys(ctx, PrefixUserKeyPattern).Result()
}

// Get returns raw JSON for one user.
func (r *UserRepo) Get(ctx context.Context, username string) ([]byte, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	key := UserKey(username)
	data, err := r.RDB.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, redis.Nil
	}
	return data, err
}

// Save stores raw JSON for a user (no expiration).
func (r *UserRepo) Save(ctx context.Context, username string, data []byte) error {
	if err := r.check(); err != nil {
		return err
	}
	return r.RDB.Set(ctx, UserKey(username), data, 0).Err()
}
