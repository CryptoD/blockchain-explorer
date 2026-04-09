package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/redis/go-redis/v9"
)

const (
	recordKindJSON   = "json"
	recordKindStream = "stream"
)

// Record is stored in Redis as JSON.
type Record struct {
	Kind        string `json:"k"`
	Status      int    `json:"st"`
	ContentType string `json:"ct,omitempty"`
	Body        []byte `json:"b,omitempty"`
}

// Store persists idempotent export outcomes in Redis.
type Store struct {
	rdb      redis.Cmdable
	cfg      *config.Config
	disabled bool
}

// NewStore returns a no-op store when Redis or idempotency is unavailable.
func NewStore(rdb redis.Cmdable, cfg *config.Config) *Store {
	if rdb == nil || cfg == nil || !cfg.IdempotencyEnabled {
		return &Store{disabled: true}
	}
	return &Store{rdb: rdb, cfg: cfg}
}

// Get returns a stored record if present.
func (s *Store) Get(ctx context.Context, redisKey string) (*Record, error) {
	if s.disabled {
		return nil, redis.Nil
	}
	raw, err := s.rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// PutJSON stores a JSON response for replay (body may be truncated to max size).
func (s *Store) PutJSON(ctx context.Context, redisKey string, status int, contentType string, body []byte) error {
	if s.disabled {
		return nil
	}
	max := s.cfg.IdempotencyMaxResponseBytes
	if max <= 0 {
		max = 262144
	}
	if len(body) > max {
		body = body[:max]
	}
	rec := Record{Kind: recordKindJSON, Status: status, ContentType: contentType, Body: body}
	return s.put(ctx, redisKey, rec)
}

// PutStreamDone records that a streaming export completed successfully once for this key.
func (s *Store) PutStreamDone(ctx context.Context, redisKey string) error {
	if s.disabled {
		return nil
	}
	rec := Record{Kind: recordKindStream, Status: httpStatusOK}
	return s.put(ctx, redisKey, rec)
}

const httpStatusOK = 200

func (s *Store) put(ctx context.Context, redisKey string, rec Record) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	ttl := time.Duration(s.cfg.IdempotencyTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return s.rdb.Set(ctx, redisKey, raw, ttl).Err()
}

// ValidateClientKey returns an error if the header value is invalid (non-empty keys only).
func ValidateClientKey(key string, maxRunes int) error {
	if maxRunes <= 0 {
		maxRunes = 128
	}
	if utf8.RuneCountInString(key) > maxRunes {
		return errors.New("Idempotency-Key too long")
	}
	return nil
}

// IsJSONReplay reports whether rec is a JSON response that can be replayed verbatim.
func IsJSONReplay(rec *Record) bool {
	return rec != nil && rec.Kind == recordKindJSON && len(rec.Body) > 0
}

// IsStreamConflict reports whether rec indicates a prior successful stream export for this key.
func IsStreamConflict(rec *Record) bool {
	return rec != nil && rec.Kind == recordKindStream
}
