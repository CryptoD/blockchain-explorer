package idempotency

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestFingerprint_QueryOrderStable(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/search/export?b=2&a=1", nil)
	fp1 := RequestFingerprint(r)
	r2 := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/search/export?a=1&b=2", nil)
	fp2 := RequestFingerprint(r2)
	if fp1 != fp2 {
		t.Fatalf("fingerprints differ:\n%q\n%q", fp1, fp2)
	}
}

func TestRedisKey_DiffersByScope(t *testing.T) {
	t.Parallel()
	k1 := RedisKey("user:alice", "k1", "GET /x")
	k2 := RedisKey("user:bob", "k1", "GET /x")
	if k1 == k2 {
		t.Fatal("expected different redis keys for different scopes")
	}
}
