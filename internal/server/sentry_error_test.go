package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

type sentryTransportSpy struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *sentryTransportSpy) Configure(sentry.ClientOptions) {}

func (t *sentryTransportSpy) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *sentryTransportSpy) Flush(time.Duration) bool { return true }

func (t *sentryTransportSpy) FlushWithContext(context.Context) bool { return true }

func (t *sentryTransportSpy) Close() {}

func TestSentryHandleErrorIncludesRequestRouteAndTags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tp := &sentryTransportSpy{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "http://public@localhost/1",
		Transport: tp,
	}); err != nil {
		t.Fatal(err)
	}
	defer sentry.Flush(2 * time.Second)

	r := gin.New()
	r.Use(sentrygin.New(sentrygin.Options{Repanic: false}))
	r.Use(correlationIDMiddleware())
	r.Use(sentryUserScopeMiddleware())
	r.GET("/api/v1/boom", func(c *gin.Context) {
		handleError(c, errors.New("intentional failure"), http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/boom", nil)
	req.Header.Set(correlation.HeaderCorrelationID, "test-req-abc")
	r.ServeHTTP(w, req)

	sentry.Flush(2 * time.Second)

	tp.mu.Lock()
	defer tp.mu.Unlock()
	if len(tp.events) != 1 {
		t.Fatalf("expected 1 Sentry event, got %d", len(tp.events))
	}
	ev := tp.events[0]
	if ev.Tags["correlation_id"] != "test-req-abc" {
		t.Errorf("correlation_id tag = %q, want test-req-abc", ev.Tags["correlation_id"])
	}
	if ev.Tags["request_id"] != "test-req-abc" {
		t.Errorf("request_id tag = %q, want test-req-abc", ev.Tags["request_id"])
	}
	if ev.Tags["route"] != "/api/v1/boom" {
		t.Errorf("route tag = %q, want /api/v1/boom", ev.Tags["route"])
	}
	if ev.Tags["source"] != "handleError" {
		t.Errorf("source tag = %q", ev.Tags["source"])
	}
	if ev.Tags["http.status_code"] != "500" {
		t.Errorf("http.status_code = %q", ev.Tags["http.status_code"])
	}
}
