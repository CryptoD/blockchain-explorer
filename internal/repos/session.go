package repos

import (
	"context"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/redis/go-redis/v9"
)

// SessionRepo stores session and CSRF tokens keyed by session id.
type SessionRepo struct {
	RDB redisstore.Client
}

func (r *SessionRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// SetSession stores username for a session id with TTL.
func (r *SessionRepo) SetSession(ctx context.Context, sessionID, username string, ttl time.Duration) error {
	if err := r.check(); err != nil {
		return err
	}
	return r.RDB.Set(ctx, SessionKey(sessionID), username, ttl).Err()
}

// GetSessionUsername returns the username for a session, or redis.Nil if missing.
func (r *SessionRepo) GetSessionUsername(ctx context.Context, sessionID string) (string, error) {
	if err := r.check(); err != nil {
		return "", err
	}
	s, err := r.RDB.Get(ctx, SessionKey(sessionID)).Result()
	if err == redis.Nil {
		return "", redis.Nil
	}
	return s, err
}

// DeleteSession removes session and CSRF keys for a session id.
func (r *SessionRepo) DeleteSession(ctx context.Context, sessionID string) error {
	if err := r.check(); err != nil {
		return err
	}
	_ = r.RDB.Del(ctx, SessionKey(sessionID)).Err()
	return r.RDB.Del(ctx, CSRFKey(sessionID)).Err()
}

// SetCSRF stores CSRF token for a session.
func (r *SessionRepo) SetCSRF(ctx context.Context, sessionID, token string, ttl time.Duration) error {
	if err := r.check(); err != nil {
		return err
	}
	return r.RDB.Set(ctx, CSRFKey(sessionID), token, ttl).Err()
}

// GetCSRF returns the CSRF token or redis.Nil.
func (r *SessionRepo) GetCSRF(ctx context.Context, sessionID string) (string, error) {
	if err := r.check(); err != nil {
		return "", err
	}
	s, err := r.RDB.Get(ctx, CSRFKey(sessionID)).Result()
	if err == redis.Nil {
		return "", redis.Nil
	}
	return s, err
}
