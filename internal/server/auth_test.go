package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/gin-gonic/gin"
)

func resetAuthState(t *testing.T) {
	t.Helper()
	rdb.FlushDB(ctx)
	userMutex.Lock()
	users = make(map[string]User)
	userMutex.Unlock()
	sessionMutex.Lock()
	sessionStore = make(map[string]string)
	sessionMutex.Unlock()
	csrfMutex.Lock()
	csrfStore = make(map[string]string)
	csrfMutex.Unlock()
	initializeDefaultAdmin()
}

// authTestRouter registers auth, user, and admin routes matching /api/v1 (versioned API).
func authTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)

	apiV1 := r.Group("/api/v1")
	{
		apiV1.POST("/login", loginHandler)
		apiV1.POST("/logout", logoutHandler)
		apiV1.POST("/register", registerHandler)

		userV1 := apiV1.Group("/user")
		userV1.Use(authMiddleware)
		{
			userV1.GET("/profile", userProfileHandler)
			userV1.PATCH("/profile", updateProfileHandler)
			userV1.PATCH("/password", changePasswordHandler)
		}

		adminV1 := apiV1.Group("/admin")
		adminV1.Use(authMiddleware)
		adminV1.Use(requireRoleMiddleware("admin"))
		{
			adminV1.GET("/status", adminStatusHandler)
		}
	}
	return r
}

