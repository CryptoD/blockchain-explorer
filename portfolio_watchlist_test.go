package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// portfolioWatchlistTestRouter wires v1 user portfolio and watchlist routes (mirrors main).
func portfolioWatchlistTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)
	apiV1 := r.Group("/api/v1")
	apiV1.POST("/login", loginHandler)
	apiV1.POST("/register", registerHandler)
	userV1 := apiV1.Group("/user")
	userV1.Use(authMiddleware)
	{
		userV1.GET("/portfolios", listPortfoliosHandler)
		userV1.POST("/portfolios", createPortfolioHandler)
		userV1.PUT("/portfolios/:id", updatePortfolioHandler)
		userV1.DELETE("/portfolios/:id", deletePortfolioHandler)

		userV1.GET("/watchlists", listWatchlistsHandler)
		userV1.POST("/watchlists", createWatchlistHandler)
		userV1.GET("/watchlists/:id", getWatchlistHandler)
		userV1.PUT("/watchlists/:id", updateWatchlistHandler)
		userV1.DELETE("/watchlists/:id", deleteWatchlistHandler)
		userV1.POST("/watchlists/:id/entries", addWatchlistEntryHandler)
		userV1.PUT("/watchlists/:id/entries/:index", updateWatchlistEntryHandler)
		userV1.DELETE("/watchlists/:id/entries/:index", deleteWatchlistEntryHandler)
	}
	return r
}

func reqJSON(t *testing.T, handler http.Handler, method, path, body string, hdr http.Header) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader = http.NoBody
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if hdr != nil {
		req.Header = hdr.Clone()
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func registerV1(t *testing.T, r http.Handler, username, password string) {
	t.Helper()
	w := postJSON(t, r, "/api/v1/register", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
	if w.Code != http.StatusCreated {
		t.Fatalf("register %q: %d %s", username, w.Code, w.Body.String())
	}
}

func apiErrCode(t *testing.T, body []byte) string {
	t.Helper()
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return ""
	}
	return wrap.Error.Code
}

func validPortfolioJSON(name string) string {
	return fmt.Sprintf(`{"name":%q,"description":"test","items":[{"type":"crypto","label":"Holdings","address":"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"}]}`, name)
}

func TestPortfolio_List_EmptyShape(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "plist", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "plist", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/portfolios", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Data       []PortfolioWithValuation `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data == nil {
		t.Fatal("expected data array (possibly empty)")
	}
	if len(out.Data) != 0 {
		t.Fatalf("expected 0 portfolios, got %d", len(out.Data))
	}
	if out.Pagination.Total != 0 {
		t.Fatalf("pagination.total: %d", out.Pagination.Total)
	}
}

func TestPortfolio_Create_List_Update_Delete_Redis(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "pfuser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "pfuser", "Str0ngPass")

	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/portfolios", validPortfolioJSON("Main"), authHeader(cookie, csrf))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created Portfolio
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Username != "pfuser" || len(created.Items) != 1 {
		t.Fatalf("unexpected body: %+v", created)
	}
	raw, err := rdb.Get(ctx, "portfolio:pfuser:"+created.ID).Result()
	if err != nil || raw == "" {
		t.Fatalf("redis missing portfolio key: %v", err)
	}

	wl := getReq(t, r, "/api/v1/user/portfolios", authHeader(cookie, csrf))
	if wl.Code != http.StatusOK {
		t.Fatalf("list: %d", wl.Code)
	}
	var listOut struct {
		Data []PortfolioWithValuation `json:"data"`
	}
	_ = json.Unmarshal(wl.Body.Bytes(), &listOut)
	if len(listOut.Data) != 1 || listOut.Data[0].ID != created.ID {
		t.Fatalf("list data mismatch: %+v", listOut.Data)
	}

	upd := fmt.Sprintf(`{"name":"Main2","description":"x","items":[{"type":"crypto","label":"L","address":"3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy"}]}`)
	wu := reqJSON(t, r, http.MethodPut, "/api/v1/user/portfolios/"+created.ID, upd, authHeader(cookie, csrf))
	if wu.Code != http.StatusOK {
		t.Fatalf("update: %d %s", wu.Code, wu.Body.String())
	}

	wd := reqJSON(t, r, http.MethodDelete, "/api/v1/user/portfolios/"+created.ID, "", authHeader(cookie, csrf))
	if wd.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", wd.Code, wd.Body.String())
	}
	if n, _ := rdb.Exists(ctx, "portfolio:pfuser:"+created.ID).Result(); n != 0 {
		t.Fatal("expected redis key removed")
	}
}

