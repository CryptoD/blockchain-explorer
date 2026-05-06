package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	apiKeyPlainPrefix = "bkx_"
	apiKeyPublicIDLen = 16 // hex chars (8 random bytes)
	ginKeyAuthKind    = "auth_kind" // "session" | "api_key"
	ginKeyAPIKeyID    = "api_key_public_id"
	ginKeyAPIKeyScopes = "api_key_scopes"
	ginKeyAPIKeyService = "api_key_is_service"
	scopeUserRead     = "user:read"
	scopeUserWrite    = "user:write"
	scopeAdminRead    = "admin:read"
	scopeAdminWrite   = "admin:write"
)

var allowedUserAPIKeyScopes = map[string]struct{}{
	scopeUserRead:  {},
	scopeUserWrite: {},
}

var allowedServiceAPIKeyScopes = map[string]struct{}{
	scopeAdminRead:  {},
	scopeAdminWrite: {},
}

func scopeSetContains(scopes []string, need string) bool {
	have := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		have[strings.TrimSpace(s)] = struct{}{}
	}
	if _, ok := have[need]; ok {
		return true
	}
	if need == scopeUserRead {
		_, w := have[scopeUserWrite]
		return w
	}
	if need == scopeAdminRead {
		_, w := have[scopeAdminWrite]
		return w
	}
	return false
}

// tryAPIKeyAuth validates Authorization: Bearer bkx_* and sets auth context. Returns true when handled (either success or an error response).
func tryAPIKeyAuth(c *gin.Context) bool {
	ah := strings.TrimSpace(c.GetHeader("Authorization"))
	if ah == "" {
		return false
	}
	const bearer = "Bearer "
	if !strings.HasPrefix(ah, bearer) {
		return false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(ah, bearer))
	if tok == "" {
		return false
	}
	if !strings.HasPrefix(tok, apiKeyPlainPrefix) {
		errorResponse(c, http.StatusUnauthorized, "invalid_authorization", "Unsupported bearer token for this API")
		c.Abort()
		return true
	}
	if appConfig != nil && !appConfig.APIKeysEnabled {
		errorResponse(c, http.StatusForbidden, "api_keys_disabled", "API keys are disabled on this server")
		c.Abort()
		return true
	}
	if appRepos == nil || appRepos.APIKeys == nil || rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "API keys require Redis")
		c.Abort()
		return true
	}
	publicID, secretPart, err := parseAPIKeyPlaintext(tok)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
	rec, err := appRepos.APIKeys.Get(c.Request.Context(), publicID)
	if err == redis.Nil || rec == nil {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_lookup_failed", "Failed to validate API key")
		c.Abort()
		return true
	}
	if !apiKeyRecordActive(rec) {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
	gotHash := sha256.Sum256([]byte(tok))
	want, decErr := hex.DecodeString(strings.TrimSpace(rec.KeyHashHex))
	if decErr != nil || len(want) != sha256.Size {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
	if subtle.ConstantTimeCompare(gotHash[:], want) != 1 {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
	if _, err := parseAPIKeySecretSegment(secretPart); err != nil {
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}

	switch rec.OwnerType {
	case "service":
		c.Set(ginKeyAuthKind, "api_key")
		c.Set(ginKeyAPIKeyService, true)
		c.Set(ginKeyAPIKeyID, rec.PublicID)
		c.Set("username", "")
		c.Set("role", "admin")
		c.Set(ginKeyAPIKeyScopes, rec.Scopes)
		c.Next()
		return true
	case "user":
		if strings.TrimSpace(rec.Username) == "" {
			errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
			c.Abort()
			return true
		}
		user, ok := getUser(rec.Username)
		if !ok {
			errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found")
			c.Abort()
			return true
		}
		c.Set(ginKeyAuthKind, "api_key")
		c.Set(ginKeyAPIKeyService, false)
		c.Set(ginKeyAPIKeyID, rec.PublicID)
		c.Set("username", strings.TrimSpace(rec.Username))
		c.Set("role", user.Role)
		c.Set(ginKeyAPIKeyScopes, rec.Scopes)
		c.Next()
		return true
	default:
		errorResponse(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		c.Abort()
		return true
	}
}

func apiKeyRecordActive(rec *repos.APIKeyRecord) bool {
	if rec == nil {
		return false
	}
	if rec.RevokedUnix != 0 {
		return false
	}
	now := time.Now().Unix()
	if rec.ExpiresUnix > 0 && rec.ExpiresUnix < now {
		return false
	}
	return true
}

func authKindFromContext(c *gin.Context) string {
	v, ok := c.Get(ginKeyAuthKind)
	if !ok {
		return "session"
	}
	s, _ := v.(string)
	if s != "" {
		return s
	}
	return "session"
}

func isServiceAPIKey(c *gin.Context) bool {
	v, ok := c.Get(ginKeyAPIKeyService)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func apiKeyScopesFromContext(c *gin.Context) []string {
	v, ok := c.Get(ginKeyAPIKeyScopes)
	if !ok {
		return nil
	}
	sl, _ := v.([]string)
	return sl
}

// forbidServiceAPIKeyMiddleware blocks enterprise service keys from user-facing resources.
func forbidServiceAPIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isServiceAPIKey(c) {
			errorResponse(c, http.StatusForbidden, "api_key_scope", "Service API keys cannot access user resources")
			c.Abort()
			return
		}
		c.Next()
	}
}

// enforceUserAPIScopes requires user:read for GET-ish use and user:write for mutations when using API keys.
func enforceUserAPIScopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authKindFromContext(c) != "api_key" {
			c.Next()
			return
		}
		m := c.Request.Method
		switch m {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			if !scopeSetContains(apiKeyScopesFromContext(c), scopeUserRead) {
				errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks user:read scope")
				c.Abort()
				return
			}
		default:
			if !scopeSetContains(apiKeyScopesFromContext(c), scopeUserWrite) {
				errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks user:write scope")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// enforceNewsAPIScopes mirrors user API scope expectations for authenticated news endpoints.
func enforceNewsAPIScopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authKindFromContext(c) != "api_key" {
			c.Next()
			return
		}
		if !scopeSetContains(apiKeyScopesFromContext(c), scopeUserRead) {
			errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks user:read scope")
			c.Abort()
			return
		}
		c.Next()
	}
}

// enforceAdminAPIScopes enforces scopes for enterprise service keys on admin endpoints.
func enforceAdminAPIScopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authKindFromContext(c) != "api_key" {
			c.Next()
			return
		}
		if !isServiceAPIKey(c) {
			errorResponse(c, http.StatusForbidden, "invalid_api_key", "Invalid API key for admin routes")
			c.Abort()
			return
		}
		fullPath := c.FullPath()
		switch fullPath {
		case "/api/v1/admin/status", "/api/admin/status":
			if !scopeSetContains(apiKeyScopesFromContext(c), scopeAdminRead) {
				errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks admin:read scope")
				c.Abort()
				return
			}
		case "/api/v1/admin/cache", "/api/admin/cache":
			switch c.Query("action") {
			case "stats":
				if !scopeSetContains(apiKeyScopesFromContext(c), scopeAdminRead) {
					errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks admin:read scope")
					c.Abort()
					return
				}
			case "clear":
				if !scopeSetContains(apiKeyScopesFromContext(c), scopeAdminWrite) {
					errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks admin:write scope")
					c.Abort()
					return
				}
			default:
				// Let handler validate action for missing/unknown values.
			}
		default:
			method := c.Request.Method
			if method == http.MethodGet {
				if !scopeSetContains(apiKeyScopesFromContext(c), scopeAdminRead) {
					errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks admin:read scope")
					c.Abort()
					return
				}
			} else if method != http.MethodOptions && method != http.MethodHead {
				if !scopeSetContains(apiKeyScopesFromContext(c), scopeAdminWrite) {
					errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks admin:write scope")
					c.Abort()
					return
				}
			}
		}
		c.Next()
	}
}

