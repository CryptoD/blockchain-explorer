package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBitcoinSearchPageHandler_EmptyQueryServesShell(t *testing.T) {
	t.Chdir("../..")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/bitcoin", bitcoinSearchPageHandler)

	req := httptest.NewRequest(http.MethodGet, "/bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestBitcoinSearchPageHandler_NotFoundHTML(t *testing.T) {
	resetCache()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/bitcoin", bitcoinSearchPageHandler)

	req := httptest.NewRequest(http.MethodGet, "/bitcoin?q=not-a-valid-lookup-xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No matching block") {
		t.Fatalf("expected not-found message in body: %s", body)
	}
	if !strings.Contains(body, `name="q"`) {
		t.Fatalf("expected search form in body")
	}
}

func TestBitcoinSearchPageHandler_CachedAddressHTML(t *testing.T) {
	resetCache()
	addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	setCache("address:"+addr, map[string]interface{}{"address": addr, "balance": 0.5})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/bitcoin", bitcoinSearchPageHandler)

	req := httptest.NewRequest(http.MethodGet, "/bitcoin?q="+addr, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Address") || !strings.Contains(body, addr) {
		t.Fatalf("expected address result in body: %s", body)
	}
	if !strings.Contains(body, "Interactive address view") {
		t.Fatalf("expected link to interactive view")
	}
}