func TestPortfolio_Create_ValidationErrors(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "valpf", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "valpf", "Str0ngPass")

	cases := []struct {
		body string
		want string // error code
	}{
		{`{"name":"","description":"","items":[]}`, "invalid_portfolio_name"},
		{`{"name":"` + strings.Repeat("x", 101) + `","description":"","items":[]}`, "invalid_portfolio_name"},
		{`{"name":"ok","description":"","items":[{"type":"nft","label":"a","address":"x"}]}`, "invalid_item_type"},
		{`{"name":"ok","description":"","items":[{"type":"crypto","label":"","address":"x"}]}`, "invalid_item_label"},
	}
	for _, tc := range cases {
		w := reqJSON(t, r, http.MethodPost, "/api/v1/user/portfolios", tc.body, authHeader(cookie, csrf))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: want 400 got %d %s", tc.body, w.Code, w.Body.String())
		}
		if c := apiErrCode(t, w.Body.Bytes()); c != tc.want {
			t.Fatalf("code want %q got %q body %s", tc.want, c, w.Body.String())
		}
	}
}

func TestPortfolio_Update_NotFound(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "nopf", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "nopf", "Str0ngPass")
	body := validPortfolioJSON("x")
	w := reqJSON(t, r, http.MethodPut, "/api/v1/user/portfolios/does-not-exist", body, authHeader(cookie, csrf))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
	}
	if apiErrCode(t, w.Body.Bytes()) != "portfolio_not_found" {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestPortfolio_CrossUser_NotFound(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "owner", "Str0ngPass")
	registerV1(t, r, "other", "Str0ngPass")
	c1, cs1 := loginV1(t, r, "owner", "Str0ngPass")
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/portfolios", validPortfolioJSON("mine"), authHeader(c1, cs1))
	var p Portfolio
	json.Unmarshal(w.Body.Bytes(), &p)

	c2, cs2 := loginV1(t, r, "other", "Str0ngPass")
	wg := reqJSON(t, r, http.MethodPut, "/api/v1/user/portfolios/"+p.ID, validPortfolioJSON("stolen"), authHeader(c2, cs2))
	if wg.Code != http.StatusNotFound {
		t.Fatalf("other user put: want 404 got %d %s", wg.Code, wg.Body.String())
	}
}

func TestWatchlist_CrossUser_NotFound(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wOwner", "Str0ngPass")
	registerV1(t, r, "wOther", "Str0ngPass")
	c1, cs1 := loginV1(t, r, "wOwner", "Str0ngPass")
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", `{"name":"w","entries":[]}`, authHeader(c1, cs1))
	var wl Watchlist
	json.Unmarshal(w.Body.Bytes(), &wl)
	c2, cs2 := loginV1(t, r, "wOther", "Str0ngPass")
	wg := getReq(t, r, "/api/v1/user/watchlists/"+wl.ID, authHeader(c2, cs2))
	if wg.Code != http.StatusNotFound {
		t.Fatalf("other user get watchlist: want 404 got %d %s", wg.Code, wg.Body.String())
	}
}

func TestWatchlist_CRUD_Entries_Redis(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wluser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "wluser", "Str0ngPass")

	createBody := `{"name":"WL1","entries":[{"type":"symbol","symbol":"bitcoin"}]}`
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", createBody, authHeader(cookie, csrf))
	if w.Code != http.StatusCreated {
		t.Fatalf("create wl: %d %s", w.Code, w.Body.String())
	}
	var wl Watchlist
	if err := json.Unmarshal(w.Body.Bytes(), &wl); err != nil {
		t.Fatal(err)
	}
	if wl.ID == "" || wl.Username != "wluser" || len(wl.Entries) != 1 {
		t.Fatalf("watchlist: %+v", wl)
	}
	key := watchlistKey("wluser", wl.ID)
	if n, _ := rdb.Exists(ctx, key).Result(); n != 1 {
		t.Fatal("redis key missing for watchlist")
	}

	wlGet := getReq(t, r, "/api/v1/user/watchlists/"+wl.ID, authHeader(cookie, csrf))
	if wlGet.Code != http.StatusOK {
		t.Fatalf("get: %d", wlGet.Code)
	}

	list := getReq(t, r, "/api/v1/user/watchlists", authHeader(cookie, csrf))
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d", list.Code)
	}
	var listOut struct {
		Data []Watchlist `json:"data"`
	}
	json.Unmarshal(list.Body.Bytes(), &listOut)
	if len(listOut.Data) != 1 {
		t.Fatalf("list len %d", len(listOut.Data))
	}

	upd := `{"name":"WL2","entries":[{"type":"symbol","symbol":"ethereum"}]}`
	wu := reqJSON(t, r, http.MethodPut, "/api/v1/user/watchlists/"+wl.ID, upd, authHeader(cookie, csrf))
	if wu.Code != http.StatusOK {
		t.Fatalf("update: %d %s", wu.Code, wu.Body.String())
	}

	add := `{"type":"symbol","symbol":"solana"}`
	wa := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists/"+wl.ID+"/entries", add, authHeader(cookie, csrf))
	if wa.Code != http.StatusCreated {
		t.Fatalf("add entry: %d %s", wa.Code, wa.Body.String())
	}
	var wl2 Watchlist
	json.Unmarshal(wa.Body.Bytes(), &wl2)
	if len(wl2.Entries) != 2 {
		t.Fatalf("entries len %d", len(wl2.Entries))
	}

	wIdx := reqJSON(t, r, http.MethodPut, "/api/v1/user/watchlists/"+wl.ID+"/entries/0", `{"type":"symbol","symbol":"btc"}`, authHeader(cookie, csrf))
	if wIdx.Code != http.StatusOK {
		t.Fatalf("update entry: %d %s", wIdx.Code, wIdx.Body.String())
	}

	wDel := reqJSON(t, r, http.MethodDelete, "/api/v1/user/watchlists/"+wl.ID+"/entries/0", "", authHeader(cookie, csrf))
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete entry: %d %s", wDel.Code, wDel.Body.String())
	}

	wDelWL := reqJSON(t, r, http.MethodDelete, "/api/v1/user/watchlists/"+wl.ID, "", authHeader(cookie, csrf))
	if wDelWL.Code != http.StatusOK {
		t.Fatalf("delete wl: %d", wDelWL.Code)
	}
	if n, _ := rdb.Exists(ctx, key).Result(); n != 0 {
		t.Fatal("watchlist key should be gone")
	}
}

