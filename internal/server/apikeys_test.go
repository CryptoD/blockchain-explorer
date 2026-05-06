package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func bearerAuthHeader(tok string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+strings.TrimSpace(tok))
	return h
}

func TestAPIKey_UserReadWrite_Profile(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter() // has user group + CSRF
	registerV1(t, r, "keyuser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "keyuser", "Str0ngPass")

	createBody := `{"name":"ci","scopes":["user:read","user:write"]}`
	wc := reqJSON(t, r, http.MethodPost, "/api/v1/user/api-keys", createBody, authHeader(cookie, csrf))
	if wc.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", wc.Code, wc.Body.String())
	}
	var cre struct {
		PlaintextKey string `json:"plaintext_key"`
	}
	if err := json.Unmarshal(wc.Body.Bytes(), &cre); err != nil || cre.PlaintextKey == "" {
		t.Fatalf("create body: %v %s", err, wc.Body.String())
	}

	wg := getReq(t, r, "/api/v1/user/profile", bearerAuthHeader(cre.PlaintextKey))
	if wg.Code != http.StatusOK {
		t.Fatalf("profile via api key GET: %d %s", wg.Code, wg.Body.String())
	}
}

func TestAPIKey_UserReadOnly_BlockWrite(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	registerV1(t, r, "rouser", "Str0ngPass")
	cookie, csrf := loginV1(t, r, "rouser", "Str0ngPass")

	wc := reqJSON(t, r, http.MethodPost, "/api/v1/user/api-keys", `{"name":"ro","scopes":["user:read"]}`, authHeader(cookie, csrf))
	if wc.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", wc.Code, wc.Body.String())
	}
	var cre struct {
		PlaintextKey string `json:"plaintext_key"`
	}
	_ = json.Unmarshal(wc.Body.Bytes(), &cre)

	body := validPortfolioJSON("X")
	wp := reqJSON(t, r, http.MethodPost, "/api/v1/user/portfolios", body, bearerAuthHeader(cre.PlaintextKey))
	if wp.Code != http.StatusForbidden || apiErrCode(t, wp.Body.Bytes()) != "insufficient_scope" {
		t.Fatalf("want insufficient_scope 403; got %d %s", wp.Code, wp.Body.String())
	}
}

func TestAPIKey_ServiceAdminRead_Status(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()
	cookie, csrf := loginV1(t, r, "admin", "admin123")

	wk := reqJSON(t, r, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"svc","scopes":["admin:read"],"label":"test"}`, authHeader(cookie, csrf))
	if wk.Code != http.StatusCreated {
		t.Fatalf("create svc key: %d %s", wk.Code, wk.Body.String())
	}
	var out struct {
		PlaintextKey string `json:"plaintext_key"`
	}
	if err := json.Unmarshal(wk.Body.Bytes(), &out); err != nil || out.PlaintextKey == "" {
		t.Fatalf("svc create: %v", err)
	}

	ws := getReq(t, r, "/api/v1/admin/status", bearerAuthHeader(out.PlaintextKey))
	if ws.Code != http.StatusOK {
		t.Fatalf("status bearer: %d %s", ws.Code, ws.Body.String())
	}
}

func TestAPIKey_ServiceKey_BlockUserRoutes(t *testing.T) {
	resetAuthState(t)
	r := portfolioWatchlistTestRouter()
	ar := adminTestRouter()
	ac, acsr := loginV1(t, ar, "admin", "admin123")
	wsvc := reqJSON(t, ar, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"s","scopes":["admin:read"]}`, authHeader(ac, acsr))
	if wsvc.Code != http.StatusCreated {
		t.Fatal(wsvc.Body.String())
	}
	var svc struct {
		PlaintextKey string `json:"plaintext_key"`
	}
	_ = json.Unmarshal(wsvc.Body.Bytes(), &svc)

	w := getReq(t, r, "/api/v1/user/profile", bearerAuthHeader(svc.PlaintextKey))
	if w.Code != http.StatusForbidden || apiErrCode(t, w.Body.Bytes()) != "api_key_scope" {
		t.Fatalf("want api_key_scope, got %d %s", w.Code, w.Body.String())
	}
}

func TestAPIKey_AdminKeysRequireBrowserSession_ToMutate(t *testing.T) {
	resetAuthState(t)
	r := adminTestRouter()

	cookie1, csrf1 := loginV1(t, r, "admin", "admin123")
	wsvc := reqJSON(t, r, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"s2","scopes":["admin:write","admin:read"]}`, authHeader(cookie1, csrf1))
	if wsvc.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", wsvc.Code, wsvc.Body.String())
	}
	var cre struct {
		PlaintextKey string `json:"plaintext_key"`
		ID           string `json:"id"`
	}
	if err := json.Unmarshal(wsvc.Body.Bytes(), &cre); err != nil || cre.ID == "" {
		t.Fatalf("unmarshal: %v body=%s", err, wsvc.Body.String())
	}

	wBad := reqJSON(t, r, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"nope","scopes":["admin:read"]}`, bearerAuthHeader(cre.PlaintextKey))
	if wBad.Code != http.StatusForbidden || apiErrCode(t, wBad.Body.Bytes()) != "session_required" {
		t.Fatalf("want session_required: %d %s", wBad.Code, wBad.Body.String())
	}

	delPath := fmt.Sprintf("/api/v1/admin/api-keys/%s", cre.ID)
	wDel := reqJSON(t, r, http.MethodDelete, delPath, "", bearerAuthHeader(cre.PlaintextKey))
	if wDel.Code != http.StatusForbidden || apiErrCode(t, wDel.Body.Bytes()) != "session_required" {
		t.Fatalf("delete svc key via bearer admin key: %d %s", wDel.Code, wDel.Body.String())
	}
}
