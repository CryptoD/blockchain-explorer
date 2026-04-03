package news

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// Contract tests replay recorded TheNewsAPI JSON (fixtures) against httptest; no API key or network required.

func loadContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "contracts", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestTheNewsAPIProvider_ContractReplay_Fetch(t *testing.T) {
	fixture := loadContractFixture(t, "thenewsapi_news_all.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/news/all" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("api_token") != "test-token" {
			t.Errorf("api_token missing or wrong")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	p := &TheNewsAPIProvider{
		BaseURL: srv.URL,
		Token:   "test-token",
		Client:  resty.New().SetTimeout(2 * time.Second).SetRetryCount(0),
	}

	arts, err := p.Fetch(context.Background(), "bitcoin", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("len=%d", len(arts))
	}
	if arts[0].Headline != "Contract headline" || arts[0].Source != "Example News" {
		t.Fatalf("%+v", arts[0])
	}
	if arts[0].URL != "https://example.com/article" {
		t.Fatalf("url: %s", arts[0].URL)
	}
}

func TestTheNewsAPIProvider_ContractRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limit"}`))
	}))
	defer srv.Close()

	p := &TheNewsAPIProvider{
		BaseURL: srv.URL,
		Token:   "test-token",
		Client:  resty.New().SetTimeout(2 * time.Second).SetRetryCount(0),
	}

	_, err := p.Fetch(context.Background(), "q", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestTheNewsAPIProvider_StrictTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &TheNewsAPIProvider{
		BaseURL: srv.URL,
		Token:   "test-token",
		Client:  resty.New().SetTimeout(50 * time.Millisecond).SetRetryCount(0),
	}

	_, err := p.Fetch(context.Background(), "q", 5)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