func TestWatchlist_ValidationErrors(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wlval", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "wlval", "Str0ngPass")

	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", `{"name":"x","entries":[{"type":"oops","symbol":"a"}]}`, authHeader(cookie, csrf))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_entry") {
		t.Fatalf("want invalid_entry 400: %d %s", w.Code, w.Body.String())
	}

	w2 := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", `{"name":"x","entries":[{"type":"symbol","symbol":""}]}`, authHeader(cookie, csrf))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("empty symbol: %d", w2.Code)
	}
}

func TestWatchlist_Get_NotFound(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wlmiss", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "wlmiss", "Str0ngPass")
	w := getReq(t, r, "/api/v1/user/watchlists/nope", authHeader(cookie, csrf))
	if w.Code != http.StatusNotFound || apiErrCode(t, w.Body.Bytes()) != "watchlist_not_found" {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestWatchlist_EntryIndex_OutOfRange(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wlidx", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "wlidx", "Str0ngPass")
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", `{"name":"e","entries":[{"type":"symbol","symbol":"eth"}]}`, authHeader(cookie, csrf))
	var wl Watchlist
	json.Unmarshal(w.Body.Bytes(), &wl)

	w2 := reqJSON(t, r, http.MethodPut, "/api/v1/user/watchlists/"+wl.ID+"/entries/99", `{"type":"symbol","symbol":"x"}`, authHeader(cookie, csrf))
	if w2.Code != http.StatusNotFound || apiErrCode(t, w2.Body.Bytes()) != "entry_not_found" {
		t.Fatalf("want entry_not_found: %d %s", w2.Code, w2.Body.String())
	}
}

func TestWatchlist_Quota_MaxWatchlists(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "wlquota", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "wlquota", "Str0ngPass")
	for i := 0; i < maxWatchlistsPerUser; i++ {
		body := fmt.Sprintf(`{"name":"w%d","entries":[]}`, i)
		w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", body, authHeader(cookie, csrf))
		if w.Code != http.StatusCreated {
			t.Fatalf("iter %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", `{"name":"overflow","entries":[]}`, authHeader(cookie, csrf))
	if w.Code != http.StatusTooManyRequests || apiErrCode(t, w.Body.Bytes()) != "watchlist_quota_exceeded" {
		t.Fatalf("want 429 watchlist_quota_exceeded: %d %s", w.Code, w.Body.String())
	}
}

func TestWatchlist_EntryQuota_MaxEntries(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "entq", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "entq", "Str0ngPass")
	entries := make([]map[string]string, maxWatchlistEntries)
	for i := 0; i < maxWatchlistEntries; i++ {
		entries[i] = map[string]string{"type": "symbol", "symbol": fmt.Sprintf("s%d", i)}
	}
	payload := map[string]interface{}{"name": "full", "entries": entries}
	b, _ := json.Marshal(payload)
	w := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists", string(b), authHeader(cookie, csrf))
	if w.Code != http.StatusCreated {
		t.Fatalf("create full: %d %s", w.Code, w.Body.String())
	}
	var wl Watchlist
	json.Unmarshal(w.Body.Bytes(), &wl)
	w2 := reqJSON(t, r, http.MethodPost, "/api/v1/user/watchlists/"+wl.ID+"/entries", `{"type":"symbol","symbol":"extra"}`, authHeader(cookie, csrf))
	if w2.Code != http.StatusTooManyRequests || apiErrCode(t, w2.Body.Bytes()) != "entry_quota_exceeded" {
		t.Fatalf("want entry_quota_exceeded: %d %s", w2.Code, w2.Body.String())
	}
}

func TestPortfolio_Delete_NonExistent_CurrentBehavior(t *testing.T) {
	// Redis DEL is idempotent: no error when key missing; handler returns 200.
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "delpf", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "delpf", "Str0ngPass")
	w := reqJSON(t, r, http.MethodDelete, "/api/v1/user/portfolios/ghost-id", "", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("delete ghost: %d %s", w.Code, w.Body.String())
	}
}
