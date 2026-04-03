package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/news"
	"github.com/gin-gonic/gin"
)

// Golden JSON snapshots (ROADMAP task 19). Compares normalized JSON to testdata/golden/*.json.
// Regenerate after intentional API shape changes:
//
//	UPDATE_GOLDEN=1 go test ./internal/server -run TestGoldenJSON -count=1

func stableJSONBytes(raw []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	stripVolatileJSONKeys(v)
	return json.MarshalIndent(v, "", "  ")
}

func stripVolatileJSONKeys(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k := range x {
			switch k {
			case "correlation_id", "timestamp", "published_at":
				delete(x, k)
			default:
				stripVolatileJSONKeys(x[k])
			}
		}
	case []interface{}:
		for _, el := range x {
			stripVolatileJSONKeys(el)
		}
	}
}

func assertJSONGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	got, err := stableJSONBytes(body)
	if err != nil {
		t.Fatalf("parse response JSON: %v", err)
	}
	path := filepath.Join("testdata", "golden", name+".json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}
	wantRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", path, err)
	}
	want, err := stableJSONBytes(wantRaw)
	if err != nil {
		t.Fatalf("parse golden %s: %v", path, err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("JSON mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// goldenNewsStub returns fixed article timestamps for stable snapshots.
type goldenNewsStub struct{}

func (goldenNewsStub) Get(ctx context.Context, cacheKey, query string, limit int) ([]news.Article, bool, bool, error) {
	_ = ctx
	_ = cacheKey
	_ = query
	_ = limit
	return []news.Article{{
		Headline:    "Golden headline",
		Summary:     "Summary line",
		Source:      "golden",
		URL:         "https://example.com/article",
		PublishedAt: time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC),
	}}, false, false, nil
}

func (goldenNewsStub) ProviderName() string { return "golden" }

func TestGoldenJSON_SearchSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetExplorerService(&mockExplorerService{
		searchFunc: func(ctx context.Context, query string) (string, map[string]interface{}, error) {
			_ = ctx
			if query == "1GoldenAddr" {
				return "address", map[string]interface{}{"mocked": true, "height": 12345.0}, nil
			}
			return "", nil, ErrNotFound
		},
	})
	defer ResetDefaultServices()

	r := gin.New()
	r.GET("/search", searchHandler)
	req := httptest.NewRequest(http.MethodGet, "/search?q=1GoldenAddr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	assertJSONGolden(t, "search_success", w.Body.Bytes())
}

func TestGoldenJSON_SearchMissingQueryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetDefaultServices()

	r := gin.New()
	r.GET("/search", searchHandler)
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
	assertJSONGolden(t, "search_error_missing_query", w.Body.Bytes())
}

func TestGoldenJSON_NewsBySymbol(t *testing.T) {
	prev := newsService
	defer SetNewsService(prev)
	SetNewsService(goldenNewsStub{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/news/:symbol", newsBySymbolHandler)
	req := httptest.NewRequest(http.MethodGet, "/news/BTC", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	assertJSONGolden(t, "news_symbol", w.Body.Bytes())
}

func TestGoldenJSON_Readyz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerHealthAndMetricsRoutes(r, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	assertJSONGolden(t, "readyz", w.Body.Bytes())
}
