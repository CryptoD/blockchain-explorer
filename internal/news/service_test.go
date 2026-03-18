package news

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	name string
	out  []Article
	err  error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Fetch(ctx context.Context, query string, limit int) ([]Article, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type memCache struct {
	fresh map[string][]Article
	stale map[string][]Article
}

func (m *memCache) GetFresh(ctx context.Context, key string) ([]Article, bool) { v, ok := m.fresh[key]; return v, ok }
func (m *memCache) GetStale(ctx context.Context, key string) ([]Article, bool) { v, ok := m.stale[key]; return v, ok }
func (m *memCache) SetFresh(ctx context.Context, key string, articles []Article, ttl time.Duration) error {
	if m.fresh == nil {
		m.fresh = map[string][]Article{}
	}
	m.fresh[key] = articles
	return nil
}
func (m *memCache) SetStale(ctx context.Context, key string, articles []Article, ttl time.Duration) error {
	if m.stale == nil {
		m.stale = map[string][]Article{}
	}
	m.stale[key] = articles
	return nil
}

func TestService_FallbackToStaleOnError(t *testing.T) {
	cache := &memCache{
		stale: map[string][]Article{
			"k": {{Headline: "h", Source: "s", URL: "https://example.com/a", PublishedAt: time.Now()}},
		},
	}
	s := &Service{
		Provider: &fakeProvider{name: "x", err: errors.New("boom")},
		Cache:    cache,
	}

	arts, cached, stale, err := s.Get(context.Background(), "k", "q", 20)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !cached || !stale {
		t.Fatalf("expected cached+stale true, got cached=%v stale=%v", cached, stale)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 article, got %d", len(arts))
	}
}

func TestService_DedupeByURL(t *testing.T) {
	now := time.Now()
	p := &fakeProvider{
		name: "x",
		out: []Article{
			{Headline: "a", Source: "s", URL: "https://example.com/a?utm_source=x", PublishedAt: now},
			{Headline: "b", Source: "s", URL: "https://example.com/a?utm_medium=y", PublishedAt: now.Add(-time.Minute)},
		},
	}
	s := &Service{Provider: p, Cache: &memCache{}}
	arts, _, _, err := s.Get(context.Background(), "k2", "q", 20)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 deduped article, got %d", len(arts))
	}
}

func TestService_ReturnsErrorWhenUnconfigured(t *testing.T) {
	s := &Service{}
	_, _, _, err := s.Get(context.Background(), "k", "q", 20)
	if err == nil {
		t.Fatalf("expected error")
	}
}

