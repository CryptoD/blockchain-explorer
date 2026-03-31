package repos

import (
	"context"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
)

// AdminRepo supports admin cache introspection (all keys).
type AdminRepo struct {
	RDB redisstore.Client
}

func (r *AdminRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// MemoryInfo returns Redis INFO memory section.
func (r *AdminRepo) MemoryInfo(ctx context.Context) string {
	if r == nil || r.RDB == nil {
		return ""
	}
	return r.RDB.Info(ctx, "memory").Val()
}

// ListAllKeys returns KEYS * (admin-only; use with care).
func (r *AdminRepo) ListAllKeys(ctx context.Context) ([]string, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	return r.RDB.Keys(ctx, "*").Result()
}

// DeleteKeys removes keys.
func (r *AdminRepo) DeleteKeys(ctx context.Context, keys []string) error {
	if err := r.check(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return r.RDB.Del(ctx, keys...).Err()
}
