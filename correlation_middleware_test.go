package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/gin-gonic/gin"
)

func TestCorrelationIDMiddleware_PropagatesInboundID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(correlationIDMiddleware())
	r.GET("/x", func(c *gin.Context) {
		cid, _ := c.Get("correlation_id")
		c.String(http.StatusOK, cid.(string))
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(correlation.HeaderCorrelationID, "client-trace-1")
	r.ServeHTTP(w, req)
	if got := w.Header().Get(correlation.HeaderCorrelationID); got != "client-trace-1" {
		t.Fatalf("X-Correlation-ID = %q, want client-trace-1", got)
	}
	if got := w.Header().Get(correlation.HeaderRequestID); got != "client-trace-1" {
		t.Fatalf("X-Request-ID = %q, want client-trace-1", got)
	}
}

func TestMergeCorrelationID_InErrorJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(correlationIDMiddleware())
	r.GET("/err", func(c *gin.Context) {
		errorResponse(c, http.StatusBadRequest, "bad", "oops")
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	r.ServeHTTP(w, req)
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["correlation_id"]; !ok {
		t.Fatalf("expected correlation_id in body: %v", body)
	}
}
