package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// adminTestRouter registers v1 and legacy admin routes (status + cache), matching main.
func adminTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)

	apiV1 := r.Group("/api/v1")
	apiV1.POST("/login", loginHandler)
	apiV1.POST("/register", registerHandler)

	adminV1 := apiV1.Group("/admin")
	adminV1.Use(authMiddleware)
	adminV1.Use(requireRoleMiddleware("admin"))
	{
		adminV1.GET("/status", adminStatusHandler)
		adminV1.GET("/cache", adminCacheHandler)
	}

	legacy := r.Group("/api/admin")
	legacy.Use(authMiddleware)
	legacy.Use(requireRoleMiddleware("admin"))
	{
		legacy.GET("/status", adminStatusHandler)
		legacy.GET("/cache", adminCacheHandler)
	}
	return r
}

func TestAdmin_Status_Shape_V1(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")
	w := getReq(t, r, "/api/v1/admin/status", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"status", "user", "role", "redis_memory", "active_rate_limits", "timestamp"} {
		if _, ok := body[k]; !ok {
			t.Fatalf("missing key %q in %v", k, body)
		}
	}
	if body["status"] != "ok" {
		t.Fatalf("status field: %v", body["status"])
	}
	if body["user"] != "admin" || body["role"] != "admin" {
		t.Fatalf("user/role: %#v / %#v", body["user"], body["role"])
	}
}

func TestAdmin_Status_LegacyPath(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")
	w := getReq(t, r, "/api/admin/status", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("legacy status: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_Cache_Stats_Then_Clear(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	_ = rdb.Set(ctx, "admin_test:marker", "1", 0).Err()
	cookie, csrf := loginV1(t, r, "admin", "admin123")

	ws := getReq(t, r, "/api/v1/admin/cache?action=stats", authHeader(cookie, csrf))
	if ws.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", ws.Code, ws.Body.String())
	}
	var stats struct {
		TotalKeys int      `json:"total_keys"`
		Keys      []string `json:"keys"`
	}
	if err := json.Unmarshal(ws.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.TotalKeys < 1 || len(stats.Keys) < 1 {
		t.Fatalf("expected at least one key, got %+v", stats)
	}

	wc := getReq(t, r, "/api/v1/admin/cache?action=clear", authHeader(cookie, csrf))
	if wc.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", wc.Code, wc.Body.String())
	}
	var cleared struct {
		Message     string `json:"message"`
		KeysRemoved int    `json:"keys_removed"`
	}
	if err := json.Unmarshal(wc.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.KeysRemoved < 1 {
		t.Fatalf("keys_removed: %+v", cleared)
	}
	n, _ := rdb.DBSize(ctx).Result()
	if n != 0 {
		t.Fatalf("expected empty redis after clear, DBSize=%d", n)
	}
}

func TestAdmin_Cache_InvalidAction(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")
	w := getReq(t, r, "/api/v1/admin/cache?action=nope", authHeader(cookie, csrf))
	if w.Code != http.StatusBadRequest || apiErrCode(t, w.Body.Bytes()) != "invalid_action" {
		t.Fatalf("want invalid_action 400: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_Status_Unauthorized(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	w := getReq(t, r, "/api/v1/admin/status", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAdmin_Cache_ForbiddenNonAdmin(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	registerV1(t, r, "regular", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "regular", "Str0ngPass")
	w := getReq(t, r, "/api/v1/admin/cache?action=stats", authHeader(cookie, csrf))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_Status_MissingCSRF(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, _ := loginV1(t, r, "admin", "admin123")
	h := make(http.Header)
	h.Add("Cookie", cookie.Name+"="+cookie.Value)
	w := getReq(t, r, "/api/v1/admin/status", h)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 csrf, got %d", w.Code)
	}
}

// TestAdmin_WhenRedisUnreachable_CacheFails tests cache endpoints when Redis cannot serve KEYS.
// Session still validates via in-memory fallback: validateSession falls back to sessionStore when rdb.Get fails.
func TestAdmin_WhenRedisUnreachable_CacheFails(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")

	old := rdb
	bad := redis.NewClient(&redis.Options{
		Addr:            "127.0.0.1:1",
		DisableIdentity: true,
		DialTimeout:     5 * time.Millisecond,
		ReadTimeout:     5 * time.Millisecond,
		WriteTimeout:    5 * time.Millisecond,
		// go-redis: -1 disables command retries (see options.go).
		MaxRetries: -1,
		PoolSize:   1,
	})
	rdb = bad
	defer func() {
		_ = bad.Close()
		rdb = old
	}()

	ws := getReq(t, r, "/api/v1/admin/cache?action=stats", authHeader(cookie, csrf))
	if ws.Code != http.StatusInternalServerError || apiErrCode(t, ws.Body.Bytes()) != "cache_stats_failed" {
		t.Fatalf("stats: want 500 cache_stats_failed, got %d %s", ws.Code, ws.Body.String())
	}

	wc := getReq(t, r, "/api/v1/admin/cache?action=clear", authHeader(cookie, csrf))
	if wc.Code != http.StatusInternalServerError || apiErrCode(t, wc.Body.Bytes()) != "cache_keys_failed" {
		t.Fatalf("clear: want 500 cache_keys_failed, got %d %s", wc.Code, wc.Body.String())
	}
}

// TestAdmin_Status_WhenRedisUnreachable_StillOK verifies adminStatusHandler returns 200 even when
// Redis INFO/SCAN fail (handler does not check command errors; redis_memory may be empty).
func TestAdmin_Status_WhenRedisUnreachable_StillOK(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")

	old := rdb
	bad := redis.NewClient(&redis.Options{
		Addr:            "127.0.0.1:1",
		DisableIdentity: true,
		DialTimeout:     5 * time.Millisecond,
		ReadTimeout:     5 * time.Millisecond,
		WriteTimeout:    5 * time.Millisecond,
		MaxRetries:      -1,
		PoolSize:        1,
	})
	rdb = bad
	defer func() {
		_ = bad.Close()
		rdb = old
	}()

	w := getReq(t, r, "/api/v1/admin/status", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d %s", w.Code, w.Body.String())
	}
	var st map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["status"] != "ok" {
		t.Fatalf("expected status ok: %#v", st["status"])
	}
}

func TestAdmin_Cache_LegacyQuery(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")
	w := getReq(t, r, "/api/admin/cache?action=stats", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("legacy cache stats: %d", w.Code)
	}
}
