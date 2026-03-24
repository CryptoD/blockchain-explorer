package news

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Service struct {
	Provider Provider
	Cache    Cache

	FreshTTL time.Duration
	StaleTTL time.Duration
}

func (s *Service) ProviderName() string {
	if s == nil || s.Provider == nil {
		return ""
	}
	return s.Provider.Name()
}

// Get returns articles for a key/query, preferring fresh cache, then provider,
// and finally stale cache on provider errors / rate limiting.
func (s *Service) Get(ctx context.Context, cacheKey, query string, limit int) (articles []Article, cached bool, stale bool, err error) {
	if s == nil || s.Provider == nil {
		return nil, false, false, ErrUnavailable("news provider not configured")
	}
	cacheKey = strings.TrimSpace(cacheKey)
	query = strings.TrimSpace(query)
	if cacheKey == "" || query == "" {
		return nil, false, false, ErrUnavailable("missing query")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if s.Cache != nil {
		if v, ok := s.Cache.GetFresh(ctx, cacheKey); ok {
			return dedupeAndSort(v, limit), true, false, nil
		}
	}

	fetched, fetchErr := s.Provider.Fetch(ctx, query, limit)
	if fetchErr == nil {
		deduped := dedupeAndSort(fetched, limit)
		if s.Cache != nil {
			freshTTL := s.FreshTTL
			if freshTTL <= 0 {
				freshTTL = 5 * time.Minute
			}
			staleTTL := s.StaleTTL
			if staleTTL <= 0 {
				staleTTL = 1 * time.Hour
			}
			_ = s.Cache.SetFresh(ctx, cacheKey, deduped, freshTTL)
			_ = s.Cache.SetStale(ctx, cacheKey, deduped, staleTTL)
		}
		return deduped, false, false, nil
	}

	// Provider error / rate limit => return stale cache if possible.
	if s.Cache != nil {
		if v, ok := s.Cache.GetStale(ctx, cacheKey); ok {
			return dedupeAndSort(v, limit), true, true, nil
		}
	}
	return nil, false, false, fetchErr
}

// CacheKey builds a stable cache key for a provider query.
func CacheKey(providerName, scope, query string, extras map[string]string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "news"
	}
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "" {
		providerName = "provider"
	}
	h := sha1.Sum([]byte(canonicalizeQuery(query, extras)))
	return scope + ":" + providerName + ":" + hex.EncodeToString(h[:])
}

func canonicalizeQuery(query string, extras map[string]string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(query))
	if len(extras) == 0 {
		return b.String()
	}
	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(k))
		b.WriteString("=")
		b.WriteString(strings.TrimSpace(extras[k]))
	}
	return b.String()
}

// dedupeAndSort removes duplicates (by normalized URL) and sorts newest-first.
func dedupeAndSort(in []Article, limit int) []Article {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]Article, 0, len(in))
	for _, a := range in {
		u := normalizeURL(a.URL)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		a.URL = u
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	// Drop common tracking params.
	q := u.Query()
	for _, k := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id"} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// ErrUnavailable is used for configuration/availability errors.
type ErrUnavailable string

func (e ErrUnavailable) Error() string { return string(e) }
