package repos

import (
	"context"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/redistest"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// redisTestClient returns a Redis client for integration-style tests: in-process miniredis
// by default, or a real server when BLOCKCHAIN_EXPLORER_TEST_REDIS=integration (see internal/redistest).
// When using miniredis, mr is non-nil so tests can call FastForward for TTL semantics.
func redisTestClient(t *testing.T) (cl *redis.Client, mr *miniredis.Miniredis, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	if redistest.UseIntegration() {
		addr := redistest.IntegrationAddr()
		c := redis.NewClient(&redis.Options{
			Addr:            addr,
			DisableIdentity: true,
		})
		if err := c.Ping(ctx).Err(); err != nil {
			_ = c.Close()
			t.Skipf("integration Redis at %s: %v", addr, err)
		}
		return c, nil, func() { _ = c.Close() }
	}
	m, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	c := redis.NewClient(&redis.Options{
		Addr:            m.Addr(),
		DisableIdentity: true,
	})
	return c, m, func() {
		_ = c.Close()
		m.Close()
	}
}

func TestRedisIntegration_PortfolioRoundTrip(t *testing.T) {
	cl, _, done := redisTestClient(t)
	defer done()
	st := NewStores(cl)
	ctx := context.Background()

	payload := map[string]interface{}{
		"id":   "p1",
		"name": "Main",
	}
	if err := st.Portfolio.Save(ctx, "alice", "p1", payload); err != nil {
		t.Fatal(err)
	}
	raw, err := st.Portfolio.Get(ctx, "alice", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty blob")
	}
	keys, err := st.Portfolio.ListKeys(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != PortfolioKey("alice", "p1") {
		t.Fatalf("keys: %#v", keys)
	}
	if err := st.Portfolio.Delete(ctx, "alice", "p1"); err != nil {
		t.Fatal(err)
	}
	_, err = st.Portfolio.Get(ctx, "alice", "p1")
	if err != redis.Nil {
		t.Fatalf("want redis.Nil after delete, got %v", err)
	}
}

func TestRedisIntegration_SessionTTLExpires_Miniredis(t *testing.T) {
	cl, mr, done := redisTestClient(t)
	defer done()
	if mr == nil {
		t.Skip("TTL fast-forward uses miniredis time; run without BLOCKCHAIN_EXPLORER_TEST_REDIS=integration")
	}
	st := NewStores(cl)
	ctx := context.Background()

	const sid = "sess-int-1"
	if err := st.Session.SetSession(ctx, sid, "bob", time.Minute); err != nil {
		t.Fatal(err)
	}
	u, err := st.Session.GetSessionUsername(ctx, sid)
	if err != nil || u != "bob" {
		t.Fatalf("GetSessionUsername = %q, %v", u, err)
	}

	mr.FastForward(2 * time.Minute)

	_, err = st.Session.GetSessionUsername(ctx, sid)
	if err != redis.Nil {
		t.Fatalf("after expiry want redis.Nil, got %v", err)
	}
}

func TestRedisIntegration_WatchlistCountAndKeys(t *testing.T) {
	cl, _, done := redisTestClient(t)
	defer done()
	st := NewStores(cl)
	ctx := context.Background()

	wl := map[string]interface{}{"id": "w1", "name": "WL", "entries": []interface{}{}}
	if err := st.Watchlist.Save(ctx, "carol", "w1", wl); err != nil {
		t.Fatal(err)
	}
	n, err := st.Watchlist.Count(ctx, "carol")
	if err != nil || n != 1 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	keys, err := st.Watchlist.ListKeys(ctx, "carol")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != WatchlistKey("carol", "w1") {
		t.Fatalf("keys: %#v", keys)
	}
}

func TestRedisIntegration_SessionCSRFDelete(t *testing.T) {
	cl, _, done := redisTestClient(t)
	defer done()
	st := NewStores(cl)
	ctx := context.Background()

	const sid = "sess-csrf-1"
	if err := st.Session.SetSession(ctx, sid, "dave", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := st.Session.SetCSRF(ctx, sid, "tok9", time.Hour); err != nil {
		t.Fatal(err)
	}
	tok, err := st.Session.GetCSRF(ctx, sid)
	if err != nil || tok != "tok9" {
		t.Fatalf("GetCSRF = %q, %v", tok, err)
	}
	if err := st.Session.DeleteSession(ctx, sid); err != nil {
		t.Fatal(err)
	}
	_, err = st.Session.GetSessionUsername(ctx, sid)
	if err != redis.Nil {
		t.Fatalf("session want redis.Nil, got %v", err)
	}
	_, err = st.Session.GetCSRF(ctx, sid)
	if err != redis.Nil {
		t.Fatalf("csrf want redis.Nil, got %v", err)
	}
}