func postJSON(t *testing.T, r http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getReq(t *testing.T, r http.Handler, path string, hdr http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if hdr != nil {
		req.Header = hdr
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type loginResponse struct {
	Message   string `json:"message"`
	CSRFToken string `json:"csrfToken"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

func loginV1(t *testing.T, router http.Handler, username, password string) (cookie *http.Cookie, csrf string) {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `"}`
	w := postJSON(t, router, "/api/v1/login", body)
	if w.Code != http.StatusOK {
		t.Fatalf("login: status %d body %s", w.Code, w.Body.String())
	}
	var lr loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &lr); err != nil {
		t.Fatalf("login JSON: %v", err)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("no session_id cookie")
	}
	csrf = lr.CSRFToken
	return cookie, csrf
}

func authHeader(cookie *http.Cookie, csrf string) http.Header {
	h := make(http.Header)
	if cookie != nil {
		h.Add("Cookie", cookie.Name+"="+cookie.Value)
	}
	if csrf != "" {
		h.Set("X-CSRF-Token", csrf)
	}
	return h
}

func TestRegister_Success(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := postJSON(t, router, "/api/v1/register", `{"username":"newuser1","password":"Str0ngPass","email":"a@b.co"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	body := `{"username":"dupuser","password":"Str0ngPass"}`
	w1 := postJSON(t, router, "/api/v1/register", body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first register: %d", w1.Code)
	}
	w2 := postJSON(t, router, "/api/v1/register", body)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := postJSON(t, router, "/api/v1/register", `{"username":"weakuser","password":"short"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLogin_Success_And_Profile(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"loguser","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "loguser", "Str0ngPass")
	w := getReq(t, router, "/api/v1/user/profile", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", w.Code, w.Body.String())
	}
	var u User
	if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil {
		t.Fatal(err)
	}
	if u.Username != "loguser" || u.Role != "user" {
		t.Fatalf("user: %+v", u)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"badlogin","password":"Str0ngPass"}`)
	w := postJSON(t, router, "/api/v1/login", `{"username":"badlogin","password":"WrongPass1"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogout_ClearsSession(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"outuser","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "outuser", "Str0ngPass")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	logoutReq.Header = authHeader(cookie, csrf)
	wl := httptest.NewRecorder()
	router.ServeHTTP(wl, logoutReq)
	if wl.Code != http.StatusOK {
		t.Fatalf("logout: %d", wl.Code)
	}

	// After logout, profile should reject (session destroyed; cookie may be cleared in response)
	w := getReq(t, router, "/api/v1/user/profile", authHeader(cookie, csrf))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", w.Code)
	}
}

func TestPasswordChange_RotatesCSRF(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"pwdrot","password":"Str0ngPass","email":"p@x.co"}`)
	cookie, oldCSRF := loginV1(t, router, "pwdrot", "Str0ngPass")

	body := `{"current_password":"Str0ngPass","new_password":"NewStr0ng99"}`
	w := reqJSON(t, router, http.MethodPatch, "/api/v1/user/password", body, authHeader(cookie, oldCSRF))
	if w.Code != http.StatusOK {
		t.Fatalf("password change: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Message   string `json:"message"`
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.CSRFToken == "" {
		t.Fatalf("response: %v body %s", err, w.Body.String())
	}
	if out.CSRFToken == oldCSRF {
		t.Fatal("expected new CSRF token after password change")
	}

	wBad := reqJSON(t, router, http.MethodPatch, "/api/v1/user/profile", `{}`, authHeader(cookie, oldCSRF))
	if wBad.Code != http.StatusForbidden {
		t.Fatalf("old CSRF want 403, got %d: %s", wBad.Code, wBad.Body.String())
	}

	wOK := reqJSON(t, router, http.MethodPatch, "/api/v1/user/profile", `{}`, authHeader(cookie, out.CSRFToken))
	if wOK.Code != http.StatusOK {
		t.Fatalf("new CSRF want 200, got %d: %s", wOK.Code, wOK.Body.String())
	}

	_, newCSRF := loginV1(t, router, "pwdrot", "NewStr0ng99")
	if newCSRF == "" {
		t.Fatal("login with new password should return CSRF")
	}
}

func TestPasswordChange_WrongCurrentPassword(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"badcur","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "badcur", "Str0ngPass")
	body := `{"current_password":"WrongPass1","new_password":"NewStr0ng99"}`
	w := reqJSON(t, router, http.MethodPatch, "/api/v1/user/password", body, authHeader(cookie, csrf))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSession_InvalidAfterServerSideDestroy(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"sesskill","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "sesskill", "Str0ngPass")

	// Resolve session id from Redis (cookie value matches; using Redis avoids any cookie parsing edge cases)
	var sid string
	keys, _ := rdb.Keys(ctx, "session:*").Result()
	for _, k := range keys {
		if u, err := rdb.Get(ctx, k).Result(); err == nil && u == "sesskill" {
			sid = strings.TrimPrefix(k, "session:")
			break
		}
	}
	if sid == "" {
		t.Fatal("expected session:* key for sesskill")
	}
	destroySession(sid)

	w := getReq(t, router, "/api/v1/user/profile", authHeader(cookie, csrf))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after destroySession, got %d", w.Code)
	}
}

// TestSession_InvalidWhenRedisSessionKeyMissing covers the same Redis.Nil path as TTL expiry:
// when the session key is gone from Redis, validateSession must not fall back to in-memory state.
func TestSession_InvalidWhenRedisSessionKeyMissing(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"sessgone","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "sessgone", "Str0ngPass")
	sid := cookie.Value
	if sid == "" {
		t.Fatal("empty session cookie")
	}
	key := repos.SessionKey(sid)
	if n, err := rdb.Exists(ctx, key).Result(); err != nil || n != 1 {
		t.Fatalf("precondition session in Redis: exists=%v err=%v key=%q", n, err, key)
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}

	w := getReq(t, router, "/api/v1/user/profile", authHeader(cookie, csrf))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when Redis session key is missing, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUserProfile_Unauthenticated(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := getReq(t, router, "/api/v1/user/profile", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAdminStatus_Unauthenticated(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := getReq(t, router, "/api/v1/admin/status", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAdminStatus_ForbiddenForRegularUser(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	postJSON(t, router, "/api/v1/register", `{"username":"plainuser","password":"Str0ngPass"}`)
	cookie, csrf := loginV1(t, router, "plainuser", "Str0ngPass")
	w := getReq(t, router, "/api/v1/admin/status", authHeader(cookie, csrf))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminStatus_SuccessForAdmin(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	cookie, csrf := loginV1(t, router, "admin", "admin123")
	w := getReq(t, router, "/api/v1/admin/status", authHeader(cookie, csrf))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["role"] != "admin" {
		t.Fatalf("expected admin role in response, got %#v", body["role"])
	}
}

func TestAdminStatus_MissingCSRFWithSession(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	cookie, _ := loginV1(t, router, "admin", "admin123")
	h := make(http.Header)
	h.Add("Cookie", cookie.Name+"="+cookie.Value)
	w := getReq(t, router, "/api/v1/admin/status", h)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 CSRF missing, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLegacyAPI_AuthRoutes(t *testing.T) {
	resetAuthState(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfMiddleware)
	r.POST("/api/register", registerHandler)
	r.POST("/api/login", loginHandler)
	r.POST("/api/logout", logoutHandler)
	u := r.Group("/api/user")
	u.Use(authMiddleware)
	u.GET("/profile", userProfileHandler)

	postJSON(t, r, "/api/register", `{"username":"legacyu","password":"Str0ngPass"}`)
	w := postJSON(t, r, "/api/login", `{"username":"legacyu","password":"Str0ngPass"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy login %d", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_id" {
			cookie = c
			break
		}
	}
	var lr loginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &lr)
	h := authHeader(cookie, lr.CSRFToken)
	w2 := getReq(t, r, "/api/user/profile", h)
	if w2.Code != http.StatusOK {
		t.Fatalf("legacy profile %d", w2.Code)
	}
}

func TestLogin_InvalidBody(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := postJSON(t, router, "/api/v1/login", `{`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegister_InvalidUsername(t *testing.T) {
	resetAuthState(t)
	router := authTestRouter()
	w := postJSON(t, router, "/api/v1/register", `{"username":"ab","password":"Str0ngPass"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
