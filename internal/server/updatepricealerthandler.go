package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func updatePriceAlertHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Alerts require Redis")
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_alert_id", "Alert id must be 1-64 characters")
		return
	}

	key := priceAlertKeyPrefix + username + ":" + id
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil || raw == "" {
		errorResponse(c, http.StatusNotFound, "alert_not_found", "Alert not found")
		return
	}

	var existing PriceAlert
	if err := json.Unmarshal([]byte(raw), &existing); err != nil {
		errorResponse(c, http.StatusInternalServerError, "alert_decode_failed", "Failed to load alert")
		return
	}

	var req updatePriceAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	updated, err := applyPriceAlertUpdate(existing, req)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_alert", err.Error())
		return
	}
	updated.Updated = time.Now().UTC()

	data, _ := json.Marshal(updated)
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "alert_save_failed", "Failed to save alert")
		return
	}
	c.JSON(http.StatusOK, updated)
}

func deletePriceAlertHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Alerts require Redis")
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_alert_id", "Alert id must be 1-64 characters")
		return
	}

	key := priceAlertKeyPrefix + username + ":" + id
	n, err := rdb.Del(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "alert_delete_failed", "Failed to delete alert")
		return
	}
	if n == 0 {
		errorResponse(c, http.StatusNotFound, "alert_not_found", "Alert not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func buildPriceAlertFromCreate(username string, req createPriceAlertRequest) (PriceAlert, error) {
	symbol := strings.ToLower(strings.TrimSpace(req.Symbol))
	if symbol == "" || len(symbol) > 50 {
		return PriceAlert{}, fmt.Errorf("symbol must be 1-50 characters")
	}
	symbol = sanitizeText(symbol, 50)
	currency := strings.ToLower(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "usd"
	}
	if !pricing.SupportedFiatCurrencies[currency] {
		return PriceAlert{}, fmt.Errorf("unsupported currency")
	}
	if req.Threshold <= 0 || req.Threshold > 1e12 {
		return PriceAlert{}, fmt.Errorf("threshold must be a positive number")
	}
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction != "above" && direction != "below" {
		return PriceAlert{}, fmt.Errorf("direction must be \"above\" or \"below\"")
	}
	method := strings.ToLower(strings.TrimSpace(req.DeliveryMethod))
	if method == "" {
		method = "in_app"
	}
	if method != "in_app" && method != "email" {
		return PriceAlert{}, fmt.Errorf("delivery_method must be \"in_app\" or \"email\"")
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	now := time.Now().UTC()
	return PriceAlert{
		ID:             fmt.Sprintf("%d", time.Now().UnixNano()),
		Username:       username,
		Symbol:         symbol,
		Currency:       currency,
		Threshold:      req.Threshold,
		Direction:      direction,
		DeliveryMethod: method,
		IsActive:       active,
		Created:        now,
		Updated:        now,
	}, nil
}

func applyPriceAlertUpdate(existing PriceAlert, req updatePriceAlertRequest) (PriceAlert, error) {
	out := existing
	if req.Symbol != nil {
		symbol := strings.ToLower(strings.TrimSpace(*req.Symbol))
		if symbol == "" || len(symbol) > 50 {
			return PriceAlert{}, fmt.Errorf("symbol must be 1-50 characters")
		}
		out.Symbol = sanitizeText(symbol, 50)
	}
	if req.Currency != nil {
		currency := strings.ToLower(strings.TrimSpace(*req.Currency))
		if currency == "" {
			currency = "usd"
		}
		if !pricing.SupportedFiatCurrencies[currency] {
			return PriceAlert{}, fmt.Errorf("unsupported currency")
		}
		out.Currency = currency
	}
	if req.Threshold != nil {
		if *req.Threshold <= 0 || *req.Threshold > 1e12 {
			return PriceAlert{}, fmt.Errorf("threshold must be a positive number")
		}
		out.Threshold = *req.Threshold
	}
	if req.Direction != nil {
		direction := strings.ToLower(strings.TrimSpace(*req.Direction))
		if direction != "above" && direction != "below" {
			return PriceAlert{}, fmt.Errorf("direction must be \"above\" or \"below\"")
		}
		out.Direction = direction
	}
	if req.DeliveryMethod != nil {
		method := strings.ToLower(strings.TrimSpace(*req.DeliveryMethod))
		if method != "in_app" && method != "email" {
			return PriceAlert{}, fmt.Errorf("delivery_method must be \"in_app\" or \"email\"")
		}
		out.DeliveryMethod = method
	}
	if req.IsActive != nil {
		out.IsActive = *req.IsActive
	}
	return out, nil
}

// evaluatePriceAlerts periodically checks active alerts against current prices
// and marks triggered alerts in Redis.
func evaluatePriceAlerts() {
	start := time.Now()
	cid := correlation.NewID()
	jobLog := logging.WithComponent(logging.ComponentAlerts).WithField(logging.FieldCorrelationID, cid)
	if rdb == nil || assetPricer == nil {
		return
	}
	jobLog.WithField(logging.FieldEvent, "alert_eval_cycle_start").Debug("alert evaluation cycle started")

	ctx := context.Background()
	var (
		scannedKeys  int
		evaluated    int
		triggered    int
		skipped      int
		decodeErrors int
		priceErrors  int
		updateErrors int
	)

	// Per-cycle price cache to avoid repeated upstream calls.
	priceCache := make(map[string]float64) // key: symbol|currency
	priceOK := make(map[string]bool)

	var cursor uint64
	pattern := priceAlertKeyPrefix + "*"
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			jobLog.WithError(err).WithField(logging.FieldEvent, "redis_scan_failed").Warn("alert evaluation scan failed")
			break
		}
		cursor = next
		scannedKeys += len(keys)
		for _, key := range keys {
			raw, err := rdb.Get(ctx, key).Result()
			if err != nil || raw == "" {
				continue
			}
			var a PriceAlert
			if err := json.Unmarshal([]byte(raw), &a); err != nil {
				decodeErrors++
				continue
			}
			if !a.IsActive || a.Symbol == "" || a.Currency == "" || a.Threshold <= 0 || (a.Direction != "above" && a.Direction != "below") {
				skipped++
				continue
			}
			if a.TriggeredAt != nil {
				skipped++
				continue
			}

			evaluated++
			symbol := strings.ToLower(strings.TrimSpace(a.Symbol))
			currency := strings.ToLower(strings.TrimSpace(a.Currency))
			cacheKey := symbol + "|" + currency

			var price float64
			ok := false
			if v, exists := priceCache[cacheKey]; exists {
				price = v
				ok = priceOK[cacheKey]
			} else {
				// Currently we treat alerts as crypto pricing (CoinGecko-backed).
				if p, ok2 := assetPricer.GetAssetPriceInFiat(ctx, pricing.AssetClassCrypto, symbol, currency, 1); ok2 && p > 0 {
					price = p
					ok = true
				} else {
					ok = false
				}
				priceCache[cacheKey] = price
				priceOK[cacheKey] = ok
			}

			if !ok {
				priceErrors++
				continue
			}

			isTriggered := (a.Direction == "above" && price >= a.Threshold) || (a.Direction == "below" && price <= a.Threshold)
			if !isTriggered {
				continue
			}

			now := time.Now().UTC()
			a.IsActive = false
			a.TriggeredAt = &now
			a.Updated = now

			b, _ := json.Marshal(a)
			if err := rdb.Set(ctx, key, b, 0).Err(); err != nil {
				updateErrors++
				continue
			}
			triggered++

			// Always create an in-app notification for a triggered alert.
			createUserNotification(a.Username, Notification{
				Type:    "price_alert",
				Title:   "Price alert triggered",
				Message: fmt.Sprintf("%s %s %.6g %s", strings.ToUpper(a.Symbol), a.Direction, a.Threshold, strings.ToUpper(a.Currency)),
			})

			// Best-effort email delivery for triggered alerts.
			if strings.ToLower(strings.TrimSpace(a.DeliveryMethod)) == "email" {
				user, ok := getUser(a.Username)
				if ok && user.NotificationsEmail && user.EmailPriceAlerts && strings.TrimSpace(user.Email) != "" {
					sendAlertTriggeredEmail(user, a)
				}
			}
		}
		if cursor == 0 {
			break
		}
	}

	elapsed := time.Since(start)
	metrics.RecordAlertEval(elapsed, triggered)
	jobLog.WithFields(log.Fields{
		logging.FieldEvent: "alert_eval_cycle",
		"scanned_keys":     scannedKeys,
		"evaluated":        evaluated,
		"triggered":        triggered,
		"skipped":          skipped,
		"decode_errors":    decodeErrors,
		"price_errors":     priceErrors,
		"update_errors":    updateErrors,
		"duration_ms":      elapsed.Milliseconds(),
	}).Info("price alert evaluation cycle complete")
}

