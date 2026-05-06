package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// APIKeyMetaKey returns apikey:v1:meta:{publicID}
	apiKeyMetaPrefix = "apikey:v1:meta:"
	apiKeyUserSet    = "apikey:v1:user:"
	apiKeyServiceSet = "apikey:v1:service"
)

// APIKeyRecord is stored at apikey:v1:meta:{PublicID}. KeyHashHex is sha256 hex of the full plaintext token.
type APIKeyRecord struct {
	PublicID    string   `json:"id"`
	KeyHashHex  string   `json:"key_hash"`
	Name        string   `json:"name"`
	Scopes      []string `json:"scopes"`
	OwnerType   string   `json:"owner_type"` // "user" | "service"
	Username    string   `json:"username,omitempty"`
	Label       string   `json:"label,omitempty"`
	CreatedUnix int64    `json:"created"`
	RevokedUnix int64    `json:"revoked"`
	ExpiresUnix int64    `json:"expires"`
}

func apiKeyMetaKey(publicID string) string {
	return apiKeyMetaPrefix + publicID
}

// APIKeyRepo persists API key metadata in Redis.
type APIKeyRepo struct {
	RDB redis.Cmdable
}

// Get returns the record for a public id, or redis.Nil if missing.
func (r *APIKeyRepo) Get(ctx context.Context, publicID string) (*APIKeyRecord, error) {
	if r == nil || r.RDB == nil {
		return nil, fmt.Errorf("redis unavailable")
	}
	raw, err := r.RDB.Get(ctx, apiKeyMetaKey(publicID)).Bytes()
	if err != nil {
		return nil, err
	}
	var rec APIKeyRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// Save writes the record and adds the id to the user or service index set.
func (r *APIKeyRepo) Save(ctx context.Context, rec *APIKeyRecord) error {
	if r == nil || r.RDB == nil {
		return fmt.Errorf("redis unavailable")
	}
	if rec == nil || rec.PublicID == "" {
		return fmt.Errorf("invalid api key record")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	pipe := r.RDB.TxPipeline()
	pipe.Set(ctx, apiKeyMetaKey(rec.PublicID), raw, 0)
	switch rec.OwnerType {
	case "user":
		if rec.Username == "" {
			return fmt.Errorf("user api key missing username")
		}
		pipe.SAdd(ctx, apiKeyUserSet+rec.Username, rec.PublicID)
	case "service":
		pipe.SAdd(ctx, apiKeyServiceSet, rec.PublicID)
	default:
		return fmt.Errorf("invalid owner_type")
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Revoke marks a key revoked (best-effort idempotent).
func (r *APIKeyRepo) Revoke(ctx context.Context, publicID string) error {
	if r == nil || r.RDB == nil {
		return fmt.Errorf("redis unavailable")
	}
	rec, err := r.Get(ctx, publicID)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	rec.RevokedUnix = now
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, apiKeyMetaKey(publicID), raw, 0).Err()
}

// CountActiveUserKeys counts non-revoked keys for a user (S members + filtered).
func (r *APIKeyRepo) CountActiveUserKeys(ctx context.Context, username string) (int, error) {
	list, err := r.ListUserKeys(ctx, username)
	if err != nil {
		return 0, err
	}
	n := 0
	now := time.Now().Unix()
	for _, rec := range list {
		if rec.RevokedUnix != 0 {
			continue
		}
		if rec.ExpiresUnix > 0 && rec.ExpiresUnix < now {
			continue
		}
		n++
	}
	return n, nil
}

// ListUserKeys returns all indexed keys for a user (includes revoked/expired rows).
func (r *APIKeyRepo) ListUserKeys(ctx context.Context, username string) ([]APIKeyRecord, error) {
	if r == nil || r.RDB == nil {
		return nil, fmt.Errorf("redis unavailable")
	}
	ids, err := r.RDB.SMembers(ctx, apiKeyUserSet+username).Result()
	if err != nil {
		return nil, err
	}
	return r.loadRecords(ctx, ids)
}

// ListServiceKeys returns all indexed service keys.
func (r *APIKeyRepo) ListServiceKeys(ctx context.Context) ([]APIKeyRecord, error) {
	if r == nil || r.RDB == nil {
		return nil, fmt.Errorf("redis unavailable")
	}
	ids, err := r.RDB.SMembers(ctx, apiKeyServiceSet).Result()
	if err != nil {
		return nil, err
	}
	return r.loadRecords(ctx, ids)
}

func (r *APIKeyRepo) loadRecords(ctx context.Context, ids []string) ([]APIKeyRecord, error) {
	var out []APIKeyRecord
	for _, id := range ids {
		rec, err := r.Get(ctx, id)
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, nil
}
