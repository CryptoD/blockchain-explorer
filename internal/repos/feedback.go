package repos

import (
	"context"
	"encoding/json"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
)

// FeedbackRepo stores anonymous feedback blobs.
type FeedbackRepo struct {
	RDB redisstore.Client
}

func (r *FeedbackRepo) check() error {
	if r == nil || r.RDB == nil {
		return ErrNotConfigured
	}
	return nil
}

// Store saves a feedback payload with a 30-day TTL (same semantics as prior handler).
func (r *FeedbackRepo) Store(ctx context.Context, name, email, message, clientIP string) error {
	if err := r.check(); err != nil {
		return err
	}
	key := FeedbackKey(time.Now().Unix())
	feedbackData := map[string]interface{}{
		"name":      name,
		"email":     email,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
		"ip":        clientIP,
	}
	jsonData, err := json.Marshal(feedbackData)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, key, jsonData, 30*24*time.Hour).Err()
}