func parseAPIKeyPlaintext(full string) (publicID, secretPart string, err error) {
	if !strings.HasPrefix(full, apiKeyPlainPrefix) {
		return "", "", fmt.Errorf("prefix")
	}
	rest := full[len(apiKeyPlainPrefix):]
	if len(rest) < apiKeyPublicIDLen+1 {
		return "", "", fmt.Errorf("short")
	}
	pub := strings.ToLower(rest[:apiKeyPublicIDLen])
	if rest[apiKeyPublicIDLen] != '_' {
		return "", "", fmt.Errorf("sep")
	}
	sec := rest[apiKeyPublicIDLen+1:]
	for i := range pub {
		if !isHexRune(pub[i]) {
			return "", "", fmt.Errorf("pub")
		}
	}
	if len(sec) < 32 {
		return "", "", fmt.Errorf("sec")
	}
	return pub, sec, nil
}

func isHexRune(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}

func parseAPIKeySecretSegment(secret string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(secret)
}

func newUserAPIKeyToken() (plaintext string, publicID string, err error) {
	pubBytes := make([]byte, apiKeyPublicIDLen/2)
	if _, err := rand.Read(pubBytes); err != nil {
		return "", "", err
	}
	publicID = strings.ToLower(hex.EncodeToString(pubBytes))
	secBytes := make([]byte, 32)
	if _, err := rand.Read(secBytes); err != nil {
		return "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(secBytes)
	plaintext = fmt.Sprintf("%s%s_%s", apiKeyPlainPrefix, publicID, secret)
	return plaintext, publicID, nil
}

func normalizeScopes(in []string, allowed map[string]struct{}) ([]string, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}
	outMap := make(map[string]struct{})
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, ok := allowed[s]; !ok {
			return nil, fmt.Errorf("invalid scope %q", s)
		}
		outMap[s] = struct{}{}
	}
	if len(outMap) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}
	out := make([]string, 0, len(outMap))
	for s := range outMap {
		out = append(out, s)
	}
	return out, nil
}

