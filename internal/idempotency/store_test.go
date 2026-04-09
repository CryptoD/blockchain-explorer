package idempotency

import (
	"context"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStore_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	rdb := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer rdb.Close()

	cfg := &config.Config{
		IdempotencyEnabled:          true,
		IdempotencyTTLSeconds:       3600,
		IdempotencyMaxResponseBytes: 1024,
		IdempotencyKeyMaxRunes:      64,
	}
	st := NewStore(rdb, cfg)
	key := "idem:v1:test"
	body := []byte(`{"ok":true}`)
	if err := st.PutJSON(context.Background(), key, 200, "application/json", body); err != nil {
		t.Fatal(err)
	}
	rec, err := st.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !IsJSONReplay(rec) {
		t.Fatalf("expected json replay, got %+v", rec)
	}
	if string(rec.Body) != string(body) {
		t.Fatalf("body mismatch")
	}
}

func TestStore_DisabledNoRedis(t *testing.T) {
	t.Parallel()
	st := NewStore(nil, &config.Config{IdempotencyEnabled: true})
	if err := st.PutJSON(context.Background(), "k", 200, "application/json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateClientKey(t *testing.T) {
	t.Parallel()
	if err := ValidateClientKey("short-key-ok", 64); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClientKey(string(make([]byte, 200)), 64); err == nil {
		t.Fatal("expected error for long key")
	}
}
