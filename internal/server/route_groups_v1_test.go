package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/news"
	"github.com/gin-gonic/gin"
)

// newAPITestRouter mirrors production route registration for /api/v1 (ROADMAP task 17).
func newAPITestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)
	registerAPIV1Routes(r)
	return r
}

func newHealthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)
	registerHealthAndMetricsRoutes(r, &config.Config{})
	return r
}

// --- Explorer (registerExplorerRoutesV1) ---

func TestV1_Explorer_Search_Smoke(t *testing.T) {
	resetCache()
	addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	setCache("address:"+addr, map[string]interface{}{"result": map[string]interface{}{"address": addr}})

	r := newAPITestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+addr, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search smoke: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Explorer_Search_MissingQuery_Error(t *testing.T) {
	resetCache()
	r := newAPITestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing q: want 400, got %d %s", w.Code, w.Body.String())
	}
}

// --- Feedback (registerFeedbackRoutesV1) ---

func TestV1_Feedback_Smoke(t *testing.T) {
	resetCache()
	r := newAPITestRouter()
	body := `{"name":"t","email":"a@b.co","message":"Hello feedback test message here"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("feedback: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Feedback_InvalidJSON_Error(t *testing.T) {
	resetCache()
	r := newAPITestRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid json: want 400, got %d %s", w.Code, w.Body.String())
	}
}

// --- News (registerNewsRoutesV1) ---

type stubNewsSvc struct{}

func (stubNewsSvc) Get(ctx context.Context, cacheKey, query string, limit int) ([]news.Article, bool, bool, error) {
	_ = ctx
	_ = cacheKey
	_ = query
	_ = limit
	return []news.Article{{
		Headline:    "h",
		Source:      "s",
		URL:         "https://example.com/a",
		PublishedAt: time.Now().UTC(),
	}}, false, false, nil
}

func (stubNewsSvc) ProviderName() string { return "stub" }

func TestV1_News_Symbol_Smoke(t *testing.T) {
	prev := newsService
	defer SetNewsService(prev)
	SetNewsService(stubNewsSvc{})

	r := newAPITestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/news/BTC", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("news symbol: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_News_Symbol_InvalidSymbol_Error(t *testing.T) {
	prev := newsService
	defer SetNewsService(prev)
	SetNewsService(stubNewsSvc{})

	r := newAPITestRouter()
	longSym := strings.Repeat("a", 51)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/news/"+longSym, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || apiErrCode(t, w.Body.Bytes()) != "invalid_symbol" {
		t.Fatalf("invalid symbol: want 400 invalid_symbol, got %d %s", w.Code, w.Body.String())
	}
}

func TestV1_News_Portfolio_Unauthorized_Error(t *testing.T) {
	prev := newsService
	defer SetNewsService(prev)
	SetNewsService(stubNewsSvc{})

	r := newAPITestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/news/portfolio/p1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("portfolio news unauth: want 401, got %d %s", w.Code, w.Body.String())
	}
}

// --- User: notifications & alerts (subgroups of registerUserRoutesV1) ---

func TestV1_User_Notifications_Smoke(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	registerV1(t, r, "notifuser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "notifuser", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/notifications", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("notifications: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Data       []interface{} `json:"data"`
		Pagination struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data == nil {
		t.Fatal("expected data array")
	}
	if out.Pagination.PageSize != apiutil.DefaultPageSize {
		t.Fatalf("default page_size: want %d, got %d", apiutil.DefaultPageSize, out.Pagination.PageSize)
	}
}

func TestV1_User_Notifications_Unauthorized_Error(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	w := getReq(t, r, "/api/v1/user/notifications", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestV1_User_Alerts_Smoke(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	registerV1(t, r, "alertuser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "alertuser", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/alerts", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("alerts: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_User_Alerts_Unauthorized_Error(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	w := getReq(t, r, "/api/v1/user/alerts", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestV1_User_Notifications_PageSizeCapped(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	registerV1(t, r, "notifcap", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "notifcap", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/notifications?page_size=99999", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("notifications: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Pagination struct {
			PageSize int `json:"page_size"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Pagination.PageSize != apiutil.MaxPageSize {
		t.Fatalf("page_size capped: want %d, got %d", apiutil.MaxPageSize, out.Pagination.PageSize)
	}
}

func TestV1_User_Alerts_PageSizeCapped(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	registerV1(t, r, "alertcap", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "alertcap", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/alerts?page_size=99999", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("alerts: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Pagination struct {
			PageSize int `json:"page_size"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Pagination.PageSize != apiutil.MaxPageSize {
		t.Fatalf("page_size capped: want %d, got %d", apiutil.MaxPageSize, out.Pagination.PageSize)
	}
}

// --- User: portfolio JSON export (registerUserPortfolioRoutes) ---

func TestV1_User_PortfoliosExport_Smoke(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	registerV1(t, r, "expuser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "expuser", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/portfolios/export", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_User_PortfoliosExport_Unauthorized_Error(t *testing.T) {
	resetAuthState(t)
	r := newAPITestRouter()
	w := getReq(t, r, "/api/v1/user/portfolios/export", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// --- Health & readiness (registerHealthAndMetricsRoutes) ---

func TestV1_Healthz_Smoke(t *testing.T) {
	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Health_Smoke(t *testing.T) {
	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Readyz_Smoke(t *testing.T) {
	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("readyz: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Ready_Smoke(t *testing.T) {
	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ready: %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Readyz_RedisNil_Error(t *testing.T) {
	old := rdb
	rdb = nil
	defer func() { rdb = old }()

	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz with nil rdb: want 503, got %d %s", w.Code, w.Body.String())
	}
}

func TestV1_Ready_RedisNil_Error(t *testing.T) {
	old := rdb
	rdb = nil
	defer func() { rdb = old }()

	r := newHealthTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready with nil rdb: want 503, got %d %s", w.Code, w.Body.String())
	}
}