type createAPIKeyBody struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Label  string   `json:"label"` // service keys only
}

// registerUserApiKeyRoutes registers /api/v1/user/api-keys*.
func registerUserApiKeyRoutes(user *gin.RouterGroup) {
	apiKeys := user.Group("/api-keys")
	apiKeys.Use(enforceSessionOrStrongUserScopes())
	{
		apiKeys.GET("", listUserAPIKeysHandler)
		apiKeys.POST("", createUserAPIKeyHandler)
		apiKeys.DELETE("/:id", revokeUserAPIKeyHandler)
	}
}

func enforceSessionOrStrongUserScopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authKindFromContext(c) != "api_key" {
			c.Next()
			return
		}
		if methodNeedsUserWriteForKeyMgmt(c.Request.Method) {
			if !scopeSetContains(apiKeyScopesFromContext(c), scopeUserWrite) {
				errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks user:write scope")
				c.Abort()
				return
			}
			c.Next()
			return
		}
		if !scopeSetContains(apiKeyScopesFromContext(c), scopeUserRead) {
			errorResponse(c, http.StatusForbidden, "insufficient_scope", "API key lacks user:read scope")
			c.Abort()
			return
		}
		c.Next()
	}
}

func methodNeedsUserWriteForKeyMgmt(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// listUserAPIKeysHandler GET /api/v1/user/api-keys
func listUserAPIKeysHandler(c *gin.Context) {
	userVal, _ := c.Get("username")
	username := strings.TrimSpace(userVal.(string))
	recs, err := appRepos.APIKeys.ListUserKeys(c.Request.Context(), username)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_list_failed", "Failed to list API keys")
		return
	}
	type row struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Scopes      []string `json:"scopes"`
		Created     int64    `json:"created"`
		RevokedUnix int64    `json:"revoked,omitempty"`
		ExpiresUnix int64    `json:"expires,omitempty"`
	}
	out := make([]row, 0, len(recs))
	for _, r := range recs {
		out = append(out, row{
			ID:          r.PublicID,
			Name:        r.Name,
			Scopes:      r.Scopes,
			Created:     r.CreatedUnix,
			RevokedUnix: r.RevokedUnix,
			ExpiresUnix: r.ExpiresUnix,
		})
	}
	c.JSON(http.StatusOK, gin.H{"keys": out})
}

// createUserAPIKeyHandler POST /api/v1/user/api-keys
func createUserAPIKeyHandler(c *gin.Context) {
	userVal, _ := c.Get("username")
	username := strings.TrimSpace(userVal.(string))

	var body createAPIKeyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 128 {
		errorResponse(c, http.StatusBadRequest, "invalid_name", "Name must be between 1 and 128 characters")
		return
	}
	scopes, err := normalizeScopes(body.Scopes, allowedUserAPIKeyScopes)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_scopes", err.Error())
		return
	}
	n, err := appRepos.APIKeys.CountActiveUserKeys(c.Request.Context(), username)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_quota_failed", "Failed to check API key quota")
		return
	}
	max := 10
	if appConfig != nil && appConfig.APIKeysMaxPerUser > 0 {
		max = appConfig.APIKeysMaxPerUser
	}
	if n >= max {
		errorResponse(c, http.StatusForbidden, "api_key_quota_exceeded", fmt.Sprintf("Maximum %d active API keys per user", max))
		return
	}
	plaintext, pub, err := newUserAPIKeyToken()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_generate_failed", "Failed to generate API key")
		return
	}
	sum := sha256.Sum256([]byte(plaintext))
	rec := repos.APIKeyRecord{
		PublicID:    pub,
		KeyHashHex:  hex.EncodeToString(sum[:]),
		Name:        sanitizeText(name, 128),
		Scopes:      scopes,
		OwnerType:   "user",
		Username:    username,
		CreatedUnix: time.Now().Unix(),
	}
	if err := appRepos.APIKeys.Save(c.Request.Context(), &rec); err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_save_failed", "Failed to save API key")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":            rec.PublicID,
		"name":          rec.Name,
		"scopes":        rec.Scopes,
		"created":       rec.CreatedUnix,
		"plaintext_key":   plaintext,
		"hint":          "Store this token once; the server retains only a hash.",
	})
}

