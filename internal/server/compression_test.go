package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

func testCompressionConfig(enabled, brotli bool) *config.Config {
	return &config.Config{
		ResponseCompressionEnabled: enabled,
		ResponseCompressionBrotli:  brotli,
	}
}

func TestResponseCompression_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(responseCompressionMiddleware(testCompressionConfig(false, false)))
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"a": 1}) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no compression, got %q", w.Header().Get("Content-Encoding"))
	}
}

func TestResponseCompression_GzipJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(responseCompressionMiddleware(testCompressionConfig(true, false)))
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"hello": "world"}) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("want gzip encoding, got %q", w.Header().Get("Content-Encoding"))
	}
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(gr)
	_ = gr.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("hello")) {
		t.Fatalf("unexpected body: %s", raw)
	}
}

func TestResponseCompression_BrotliJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(responseCompressionMiddleware(testCompressionConfig(true, true)))
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"hello": "brotli"}) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("want br encoding, got %q", w.Header().Get("Content-Encoding"))
	}
	br := brotli.NewReader(w.Body)
	raw, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("brotli")) {
		t.Fatalf("unexpected body: %s", raw)
	}
}

func TestResponseCompression_SkipsPprofPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(responseCompressionMiddleware(testCompressionConfig(true, true)))
	r.GET("/debug/pprof/heap", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("pprof path should skip compression, got %q", w.Header().Get("Content-Encoding"))
	}
}