// -----------------------------
// Notifications (Redis-backed)
// -----------------------------

type updateNotificationRequest struct {
	Read      *bool `json:"read,omitempty"`
	Dismissed *bool `json:"dismissed,omitempty"`
}

func listNotificationsHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Notifications require Redis")
		return
	}

	includeDismissed := strings.ToLower(strings.TrimSpace(c.Query("include_dismissed"))) == "true"
	unreadOnly := strings.ToLower(strings.TrimSpace(c.Query("unread_only"))) == "true"
	pagination := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSize)

	items := make([]Notification, 0, 64)
	var cursor uint64
	pattern := notificationKeyPrefix + username + ":*"
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "notifications_fetch_failed", "Failed to fetch notifications")
			return
		}
		cursor = next
		for _, key := range keys {
			raw, err := rdb.Get(ctx, key).Result()
			if err != nil || raw == "" {
				continue
			}
			var n Notification
			if err := json.Unmarshal([]byte(raw), &n); err != nil {
				continue
			}
			if !includeDismissed && n.DismissedAt != nil {
				continue
			}
			if unreadOnly && n.ReadAt != nil {
				continue
			}
			items = append(items, n)
		}
		if cursor == 0 {
			break
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Created.After(items[j].Created)
	})
	total := len(items)
	start := pagination.Offset
	if start > total {
		start = total
	}
	end := start + pagination.PageSize
	if end > total {
		end = total
	}
	pageItems := items[start:end]

	c.JSON(http.StatusOK, gin.H{
		"data": pageItems,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
	})
}

func updateNotificationHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Notifications require Redis")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_notification_id", "Notification id must be 1-64 characters")
		return
	}

	var req updateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	key := notificationKeyPrefix + username + ":" + id
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil || raw == "" {
		errorResponse(c, http.StatusNotFound, "notification_not_found", "Notification not found")
		return
	}
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		errorResponse(c, http.StatusInternalServerError, "notification_decode_failed", "Failed to load notification")
		return
	}
	now := time.Now().UTC()
	if req.Read != nil {
		if *req.Read {
			if n.ReadAt == nil {
				n.ReadAt = &now
			}
		} else {
			n.ReadAt = nil
		}
	}
	if req.Dismissed != nil {
		if *req.Dismissed {
			if n.DismissedAt == nil {
				n.DismissedAt = &now
			}
		} else {
			n.DismissedAt = nil
		}
	}

	b, _ := json.Marshal(n)
	if err := rdb.Set(ctx, key, b, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "notification_save_failed", "Failed to save notification")
		return
	}
	c.JSON(http.StatusOK, n)
}

func dismissNotificationHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Notifications require Redis")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_notification_id", "Notification id must be 1-64 characters")
		return
	}
	// Soft-dismiss by setting dismissed_at (keeps history for audit if needed).
	key := notificationKeyPrefix + username + ":" + id
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil || raw == "" {
		errorResponse(c, http.StatusNotFound, "notification_not_found", "Notification not found")
		return
	}
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		errorResponse(c, http.StatusInternalServerError, "notification_decode_failed", "Failed to load notification")
		return
	}
	now := time.Now().UTC()
	n.DismissedAt = &now
	b, _ := json.Marshal(n)
	if err := rdb.Set(ctx, key, b, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "notification_save_failed", "Failed to save notification")
		return
	}
	c.JSON(http.StatusOK, gin.H{"dismissed": true})
}

