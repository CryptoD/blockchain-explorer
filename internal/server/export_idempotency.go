package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/idempotency"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// idempotencyStore is wired in Run after Redis is available; nil disables idempotency.
var idempotencyStore *idempotency.Store

func exportIdempotencyScope(c *gin.Context) string {
	if u, ok := c.Get("username"); ok {
		if s, ok2 := u.(string); ok2 && s != "" {
			return "user:" + s
		}
	}
	return "anon:" + c.ClientIP()
}

// beginExportIdempotency inspects Idempotency-Key. If done is true, the response is already written (replay or 409).
func beginExportIdempotency(c *gin.Context) (clientKey string, done bool) {
	if idempotencyStore == nil || appConfig == nil || !appConfig.IdempotencyEnabled {
		return "", false
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		return "", false
	}
	if err := idempotency.ValidateClientKey(key, appConfig.IdempotencyKeyMaxRunes); err != nil {
		errorResponse(c, http.StatusBadRequest, "idempotency_key_invalid", err.Error())
		return key, true
	}
	scope := exportIdempotencyScope(c)
	fp := idempotency.RequestFingerprint(c.Request)
	rk := idempotency.RedisKey(scope, key, fp)

	rec, err := idempotencyStore.Get(c.Request.Context(), rk)
	if err != nil && !errors.Is(err, redis.Nil) {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "idempotency_get_failed").Warn("idempotency Redis get failed; export proceeds without replay")
		c.Set("idempotency_redis_key", rk)
		return key, false
	}
	if err == nil && rec != nil {
		if idempotency.IsJSONReplay(rec) {
			c.Header("Idempotent-Replayed", "true")
			c.Data(rec.Status, rec.ContentType, rec.Body)
			return key, true
		}
		if idempotency.IsStreamConflict(rec) {
			errorResponse(c, http.StatusConflict, "idempotency_replay",
				"An export already completed for this Idempotency-Key and request fingerprint. Use the original response or send a new key.")
			return key, true
		}
	}
	c.Set("idempotency_redis_key", rk)
	return key, false
}

func commitExportJSONIdempotency(c *gin.Context, clientKey string, status int, contentType string, body []byte) {
	if clientKey == "" || idempotencyStore == nil {
		return
	}
	rkVal, _ := c.Get("idempotency_redis_key")
	rk, _ := rkVal.(string)
	if rk == "" {
		return
	}
	if err := idempotencyStore.PutJSON(c.Request.Context(), rk, status, contentType, body); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "idempotency_put_failed").Warn("failed to persist idempotent JSON export record")
	}
}

func commitExportStreamIdempotency(c *gin.Context, clientKey string) {
	if clientKey == "" || idempotencyStore == nil {
		return
	}
	rkVal, _ := c.Get("idempotency_redis_key")
	rk, _ := rkVal.(string)
	if rk == "" {
		return
	}
	if err := idempotencyStore.PutStreamDone(c.Request.Context(), rk); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "idempotency_put_failed").Warn("failed to persist idempotent stream export record")
	}
}
