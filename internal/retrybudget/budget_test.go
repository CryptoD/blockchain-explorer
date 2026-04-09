package retrybudget

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWithAttemptBudget_Exhausts(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(204)
	}))
	defer ts.Close()

	rt := WrapRoundTripper(http.DefaultTransport)
	client := &http.Client{Transport: rt}

	ctx := WithAttemptBudget(context.Background(), 2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(req); err != nil {
		t.Fatal(err)
	}
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if _, err := client.Do(req2); err != nil {
		t.Fatal(err)
	}
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	_, err = client.Do(req3)
	if err == nil {
		t.Fatal("expected budget error")
	}
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("want ErrBudgetExhausted, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls: want 2, got %d", calls.Load())
	}
}

func TestWithAttemptBudget_UnlimitedWithoutContext(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(204)
	}))
	defer ts.Close()

	rt := WrapRoundTripper(http.DefaultTransport)
	client := &http.Client{Transport: rt}

	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Do(req); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("want 5 calls, got %d", calls.Load())
	}
}