func createUserNotification(username string, n Notification) {
	username = strings.TrimSpace(username)
	if username == "" || rdb == nil {
		return
	}
	now := time.Now().UTC()
	n.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	n.Username = username
	if strings.TrimSpace(n.Type) == "" {
		n.Type = "system"
	}
	n.Title = sanitizeText(n.Title, 120)
	n.Message = sanitizeText(n.Message, 500)
	n.Created = now
	key := notificationKeyPrefix + username + ":" + n.ID
	if b, err := json.Marshal(n); err == nil {
		_ = rdb.Set(ctx, key, b, 0).Err()
	}
}

func sendWelcomeEmail(username, toEmail string) {
	if emailService == nil || emailTemplates == nil || !emailService.Enabled() {
		return
	}
	subj, textBody, htmlBody := emailTemplates.Welcome(email.WelcomeData{Username: username})
	emailService.Enqueue(email.Message{
		To:      email.Address{Email: toEmail},
		Subject: subj,
		Text:    textBody,
		HTML:    htmlBody,
		Tags:    map[string]string{"type": "welcome"},
	})
}

func sendAlertTriggeredEmail(user User, alert PriceAlert) {
	if emailService == nil || emailTemplates == nil || !emailService.Enabled() {
		return
	}
	subj, textBody, htmlBody := emailTemplates.AlertTriggered(email.AlertTriggeredData{
		Username:  user.Username,
		Symbol:    alert.Symbol,
		Currency:  strings.ToUpper(alert.Currency),
		Direction: alert.Direction,
		Threshold: alert.Threshold,
	})
	ok := emailService.Enqueue(email.Message{
		To:      email.Address{Email: user.Email, Name: user.Username},
		Subject: subj,
		Text:    textBody,
		HTML:    htmlBody,
		Tags:    map[string]string{"type": "alert_triggered", "symbol": alert.Symbol},
	})
	if !ok {
		logging.WithComponent(logging.ComponentEmail).WithFields(log.Fields{
			logging.FieldUsername: user.Username,
			"symbol":              alert.Symbol,
			logging.FieldEvent:    "queue_full",
		}).Warn("email queue full; dropping alert email")
	}
}

// loginHandler handles user authentication
func loginHandler(c *gin.Context) {
	var loginReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	loginReq.Username = strings.TrimSpace(loginReq.Username)
	if len(loginReq.Username) < 3 || len(loginReq.Username) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_username", "Username must be between 3 and 64 characters")
		return
	}
	usernamePattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !usernamePattern.MatchString(loginReq.Username) {
		errorResponse(c, http.StatusBadRequest, "invalid_username", "Username may only contain letters, numbers, dots, dashes, and underscores")
		return
	}
	if len(loginReq.Password) < 6 || len(loginReq.Password) > 128 {
		errorResponse(c, http.StatusBadRequest, "invalid_password", "Password must be between 6 and 128 characters")
		return
	}

	user, authenticated := authSvc.AuthenticateUser(loginReq.Username, loginReq.Password)
	if !authenticated {
		errorResponse(c, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
		logging.WithComponent(logging.ComponentAuth).WithFields(log.Fields{
			logging.FieldUsername: loginReq.Username,
			logging.FieldEvent:    "login_failed",
		}).Warn("failed login attempt")
		return
	}

	// Session fixation: invalidate any existing session cookie before issuing a new authenticated session
	// (login elevation and account switches).
	if oldSID, err := c.Cookie("session_id"); err == nil && strings.TrimSpace(oldSID) != "" {
		authSvc.DestroySession(oldSID)
	}

	sessionID, err := authSvc.CreateSession(loginReq.Username)
	if err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldEvent, "session_create_failed").Error("failed to create session")
		errorResponse(c, http.StatusInternalServerError, "session_creation_failed", "Failed to create session")
		return
	}

	csrfToken, err := authSvc.CreateOrUpdateCSRFToken(sessionID)
	if err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldEvent, "csrf_create_failed").Error("failed to create CSRF token")
		errorResponse(c, http.StatusInternalServerError, "csrf_creation_failed", "Failed to create CSRF token")
		return
	}

	c.SetCookie("session_id", sessionID, 86400, "/", "", config.UseSecureCookies(), true) // 24 hours
	c.JSON(http.StatusOK, gin.H{
		"message":   "Login successful",
		"username":  loginReq.Username,
		"role":      user.Role,
		"csrfToken": csrfToken,
	})
	logging.WithComponent(logging.ComponentAuth).WithFields(log.Fields{
		logging.FieldUsername: loginReq.Username,
		"role":                user.Role,
		logging.FieldEvent:    "login_success",
	}).Info("user logged in successfully")
}

