package correlation

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewID_LengthAndHex(t *testing.T) {
	id := NewID()
	if len(id) != 32 {
		t.Fatalf("len(NewID()) = %d, want 32", len(id))
	}
}

func TestFromHeaders_PrefersCorrelationID(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderCorrelationID, "abc")
	h.Set(HeaderRequestID, "def")
	if got := FromHeaders(h); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestFromHeaders_FallsBackToRequestID(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderRequestID, "req-1")
	if got := FromHeaders(h); got != "req-1" {
		t.Fatalf("got %q", got)
	}
}

func TestFromHeaders_TooLongIgnored(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderCorrelationID, strings.Repeat("a", MaxIDLength+1))
	if got := FromHeaders(h); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
