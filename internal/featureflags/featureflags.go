// Package featureflags resolves per-feature enablement from config (env at startup) with optional Redis overrides.
package featureflags

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/redis/go-redis/v9"
)

const (
	// KeyNews is the Redis string key for runtime news toggle (see docs/FEATURE_FLAGS.md).
	KeyNews = "feature:news"
	// KeyPriceAlerts is the Redis string key for runtime price-alert toggle.
	KeyPriceAlerts = "feature:price_alerts"
)

const cacheTTL = 5 * time.Second

// Resolver combines env-derived defaults from [config.Config] with optional Redis overrides.
type Resolver struct {
	rdb redis.Cmdable
	cfg *config.Config

	mu sync.Mutex
	// cached effective values after Redis resolution
	newsUntil, alertsUntil time.Time
	newsVal, alertsVal     bool
}

// NewResolver returns a resolver. RDB may be nil (env-only). Config may be nil (treated as all enabled).
func NewResolver(rdb redis.Cmdable, cfg *config.Config) *Resolver {
	return &Resolver{rdb: rdb, cfg: cfg}
}

func (r *Resolver) baseNews() bool {
	if r == nil || r.cfg == nil {
		return true
	}
	return r.cfg.FeatureNewsEnabled
}

func (r *Resolver) basePriceAlerts() bool {
	if r == nil || r.cfg == nil {
		return true
	}
	return r.cfg.FeaturePriceAlertsEnabled
}

// NewsEnabled returns whether the news API should run (symbol + portfolio news).
func (r *Resolver) NewsEnabled(ctx context.Context) bool {
	if r == nil {
		return true
	}
	return r.resolve(ctx, KeyNews, r.baseNews, &r.newsUntil, &r.newsVal)
}

// PriceAlertsEnabled returns whether price-alert background evaluation and HTTP CRUD are allowed.
func (r *Resolver) PriceAlertsEnabled(ctx context.Context) bool {
	if r == nil {
		return true
	}
	return r.resolve(ctx, KeyPriceAlerts, r.basePriceAlerts, &r.alertsUntil, &r.alertsVal)
}

func (r *Resolver) resolve(ctx context.Context, redisKey string, base func() bool, until *time.Time, cached *bool) bool {
	now := time.Now()
	r.mu.Lock()
	if now.Before(*until) {
		v := *cached
		r.mu.Unlock()
		return v
	}
	r.mu.Unlock()

	if r.rdb == nil {
		v := base()
		r.mu.Lock()
		*cached = v
		*until = now.Add(cacheTTL)
		r.mu.Unlock()
		return v
	}

	s, err := r.rdb.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		v := base()
		r.storeCached(cached, until, v, now)
		return v
	}
	if err != nil {
		v := base()
		r.storeCached(cached, until, v, now)
		return v
	}

	v, ok := ParseOverride(s)
	if !ok {
		v = base()
	}
	r.storeCached(cached, until, v, now)
	return v
}

func (r *Resolver) storeCached(cached *bool, until *time.Time, v bool, now time.Time) {
	r.mu.Lock()
	*cached = v
	*until = now.Add(cacheTTL)
	r.mu.Unlock()
}

// ParseOverride interprets Redis (or CLI) values: "1", "true", "yes", "on", "enabled" vs "0", "false", etc.
func ParseOverride(s string) (enabled bool, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "1", "true", "yes", "on", "enabled":
		return true, true
	case "0", "false", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}

// Snapshot returns effective flags for admin/ops (bypasses short cache with a fresh resolve).
func (r *Resolver) Snapshot(ctx context.Context) map[string]bool {
	if r == nil {
		return map[string]bool{"news": true, "price_alerts": true}
	}
	r.mu.Lock()
	r.newsUntil = time.Time{}
	r.alertsUntil = time.Time{}
	r.mu.Unlock()
	return map[string]bool{
		"news":         r.NewsEnabled(ctx),
		"price_alerts": r.PriceAlertsEnabled(ctx),
	}
}