// logoutHandler handles user logout
func logoutHandler(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err == nil {
		authSvc.DestroySession(sessionID)
	}

	c.SetCookie("session_id", "", -1, "/", "", config.UseSecureCookies(), true)
	c.JSON(http.StatusOK, gin.H{"message": "Logout successful"})
}

// csrfMiddleware enforces CSRF protection for state-changing and admin endpoints
// when cookie-based authentication is in use.
func csrfMiddleware(c *gin.Context) {
	path := c.FullPath()
	method := c.Request.Method

	// Skip CSRF checks for login and registration endpoints
	if path == "/api/login" || path == "/api/register" || path == "/api/v1/login" || path == "/api/v1/register" {
		c.Next()
		return
	}

	// Determine if this request should be protected
	isAdmin := strings.HasPrefix(path, "/api/admin") || strings.HasPrefix(path, "/api/v1/admin")
	isStateChanging := method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch

	// Only enforce on state-changing endpoints or any admin endpoint
	if !isAdmin && !isStateChanging {
		c.Next()
		return
	}

	// Only apply CSRF protection when cookie-based auth is in use
	sessionID, err := c.Cookie("session_id")
	if err != nil || sessionID == "" {
		c.Next()
		return
	}

	providedToken := c.GetHeader("X-CSRF-Token")
	if providedToken == "" {
		abortErrorResponse(c, http.StatusForbidden, "csrf_token_missing", "CSRF token missing")
		return
	}

	expectedToken, _ := getCSRFTokenForSession(sessionID)
	if expectedToken == "" || !secureCompare(providedToken, expectedToken) {
		abortErrorResponse(c, http.StatusForbidden, "csrf_token_invalid", "Invalid CSRF token")
		return
	}

	c.Next()
}

// secureCompare performs a constant-time comparison of two strings.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// registerHandler handles user registration
func registerHandler(c *gin.Context) {
	var registerReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email"` // Optional for now
	}

	if err := c.ShouldBindJSON(&registerReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	// Basic validation
	registerReq.Username = strings.TrimSpace(registerReq.Username)
	if len(registerReq.Username) < 3 || len(registerReq.Username) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_username", "Username must be between 3 and 64 characters")
		return
	}
	usernamePattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !usernamePattern.MatchString(registerReq.Username) {
		errorResponse(c, http.StatusBadRequest, "invalid_username", "Username may only contain letters, numbers, dots, dashes, and underscores")
		return
	}
	if !isStrongPassword(registerReq.Password) {
		errorResponse(c, http.StatusBadRequest, "invalid_password", "Password must be 8-128 characters and include at least one letter and one digit")
		return
	}
	if registerReq.Email != "" {
		registerReq.Email = strings.TrimSpace(registerReq.Email)
		if len(registerReq.Email) > 254 {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Email must be at most 254 characters")
			return
		}
		emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
		if !emailPattern.MatchString(registerReq.Email) {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Invalid email format")
			return
		}
	}

	// Create user with default "user" role
	err := authSvc.CreateUser(registerReq.Username, registerReq.Password, "user", registerReq.Email)
	if err != nil {
		if err.Error() == "user already exists" {
			errorResponse(c, http.StatusConflict, "username_taken", "Username already exists")
		} else {
			logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldEvent, "user_create_failed").Error("failed to create user")
			errorResponse(c, http.StatusInternalServerError, "user_creation_failed", "Failed to create user")
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
	logging.WithComponent(logging.ComponentAuth).WithFields(log.Fields{
		logging.FieldUsername: registerReq.Username,
		logging.FieldEvent:    "register_success",
	}).Info("new user registered")

	// Best-effort onboarding email if configured and email provided.
	if strings.TrimSpace(registerReq.Email) != "" {
		sendWelcomeEmail(registerReq.Username, registerReq.Email)
	}
}

// feedbackHandler handles user feedback submissions
// POST /api/feedback
// Stores feedback in Redis with a 30-day expiration
