package featureflags

import (
	"context"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestParseOverride(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   bool
		wantOK bool
	}{
		{"true", true, true},
		{"1", true, true},
		{"yes", true, true},
		{"on", true, true},
		{"enabled", true, true},
		{"false", false, true},
		{"0", false, true},
		{"no", false, true},
		{"off", false, true},
		{"disabled", false, true},
		{"", false, false},
		{"maybe", false, false},
	}
	for _, tc := range cases {
		got, ok := ParseOverride(tc.in)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Fatalf("ParseOverride(%q) = %v,%v want %v,%v", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestResolver_RedisOverridesEnv(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	cfg := &config.Config{
		FeatureNewsEnabled:        true,
		FeaturePriceAlertsEnabled: true,
	}
	res := NewResolver(rdb, cfg)
	ctx := context.Background()

	if !res.NewsEnabled(ctx) {
		t.Fatal("news expected on")
	}
	if err := rdb.Set(ctx, KeyNews, "0", 0).Err(); err != nil {
		t.Fatal(err)
	}
	// bust cache
	res.mu.Lock()
	res.newsUntil = time.Time{}
	res.mu.Unlock()
	if res.NewsEnabled(ctx) {
		t.Fatal("news expected off from redis")
	}

	if err := rdb.Set(ctx, KeyNews, "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	res.mu.Lock()
	res.newsUntil = time.Time{}
	res.mu.Unlock()
	if !res.NewsEnabled(ctx) {
		t.Fatal("news expected on again")
	}
}

func TestResolver_EnvOffWithoutRedis(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		FeatureNewsEnabled:        false,
		FeaturePriceAlertsEnabled: false,
	}
	res := NewResolver(nil, cfg)
	ctx := context.Background()
	if res.NewsEnabled(ctx) {
		t.Fatal("news")
	}
	if res.PriceAlertsEnabled(ctx) {
		t.Fatal("alerts")
	}
}