// revokeUserAPIKeyHandler DELETE /api/v1/user/api-keys/:id
func revokeUserAPIKeyHandler(c *gin.Context) {
	userVal, _ := c.Get("username")
	username := strings.TrimSpace(userVal.(string))
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Missing key id")
		return
	}
	rec, err := appRepos.APIKeys.Get(c.Request.Context(), id)
	if err == redis.Nil || rec == nil || rec.OwnerType != "user" || rec.Username != username {
		errorResponse(c, http.StatusNotFound, "api_key_not_found", "API key not found")
		return
	}
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_lookup_failed", "Failed to load API key")
		return
	}
	if err := appRepos.APIKeys.Revoke(c.Request.Context(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_revoke_failed", "Failed to revoke API key")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API key revoked", "id": id})
}

func forbidAdminKeyMutationsExceptSession(c *gin.Context) bool {
	path := strings.TrimSuffix(c.FullPath(), "/")
	if path == "" {
		path = strings.TrimSuffix(c.Request.URL.Path, "/")
	}
	if authKindFromContext(c) != "api_key" {
		return false
	}
	postTarget := path == "/api/v1/admin/api-keys" || path == "/api/admin/api-keys"
	deleteTarget := strings.HasPrefix(path, "/api/v1/admin/api-keys/") || strings.HasPrefix(path, "/api/admin/api-keys/")
	if !(postTarget || deleteTarget) {
		return false
	}
	method := c.Request.Method
	if method == http.MethodGet {
		return false
	}
	errorResponse(c, http.StatusForbidden, "session_required", "Managing admin API keys requires a browser session")
	c.Abort()
	return true
}

// registerAdminAPIKeyRoutes attaches /admin/api-keys for service keys.
func registerAdminAPIKeyRoutes(admin *gin.RouterGroup) {
	keys := admin.Group("/api-keys")
	keys.POST("", func(c *gin.Context) {
		if forbidAdminKeyMutationsExceptSession(c) {
			return
		}
		createServiceAPIKeyHandler(c)
	})
	keys.GET("", listAdminAPIKeysHandler)
	keys.DELETE("/:id", func(c *gin.Context) {
		if forbidAdminKeyMutationsExceptSession(c) {
			return
		}
		revokeServiceAPIKeyHandler(c)
	})
}

func listAdminAPIKeysHandler(c *gin.Context) {
	recs, err := appRepos.APIKeys.ListServiceKeys(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_list_failed", "Failed to list API keys")
		return
	}
	type row struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Label       string   `json:"label,omitempty"`
		Scopes      []string `json:"scopes"`
		Created     int64    `json:"created"`
		RevokedUnix int64    `json:"revoked,omitempty"`
	}
	out := make([]row, 0, len(recs))
	for _, r := range recs {
		out = append(out, row{
			ID:          r.PublicID,
			Name:        r.Name,
			Label:       r.Label,
			Scopes:      r.Scopes,
			Created:     r.CreatedUnix,
			RevokedUnix: r.RevokedUnix,
		})
	}
	c.JSON(http.StatusOK, gin.H{"keys": out})
}

func createServiceAPIKeyHandler(c *gin.Context) {
	var body createAPIKeyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 128 {
		errorResponse(c, http.StatusBadRequest, "invalid_name", "Name must be between 1 and 128 characters")
		return
	}
	label := sanitizeText(strings.TrimSpace(body.Label), 128)
	scopes, err := normalizeScopes(body.Scopes, allowedServiceAPIKeyScopes)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_scopes", err.Error())
		return
	}
	plaintext, pub, err := newUserAPIKeyToken()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_generate_failed", "Failed to generate API key")
		return
	}
	sum := sha256.Sum256([]byte(plaintext))
	rec := repos.APIKeyRecord{
		PublicID:    pub,
		KeyHashHex:  hex.EncodeToString(sum[:]),
		Name:        sanitizeText(name, 128),
		Scopes:      scopes,
		OwnerType:   "service",
		Label:       label,
		CreatedUnix: time.Now().Unix(),
	}
	if err := appRepos.APIKeys.Save(c.Request.Context(), &rec); err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_save_failed", "Failed to save API key")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":            rec.PublicID,
		"name":          rec.Name,
		"label":         rec.Label,
		"scopes":        rec.Scopes,
		"created":       rec.CreatedUnix,
		"plaintext_key": plaintext,
		"hint":          "Store this token once; the server retains only a hash.",
	})
}

func revokeServiceAPIKeyHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Missing key id")
		return
	}
	rec, err := appRepos.APIKeys.Get(c.Request.Context(), id)
	if err == redis.Nil || rec == nil || rec.OwnerType != "service" {
		errorResponse(c, http.StatusNotFound, "api_key_not_found", "API key not found")
		return
	}
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_lookup_failed", "Failed to load API key")
		return
	}
	if err := appRepos.APIKeys.Revoke(context.Background(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, "api_key_revoke_failed", "Failed to revoke API key")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API key revoked", "id": id})
}
