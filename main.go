package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/news"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/CryptoD/blockchain-explorer/internal/sentryutil"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/jung-kurt/gofpdf/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// Simple i18n support
var translations = map[string]map[string]string{
	"en": {
		"login_required":      "Login required",
		"invalid_credentials": "Invalid credentials",
		"logout_successful":   "Logout successful",
		"cache_cleared":       "Cache cleared successfully",
		"cache_stats":         "Cache statistics retrieved",
		"admin_only":          "Admin access required",
	},
	"es": {
		"login_required":      "Inicio de sesión requerido",
		"invalid_credentials": "Credenciales inválidas",
		"logout_successful":   "Cierre de sesión exitoso",
		"cache_cleared":       "Caché limpiado exitosamente",
		"cache_stats":         "Estadísticas de caché recuperadas",
		"admin_only":          "Acceso de administrador requerido",
	},
}

// Supported profile preference values (validated on PATCH /user/profile)
var (
	supportedThemes       = map[string]bool{"light": true, "dark": true, "system": true}
	supportedLandingPages = map[string]bool{"explorer": true, "dashboard": true, "portfolios": true}
	supportedLangs        map[string]bool // populated once from translations keys
)

func init() {
	supportedLangs = make(map[string]bool)
	for lang := range translations {
		supportedLangs[lang] = true
	}
}

func T(lang, key string) string {
	if langMap, exists := translations[lang]; exists {
		if translation, exists := langMap[key]; exists {
			return translation
		}
	}
	// Fallback to English
	if langMap, exists := translations["en"]; exists {
		if translation, exists := langMap[key]; exists {
			return translation
		}
	}
	return key // Return key if no translation found
}

// global Redis client and context
var ErrNotFound = errors.New("not found")
var ctx = context.Background()

var rdb redisstore.Client = redis.NewClient(&redis.Options{
	Addr: config.GetEnvWithDefault("REDIS_HOST", "localhost") + ":6379",
	DB:   0, // use default DB
})

// Rate limiting variables (used as a fallback when Redis is unavailable)
var rateLimitCount = make(map[string]int)
var rateLimitReset = make(map[string]time.Time)
var rateLimitMutex sync.Mutex

// Export-specific rate limiting (stricter; separate from general API limits)
var exportRateLimitCount = make(map[string]int)
var exportRateLimitReset = make(map[string]time.Time)
var exportRateLimitMutex sync.Mutex

// checkExportRateLimit enforces per-IP and per-user limits for export endpoints.
// If heavy is true, stricter limits apply (e.g. for transactions CSV).
// Aborts with 429 and returns false when exceeded; returns true otherwise.
func checkExportRateLimit(c *gin.Context, heavy bool) bool {
	ip := c.ClientIP()
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)

	windowSeconds := 60
	perIP := 5
	perUser := 20
	if heavy {
		perIP = 2
		perUser = 5
	}
	if appConfig != nil {
		if appConfig.ExportRateLimitPerIP > 0 && !heavy {
			perIP = appConfig.ExportRateLimitPerIP
		}
		if appConfig.ExportRateLimitPerUser > 0 && !heavy {
			perUser = appConfig.ExportRateLimitPerUser
		}
		if heavy && appConfig.ExportRateLimitHeavyPerIP > 0 {
			perIP = appConfig.ExportRateLimitHeavyPerIP
		}
		if heavy && appConfig.ExportRateLimitHeavyPerUser > 0 {
			perUser = appConfig.ExportRateLimitHeavyPerUser
		}
	}
	window := time.Duration(windowSeconds) * time.Second
	prefix := "export"
	if heavy {
		prefix = "export:heavy"
	}

	if rdb != nil {
		ctx := context.Background()
		ipKey := fmt.Sprintf("rate:%s:ip:%s", prefix, ip)
		ipCount, err := rdb.Incr(ctx, ipKey).Result()
		if err == nil {
			if ipCount == 1 {
				_ = rdb.Expire(ctx, ipKey, window).Err()
			}
			if ipCount > int64(perIP) {
				log.WithFields(log.Fields{
					logging.FieldComponent: logging.ComponentRateLimit,
					logging.FieldEvent:     "export_rate_limit",
					logging.FieldIP:        ip,
					logging.FieldExport:    prefix,
					"backend":              "redis",
				}).Warn("Export rate limit exceeded (IP)")
				errorResponse(c, http.StatusTooManyRequests, "export_rate_limited", "Too many export requests; try again later")
				c.Abort()
				return false
			}
		}
		if username != "" {
			userKey := fmt.Sprintf("rate:%s:user:%s", prefix, username)
			userCount, err := rdb.Incr(ctx, userKey).Result()
			if err == nil {
				if userCount == 1 {
					_ = rdb.Expire(ctx, userKey, window).Err()
				}
				if userCount > int64(perUser) {
					log.WithFields(log.Fields{
						logging.FieldComponent: logging.ComponentRateLimit,
						logging.FieldEvent:     "export_rate_limit",
						logging.FieldUsername:  username,
						logging.FieldExport:    prefix,
						"backend":              "redis",
					}).Warn("Export rate limit exceeded (user)")
					errorResponse(c, http.StatusTooManyRequests, "export_rate_limited", "Too many export requests; try again later")
					c.Abort()
					return false
				}
			}
		}
		return true
	}

	// In-memory fallback
	exportRateLimitMutex.Lock()
	defer exportRateLimitMutex.Unlock()
	now := time.Now()
	ipKey := prefix + ":ip:" + ip
	if reset, ok := exportRateLimitReset[ipKey]; ok && now.After(reset) {
		exportRateLimitCount[ipKey] = 0
		exportRateLimitReset[ipKey] = now.Add(window)
	}
	if _, ok := exportRateLimitReset[ipKey]; !ok {
		exportRateLimitCount[ipKey] = 0
		exportRateLimitReset[ipKey] = now.Add(window)
	}
	exportRateLimitCount[ipKey]++
	if exportRateLimitCount[ipKey] > perIP {
		log.WithFields(log.Fields{
			logging.FieldComponent: logging.ComponentRateLimit,
			logging.FieldEvent:     "export_rate_limit",
			logging.FieldIP:        ip,
			logging.FieldExport:    prefix,
			"backend":              "memory",
		}).Warn("Export rate limit exceeded (in-memory)")
		errorResponse(c, http.StatusTooManyRequests, "export_rate_limited", "Too many export requests; try again later")
		c.Abort()
		return false
	}
	if username != "" {
		userKey := prefix + ":user:" + username
		if reset, ok := exportRateLimitReset[userKey]; ok && now.After(reset) {
			exportRateLimitCount[userKey] = 0
			exportRateLimitReset[userKey] = now.Add(window)
		}
		if _, ok := exportRateLimitReset[userKey]; !ok {
			exportRateLimitCount[userKey] = 0
			exportRateLimitReset[userKey] = now.Add(window)
		}
		exportRateLimitCount[userKey]++
		if exportRateLimitCount[userKey] > perUser {
			log.WithFields(log.Fields{
				logging.FieldComponent: logging.ComponentRateLimit,
				logging.FieldEvent:     "export_rate_limit",
				logging.FieldUsername:  username,
				logging.FieldExport:    prefix,
				"backend":              "memory",
			}).Warn("Export rate limit exceeded (in-memory)")
			errorResponse(c, http.StatusTooManyRequests, "export_rate_limited", "Too many export requests; try again later")
			c.Abort()
			return false
		}
	}
	return true
}

// logLargeExport logs when an export request is large or resource-intensive (for monitoring/abuse detection).
func logLargeExport(c *gin.Context, endpoint string, details map[string]interface{}) {
	fields := log.Fields{
		"endpoint": endpoint,
		"ip":       c.ClientIP(),
	}
	if u, ok := c.Get("username"); ok {
		fields["username"] = u
	}
	for k, v := range details {
		fields[k] = v
	}
	fields[logging.FieldComponent] = logging.ComponentExport
	fields[logging.FieldEvent] = "large_export"
	log.WithFields(fields).Info("Large or intensive export request")
}

// User struct definition
type User struct {
	Username                 string    `json:"username"`
	Password                 string    `json:"-"`                               // Hashed password, never sent in JSON
	Role                     string    `json:"role"`                            // "admin" or "user"
	Email                    string    `json:"email,omitempty"`                 // Optional email for notifications/onboarding
	PreferredCurrency        string    `json:"preferred_currency,omitempty"`    // Fiat code (e.g. usd, eur); validated against supported list
	Theme                    string    `json:"theme,omitempty"`                 // "light", "dark", "system"
	Language                 string    `json:"language,omitempty"`              // e.g. "en", "es"; validated against supported list
	NotificationsEmail       bool      `json:"notifications_email"`             // Whether to receive email notifications
	NotificationsPriceAlerts bool      `json:"notifications_price_alerts"`      // Whether to receive price alert notifications
	EmailPriceAlerts         bool      `json:"email_price_alerts"`              // Whether to receive email notifications for price alerts
	EmailPortfolioEvents     bool      `json:"email_portfolio_events"`          // Whether to receive email notifications for portfolio events
	EmailProductUpdates      bool      `json:"email_product_updates"`           // Whether to receive email notifications for product updates
	DefaultLandingPage       string    `json:"default_landing_page,omitempty"`  // "explorer", "dashboard", "portfolios"
	NewsSourcesFavorite      []string  `json:"news_sources_favorite,omitempty"` // preferred sources (domains)
	NewsSourcesBlocked       []string  `json:"news_sources_blocked,omitempty"`  // muted sources (domains)
	Created                  time.Time `json:"created"`
}

type PortfolioItem struct {
	Type    string  `json:"type"`             // "crypto", "commodity", "bond", "stock"
	Address string  `json:"address"`          // Optional; e.g. wallet address for crypto
	Label   string  `json:"label"`            // Display name
	Amount  float64 `json:"amount"`           // Quantity (units: coins, oz, face value, etc.)
	Symbol  string  `json:"symbol,omitempty"` // Pricing id: "bitcoin", "XAU", "US10Y"; crypto defaults to "bitcoin" when empty
}

type Portfolio struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Items       []PortfolioItem `json:"items"`
	Created     time.Time       `json:"created"`
	Updated     time.Time       `json:"updated"`
}

type PriceAlert struct {
	ID             string     `json:"id"`
	Username       string     `json:"username"`
	Symbol         string     `json:"symbol"`          // e.g., "bitcoin", "btc"
	Currency       string     `json:"currency"`        // e.g., "usd"
	Threshold      float64    `json:"threshold"`       // price threshold in Currency
	Direction      string     `json:"direction"`       // "above" or "below"
	DeliveryMethod string     `json:"delivery_method"` // "in_app" or "email"
	IsActive       bool       `json:"is_active"`
	TriggeredAt    *time.Time `json:"triggered_at,omitempty"`
	Created        time.Time  `json:"created"`
	Updated        time.Time  `json:"updated"`
}

// Notification is an in-app message for an authenticated user.
// Stored in Redis under notification:{username}:{id}.
type Notification struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Type        string     `json:"type"` // "price_alert" | "system"
	Title       string     `json:"title"`
	Message     string     `json:"message"`
	Created     time.Time  `json:"created"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	DismissedAt *time.Time `json:"dismissed_at,omitempty"`
}

// WatchlistEntry is a single item in a watchlist (symbol or address with optional tags/notes and group).
// Exactly one of Symbol or Address should be set; Type indicates which ("symbol" or "address").
// Group is optional and used for grouping display (e.g. "Crypto", "High risk", or custom).
type WatchlistEntry struct {
	Type    string   `json:"type"`              // "symbol" or "address"
	Symbol  string   `json:"symbol,omitempty"`  // e.g. "bitcoin", "ethereum"; used when Type == "symbol"
	Address string   `json:"address,omitempty"` // blockchain address; used when Type == "address"
	Tags    []string `json:"tags,omitempty"`    // optional labels (max 10, each max 50 chars)
	Notes   string   `json:"notes,omitempty"`   // optional free text (max 500 chars)
	Group   string   `json:"group,omitempty"`   // optional group label for grouping (e.g. by asset type, risk, custom); max 50 chars
}

// Watchlist is keyed by user and watchlist ID. Stored in Redis at watchlist:{username}:{id}.
// No TTL: watchlists are persistent user data (same strategy as portfolios).
type Watchlist struct {
	ID       string           `json:"id"`
	Username string           `json:"username"`
	Name     string           `json:"name"` // optional display name
	Entries  []WatchlistEntry `json:"entries"`
	Created  time.Time        `json:"created"`
	Updated  time.Time        `json:"updated"`
}

// User authentication variables
var (
	users     = make(map[string]User) // username -> User
	userMutex sync.RWMutex
	// In-memory session store as a fallback
	sessionStore = make(map[string]string)
	sessionMutex sync.RWMutex
	// In-memory CSRF token store as a fallback
	csrfStore = make(map[string]string)
	csrfMutex sync.RWMutex
)

// generateSessionID creates a random session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// createSession creates a new session for the user
func createSession(username string) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	sessionMutex.Lock()
	sessionStore[sessionID] = username
	sessionMutex.Unlock()

	// Store session in Redis with 24 hour expiration if Redis is configured
	if rdb != nil {
		_ = rdb.Set(ctx, "session:"+sessionID, username, 24*time.Hour).Err()
	}

	return sessionID, nil
}

// validateSession checks if a session is valid
func validateSession(sessionID string) (string, bool) {
	// Check Redis first
	if rdb != nil {
		if username, err := rdb.Get(ctx, "session:"+sessionID).Result(); err == nil && username != "" {
			return username, true
		}
	}

	// Fallback to in-memory store
	sessionMutex.RLock()
	username, exists := sessionStore[sessionID]
	sessionMutex.RUnlock()

	return username, exists
}

// destroySession removes a session
func destroySession(sessionID string) {
	sessionMutex.Lock()
	delete(sessionStore, sessionID)
	sessionMutex.Unlock()

	csrfMutex.Lock()
	delete(csrfStore, sessionID)
	csrfMutex.Unlock()

	if rdb != nil {
		_ = rdb.Del(ctx, "session:"+sessionID).Err()
		_ = rdb.Del(ctx, "csrf:"+sessionID).Err()
	}
}

// createOrUpdateCSRFToken generates and stores a CSRF token associated with a session.
func createOrUpdateCSRFToken(sessionID string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(bytes)

	csrfMutex.Lock()
	csrfStore[sessionID] = token
	csrfMutex.Unlock()

	if rdb != nil {
		if err := rdb.Set(ctx, "csrf:"+sessionID, token, 24*time.Hour).Err(); err != nil {
			logging.WithComponent(logging.ComponentAuth).WithError(err).Warn("Failed to store CSRF token in Redis")
		}
	}

	return token, nil
}

// getCSRFTokenForSession retrieves the CSRF token associated with a session.
func getCSRFTokenForSession(sessionID string) (string, error) {
	if rdb != nil {
		if val, err := rdb.Get(ctx, "csrf:"+sessionID).Result(); err == nil && val != "" {
			return val, nil
		}
	}

	csrfMutex.RLock()
	defer csrfMutex.RUnlock()
	if token, ok := csrfStore[sessionID]; ok {
		return token, nil
	}
	return "", nil
}

// loadUsersFromRedis loads all users from Redis
func loadUsersFromRedis() error {
	if rdb == nil {
		return nil // No Redis, use in-memory only
	}

	keys, err := rdb.Keys(ctx, "user:*").Result()
	if err != nil {
		return err
	}

	userMutex.Lock()
	defer userMutex.Unlock()

	for _, key := range keys {
		username := strings.TrimPrefix(key, "user:")
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to load user from Redis")
			continue
		}

		var user User
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to unmarshal user from Redis")
			continue
		}

		users[username] = user
	}

	return nil
}

// saveUserToRedis saves a user to Redis
func saveUserToRedis(user User) error {
	if rdb == nil {
		return nil // No Redis, use in-memory only
	}

	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return rdb.Set(ctx, "user:"+user.Username, data, 0).Err() // No expiration
}

// sanitizeText trims, truncates, strips control characters (except basic
// whitespace), and HTML-escapes user-supplied text before storage or rendering.
func sanitizeText(input string, maxLen int) string {
	s := strings.TrimSpace(input)
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
	}

	var b strings.Builder
	for _, r := range s {
		// Skip non-printable control characters except tab/newline/carriage-return
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	return html.EscapeString(cleaned)
}

// isStrongPassword enforces a basic password policy:
// - length between 8 and 128 characters
// - must contain at least one letter and one digit.
func isStrongPassword(pw string) bool {
	if len(pw) < 8 || len(pw) > 128 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range pw {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

// initializeDefaultAdmin creates the default admin user if it doesn't exist.
// In non-development environments, ADMIN_USERNAME and ADMIN_PASSWORD must be provided.
// In development, sensible but insecure defaults are allowed for convenience.
func initializeDefaultAdmin() {
	// First try to load existing users from Redis
	if err := loadUsersFromRedis(); err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).Warn("Failed to load users from Redis")
	}

	appEnv := config.GetAppEnv()

	userMutex.Lock()
	defer userMutex.Unlock()

	adminUsername := os.Getenv("ADMIN_USERNAME")
	adminPassword := os.Getenv("ADMIN_PASSWORD")

	if appEnv == "development" {
		if adminUsername == "" {
			adminUsername = "admin"
		}
		if adminPassword == "" {
			adminPassword = "admin123"
		}
	} else {
		if adminUsername == "" || adminPassword == "" {
			logging.WithComponent(logging.ComponentAdmin).Fatal("ADMIN_USERNAME and ADMIN_PASSWORD must be set in non-development environments")
		}
		if !isStrongPassword(adminPassword) {
			logging.WithComponent(logging.ComponentAdmin).Fatal("ADMIN_PASSWORD must be 8-128 characters and include at least one letter and one digit in non-development environments")
		}
	}

	if _, exists := users[adminUsername]; !exists {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			logging.WithComponent(logging.ComponentAdmin).WithError(err).Fatal("Failed to hash default admin password")
		}

		adminUser := User{
			Username: adminUsername,
			Password: string(hashedPassword),
			Role:     "admin",
			Created:  time.Now(),
		}

		users[adminUsername] = adminUser

		// Save to Redis
		if err := saveUserToRedis(adminUser); err != nil {
			logging.WithComponent(logging.ComponentAdmin).WithError(err).Warn("Failed to save default admin to Redis")
		}

		if appEnv == "development" && (os.Getenv("ADMIN_USERNAME") == "" || os.Getenv("ADMIN_PASSWORD") == "") {
			logging.WithComponent(logging.ComponentAdmin).WithField(logging.FieldEnv, appEnv).Warn("Default admin credentials (admin/admin123) are being used in development")
		}
		logging.WithComponent(logging.ComponentAdmin).WithFields(log.Fields{
			logging.FieldEvent:    "default_admin_initialized",
			logging.FieldUsername: adminUsername,
			logging.FieldEnv:      appEnv,
		}).Info("Default admin user initialized")
	}
}

// createUser adds a new user to the system
func createUser(username, password, role, email string) error {
	userMutex.Lock()
	defer userMutex.Unlock()

	if _, exists := users[username]; exists {
		return errors.New("user already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user := User{
		Username: username,
		Password: string(hashedPassword),
		Role:     role,
		Email:    strings.TrimSpace(email),
		Created:  time.Now(),
	}

	users[username] = user

	// Save to Redis
	if rdb != nil {
		data, _ := json.Marshal(user)
		_ = rdb.Set(ctx, "user:"+user.Username, data, 0).Err()
	}

	return nil
}

// getUser retrieves a user by username
func getUser(username string) (User, bool) {
	userMutex.RLock()
	defer userMutex.RUnlock()
	user, exists := users[username]
	return user, exists
}

// authenticateUser checks if username/password combination is valid
func authenticateUser(username, password string) (User, bool) {
	user, exists := getUser(username)
	if !exists {
		return User{}, false
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	return user, err == nil
}

// userProfileHandler returns the profile of the authenticated user (includes preferred_currency from Redis).
func userProfileHandler(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found in session")
		return
	}

	user, exists := getUser(username.(string))
	if !exists {
		errorResponse(c, http.StatusNotFound, "user_profile_not_found", "User profile not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

// updateProfileRequest is the body for PATCH /api/user/profile (profile settings).
type updateProfileRequest struct {
	Email                    *string   `json:"email"`
	PreferredCurrency        *string   `json:"preferred_currency"` // Fiat code (e.g. usd, eur); empty string clears preference
	Theme                    *string   `json:"theme"`              // "light", "dark", "system"
	Language                 *string   `json:"language"`           // e.g. "en", "es"
	NotificationsEmail       *bool     `json:"notifications_email"`
	NotificationsPriceAlerts *bool     `json:"notifications_price_alerts"`
	EmailPriceAlerts         *bool     `json:"email_price_alerts"`
	EmailPortfolioEvents     *bool     `json:"email_portfolio_events"`
	EmailProductUpdates      *bool     `json:"email_product_updates"`
	DefaultLandingPage       *string   `json:"default_landing_page"` // "explorer", "dashboard", "portfolios"
	NewsSourcesFavorite      *[]string `json:"news_sources_favorite"`
	NewsSourcesBlocked       *[]string `json:"news_sources_blocked"`
}

// updateProfileHandler updates the authenticated user's profile settings (e.g. preferred_currency).
func updateProfileHandler(c *gin.Context) {
	usernameVal, exists := c.Get("username")
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found in session")
		return
	}
	username := usernameVal.(string)

	var body updateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	user, exists := getUser(username)
	if !exists {
		errorResponse(c, http.StatusNotFound, "user_profile_not_found", "User profile not found")
		return
	}

	if body.Email != nil {
		v := strings.TrimSpace(*body.Email)
		if v != "" {
			if len(v) > 254 {
				errorResponse(c, http.StatusBadRequest, "invalid_email", "Email must be at most 254 characters")
				return
			}
			emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
			if !emailPattern.MatchString(v) {
				errorResponse(c, http.StatusBadRequest, "invalid_email", "Invalid email format")
				return
			}
		}
		user.Email = v
	}

	if body.PreferredCurrency != nil {
		code := strings.ToLower(strings.TrimSpace(*body.PreferredCurrency))
		if code != "" && !pricing.SupportedFiatCurrencies[code] {
			errorResponse(c, http.StatusBadRequest, "unsupported_currency", "Unsupported currency code; use a supported fiat code (e.g. usd, eur, gbp)")
			return
		}
		user.PreferredCurrency = code
	}
	if body.Theme != nil {
		v := strings.ToLower(strings.TrimSpace(*body.Theme))
		if v != "" && !supportedThemes[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_theme", "Theme must be one of: light, dark, system")
			return
		}
		user.Theme = v
	}
	if body.Language != nil {
		v := strings.ToLower(strings.TrimSpace(*body.Language))
		if v != "" && !supportedLangs[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_language", "Unsupported language code; use a supported code (e.g. en, es)")
			return
		}
		user.Language = v
	}
	if body.NotificationsEmail != nil {
		user.NotificationsEmail = *body.NotificationsEmail
	}
	if body.NotificationsPriceAlerts != nil {
		user.NotificationsPriceAlerts = *body.NotificationsPriceAlerts
	}
	if body.EmailPriceAlerts != nil {
		user.EmailPriceAlerts = *body.EmailPriceAlerts
	}
	if body.EmailPortfolioEvents != nil {
		user.EmailPortfolioEvents = *body.EmailPortfolioEvents
	}
	if body.EmailProductUpdates != nil {
		user.EmailProductUpdates = *body.EmailProductUpdates
	}
	if body.DefaultLandingPage != nil {
		v := strings.ToLower(strings.TrimSpace(*body.DefaultLandingPage))
		if v != "" && !supportedLandingPages[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_default_landing_page", "Default landing page must be one of: explorer, dashboard, portfolios")
			return
		}
		user.DefaultLandingPage = v
	}

	if body.NewsSourcesFavorite != nil {
		list, err := normalizeNewsSourceList(*body.NewsSourcesFavorite, 50)
		if err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_news_sources_favorite", err.Error())
			return
		}
		user.NewsSourcesFavorite = list
	}
	if body.NewsSourcesBlocked != nil {
		list, err := normalizeNewsSourceList(*body.NewsSourcesBlocked, 200)
		if err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_news_sources_blocked", err.Error())
			return
		}
		user.NewsSourcesBlocked = list
	}

	userMutex.Lock()
	users[username] = user
	userMutex.Unlock()

	if err := saveUserToRedis(user); err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to persist profile to Redis")
		errorResponse(c, http.StatusInternalServerError, "save_failed", "Failed to save profile")
		return
	}

	c.JSON(http.StatusOK, user)
}

// authMiddleware checks for valid authentication
func authMiddleware(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		c.Abort()
		return
	}

	username, valid := validateSession(sessionID)
	if !valid {
		errorResponse(c, http.StatusUnauthorized, "invalid_session", "Invalid session")
		c.Abort()
		return
	}

	user, exists := getUser(username)
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found")
		c.Abort()
		return
	}

	c.Set("username", username)
	c.Set("role", user.Role)
	c.Next()
}

// requireRoleMiddleware checks for specific role access
func requireRoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			errorResponse(c, http.StatusForbidden, "role_missing", "Role information missing")
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok {
			errorResponse(c, http.StatusForbidden, "role_invalid", "Invalid role format")
			c.Abort()
			return
		}

		if userRole != requiredRole && userRole != "admin" { // Admin can access everything
			errorResponse(c, http.StatusForbidden, "insufficient_permissions", "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware limits requests per IP and per authenticated user.
// It prefers a Redis-backed implementation for multi-instance resilience,
// and falls back to an in-memory limiter when Redis is unavailable.
func rateLimitMiddleware(c *gin.Context) {
	if c.Request.URL.Path == "/metrics" {
		c.Next()
		return
	}
	ip := c.ClientIP()
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)

	// Configurable limits via configuration (with sane defaults if config is nil)
	windowSeconds := 60
	perIPLimit := 10
	perUserLimit := 10
	if appConfig != nil {
		if appConfig.RateLimitWindowSeconds > 0 {
			windowSeconds = appConfig.RateLimitWindowSeconds
		}
		if appConfig.RateLimitPerIP > 0 {
			perIPLimit = appConfig.RateLimitPerIP
		}
		if appConfig.RateLimitPerUser > 0 {
			perUserLimit = appConfig.RateLimitPerUser
		}
	}
	window := time.Duration(windowSeconds) * time.Second

	// Prefer Redis-backed rate limiting when available
	if rdb != nil {
		ctx := context.Background()
		var exceeded bool

		// Per-IP limiting
		if perIPLimit > 0 {
			ipKey := fmt.Sprintf("rate:ip:%s", ip)
			ipCount, err := rdb.Incr(ctx, ipKey).Result()
			if err == nil {
				if ipCount == 1 {
					_ = rdb.Expire(ctx, ipKey, window).Err()
				}
				if ipCount > int64(perIPLimit) {
					exceeded = true
				}
			} else {
				logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_incr_failed").Warn("redis rate limit per IP failed; falling back to in-memory limiter")
			}
		}

		// Per-user limiting (only if authenticated)
		if !exceeded && username != "" && perUserLimit > 0 {
			userKey := fmt.Sprintf("rate:user:%s", username)
			userCount, err := rdb.Incr(ctx, userKey).Result()
			if err == nil {
				if userCount == 1 {
					_ = rdb.Expire(ctx, userKey, window).Err()
				}
				if userCount > int64(perUserLimit) {
					exceeded = true
				}
			} else {
				logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_incr_failed").Warn("redis rate limit per user failed; falling back to in-memory limiter")
			}
		}

		if exceeded {
			log.WithFields(log.Fields{
				logging.FieldComponent: logging.ComponentRateLimit,
				logging.FieldEvent:     "api_rate_limit",
				logging.FieldIP:        ip,
				logging.FieldUsername:  username,
				"backend":              "redis",
			}).Warn("Rate limit exceeded (Redis)")
			errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
			c.Abort()
			return
		}

		// If Redis calls succeeded and limits not exceeded, proceed.
		if err := rdb.Ping(ctx).Err(); err == nil {
			c.Next()
			return
		}
		// If Redis is unhealthy, fall through to in-memory limiter.
	}

	// In-memory fallback (per IP only, 10 req/min default)
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()
	if reset, ok := rateLimitReset[ip]; ok && now.After(reset) {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(window)
	}
	if _, ok := rateLimitCount[ip]; !ok {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(window)
	}
	rateLimitCount[ip]++
	if rateLimitCount[ip] > perIPLimit {
		log.WithFields(log.Fields{
			logging.FieldComponent: logging.ComponentRateLimit,
			logging.FieldEvent:     "api_rate_limit",
			logging.FieldIP:        ip,
			"backend":              "memory",
		}).Warn("Rate limit exceeded (in-memory)")
		errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		c.Abort()
		return
	}
	c.Next()
}

/* Duplicate authentication handlers removed. The remaining/primary handlers are defined elsewhere in the file. */

// InvalidCachedJSONError is returned when cached []byte exists but cannot be unmarshaled.
type InvalidCachedJSONError struct {
	TxID string
	Err  error
}

func (e *InvalidCachedJSONError) Error() string {
	return fmt.Sprintf("invalid cached JSON for transaction %s: %v", e.TxID, e.Err)
}

func (e *InvalidCachedJSONError) Unwrap() error { return e.Err }

// IsInvalidCachedJSON returns true if err is (or wraps) InvalidCachedJSONError
func IsInvalidCachedJSON(err error) bool {
	var target *InvalidCachedJSONError
	return errors.As(err, &target)
}

func fetchLatestBlocks(n int) ([]map[string]interface{}, error) {
	// Get the latest block height
	networkStatus, err := getNetworkStatus()
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	result, ok := networkStatus["result"].(map[string]interface{})
	if !ok {
		return nil, errors.New("could not parse result from network status")
	}
	latestHeight, ok := result["best_block_height"].(float64)
	if !ok {
		return nil, errors.New("could not parse latest block height")
	}

	blocks := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		height := int(latestHeight) - i
		block, err := getBlockDetails(fmt.Sprintf("%d", height))
		if err != nil {
			continue // skip errors, e.g., for orphaned blocks
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// Fetch the latest N transactions (from the latest N blocks)

// updated main function background job to use rdb client
func main() {
	logging.Configure()

	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		logging.WithComponent(logging.ComponentServer).WithError(err).Fatal("Failed to load configuration")
	}
	appConfig = cfg

	appEnv := cfg.AppEnv
	if appEnv == "development" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
		logging.FieldEvent: "startup",
		logging.FieldEnv:   appEnv,
	}).Info("Application starting")

	// Initialize GetBlock settings from configuration.
	baseURL = cfg.GetBlockBaseURL
	apiKey = cfg.GetBlockAccessToken

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		logging.WithComponent(logging.ComponentRedis).WithError(err).WithField(logging.FieldEvent, "ping_failed").Warn("redis ping failed before startup")
	}

	// Initialize Sentry
	_ = pong
	if cfg.SentryDSN != "" {
		if initErr := sentryutil.Init(cfg); initErr != nil {
			logging.WithComponent(logging.ComponentSentry).WithError(initErr).Fatal("sentry.Init failed")
		}
		logging.WithComponent(logging.ComponentSentry).WithFields(log.Fields{
			"traces_sample_rate": cfg.SentryTracesSampleRate,
			"error_sample_rate":  cfg.SentryErrorSampleRate,
		}).Info("sentry initialized")
	} else {
		logging.WithComponent(logging.ComponentSentry).Warn("sentry disabled: SENTRY_DSN not set")
	}

	r := gin.Default()

	r.Use(sentrygin.New(sentrygin.Options{Repanic: true, Timeout: 2 * time.Second}))
	r.Use(correlationIDMiddleware())
	r.Use(sentryUserScopeMiddleware())
	if cfg.MetricsEnabled {
		r.Use(metrics.Middleware())
	}
	r.Use(rateLimitMiddleware)
	r.Use(csrfMiddleware)

	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: cfg.RedisHost + ":6379",
	})

	// Configure Redis for LRU eviction
	rdb.ConfigSet(ctx, "maxmemory", "100mb")
	rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")

	// Initialize pluggable service clients
	blockchainClient = blockchain.NewGetBlockRPCClient(baseURL, apiKey, httpClient)
	cgClient := pricing.NewCoinGeckoClient(httpClient)
	pricingClient = cgClient
	assetPricer = &pricing.CompositePricer{
		Crypto:    cgClient,
		Commodity: &pricing.StaticCommoditySource{},
		Bond:      &pricing.StaticBondSource{PricePer100: pricing.DefaultBondPrices()},
	}

	// Initialize news service (provider + Redis cache).
	// Symbol endpoint can operate without auth; portfolio endpoint requires auth.
	if cfg.NewsProvider == "" {
		cfg.NewsProvider = "thenewsapi"
	}
	if cfg.NewsProvider == "thenewsapi" && cfg.TheNewsAPIToken != "" {
		prov := &news.TheNewsAPIProvider{
			BaseURL:           cfg.TheNewsAPIBaseURL,
			Token:             cfg.TheNewsAPIToken,
			Client:            httpClient,
			DefaultLanguage:   cfg.TheNewsAPIDefaultLanguage,
			DefaultLocale:     cfg.TheNewsAPIDefaultLocale,
			DefaultCategories: cfg.TheNewsAPIDefaultCategories,
		}
		newsService = &news.Service{
			Provider: prov,
			Cache:    &news.RedisCache{RDB: rdb},
			FreshTTL: time.Duration(cfg.NewsCacheTTLSeconds) * time.Second,
			StaleTTL: time.Duration(cfg.NewsStaleTTLSeconds) * time.Second,
		}
	} else {
		logging.WithComponent(logging.ComponentNews).WithField(logging.FieldProvider, cfg.NewsProvider).Warn("news provider not configured; news endpoints will be unavailable")
	}

	// Initialize email service (SMTP)
	if cfg.EmailProvider == "" {
		cfg.EmailProvider = "smtp"
	}
	if cfg.EmailProvider == "smtp" && strings.TrimSpace(cfg.EmailFrom) != "" && strings.TrimSpace(cfg.SMTPHost) != "" {
		sender := &email.SMTPSender{
			Host:        cfg.SMTPHost,
			Port:        cfg.SMTPPort,
			Username:    cfg.SMTPUsername,
			Password:    cfg.SMTPPassword,
			UseSTARTTLS: cfg.SMTPStartTLS,
			SkipVerify:  cfg.SMTPSkipVerify,
		}
		emailService = email.NewService(sender, email.Address{Email: cfg.EmailFrom, Name: cfg.EmailFromName})
		emailTemplates = email.NewTemplates(cfg.AppBaseURL)
		logging.WithComponent(logging.ComponentEmail).WithFields(log.Fields{
			logging.FieldProvider: sender.Name(),
			"from_configured":     strings.TrimSpace(cfg.EmailFrom) != "",
		}).Info("email service initialized")
	} else {
		logging.WithComponent(logging.ComponentEmail).WithField(logging.FieldProvider, cfg.EmailProvider).Warn("email service not configured; emails disabled")
	}

	// Initialize default admin user
	initializeDefaultAdmin()

	// Serve static assets: images plus specific static files
	r.Static("/images", "./images")
	// Serve built Tailwind CSS
	r.Static("/dist", "./dist")
	r.StaticFile("/bitcoin.html", "bitcoin.html")
	r.StaticFile("/", "index.html")
	r.StaticFile("/admin", "admin.html")
	r.StaticFile("/dashboard", "dashboard.html")
	r.StaticFile("/profile", "profile.html")
	r.StaticFile("/symbols", "symbols.html")

	// Health and readiness endpoints
	r.GET("/healthz", healthHandler)
	r.GET("/readyz", readinessHandler)

	if cfg.MetricsEnabled {
		if cfg.MetricsToken != "" {
			r.GET("/metrics", metrics.TokenAuthMiddleware(cfg.MetricsToken), gin.WrapH(metrics.Handler()))
		} else {
			r.GET("/metrics", gin.WrapH(metrics.Handler()))
		}
	}

	// Versioned API (v1)
	apiV1 := r.Group("/api/v1")
	{
		apiV1.GET("/search", searchHandler)
		apiV1.GET("/search/export", exportSearchHandler)
		apiV1.GET("/search/advanced", advancedSearchHandler)
		apiV1.GET("/search/advanced/export", exportAdvancedSearchHandler)
		apiV1.GET("/search/categories", getSymbolCategoriesHandler)
		apiV1.GET("/blocks/export/csv", exportBlocksCSVHandler)
		apiV1.GET("/transactions/export/csv", exportTransactionsCSVHandler)
		apiV1.GET("/autocomplete", autocompleteHandler)
		apiV1.GET("/metrics", metricsHandler)
		apiV1.GET("/network-status", networkStatusHandler)
		apiV1.GET("/rates", ratesHandler)
		apiV1.GET("/price-history", priceHistoryHandler)
		apiV1.POST("/feedback", feedbackHandler)

		// Public contextual news (by symbol/keyword)
		apiV1.GET("/news/:symbol", newsBySymbolHandler)

		// Authentication routes
		apiV1.POST("/login", loginHandler)
		apiV1.POST("/logout", logoutHandler)
		apiV1.POST("/register", registerHandler)

		// User routes (require authentication)
		userV1 := apiV1.Group("/user")
		userV1.Use(authMiddleware)
		{
			userV1.GET("/profile", userProfileHandler)
			userV1.PATCH("/profile", updateProfileHandler)
			userV1.GET("/notifications", listNotificationsHandler)
			userV1.PUT("/notifications/:id", updateNotificationHandler)
			userV1.DELETE("/notifications/:id", dismissNotificationHandler)
			userV1.GET("/alerts", listPriceAlertsHandler)
			userV1.POST("/alerts", createPriceAlertHandler)
			userV1.PUT("/alerts/:id", updatePriceAlertHandler)
			userV1.DELETE("/alerts/:id", deletePriceAlertHandler)
			userV1.GET("/portfolios", listPortfoliosHandler)
			userV1.GET("/portfolios/export", exportPortfoliosHandler)
			userV1.GET("/portfolios/:id/export/csv", exportPortfolioCSVHandler)
			userV1.GET("/portfolios/:id/export/pdf", exportPortfolioPDFHandler)
			userV1.POST("/portfolios", createPortfolioHandler)
			userV1.PUT("/portfolios/:id", updatePortfolioHandler)
			userV1.DELETE("/portfolios/:id", deletePortfolioHandler)
			userV1.GET("/watchlists", listWatchlistsHandler)
			userV1.GET("/watchlists/:id", getWatchlistHandler)
			userV1.POST("/watchlists", createWatchlistHandler)
			userV1.PUT("/watchlists/:id", updateWatchlistHandler)
			userV1.DELETE("/watchlists/:id", deleteWatchlistHandler)
			userV1.POST("/watchlists/:id/entries", addWatchlistEntryHandler)
			userV1.PUT("/watchlists/:id/entries/:index", updateWatchlistEntryHandler)
			userV1.DELETE("/watchlists/:id/entries/:index", deleteWatchlistEntryHandler)
		}

		// Portfolio news requires auth because portfolio IDs are per-user.
		newsV1 := apiV1.Group("/news")
		newsV1.Use(authMiddleware)
		{
			newsV1.GET("/portfolio/:id", newsByPortfolioHandler)
		}

		// Admin routes (require authentication and admin role)
		adminV1 := apiV1.Group("/admin")
		adminV1.Use(authMiddleware)
		adminV1.Use(requireRoleMiddleware("admin"))
		{
			adminV1.GET("/status", adminStatusHandler)
			adminV1.GET("/cache", adminCacheHandler)
		}
	}

	// Legacy non-versioned API routes for backward compatibility.
	// These may be removed in a future major release.
	r.GET("/api/search", searchHandler)
	r.GET("/api/search/export", exportSearchHandler)
	// Enhanced search with filters and sorting
	r.GET("/api/search/advanced", advancedSearchHandler)
	r.GET("/api/search/advanced/export", exportAdvancedSearchHandler)
	r.GET("/api/search/categories", getSymbolCategoriesHandler)
	r.GET("/api/blocks/export/csv", exportBlocksCSVHandler)
	r.GET("/api/transactions/export/csv", exportTransactionsCSVHandler)

	r.GET("/api/autocomplete", autocompleteHandler)
	r.GET("/api/metrics", metricsHandler)
	r.GET("/api/network-status", networkStatusHandler)
	r.GET("/api/rates", ratesHandler)
	r.GET("/api/price-history", priceHistoryHandler)

	// Public contextual news (by symbol/keyword)
	r.GET("/api/news/:symbol", newsBySymbolHandler)

	r.POST("/api/feedback", feedbackHandler)

	// Authentication routes
	r.POST("/api/login", loginHandler)
	r.POST("/api/logout", logoutHandler)
	r.POST("/api/register", registerHandler)

	// User routes (require authentication)
	user := r.Group("/api/user")
	user.Use(authMiddleware)
	{
		user.GET("/profile", userProfileHandler)
		user.PATCH("/profile", updateProfileHandler)
		user.GET("/notifications", listNotificationsHandler)
		user.PUT("/notifications/:id", updateNotificationHandler)
		user.DELETE("/notifications/:id", dismissNotificationHandler)
		user.GET("/alerts", listPriceAlertsHandler)
		user.POST("/alerts", createPriceAlertHandler)
		user.PUT("/alerts/:id", updatePriceAlertHandler)
		user.DELETE("/alerts/:id", deletePriceAlertHandler)
		user.GET("/portfolios", listPortfoliosHandler)
		user.GET("/portfolios/export", exportPortfoliosHandler)
		user.GET("/portfolios/:id/export/csv", exportPortfolioCSVHandler)
		user.GET("/portfolios/:id/export/pdf", exportPortfolioPDFHandler)
		user.POST("/portfolios", createPortfolioHandler)
		user.PUT("/portfolios/:id", updatePortfolioHandler)
		user.DELETE("/portfolios/:id", deletePortfolioHandler)
		user.GET("/watchlists", listWatchlistsHandler)
		user.GET("/watchlists/:id", getWatchlistHandler)
		user.POST("/watchlists", createWatchlistHandler)
		user.PUT("/watchlists/:id", updateWatchlistHandler)
		user.DELETE("/watchlists/:id", deleteWatchlistHandler)
		user.POST("/watchlists/:id/entries", addWatchlistEntryHandler)
		user.PUT("/watchlists/:id/entries/:index", updateWatchlistEntryHandler)
		user.DELETE("/watchlists/:id/entries/:index", deleteWatchlistEntryHandler)
	}

	// Portfolio news requires auth because portfolio IDs are per-user.
	newsLegacy := r.Group("/api/news")
	newsLegacy.Use(authMiddleware)
	{
		newsLegacy.GET("/portfolio/:id", newsByPortfolioHandler)
	}

	// Admin routes (require authentication and admin role)
	admin := r.Group("/api/admin")
	admin.Use(authMiddleware)
	admin.Use(requireRoleMiddleware("admin"))
	{
		admin.GET("/status", adminStatusHandler)
		admin.GET("/cache", adminCacheHandler)
	}

	r.GET("/bitcoin", func(c *gin.Context) {
		query := c.Query("q")
		c.Redirect(http.StatusFound, "/bitcoin.html?q="+query)
	})

	// Start background job to prefetch latest blocks and transactions
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			// Initial run and every tick
			func() {
				cid := correlation.NewID()
				jobLog := logging.WithComponent(logging.ComponentBackground).WithFields(log.Fields{
					logging.FieldCorrelationID: cid,
					logging.FieldEvent:         "prefetch_tick",
				})
				const numBlocks = 5
				const numTxs = 10
				blocks, blocksErr := fetchLatestBlocks(numBlocks)
				if blocksErr == nil {
					blocksJSON, _ := json.Marshal(blocks)
					rdb.Set(context.Background(), "latest_blocks", blocksJSON, 5*time.Minute)
				} else {
					jobLog.WithError(blocksErr).WithField(logging.FieldEvent, "prefetch_blocks_failed").Error("failed to prefetch latest blocks")
				}
				txs, txsErr := fetchLatestTransactions(numBlocks, numTxs)
				if txsErr == nil {
					txsJSON, _ := json.Marshal(txs)
					rdb.Set(context.Background(), "latest_transactions", txsJSON, 5*time.Minute)
				} else {
					jobLog.WithError(txsErr).WithField(logging.FieldEvent, "prefetch_txs_failed").Error("failed to prefetch latest transactions")
				}
				if blocksErr == nil && txsErr == nil {
					jobLog.WithField(logging.FieldEvent, "prefetch_tick_ok").Debug("prefetch tick completed")
				}
				metrics.RecordPrefetchTick(blocksErr, txsErr)
			}()
			<-ticker.C
		}
	}()

	// Start background job to collect metrics for charts
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			collectMetrics()
			<-ticker.C
		}
	}()

	// Start background job to evaluate price alerts
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			evaluatePriceAlerts()
			<-ticker.C
		}
	}()

	if cfg.SentryDSN != "" {
		defer sentry.Flush(2 * time.Second)
	}

	logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
		logging.FieldEvent: "listen",
		"addr":             ":8080",
	}).Info("HTTP server listening")

	r.Run(":8080")
}

// Fetch the latest N transactions (from the latest N blocks)
func fetchLatestTransactions(nBlocks, nTxs int) ([]map[string]interface{}, error) {
	blocks, err := fetchLatestBlocks(nBlocks)
	if err != nil {
		return nil, err
	}
	transactions := make([]map[string]interface{}, 0, nTxs)
	for _, block := range blocks {
		// Access the nested structure properly
		blockData, ok := block["result"].(map[string]interface{})
		if !ok {
			continue
		}

		txs, ok := blockData["tx"]
		if !ok {
			continue
		}
		txList, ok := txs.([]interface{})
		if !ok {
			continue
		}
		for _, txid := range txList {
			if len(transactions) >= nTxs {
				return transactions, nil
			}
			txDetail, err := getTransactionDetails(fmt.Sprintf("%v", txid))
			if err != nil {
				continue
			}
			transactions = append(transactions, txDetail)
		}
	}
	return transactions, nil
}

// GET /api/news/:symbol and GET /api/v1/news/:symbol
// Public endpoint; returns deduplicated articles for the given symbol/keyword.
func newsBySymbolHandler(c *gin.Context) {
	if newsService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "news_unavailable", "News service is not configured")
		return
	}
	symbol := strings.TrimSpace(c.Param("symbol"))
	if symbol == "" || len(symbol) > 50 {
		errorResponse(c, http.StatusBadRequest, "invalid_symbol", "Symbol must be 1-50 characters")
		return
	}
	symbol = sanitizeText(symbol, 50)

	limit := apiutil.ParsePagination(c, 20, 50).PageSize

	// Build query: caller symbol plus default search (if configured).
	query := buildNewsQueryForSymbol(symbol, appConfig)
	extras := newsExtrasFromConfig(appConfig)
	key := news.CacheKey(newsService.ProviderName(), "news:symbol", query, extras)

	articles, cached, stale, err := newsService.Get(c.Request.Context(), key, query, limit)
	if err != nil {
		// Provider error: we already attempted stale fallback inside service.
		status := http.StatusBadGateway
		if errors.Is(err, news.ErrRateLimited) {
			status = http.StatusTooManyRequests
		}
		errorResponse(c, status, "news_fetch_failed", "Failed to fetch news")
		return
	}
	metrics.RecordNews(cached, stale)

	user := getOptionalAuthenticatedUser(c)
	favoritesOnly := strings.ToLower(strings.TrimSpace(c.Query("favorites_only"))) == "true"
	articles = applyUserNewsPrefs(articles, user, favoritesOnly)

	c.JSON(http.StatusOK, news.ListResponse{
		Data: articles,
		Meta: news.Meta{
			Provider: newsService.ProviderName(),
			Cached:   cached,
			Stale:    stale,
			Query:    query,
		},
	})
}

// GET /api/news/portfolio/:id and GET /api/v1/news/portfolio/:id
// Authenticated endpoint; returns deduplicated articles relevant to the portfolio's assets.
func newsByPortfolioHandler(c *gin.Context) {
	if newsService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "news_unavailable", "News service is not configured")
		return
	}
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	portfolioID := strings.TrimSpace(c.Param("id"))
	if portfolioID == "" || len(portfolioID) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_id", "Portfolio id must be 1-64 characters")
		return
	}

	key := "portfolio:" + username + ":" + portfolioID
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		return
	}
	var p Portfolio
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_decode_failed", "Failed to load portfolio")
		return
	}

	limit := apiutil.ParsePagination(c, 20, 50).PageSize

	query := buildNewsQueryForPortfolio(&p, appConfig)
	if query == "" {
		errorResponse(c, http.StatusBadRequest, "portfolio_empty", "Portfolio has no searchable assets")
		return
	}
	extras := newsExtrasFromConfig(appConfig)
	cacheKey := news.CacheKey(newsService.ProviderName(), "news:portfolio:"+portfolioID, query, extras)

	articles, cached, stale, err := newsService.Get(c.Request.Context(), cacheKey, query, limit)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, news.ErrRateLimited) {
			status = http.StatusTooManyRequests
		}
		errorResponse(c, status, "news_fetch_failed", "Failed to fetch news")
		return
	}
	metrics.RecordNews(cached, stale)

	u := getOptionalAuthenticatedUser(c)
	favoritesOnly := strings.ToLower(strings.TrimSpace(c.Query("favorites_only"))) == "true"
	articles = applyUserNewsPrefs(articles, u, favoritesOnly)

	c.JSON(http.StatusOK, news.ListResponse{
		Data: articles,
		Meta: news.Meta{
			Provider: newsService.ProviderName(),
			Cached:   cached,
			Stale:    stale,
			Query:    query,
		},
	})
}

func buildNewsQueryForSymbol(symbol string, cfg *config.Config) string {
	s := strings.TrimSpace(symbol)
	if s == "" {
		return ""
	}
	// TheNewsAPI query language uses | for OR and quotes for phrases.
	// We keep it simple: symbol or quoted symbol plus optional default search.
	quoted := `"` + strings.ReplaceAll(s, `"`, "") + `"`
	q := "(" + s + " | " + quoted + ")"
	if cfg != nil {
		if base := strings.TrimSpace(cfg.TheNewsAPIDefaultSearch); base != "" {
			q = q + " + (" + base + ")"
		}
	}
	return q
}

func buildNewsQueryForPortfolio(p *Portfolio, cfg *config.Config) string {
	if p == nil || len(p.Items) == 0 {
		return ""
	}
	terms := make([]string, 0, len(p.Items))
	seen := make(map[string]bool, len(p.Items))
	for _, it := range p.Items {
		// Prefer symbol; fall back to label.
		sym := strings.TrimSpace(it.Symbol)
		if sym == "" {
			sym = strings.TrimSpace(it.Label)
		}
		sym = sanitizeText(sym, 50)
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		k := strings.ToLower(sym)
		if seen[k] {
			continue
		}
		seen[k] = true
		terms = append(terms, sym)
		if len(terms) >= 10 {
			break
		}
	}
	if len(terms) == 0 {
		return ""
	}
	// Build OR query: (a | "a" | b | "b" ...)
	var parts []string
	for _, t := range terms {
		clean := strings.ReplaceAll(t, `"`, "")
		parts = append(parts, clean, `"`+clean+`"`)
	}
	q := "(" + strings.Join(parts, " | ") + ")"
	if cfg != nil {
		if base := strings.TrimSpace(cfg.TheNewsAPIDefaultSearch); base != "" {
			q = q + " + (" + base + ")"
		}
	}
	return q
}

func newsExtrasFromConfig(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	out := map[string]string{}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultLanguage); v != "" {
		out["language"] = v
	}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultLocale); v != "" {
		out["locale"] = v
	}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultCategories); v != "" {
		out["categories"] = v
	}
	return out
}

func normalizeNewsSourceList(in []string, maxItems int) ([]string, error) {
	if maxItems <= 0 {
		maxItems = 50
	}
	if len(in) > maxItems {
		return nil, fmt.Errorf("too many sources (max %d)", maxItems)
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		if len(s) > 100 {
			return nil, fmt.Errorf("source too long (max 100 chars)")
		}
		// Basic allowlist for host-like identifiers (domain or subdomain).
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
				continue
			}
			return nil, fmt.Errorf("invalid source %q (use domains like reuters.com)", raw)
		}
		if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.Contains(s, "..") {
			return nil, fmt.Errorf("invalid source %q (use domains like reuters.com)", raw)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	// Keep stable order as provided by user.
	return out, nil
}

func getOptionalAuthenticatedUser(c *gin.Context) *User {
	if c == nil {
		return nil
	}
	// If upstream middleware already set username, use it.
	if v, ok := c.Get("username"); ok {
		if username, ok2 := v.(string); ok2 && username != "" {
			if u, ok3 := getUser(username); ok3 {
				return &u
			}
		}
		return nil
	}
	// Best-effort session cookie lookup; do not abort if missing/invalid.
	sessionID, err := c.Cookie("session_id")
	if err != nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	username, valid := validateSession(sessionID)
	if !valid || strings.TrimSpace(username) == "" {
		return nil
	}
	u, ok := getUser(username)
	if !ok {
		return nil
	}
	return &u
}

func applyUserNewsPrefs(articles []news.Article, user *User, favoritesOnly bool) []news.Article {
	if user == nil || len(articles) == 0 {
		return articles
	}
	blocked := make(map[string]bool, len(user.NewsSourcesBlocked))
	for _, s := range user.NewsSourcesBlocked {
		blocked[strings.ToLower(strings.TrimSpace(s))] = true
	}
	favs := make(map[string]bool, len(user.NewsSourcesFavorite))
	for _, s := range user.NewsSourcesFavorite {
		favs[strings.ToLower(strings.TrimSpace(s))] = true
	}

	filtered := make([]news.Article, 0, len(articles))
	for _, a := range articles {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src != "" && blocked[src] {
			continue
		}
		if favoritesOnly {
			if src == "" || !favs[src] {
				continue
			}
		}
		filtered = append(filtered, a)
	}
	if favoritesOnly || len(favs) == 0 || len(filtered) == 0 {
		return filtered
	}
	// Stable partition: favorites first, then the rest; keep relative order within groups.
	out := make([]news.Article, 0, len(filtered))
	for _, a := range filtered {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src != "" && favs[src] {
			out = append(out, a)
		}
	}
	for _, a := range filtered {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src == "" || !favs[src] {
			out = append(out, a)
		}
	}
	return out
}

// -----------------------------
// Price alerts (Redis-backed)
// -----------------------------

const (
	priceAlertKeyPrefix   = "alert:"
	notificationKeyPrefix = "notification:"
)

type createPriceAlertRequest struct {
	Symbol         string  `json:"symbol"`
	Currency       string  `json:"currency"`
	Threshold      float64 `json:"threshold"`
	Direction      string  `json:"direction"`       // above|below
	DeliveryMethod string  `json:"delivery_method"` // in_app|email
	IsActive       *bool   `json:"is_active,omitempty"`
}

type updatePriceAlertRequest struct {
	Symbol         *string  `json:"symbol,omitempty"`
	Currency       *string  `json:"currency,omitempty"`
	Threshold      *float64 `json:"threshold,omitempty"`
	Direction      *string  `json:"direction,omitempty"`
	DeliveryMethod *string  `json:"delivery_method,omitempty"`
	IsActive       *bool    `json:"is_active,omitempty"`
}

func listPriceAlertsHandler(c *gin.Context) {
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

	alerts := make([]PriceAlert, 0, 32)
	var cursor uint64
	pattern := priceAlertKeyPrefix + username + ":*"
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "alerts_fetch_failed", "Failed to fetch alerts")
			return
		}
		cursor = next
		for _, key := range keys {
			raw, err := rdb.Get(ctx, key).Result()
			if err != nil || raw == "" {
				continue
			}
			var a PriceAlert
			if err := json.Unmarshal([]byte(raw), &a); err == nil {
				alerts = append(alerts, a)
			}
		}
		if cursor == 0 {
			break
		}
	}

	// Newest first
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].Created.After(alerts[j].Created)
	})

	c.JSON(http.StatusOK, gin.H{"data": alerts})
}

func createPriceAlertHandler(c *gin.Context) {
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

	var req createPriceAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	alert, err := buildPriceAlertFromCreate(username, req)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_alert", err.Error())
		return
	}

	key := priceAlertKeyPrefix + username + ":" + alert.ID
	data, _ := json.Marshal(alert)
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "alert_save_failed", "Failed to save alert")
		return
	}
	c.JSON(http.StatusCreated, alert)
}

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
	limit := apiutil.ParsePagination(c, 20, 100).PageSize

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
	if len(items) > limit {
		items = items[:limit]
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
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

func sendAdminCriticalEmail(title, message string) {
	if emailService == nil || emailTemplates == nil || !emailService.Enabled() || appConfig == nil || strings.TrimSpace(appConfig.AdminEmail) == "" {
		return
	}
	subj, textBody, htmlBody := emailTemplates.AdminCritical(email.AdminCriticalData{Title: title, Message: message})
	emailService.Enqueue(email.Message{
		To:      email.Address{Email: appConfig.AdminEmail},
		Subject: subj,
		Text:    textBody,
		HTML:    htmlBody,
		Tags:    map[string]string{"type": "admin_critical"},
	})
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

	user, authenticated := authenticateUser(loginReq.Username, loginReq.Password)
	if !authenticated {
		errorResponse(c, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
		logging.WithComponent(logging.ComponentAuth).WithFields(log.Fields{
			logging.FieldUsername: loginReq.Username,
			logging.FieldEvent:    "login_failed",
		}).Warn("failed login attempt")
		return
	}

	sessionID, err := createSession(loginReq.Username)
	if err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldEvent, "session_create_failed").Error("failed to create session")
		errorResponse(c, http.StatusInternalServerError, "session_creation_failed", "Failed to create session")
		return
	}

	csrfToken, err := createOrUpdateCSRFToken(sessionID)
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
		destroySession(sessionID)
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
	err := createUser(registerReq.Username, registerReq.Password, "user", registerReq.Email)
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
func feedbackHandler(c *gin.Context) {
	var feedbackReq struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&feedbackReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	feedbackReq.Name = strings.TrimSpace(feedbackReq.Name)
	feedbackReq.Email = strings.TrimSpace(feedbackReq.Email)
	feedbackReq.Message = strings.TrimSpace(feedbackReq.Message)

	if len(feedbackReq.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_name", "Name must be at most 100 characters")
		return
	}
	if feedbackReq.Email != "" {
		if len(feedbackReq.Email) > 254 {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Email must be at most 254 characters")
			return
		}
		emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
		if !emailPattern.MatchString(feedbackReq.Email) {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Invalid email format")
			return
		}
	}
	if len(feedbackReq.Message) < 5 || len(feedbackReq.Message) > 1000 {
		errorResponse(c, http.StatusBadRequest, "invalid_message", "Message must be between 5 and 1000 characters")
		return
	}

	// Store feedback in Redis with timestamp
	feedbackKey := fmt.Sprintf("feedback:%d", time.Now().Unix())
	feedbackData := map[string]interface{}{
		"name":      sanitizeText(feedbackReq.Name, 100),
		"email":     feedbackReq.Email,
		"message":   sanitizeText(feedbackReq.Message, 1000),
		"timestamp": time.Now().Format(time.RFC3339),
		"ip":        c.ClientIP(),
	}

	jsonData, err := json.Marshal(feedbackData)
	if err != nil {
		logging.WithComponent(logging.ComponentFeedback).WithError(err).WithField(logging.FieldEvent, "marshal_failed").Error("failed to marshal feedback data")
		errorResponse(c, http.StatusInternalServerError, "feedback_processing_failed", "Failed to process feedback")
		return
	}

	err = rdb.Set(ctx, feedbackKey, jsonData, 30*24*time.Hour).Err() // Store for 30 days
	if err != nil {
		logging.WithComponent(logging.ComponentFeedback).WithError(err).WithField(logging.FieldEvent, "redis_set_failed").Error("failed to store feedback in Redis")
		errorResponse(c, http.StatusInternalServerError, "feedback_save_failed", "Failed to save feedback")
		return
	}

	logging.WithComponent(logging.ComponentFeedback).WithFields(log.Fields{
		logging.FieldEvent: "feedback_stored",
		"message_len":      len(feedbackReq.Message),
		"has_email":        feedbackReq.Email != "",
		"has_name":         feedbackReq.Name != "",
	}).Info("feedback stored")

	c.JSON(http.StatusOK, gin.H{"message": "Thank you for your feedback!"})
}

// adminStatusHandler provides system status for admin
func adminStatusHandler(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	// Get Redis info
	info := rdb.Info(ctx, "memory").Val()

	// Get rate limiting stats
	activeLimits := getActiveRateLimitCount()

	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"user":               username,
		"role":               role,
		"redis_memory":       info,
		"active_rate_limits": activeLimits,
		"timestamp":          time.Now().Unix(),
	})
}

// getActiveRateLimitCount returns the number of active rate limit entries.
// When Redis is available, it counts keys with the "rate:" prefix; otherwise,
// it falls back to the in-memory map size.
func getActiveRateLimitCount() int {
	if rdb != nil {
		ctx := context.Background()
		iter := rdb.Scan(ctx, 0, "rate:*", 0).Iterator()
		count := 0
		for iter.Next(ctx) {
			count++
		}
		if err := iter.Err(); err != nil {
			logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_scan_failed").Warn("failed to scan rate limit keys from Redis")
		}
		return count
	}

	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()
	return len(rateLimitCount)
}

// adminCacheHandler provides cache management for admin
func adminCacheHandler(c *gin.Context) {
	action := c.Query("action")
	username, _ := c.Get("username")

	switch action {
	case "clear":
		// Clear all cache keys
		keys, err := rdb.Keys(ctx, "*").Result()
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "cache_keys_failed", "Failed to get cache keys")
			return
		}

		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}

		logging.WithComponent(logging.ComponentAdmin).WithFields(log.Fields{
			logging.FieldUsername: username,
			logging.FieldEvent:    "cache_cleared",
		}).Info("cache cleared by admin")
		c.JSON(http.StatusOK, gin.H{"message": "Cache cleared successfully", "keys_removed": len(keys)})

	case "stats":
		keys, err := rdb.Keys(ctx, "*").Result()
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "cache_stats_failed", "Failed to get cache stats")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total_keys": len(keys),
			"keys":       keys,
		})

	default:
		errorResponse(c, http.StatusBadRequest, "invalid_action", "Invalid action. Use 'clear' or 'stats'")
	}
}

// Portfolio management handlers

// listPortfoliosHandler returns all portfolios for the authenticated user
func listPortfoliosHandler(c *gin.Context) {
	username, _ := c.Get("username")

	keys, err := rdb.Keys(ctx, "portfolio:"+username.(string)+":*").Result()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to fetch portfolios")
		return
	}

	portfolios := []Portfolio{}
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var p Portfolio
		if err := json.Unmarshal([]byte(data), &p); err == nil {
			portfolios = append(portfolios, p)
		}
	}

	// Apply sorting on created or updated timestamps
	sortParams := apiutil.ParseSort(c, "created", "desc", map[string]bool{
		"created": true,
		"updated": true,
	})

	switch sortParams.Field {
	case "updated":
		// sort by Updated desc/asc
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Updated.Before(portfolios[j+1].Updated)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	default:
		// sort by Created desc/asc
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Created.Before(portfolios[j+1].Created)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	}

	// Apply pagination using shared primitive
	pagination := apiutil.ParsePagination(c, 20, 100)
	total := len(portfolios)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginated := portfolios[start:end]

	// Valuation currency: optional query override, else user's preferred fiat (fallback to usd)
	valuationCurrency := "usd"
	if q := strings.ToLower(strings.TrimSpace(c.Query("currency"))); q != "" && pricing.SupportedFiatCurrencies[q] {
		valuationCurrency = q
	} else if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}

	dataWithValuation := make([]PortfolioWithValuation, 0, len(paginated))
	for i := range paginated {
		p := &paginated[i]
		totalFiat, itemsWithVal := computePortfolioValuation(p, valuationCurrency, usdPerFiat)
		dataWithValuation = append(dataWithValuation, PortfolioWithValuation{
			ID:                p.ID,
			Username:          p.Username,
			Name:              p.Name,
			Description:       p.Description,
			Created:           p.Created,
			Updated:           p.Updated,
			ValuationCurrency: valuationCurrency,
			TotalValueFiat:    totalFiat,
			Items:             itemsWithVal,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dataWithValuation,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
	})
}

// createPortfolioHandler creates a new portfolio
func createPortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")

	var p Portfolio
	if err := c.ShouldBindJSON(&p); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	if p.Name == "" || len(p.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_name", "Portfolio name must be between 1 and 100 characters")
		return
	}
	if len(p.Description) > 500 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_description", "Portfolio description must be at most 500 characters")
		return
	}
	if len(p.Items) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_items", "Portfolio cannot contain more than 100 items")
		return
	}
	for i, item := range p.Items {
		item.Type = strings.TrimSpace(item.Type)
		item.Label = strings.TrimSpace(item.Label)
		item.Address = strings.TrimSpace(item.Address)
		if item.Label == "" || len(item.Label) > 100 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_label", fmt.Sprintf("Item %d label must be between 1 and 100 characters", i+1))
			return
		}
		if item.Address == "" || len(item.Address) > 256 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_address", fmt.Sprintf("Item %d address must be between 1 and 256 characters", i+1))
			return
		}
		switch strings.ToLower(item.Type) {
		case "stock", "crypto", "bond", "commodity":
			// allowed
		default:
			errorResponse(c, http.StatusBadRequest, "invalid_item_type", fmt.Sprintf("Item %d has invalid type", i+1))
			return
		}
		item.Label = sanitizeText(item.Label, 100)
		// Addresses are identifiers; normalize whitespace and strip control chars without HTML-escaping.
		item.Address = sanitizeText(item.Address, 256)
		p.Items[i] = item
	}

	p.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	p.Username = username.(string)
	p.Name = sanitizeText(p.Name, 100)
	p.Description = sanitizeText(p.Description, 500)
	p.Created = time.Now()
	p.Updated = time.Now()

	data, _ := json.Marshal(p)
	err := rdb.Set(ctx, "portfolio:"+p.Username+":"+p.ID, data, 0).Err()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_save_failed", "Failed to save portfolio")
		return
	}

	c.JSON(http.StatusCreated, p)
}

// updatePortfolioHandler updates an existing portfolio
func updatePortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")
	portfolioID := c.Param("id")

	var updateReq Portfolio
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	updateReq.Name = strings.TrimSpace(updateReq.Name)
	updateReq.Description = strings.TrimSpace(updateReq.Description)
	if updateReq.Name == "" || len(updateReq.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_name", "Portfolio name must be between 1 and 100 characters")
		return
	}
	if len(updateReq.Description) > 500 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_description", "Portfolio description must be at most 500 characters")
		return
	}
	if len(updateReq.Items) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_items", "Portfolio cannot contain more than 100 items")
		return
	}
	for i, item := range updateReq.Items {
		item.Type = strings.TrimSpace(item.Type)
		item.Label = strings.TrimSpace(item.Label)
		item.Address = strings.TrimSpace(item.Address)
		if item.Label == "" || len(item.Label) > 100 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_label", fmt.Sprintf("Item %d label must be between 1 and 100 characters", i+1))
			return
		}
		if item.Address == "" || len(item.Address) > 256 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_address", fmt.Sprintf("Item %d address must be between 1 and 256 characters", i+1))
			return
		}
		switch strings.ToLower(item.Type) {
		case "stock", "crypto", "bond", "commodity":
			// allowed
		default:
			errorResponse(c, http.StatusBadRequest, "invalid_item_type", fmt.Sprintf("Item %d has invalid type", i+1))
			return
		}
		item.Label = sanitizeText(item.Label, 100)
		item.Address = sanitizeText(item.Address, 256)
		updateReq.Items[i] = item
	}

	key := "portfolio:" + username.(string) + ":" + portfolioID
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		return
	}

	var p Portfolio
	json.Unmarshal([]byte(data), &p)

	// Update fields
	p.Name = sanitizeText(updateReq.Name, 100)
	p.Description = sanitizeText(updateReq.Description, 500)
	p.Items = updateReq.Items
	p.Updated = time.Now()

	newData, _ := json.Marshal(p)
	rdb.Set(ctx, key, newData, 0)

	c.JSON(http.StatusOK, p)
}

// deletePortfolioHandler deletes a portfolio
func deletePortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")
	portfolioID := c.Param("id")

	key := "portfolio:" + username.(string) + ":" + portfolioID
	err := rdb.Del(ctx, key).Err()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_delete_failed", "Failed to delete portfolio")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Portfolio deleted successfully"})
}

// Redis key convention for watchlists: watchlist:{username}:{id}
// TTL: none (0) — watchlists are persistent user data like portfolios.
const watchlistKeyPrefix = "watchlist:"

func watchlistKey(username, id string) string {
	return watchlistKeyPrefix + username + ":" + id
}

const (
	maxWatchlistsPerUser = 20 // quota: max watchlists per user to avoid unbounded storage
	maxWatchlistEntries  = 100
	maxWatchlistNameLen  = 100
	maxEntrySymbolLen    = 128
	maxEntryAddressLen   = 256
	maxEntryNotesLen     = 500
	maxEntryTags         = 10
	maxTagLen            = 50
	maxEntryGroupLen     = 50
)

func validateWatchlistEntry(i int, e *WatchlistEntry) error {
	e.Type = strings.ToLower(strings.TrimSpace(e.Type))
	if e.Type != "symbol" && e.Type != "address" {
		return fmt.Errorf("entry %d: type must be symbol or address", i+1)
	}
	e.Symbol = strings.TrimSpace(e.Symbol)
	e.Address = strings.TrimSpace(e.Address)
	if e.Type == "symbol" {
		if e.Symbol == "" || len(e.Symbol) > maxEntrySymbolLen {
			return fmt.Errorf("entry %d: symbol must be 1-%d characters", i+1, maxEntrySymbolLen)
		}
		e.Symbol = sanitizeText(e.Symbol, maxEntrySymbolLen)
	} else {
		if e.Address == "" || len(e.Address) > maxEntryAddressLen {
			return fmt.Errorf("entry %d: address must be 1-%d characters", i+1, maxEntryAddressLen)
		}
		e.Address = sanitizeText(e.Address, maxEntryAddressLen)
	}
	e.Notes = sanitizeText(strings.TrimSpace(e.Notes), maxEntryNotesLen)
	if len(e.Tags) > maxEntryTags {
		return fmt.Errorf("entry %d: at most %d tags allowed", i+1, maxEntryTags)
	}
	for j, t := range e.Tags {
		t = strings.TrimSpace(t)
		if len(t) > maxTagLen {
			return fmt.Errorf("entry %d tag %d: tag max %d characters", i+1, j+1, maxTagLen)
		}
		e.Tags[j] = sanitizeText(t, maxTagLen)
	}
	e.Group = strings.TrimSpace(e.Group)
	if len(e.Group) > maxEntryGroupLen {
		return fmt.Errorf("entry %d: group must be at most %d characters", i+1, maxEntryGroupLen)
	}
	e.Group = sanitizeText(e.Group, maxEntryGroupLen)
	return nil
}

// getWatchlistCount returns the number of watchlists for a user (for quota enforcement).
func getWatchlistCount(ctx context.Context, username string) (int, error) {
	if rdb == nil {
		return 0, errors.New("redis unavailable")
	}
	keys, err := rdb.Keys(ctx, watchlistKey(username, "*")).Result()
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// listWatchlistsHandler returns all watchlists for the authenticated user.
func listWatchlistsHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	keys, err := rdb.Keys(ctx, watchlistKey(uname, "*")).Result()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to fetch watchlists")
		return
	}
	watchlists := []Watchlist{}
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var w Watchlist
		if err := json.Unmarshal([]byte(data), &w); err == nil {
			watchlists = append(watchlists, w)
		}
	}
	sort.Slice(watchlists, func(i, j int) bool {
		return watchlists[i].Updated.After(watchlists[j].Updated)
	})
	c.JSON(http.StatusOK, gin.H{"data": watchlists})
}

// createWatchlistHandler creates a new watchlist. Enforces per-user watchlist quota.
func createWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	count, err := getWatchlistCount(ctx, uname)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to check watchlist count")
		return
	}
	if count >= maxWatchlistsPerUser {
		errorResponse(c, http.StatusTooManyRequests, "watchlist_quota_exceeded", fmt.Sprintf("Maximum %d watchlists per user", maxWatchlistsPerUser))
		return
	}
	var w Watchlist
	if err := c.ShouldBindJSON(&w); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	w.Name = strings.TrimSpace(w.Name)
	if len(w.Name) > maxWatchlistNameLen {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_name", "Watchlist name must be at most 100 characters")
		return
	}
	w.Name = sanitizeText(w.Name, maxWatchlistNameLen)
	if len(w.Entries) > maxWatchlistEntries {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_entries", "Watchlist cannot contain more than 100 entries")
		return
	}
	for i := range w.Entries {
		if err := validateWatchlistEntry(i, &w.Entries[i]); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
			return
		}
	}
	w.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	w.Username = uname
	w.Created = time.Now()
	w.Updated = time.Now()
	data, _ := json.Marshal(w)
	key := watchlistKey(uname, w.ID)
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusCreated, w)
}

// getWatchlistHandler returns a single watchlist by ID.
func getWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	key := watchlistKey(uname, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		return
	}
	var w Watchlist
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		return
	}
	c.JSON(http.StatusOK, w)
}

// updateWatchlistHandler updates an existing watchlist. Enforces entry count quota.
func updateWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	var w Watchlist
	if err := c.ShouldBindJSON(&w); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	w.Name = strings.TrimSpace(w.Name)
	if len(w.Name) > maxWatchlistNameLen {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_name", "Watchlist name must be at most 100 characters")
		return
	}
	w.Name = sanitizeText(w.Name, maxWatchlistNameLen)
	if len(w.Entries) > maxWatchlistEntries {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_entries", "Watchlist cannot contain more than 100 entries")
		return
	}
	for i := range w.Entries {
		if err := validateWatchlistEntry(i, &w.Entries[i]); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
			return
		}
	}
	key := watchlistKey(uname, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		return
	}
	var existing Watchlist
	if err := json.Unmarshal([]byte(data), &existing); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		return
	}
	existing.Name = w.Name
	existing.Entries = w.Entries
	existing.Updated = time.Now()
	newData, _ := json.Marshal(existing)
	if err := rdb.Set(ctx, key, newData, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, existing)
}

// deleteWatchlistHandler deletes a watchlist.
func deleteWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	key := watchlistKey(uname, id)
	if err := rdb.Del(ctx, key).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_delete_failed", "Failed to delete watchlist")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Watchlist deleted successfully"})
}

// addWatchlistEntryHandler appends one entry to a watchlist. Enforces per-watchlist entry quota.
func addWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	var e WatchlistEntry
	if err := c.ShouldBindJSON(&e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	if err := validateWatchlistEntry(0, &e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
		return
	}
	key := watchlistKey(uname, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		return
	}
	var w Watchlist
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		return
	}
	if len(w.Entries) >= maxWatchlistEntries {
		errorResponse(c, http.StatusTooManyRequests, "entry_quota_exceeded", fmt.Sprintf("Maximum %d entries per watchlist", maxWatchlistEntries))
		return
	}
	w.Entries = append(w.Entries, e)
	w.Updated = time.Now()
	newData, _ := json.Marshal(w)
	if err := rdb.Set(ctx, key, newData, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusCreated, w)
}

// updateWatchlistEntryHandler updates an entry at the given 0-based index.
func updateWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	indexStr := c.Param("index")
	if id == "" || indexStr == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID and entry index required")
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_index", "Entry index must be a non-negative integer")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	var e WatchlistEntry
	if err := c.ShouldBindJSON(&e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	if err := validateWatchlistEntry(0, &e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
		return
	}
	key := watchlistKey(uname, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		return
	}
	var w Watchlist
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		return
	}
	if index >= len(w.Entries) {
		errorResponse(c, http.StatusNotFound, "entry_not_found", "Entry index out of range")
		return
	}
	w.Entries[index] = e
	w.Updated = time.Now()
	newData, _ := json.Marshal(w)
	if err := rdb.Set(ctx, key, newData, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, w)
}

// deleteWatchlistEntryHandler removes the entry at the given 0-based index.
func deleteWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	indexStr := c.Param("index")
	if id == "" || indexStr == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID and entry index required")
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_index", "Entry index must be a non-negative integer")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	key := watchlistKey(uname, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		return
	}
	var w Watchlist
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		return
	}
	if index >= len(w.Entries) {
		errorResponse(c, http.StatusNotFound, "entry_not_found", "Entry index out of range")
		return
	}
	w.Entries = append(w.Entries[:index], w.Entries[index+1:]...)
	w.Updated = time.Now()
	newData, _ := json.Marshal(w)
	if err := rdb.Set(ctx, key, newData, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, w)
}

// exportPortfolioCSVHandler streams a single portfolio's holdings as CSV.
// Requires authentication. Sets Content-Type and Content-Disposition for browser download.
func exportPortfolioCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")
	portfolioID := c.Param("id")
	key := "portfolio:" + username.(string) + ":" + portfolioID
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		return
	}
	var p Portfolio
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to load portfolio")
		return
	}

	// Safe filename: alphanumeric, dash, underscore only
	var b strings.Builder
	for _, r := range p.Name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ' {
			if r == ' ' {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	safeName := b.String()
	if safeName == "" {
		safeName = "portfolio"
	}
	filename := fmt.Sprintf("portfolio-%s-%s.csv", portfolioID, safeName)
	if len(filename) > 200 {
		filename = "portfolio-" + portfolioID + ".csv"
	}

	if len(p.Items) > 20 {
		logLargeExport(c, "portfolios/:id/export/csv", map[string]interface{}{"portfolio_id": portfolioID, "item_count": len(p.Items)})
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"symbol", "type", "address", "amount", "value_" + valuationCurrency, "portfolio_created", "portfolio_updated"})
	createdStr := p.Created.UTC().Format(time.RFC3339)
	updatedStr := p.Updated.UTC().Format(time.RFC3339)
	for _, item := range p.Items {
		amountStr := strconv.FormatFloat(item.Amount, 'f', -1, 64)
		valueStr := ""
		assetType := strings.ToLower(strings.TrimSpace(item.Type))
		symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
		if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
			valueStr = strconv.FormatFloat(item.Amount*price, 'f', 2, 64)
		}
		_ = w.Write([]string{
			item.Label,
			item.Type,
			item.Address,
			amountStr,
			valueStr,
			createdStr,
			updatedStr,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "csv_write_failed").Error("CSV export write failed")
	}
}

// generatePortfolioPDF writes a short portfolio summary report (overall value, allocations by type, positions table) to w.
// Uses unified asset pricer for crypto, commodity, bond; usdPerFiat converts USD to user fiat when needed.
func generatePortfolioPDF(p *Portfolio, w io.Writer, valuationCurrency string, usdPerFiat float64) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, p.Name, "", 0, "L", false, 0, "")
	pdf.Ln(12)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(0, 6, "Portfolio Report — Generated "+time.Now().UTC().Format("2006-01-02 15:04 MST"), "", 0, "L", false, 0, "")
	pdf.Ln(10)

	// Overall value: quantity and total fiat (from unified pricer)
	var totalQty float64
	for _, item := range p.Items {
		totalQty += item.Amount
	}
	totalFiat := 0.0
	hasAnyRate := false
	if valuationCurrency != "" && assetPricer != nil {
		for _, item := range p.Items {
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
				totalFiat += item.Amount * price
				hasAnyRate = true
			}
		}
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Summary", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 10)
	summaryLine := fmt.Sprintf("Total (quantity): %s  |  Positions: %d  |  Created: %s  |  Updated: %s",
		formatFloat(totalQty), len(p.Items),
		p.Created.UTC().Format("2006-01-02"),
		p.Updated.UTC().Format("2006-01-02"))
	if hasAnyRate && valuationCurrency != "" && totalFiat > 0 {
		summaryLine += fmt.Sprintf("  |  Total value (%s): %s", strings.ToUpper(valuationCurrency), formatFloat(totalFiat))
	}
	pdf.CellFormat(0, 6, summaryLine, "", 0, "L", false, 0, "")
	pdf.Ln(12)

	// Allocations by asset type (by quantity)
	typeAlloc := make(map[string]float64)
	for _, item := range p.Items {
		t := strings.ToLower(strings.TrimSpace(item.Type))
		if t == "" {
			t = "other"
		}
		typeAlloc[t] += item.Amount
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Allocations by asset type", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	colW := []float64{35, 45, 25, 50}
	headers := []string{"Type", "Amount", "%", "Bar"}
	for i, h := range headers {
		pdf.CellFormat(colW[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	totalForPct := totalQty
	if totalForPct == 0 {
		totalForPct = 1
	}
	for _, t := range []string{"crypto", "stock", "bond", "commodity", "other"} {
		amt, ok := typeAlloc[t]
		if !ok || amt == 0 {
			continue
		}
		pct := amt / totalForPct * 100
		pdf.CellFormat(colW[0], 6, t, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], 6, formatFloat(amt), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[2], 6, formatFloat(pct)+"%", "1", 0, "R", false, 0, "")
		barX, barY := pdf.GetX(), pdf.GetY()
		pdf.CellFormat(colW[3], 6, "", "1", 0, "L", false, 0, "")
		barW := (colW[3] - 2) * (pct / 100)
		if barW > 0.5 {
			pdf.Rect(barX+1, barY+0.5, barW, 5, "F")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(8)

	// Positions table: add Value (fiat) column when rate available (unified pricer)
	posColW := []float64{45, 25, 70, 35, 40}
	posHeaders := []string{"Label", "Type", "Address", "Amount", "Value (" + strings.ToUpper(valuationCurrency) + ")"}
	if !hasAnyRate || valuationCurrency == "" {
		posColW = []float64{45, 25, 70, 35}
		posHeaders = []string{"Label", "Type", "Address", "Amount"}
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Positions", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	for i, h := range posHeaders {
		pdf.CellFormat(posColW[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	for _, item := range p.Items {
		label := item.Label
		if len(label) > 28 {
			label = label[:25] + "..."
		}
		addr := item.Address
		if len(addr) > 38 {
			addr = addr[:35] + "..."
		}
		pdf.CellFormat(posColW[0], 6, label, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[1], 6, item.Type, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[2], 6, addr, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[3], 6, formatFloat(item.Amount), "1", 0, "R", false, 0, "")
		if hasAnyRate && valuationCurrency != "" && len(posColW) > 4 {
			valStr := ""
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
				valStr = formatFloat(item.Amount * price)
			}
			pdf.CellFormat(posColW[4], 6, valStr, "1", 0, "R", false, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "I", 8)
	footer := "Performance history is not available in this report."
	if hasAnyRate && valuationCurrency != "" {
		footer = "Values in " + strings.ToUpper(valuationCurrency) + " use current rates (crypto, commodity, bond); missing rate data is shown blank."
	} else {
		footer += " Value above is total quantity (amounts)."
	}
	pdf.CellFormat(0, 5, footer, "", 0, "L", false, 0, "")

	return pdf.Output(w)
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	if f >= 1e6 || (f < 0.0001 && f > 0) {
		return strconv.FormatFloat(f, 'e', 2, 64)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// exportPortfolioPDFHandler generates and streams a portfolio summary report as PDF. Requires authentication.
func exportPortfolioPDFHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")
	portfolioID := c.Param("id")
	key := "portfolio:" + username.(string) + ":" + portfolioID
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		return
	}
	var p Portfolio
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to load portfolio")
		return
	}
	if len(p.Items) > 20 {
		logLargeExport(c, "portfolios/:id/export/pdf", map[string]interface{}{"portfolio_id": portfolioID, "item_count": len(p.Items)})
	}
	var b strings.Builder
	for _, r := range p.Name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ' {
			if r == ' ' {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	safeName := b.String()
	if safeName == "" {
		safeName = "portfolio"
	}
	filename := fmt.Sprintf("portfolio-%s-%s.pdf", portfolioID, safeName)
	if len(filename) > 200 {
		filename = "portfolio-" + portfolioID + ".pdf"
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := generatePortfolioPDF(&p, c.Writer, valuationCurrency, usdPerFiat); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "pdf_export_failed").Error("portfolio PDF export failed")
		errorResponse(c, http.StatusInternalServerError, "pdf_generation_failed", "Failed to generate PDF")
		return
	}
}

const exportVersion = "1.0"

// CSV export limits to prevent abuse and control memory/RPC load.
const (
	maxBlockRangeExport   = 500  // max blocks in one blocks CSV export (end_height - start_height + 1)
	maxBlockRowsExport    = 2000 // max rows for blocks CSV
	maxTxBlockRangeExport = 100  // max block range when exporting transactions (each block may have many txs)
	maxTxRowsExport       = 5000 // max transaction rows per CSV export
	defaultBlockRows      = 500
	defaultTxRows         = 1000
)

// exportBlocksCSVHandler streams blocks in a height range as CSV. Memory-efficient: one block at a time.
// Query params: start_height, end_height (required), limit (optional, default 500, max 2000).
// Range is capped at maxBlockRangeExport blocks.
func exportBlocksCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	startStr := c.Query("start_height")
	endStr := c.Query("end_height")
	if startStr == "" || endStr == "" {
		errorResponse(c, http.StatusBadRequest, "missing_range", "start_height and end_height are required")
		return
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start < 0 || end < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height and end_height must be non-negative integers")
		return
	}
	if start > end {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height must be <= end_height")
		return
	}
	rangeSize := end - start + 1
	if rangeSize > maxBlockRangeExport {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range may not exceed %d blocks", maxBlockRangeExport))
		return
	}
	limit := defaultBlockRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > maxBlockRowsExport {
				n = maxBlockRowsExport
			}
			limit = n
		}
	}

	status, err := getNetworkStatus()
	if err != nil {
		errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "could not get chain height")
		return
	}
	bestF, _ := status["block_height"].(float64)
	best := int(bestF)
	if end > best {
		errorResponse(c, http.StatusBadRequest, "invalid_range", fmt.Sprintf("end_height cannot exceed current chain height %d", best))
		return
	}
	if rangeSize >= 100 || limit >= 1000 {
		logLargeExport(c, "blocks/export/csv", map[string]interface{}{"start_height": start, "end_height": end, "range_size": rangeSize, "limit": limit})
	}
	filename := fmt.Sprintf("blocks-%d-%d.csv", start, end)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"height", "hash", "time", "time_iso", "tx_count", "size", "weight", "difficulty", "confirmations"})
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		height := int(float64OrZero(block["height"]))
		if height == 0 {
			height = h
		}
		hash := stringOrEmpty(block["hash"])
		timeVal := float64OrZero(block["time"])
		tm := time.Unix(int64(timeVal), 0).UTC()
		txCount := 0
		if txs, ok := block["tx"].([]interface{}); ok {
			txCount = len(txs)
		}
		size := float64OrZero(block["size"])
		weight := float64OrZero(block["weight"])
		difficulty := float64OrZero(block["difficulty"])
		confs := best - height + 1
		if confs < 0 {
			confs = 0
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", height),
			hash,
			fmt.Sprintf("%.0f", timeVal),
			tm.Format(time.RFC3339),
			fmt.Sprintf("%d", txCount),
			fmt.Sprintf("%.0f", size),
			fmt.Sprintf("%.0f", weight),
			fmt.Sprintf("%.0f", difficulty),
			fmt.Sprintf("%d", confs),
		})
		written++
	}
	w.Flush()
	if w.Error() != nil {
		logging.WithComponent(logging.ComponentExport).WithError(w.Error()).WithField(logging.FieldEvent, "csv_write_failed").Error("blocks CSV export write failed")
	}
}

// exportTransactionsCSVHandler streams transactions from blocks in a height range as CSV.
// One block at a time, then one tx at a time per block. Query params: start_height, end_height (required), limit (optional, default 1000, max 5000).
// Uses stricter (heavy) export rate limit due to RPC load.
func exportTransactionsCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, true) {
		return
	}
	startStr := c.Query("start_height")
	endStr := c.Query("end_height")
	if startStr == "" || endStr == "" {
		errorResponse(c, http.StatusBadRequest, "missing_range", "start_height and end_height are required")
		return
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start < 0 || end < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height and end_height must be non-negative integers")
		return
	}
	if start > end {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height must be <= end_height")
		return
	}
	rangeSize := end - start + 1
	if rangeSize > maxTxBlockRangeExport {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range for transaction export may not exceed %d blocks", maxTxBlockRangeExport))
		return
	}
	limit := defaultTxRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > maxTxRowsExport {
				n = maxTxRowsExport
			}
			limit = n
		}
	}

	status, err := getNetworkStatus()
	if err != nil {
		errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "could not get chain height")
		return
	}
	bestF, _ := status["block_height"].(float64)
	best := int(bestF)
	if end > best {
		errorResponse(c, http.StatusBadRequest, "invalid_range", fmt.Sprintf("end_height cannot exceed current chain height %d", best))
		return
	}
	if rangeSize >= 50 || limit >= 2000 {
		logLargeExport(c, "transactions/export/csv", map[string]interface{}{"start_height": start, "end_height": end, "range_size": rangeSize, "limit": limit})
	}
	filename := fmt.Sprintf("transactions-%d-%d.csv", start, end)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"txid", "block_height", "block_hash", "block_time", "block_time_iso", "size", "vsize", "weight", "fee", "locktime", "version"})
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		blockHash := stringOrEmpty(block["hash"])
		blockTime := float64OrZero(block["time"])
		blockTimeISO := time.Unix(int64(blockTime), 0).UTC().Format(time.RFC3339)
		txList, ok := block["tx"].([]interface{})
		if !ok {
			continue
		}
		for _, txi := range txList {
			if written >= limit {
				break
			}
			txid, _ := txi.(string)
			if txid == "" {
				continue
			}
			tx, err := getTransactionDetails(txid)
			if err != nil {
				continue
			}
			size := float64OrZero(tx["size"])
			vsize := float64OrZero(tx["vsize"])
			weight := float64OrZero(tx["weight"])
			fee := float64OrZero(tx["fee"])
			locktime := float64OrZero(tx["locktime"])
			version := float64OrZero(tx["version"])
			_ = w.Write([]string{
				txid,
				fmt.Sprintf("%d", h),
				blockHash,
				fmt.Sprintf("%.0f", blockTime),
				blockTimeISO,
				fmt.Sprintf("%.0f", size),
				fmt.Sprintf("%.0f", vsize),
				fmt.Sprintf("%.0f", weight),
				fmt.Sprintf("%.6f", fee),
				fmt.Sprintf("%.0f", locktime),
				fmt.Sprintf("%.0f", version),
			})
			written++
		}
	}
	w.Flush()
	if w.Error() != nil {
		logging.WithComponent(logging.ComponentExport).WithError(w.Error()).WithField(logging.FieldEvent, "csv_write_failed").Error("transactions CSV export write failed")
	}
}

func float64OrZero(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func stringOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// exportPortfoliosHandler returns portfolios as machine-friendly JSON for archival or analysis.
// Requires authentication. Respects pagination (page, page_size) and sort (sort_by, sort_dir).
func exportPortfoliosHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")

	keys, err := rdb.Keys(ctx, "portfolio:"+username.(string)+":*").Result()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to fetch portfolios")
		return
	}

	portfolios := []Portfolio{}
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var p Portfolio
		if err := json.Unmarshal([]byte(data), &p); err == nil {
			portfolios = append(portfolios, p)
		}
	}

	sortParams := apiutil.ParseSort(c, "created", "desc", map[string]bool{
		"created": true,
		"updated": true,
	})

	switch sortParams.Field {
	case "updated":
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Updated.Before(portfolios[j+1].Updated)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	default:
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Created.Before(portfolios[j+1].Created)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	}

	pagination := apiutil.ParsePagination(c, 20, 100)
	total := len(portfolios)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginated := portfolios[start:end]
	if total >= 50 || pagination.PageSize >= 50 {
		logLargeExport(c, "portfolios/export", map[string]interface{}{"total": total, "page_size": pagination.PageSize})
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}
	dataWithValuation := make([]PortfolioWithValuation, 0, len(paginated))
	for i := range paginated {
		p := &paginated[i]
		totalFiat, itemsWithVal := computePortfolioValuation(p, valuationCurrency, usdPerFiat)
		dataWithValuation = append(dataWithValuation, PortfolioWithValuation{
			ID:                p.ID,
			Username:          p.Username,
			Name:              p.Name,
			Description:       p.Description,
			Created:           p.Created,
			Updated:           p.Updated,
			ValuationCurrency: valuationCurrency,
			TotalValueFiat:    totalFiat,
			Items:             itemsWithVal,
		})
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp":    time.Now().UTC().Format(time.RFC3339),
			"export_version":      exportVersion,
			"endpoint":            "portfolios",
			"valuation_currency":  valuationCurrency,
			"rate_data_available": assetPricer != nil,
		},
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"data": dataWithValuation,
	})
}

func searchBlockchain(query string) (string, map[string]interface{}, error) {
	query = strings.TrimSpace(query)

	// Check if it's a valid Bitcoin address
	if isValidAddress(query) {
		addressDetails, err := getAddressDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "address", addressDetails, nil
	}

	// Check if it's a valid transaction ID
	if isValidTransactionID(query) {
		transactionDetails, err := getTransactionDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "transaction", transactionDetails, nil
	}

	// Check if it's a valid block height
	if isValidBlockHeight(query) {
		blockDetails, err := getBlockDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "block", blockDetails, nil
	}

	// Check if it might be a block hash (64-char hex, but not a tx ID)
	if len(query) == 64 {
		matched, _ := regexp.MatchString("^[0-9a-fA-F]{64}$", query)
		if matched {
			blockDetails, err := getBlockDetails(query)
			if err == nil {
				return "block", blockDetails, nil
			}
		}
	}

	return "", nil, ErrNotFound
}

// Fix 1: Refactor searchHandler to use Gin's context
func searchHandler(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	qf := logging.QueryLogFields(query)
	if query != "" {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_request").Debug("search request received")
	}
	if query == "" {
		logging.WithComponent(logging.ComponentSearch).WithField(logging.FieldEvent, "search_empty_query").Warn("search request with empty query")
		errorResponse(c, http.StatusBadRequest, "missing_query", "Missing query parameter")
		return
	}
	if len(query) > 100 {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_query_too_long").Warn("search request query too long")
		errorResponse(c, http.StatusBadRequest, "query_too_long", "Query too long")
		return
	}
	resultType, result, err := searchBlockchain(query)
	if err != nil {
		logging.WithComponent(logging.ComponentSearch).WithError(err).WithFields(qf).WithField(logging.FieldEvent, "search_failed").Error("search failed")
		if err == ErrNotFound {
			errorResponse(c, http.StatusNotFound, "not_found", "Not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	// Marshal the result to JSON for ETag calculation
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		logging.WithComponent(logging.ComponentSearch).WithError(err).WithFields(qf).WithField(logging.FieldEvent, "marshal_failed").Error("failed to marshal search response")
		errorResponse(c, http.StatusInternalServerError, "marshal_failed", "Failed to marshal response")
		return
	}
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(jsonBytes))
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60")
	if match := c.GetHeader("If-None-Match"); match == etag {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_cache_hit").Debug("search cache hit")
		metrics.RecordSearchETag(true)
		c.Status(304)
		return
	}
	metrics.RecordSearchETag(false)
	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithFields(log.Fields{
		logging.FieldResult: resultType,
		logging.FieldEvent:  "search_success",
	}).Info("search completed")
	c.JSON(200, gin.H{"type": resultType, "result": result})
}

// exportSearchHandler returns blockchain search results as machine-friendly JSON for archival or analysis.
// Same parameters as GET /api/search (q required). Public endpoint; no auth required.
func exportSearchHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		errorResponse(c, http.StatusBadRequest, "missing_query", "Missing query parameter")
		return
	}
	if len(query) > 100 {
		errorResponse(c, http.StatusBadRequest, "query_too_long", "Query too long")
		return
	}
	resultType, result, err := searchBlockchain(query)
	if err != nil {
		if err == ErrNotFound {
			errorResponse(c, http.StatusNotFound, "not_found", "Not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp": time.Now().UTC().Format(time.RFC3339),
			"export_version":   exportVersion,
			"endpoint":         "search",
			"query":            query,
		},
		"data": gin.H{
			"type":   resultType,
			"result": result,
		},
	})
}

func autocompleteHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, gin.H{"suggestions": []map[string]string{}})
		return
	}

	suggestions := []map[string]string{}

	// Check if query looks like an address
	if isValidAddress(query) {
		suggestions = append(suggestions, map[string]string{"type": "address", "value": query, "label": query})
	}

	// Check if query looks like a transaction ID
	if isValidTransactionID(query) {
		suggestions = append(suggestions, map[string]string{"type": "tx", "value": query, "label": query})
	}

	// Check if query looks like a block height
	if isValidBlockHeight(query) {
		suggestions = append(suggestions, map[string]string{"type": "block", "value": query, "label": "Block " + query})
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// SymbolInfo represents a cryptocurrency or asset symbol
type SymbolInfo struct {
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`     // "crypto", "stock", "commodity", etc.
	Category    string  `json:"category"` // e.g., "layer1", "defi", "nft", "payment"
	MarketCap   float64 `json:"market_cap"`
	Price       float64 `json:"price"`
	Volume24h   float64 `json:"volume_24h"`
	Change24h   float64 `json:"change_24h"` // percentage change
	Rank        int     `json:"rank"`
	IsActive    bool    `json:"is_active"`
	ListedSince int64   `json:"listed_since"` // timestamp
}

// SearchFilters represents the filter parameters for symbol search
type SearchFilters struct {
	Types        []string `json:"types"`      // Filter by symbol types
	Categories   []string `json:"categories"` // Filter by categories
	MinPrice     float64  `json:"min_price"`  // Minimum price filter
	MaxPrice     float64  `json:"max_price"`  // Maximum price filter
	MinMarketCap float64  `json:"min_market_cap"`
	MaxMarketCap float64  `json:"max_market_cap"`
	IsActive     *bool    `json:"is_active"` // Filter by active status
}

// SortOptions represents sorting configuration
type SortOptions struct {
	Field     string `json:"field"`     // Field to sort by
	Direction string `json:"direction"` // "asc" or "desc"
}

// In-memory symbol database (in production, this would be a real database)
var (
	symbolDatabase = []SymbolInfo{
		{Symbol: "BTC", Name: "Bitcoin", Type: "crypto", Category: "layer1", MarketCap: 850000000000, Price: 45000.00, Volume24h: 25000000000, Change24h: 2.5, Rank: 1, IsActive: true, ListedSince: 1279408155},
		{Symbol: "ETH", Name: "Ethereum", Type: "crypto", Category: "layer1", MarketCap: 280000000000, Price: 2300.00, Volume24h: 15000000000, Change24h: -1.2, Rank: 2, IsActive: true, ListedSince: 1438905600},
		{Symbol: "USDT", Name: "Tether", Type: "crypto", Category: "stablecoin", MarketCap: 95000000000, Price: 1.00, Volume24h: 45000000000, Change24h: 0.01, Rank: 3, IsActive: true, ListedSince: 1420070400},
		{Symbol: "BNB", Name: "BNB", Type: "crypto", Category: "exchange", MarketCap: 45000000000, Price: 320.00, Volume24h: 1200000000, Change24h: 0.8, Rank: 4, IsActive: true, ListedSince: 1502928000},
		{Symbol: "SOL", Name: "Solana", Type: "crypto", Category: "layer1", MarketCap: 42000000000, Price: 98.00, Volume24h: 2500000000, Change24h: 5.2, Rank: 5, IsActive: true, ListedSince: 1584403200},
		{Symbol: "XRP", Name: "XRP", Type: "crypto", Category: "payment", MarketCap: 28000000000, Price: 0.52, Volume24h: 1800000000, Change24h: -0.5, Rank: 6, IsActive: true, ListedSince: 1386547200},
		{Symbol: "USDC", Name: "USD Coin", Type: "crypto", Category: "stablecoin", MarketCap: 26000000000, Price: 1.00, Volume24h: 8000000000, Change24h: 0.0, Rank: 7, IsActive: true, ListedSince: 1538352000},
		{Symbol: "ADA", Name: "Cardano", Type: "crypto", Category: "layer1", MarketCap: 15000000000, Price: 0.42, Volume24h: 450000000, Change24h: 1.8, Rank: 8, IsActive: true, ListedSince: 1506816000},
		{Symbol: "AVAX", Name: "Avalanche", Type: "crypto", Category: "layer1", MarketCap: 12000000000, Price: 32.00, Volume24h: 600000000, Change24h: 3.1, Rank: 9, IsActive: true, ListedSince: 1609459200},
		{Symbol: "DOGE", Name: "Dogecoin", Type: "crypto", Category: "meme", MarketCap: 11000000000, Price: 0.08, Volume24h: 900000000, Change24h: -2.1, Rank: 10, IsActive: true, ListedSince: 1388966400},
		{Symbol: "LINK", Name: "Chainlink", Type: "crypto", Category: "defi", MarketCap: 8500000000, Price: 14.50, Volume24h: 400000000, Change24h: 2.8, Rank: 11, IsActive: true, ListedSince: 1509494400},
		{Symbol: "UNI", Name: "Uniswap", Type: "crypto", Category: "defi", MarketCap: 5200000000, Price: 6.80, Volume24h: 250000000, Change24h: 4.2, Rank: 12, IsActive: true, ListedSince: 1600041600},
		{Symbol: "AAVE", Name: "Aave", Type: "crypto", Category: "defi", MarketCap: 1800000000, Price: 120.00, Volume24h: 120000000, Change24h: 1.5, Rank: 13, IsActive: true, ListedSince: 1609459200},
		{Symbol: "SUSHI", Name: "SushiSwap", Type: "crypto", Category: "defi", MarketCap: 450000000, Price: 1.80, Volume24h: 45000000, Change24h: -1.8, Rank: 14, IsActive: true, ListedSince: 1598918400},
		{Symbol: "COMP", Name: "Compound", Type: "crypto", Category: "defi", MarketCap: 380000000, Price: 52.00, Volume24h: 28000000, Change24h: 0.9, Rank: 15, IsActive: true, ListedSince: 1592179200},
		{Symbol: "MKR", Name: "Maker", Type: "crypto", Category: "defi", MarketCap: 1600000000, Price: 1750.00, Volume24h: 85000000, Change24h: -0.7, Rank: 16, IsActive: true, ListedSince: 1514764800},
		{Symbol: "YFI", Name: "yearn.finance", Type: "crypto", Category: "defi", MarketCap: 220000000, Price: 6600.00, Volume24h: 18000000, Change24h: 3.5, Rank: 17, IsActive: true, ListedSince: 1598918400},
		{Symbol: "SNX", Name: "Synthetix", Type: "crypto", Category: "defi", MarketCap: 650000000, Price: 2.10, Volume24h: 35000000, Change24h: -2.3, Rank: 18, IsActive: true, ListedSince: 1567296000},
		{Symbol: "CRV", Name: "Curve DAO Token", Type: "crypto", Category: "defi", MarketCap: 480000000, Price: 0.55, Volume24h: 42000000, Change24h: 1.2, Rank: 19, IsActive: true, ListedSince: 1593561600},
		{Symbol: "BAL", Name: "Balancer", Type: "crypto", Category: "defi", MarketCap: 180000000, Price: 3.20, Volume24h: 12000000, Change24h: -0.5, Rank: 20, IsActive: true, ListedSince: 1590969600},
	}
	symbolDBMutex sync.RWMutex
)

// parseSearchFilters parses filter parameters from the request
func parseSearchFilters(c *gin.Context) SearchFilters {
	filters := SearchFilters{}

	// Parse types filter (comma-separated)
	if types := c.Query("types"); types != "" {
		filters.Types = strings.Split(types, ",")
	}

	// Parse categories filter (comma-separated)
	if categories := c.Query("categories"); categories != "" {
		filters.Categories = strings.Split(categories, ",")
	}

	// Parse price range
	if minPrice := c.Query("min_price"); minPrice != "" {
		filters.MinPrice, _ = strconv.ParseFloat(minPrice, 64)
	}
	if maxPrice := c.Query("max_price"); maxPrice != "" {
		filters.MaxPrice, _ = strconv.ParseFloat(maxPrice, 64)
	}

	// Parse market cap range
	if minCap := c.Query("min_market_cap"); minCap != "" {
		filters.MinMarketCap, _ = strconv.ParseFloat(minCap, 64)
	}
	if maxCap := c.Query("max_market_cap"); maxCap != "" {
		filters.MaxMarketCap, _ = strconv.ParseFloat(maxCap, 64)
	}

	// Parse active status
	if active := c.Query("is_active"); active != "" {
		isActive := active == "true"
		filters.IsActive = &isActive
	}

	return filters
}

// parseSortOptions parses sorting parameters from the request
func parseSortOptions(c *gin.Context) SortOptions {
	validFields := map[string]bool{
		"symbol": true, "name": true, "type": true, "category": true,
		"market_cap": true, "price": true, "volume_24h": true,
		"change_24h": true, "rank": true, "listed_since": true,
	}
	s := apiutil.ParseSort(c, "rank", "asc", validFields)
	return SortOptions{
		Field:     s.Field,
		Direction: s.Direction,
	}
}

// matchesFilters checks if a symbol matches the given filters
func matchesFilters(symbol SymbolInfo, filters SearchFilters) bool {
	// Type filter
	if len(filters.Types) > 0 {
		found := false
		for _, t := range filters.Types {
			if strings.EqualFold(symbol.Type, t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Category filter
	if len(filters.Categories) > 0 {
		found := false
		for _, c := range filters.Categories {
			if strings.EqualFold(symbol.Category, c) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Price range filter
	if filters.MinPrice > 0 && symbol.Price < filters.MinPrice {
		return false
	}
	if filters.MaxPrice > 0 && symbol.Price > filters.MaxPrice {
		return false
	}

	// Market cap range filter
	if filters.MinMarketCap > 0 && symbol.MarketCap < filters.MinMarketCap {
		return false
	}
	if filters.MaxMarketCap > 0 && symbol.MarketCap > filters.MaxMarketCap {
		return false
	}

	// Active status filter
	if filters.IsActive != nil && symbol.IsActive != *filters.IsActive {
		return false
	}

	return true
}

// sortSymbols sorts the symbols based on the given options
func sortSymbols(symbols []SymbolInfo, sort SortOptions) {
	less := func(i, j int) bool {
		var result bool
		switch sort.Field {
		case "symbol":
			result = symbols[i].Symbol < symbols[j].Symbol
		case "name":
			result = symbols[i].Name < symbols[j].Name
		case "type":
			result = symbols[i].Type < symbols[j].Type
		case "category":
			result = symbols[i].Category < symbols[j].Category
		case "market_cap":
			result = symbols[i].MarketCap < symbols[j].MarketCap
		case "price":
			result = symbols[i].Price < symbols[j].Price
		case "volume_24h":
			result = symbols[i].Volume24h < symbols[j].Volume24h
		case "change_24h":
			result = symbols[i].Change24h < symbols[j].Change24h
		case "rank":
			result = symbols[i].Rank < symbols[j].Rank
		case "listed_since":
			result = symbols[i].ListedSince < symbols[j].ListedSince
		default:
			result = symbols[i].Rank < symbols[j].Rank
		}

		if sort.Direction == "desc" {
			return !result
		}
		return result
	}

	// Simple bubble sort for simplicity (for larger datasets, use sort.Slice)
	n := len(symbols)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if !less(j, j+1) {
				symbols[j], symbols[j+1] = symbols[j+1], symbols[j]
			}
		}
	}
}

// advancedSearchHandler handles advanced symbol search with filters and sorting
func advancedSearchHandler(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	qf := logging.QueryLogFields(query)
	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "advanced_search_request").Debug("advanced search request received")

	// Parse pagination (reused primitive)
	pagination := apiutil.ParsePagination(c, 20, 100)

	// Parse filters and sort options
	filters := parseSearchFilters(c)
	sort := parseSortOptions(c)

	// Search and filter symbols
	symbolDBMutex.RLock()
	var results []SymbolInfo
	for _, symbol := range symbolDatabase {
		// Text search on symbol or name
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(symbol.Symbol), queryLower) &&
				!strings.Contains(strings.ToLower(symbol.Name), queryLower) {
				continue
			}
		}

		// Apply filters
		if matchesFilters(symbol, filters) {
			results = append(results, symbol)
		}
	}
	symbolDBMutex.RUnlock()

	// Sort results
	sortSymbols(results, sort)

	// Pagination
	total := len(results)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginatedResults := results[start:end]

	// Get available filter options
	availableTypes := make(map[string]bool)
	availableCategories := make(map[string]bool)
	symbolDBMutex.RLock()
	for _, symbol := range symbolDatabase {
		availableTypes[symbol.Type] = true
		availableCategories[symbol.Category] = true
	}
	symbolDBMutex.RUnlock()

	typeList := make([]string, 0, len(availableTypes))
	for t := range availableTypes {
		typeList = append(typeList, t)
	}

	categoryList := make([]string, 0, len(availableCategories))
	for c := range availableCategories {
		categoryList = append(categoryList, c)
	}

	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithFields(log.Fields{
		logging.FieldEvent: "advanced_search_completed",
		"result_count":     len(paginatedResults),
		"total":            total,
		"page":             pagination.Page,
		"sort_by":          sort.Field,
		"sort_dir":         sort.Direction,
	}).Info("advanced search completed")

	c.JSON(http.StatusOK, gin.H{
		"data": paginatedResults,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"filters_applied": gin.H{
			"types":          filters.Types,
			"categories":     filters.Categories,
			"min_price":      filters.MinPrice,
			"max_price":      filters.MaxPrice,
			"min_market_cap": filters.MinMarketCap,
			"max_market_cap": filters.MaxMarketCap,
		},
		"sort_applied": gin.H{
			"field":     sort.Field,
			"direction": sort.Direction,
		},
		"available_filters": gin.H{
			"types":      typeList,
			"categories": categoryList,
		},
	})
}

// exportAdvancedSearchHandler returns advanced symbol search results as machine-friendly JSON for archival or analysis.
// Same parameters as GET /api/search/advanced (q, types, categories, min_price, max_price, page, page_size, sort_by, sort_dir).
// Public endpoint; no auth required.
func exportAdvancedSearchHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	pagination := apiutil.ParsePagination(c, 20, 100)
	filters := parseSearchFilters(c)
	sort := parseSortOptions(c)

	symbolDBMutex.RLock()
	var results []SymbolInfo
	for _, symbol := range symbolDatabase {
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(symbol.Symbol), queryLower) &&
				!strings.Contains(strings.ToLower(symbol.Name), queryLower) {
				continue
			}
		}
		if matchesFilters(symbol, filters) {
			results = append(results, symbol)
		}
	}
	symbolDBMutex.RUnlock()

	sortSymbols(results, sort)

	total := len(results)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginatedResults := results[start:end]
	if total >= 50 || pagination.PageSize >= 50 {
		logLargeExport(c, "search/advanced/export", map[string]interface{}{"total": total, "page_size": pagination.PageSize, "query": query})
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp": time.Now().UTC().Format(time.RFC3339),
			"export_version":   exportVersion,
			"endpoint":         "search/advanced",
			"query":            query,
			"filters_applied": gin.H{
				"types":          filters.Types,
				"categories":     filters.Categories,
				"min_price":      filters.MinPrice,
				"max_price":      filters.MaxPrice,
				"min_market_cap": filters.MinMarketCap,
				"max_market_cap": filters.MaxMarketCap,
			},
			"sort_applied": gin.H{
				"field":     sort.Field,
				"direction": sort.Direction,
			},
		},
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"data": paginatedResults,
	})
}

// getSymbolCategoriesHandler returns available symbol categories
func getSymbolCategoriesHandler(c *gin.Context) {
	categories := map[string][]string{
		"types":      {"crypto", "stock", "commodity", "forex"},
		"categories": {"layer1", "layer2", "defi", "nft", "stablecoin", "payment", "exchange", "meme", "privacy", "infrastructure"},
	}
	c.JSON(http.StatusOK, categories)
}

func metricsHandler(c *gin.Context) {
	if rdb == nil {
		c.JSON(http.StatusOK, gin.H{"mempool_size": []map[string]interface{}{}, "block_times": []map[string]interface{}{}, "tx_volume": []map[string]interface{}{}})
		return
	}
	mempoolData, _ := rdb.ZRangeWithScores(ctx, "mempool_size", -100, -1).Result()
	blockTimeData, _ := rdb.ZRangeWithScores(ctx, "block_times", -100, -1).Result()
	txVolumeData, _ := rdb.ZRangeWithScores(ctx, "tx_volume", -100, -1).Result()

	// Convert to chart-friendly format
	mempool := []map[string]interface{}{}
	for _, z := range mempoolData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		mempool = append(mempool, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	blockTimes := []map[string]interface{}{}
	for _, z := range blockTimeData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		blockTimes = append(blockTimes, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	txVolumes := []map[string]interface{}{}
	for _, z := range txVolumeData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		txVolumes = append(txVolumes, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"mempool_size": mempool,
		"block_times":  blockTimes,
		"tx_volume":    txVolumes,
	})
}

func networkStatusHandler(c *gin.Context) {
	data, err := getNetworkStatus()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, data)
}

// Default and max points for price history (5-min interval: 288 = 24h, 8640 = 30d).
const priceHistoryDefaultPoints = 288
const priceHistoryMaxPoints = 8640

func priceHistoryHandler(c *gin.Context) {
	if rdb == nil {
		handleError(c, errors.New("redis not available"), http.StatusInternalServerError)
		return
	}

	currency := strings.ToLower(strings.TrimSpace(c.DefaultQuery("currency", "usd")))
	if !pricing.SupportedFiatCurrencies[currency] {
		currency = "usd"
	}
	limit := priceHistoryDefaultPoints
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > priceHistoryMaxPoints {
				n = priceHistoryMaxPoints
			}
			limit = n
		}
	}

	key := btcPriceHistoryKey(currency)
	if currency == "usd" {
		// Prefer legacy key if per-currency key is empty (backfill)
		n, _ := rdb.ZCard(ctx, key).Result()
		if n == 0 {
			key = "btc_price_history"
		}
	}

	history, err := rdb.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	type PricePoint struct {
		Timestamp int64   `json:"timestamp"`
		Price     float64 `json:"price"`
	}

	result := make([]PricePoint, 0, len(history))
	for _, z := range history {
		priceStr, ok := z.Member.(string)
		if !ok {
			continue
		}
		price, _ := strconv.ParseFloat(priceStr, 64)
		result = append(result, PricePoint{
			Timestamp: int64(z.Score),
			Price:     price,
		})
	}

	c.Header("X-Price-History-Currency", currency)
	c.JSON(http.StatusOK, result)
}

// Redis key for BTC multi-currency rates. TTL set by RATES_CACHE_TTL_SECONDS (default 60s).
const ratesRedisKeyBTC = "rates:btc"

// ratesCacheTTL returns the TTL for rates cache from config or default 60s.
func ratesCacheTTL() time.Duration {
	if appConfig != nil && appConfig.RatesCacheTTLSeconds > 0 {
		return time.Duration(appConfig.RatesCacheTTLSeconds) * time.Second
	}
	return 60 * time.Second
}

func ratesHandler(c *gin.Context) {
	ctx := context.Background()

	// Optional: filter by user-requested fiat currencies (comma-separated, e.g. currency=usd,eur)
	var wantCurrencies []string
	if q := strings.TrimSpace(c.Query("currency")); q != "" {
		for _, code := range strings.Split(q, ",") {
			code = strings.ToLower(strings.TrimSpace(code))
			if code != "" && pricing.SupportedFiatCurrencies[code] {
				wantCurrencies = append(wantCurrencies, code)
			}
		}
	}

	// Try cache first (we store the default set under rates:btc)
	if rdb != nil {
		cached, err := rdb.Get(ctx, ratesRedisKeyBTC).Result()
		if err == nil && cached != "" {
			var data map[string]interface{}
			if unmarshalErr := json.Unmarshal([]byte(cached), &data); unmarshalErr == nil {
				metrics.RecordRates(true)
				rates := filterRatesByCurrencies(data, wantCurrencies)
				c.JSON(http.StatusOK, rates)
				return
			}
		}
	}
	metrics.RecordRates(false)

	if pricingClient == nil {
		handleError(c, errors.New("pricing client not initialized"), http.StatusInternalServerError)
		return
	}

	// Always fetch the default currency set so we can cache one canonical entry under rates:btc
	rates, err := pricingClient.GetMultiCurrencyRatesIn(ctx, nil)
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	// Store in Redis with clear key naming and configured TTL
	if rdb != nil {
		if ratesJSON, err := json.Marshal(rates); err == nil {
			_ = rdb.Set(ctx, ratesRedisKeyBTC, ratesJSON, ratesCacheTTL()).Err()
		}
	}

	out := filterRatesByCurrencies(rates, wantCurrencies)
	ttl := ratesCacheTTL()
	c.Header("X-Rates-Cache-TTL-Seconds", strconv.Itoa(int(ttl.Seconds())))
	c.Header("X-Rates-Updated-At", time.Now().UTC().Format(time.RFC3339))
	c.JSON(http.StatusOK, out)
}

// filterRatesByCurrencies returns a copy of the rates map containing only the requested currencies.
// CoinGecko response shape: {"bitcoin": {"usd": 123, "eur": 100}}. If want is empty, return as-is.
func filterRatesByCurrencies(rates map[string]interface{}, want []string) map[string]interface{} {
	if len(want) == 0 {
		return rates
	}
	wantSet := make(map[string]bool)
	for _, c := range want {
		wantSet[c] = true
	}
	out := make(map[string]interface{})
	for asset, v := range rates {
		vm, ok := v.(map[string]interface{})
		if !ok {
			out[asset] = v
			continue
		}
		filtered := make(map[string]interface{})
		for k, val := range vm {
			if wantSet[k] {
				filtered[k] = val
			}
		}
		if len(filtered) > 0 {
			out[asset] = filtered
		}
	}
	return out
}

// getBTCPriceInFiat returns the BTC price in the given fiat code (e.g. "usd", "eur").
// Uses the rates service; returns (0, false) if pricing is unavailable or currency unsupported.
func getBTCPriceInFiat(ctx context.Context, fiatCode string) (float64, bool) {
	if assetPricer == nil {
		return 0, false
	}
	return assetPricer.GetAssetPriceInFiat(ctx, pricing.AssetClassCrypto, "bitcoin", fiatCode, 1)
}

// getUSDPerFiat returns how many USD equal 1 unit of fiat (e.g. 1 EUR = 1.08 USD).
// Derived from BTC/USD and BTC/fiat; used to convert commodity/bond USD prices into user fiat.
func getUSDPerFiat(ctx context.Context, fiatCode string) (float64, bool) {
	fiatCode = strings.ToLower(strings.TrimSpace(fiatCode))
	if fiatCode == "" || !pricing.SupportedFiatCurrencies[fiatCode] {
		fiatCode = "usd"
	}
	if fiatCode == "usd" {
		return 1, true
	}
	if pricingClient == nil {
		return 0, false
	}
	rates, err := pricingClient.GetMultiCurrencyRatesIn(ctx, []string{"usd", fiatCode})
	if err != nil {
		return 0, false
	}
	btc, ok := rates["bitcoin"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	var btcUSD, btcFiat float64
	for k, val := range btc {
		var v float64
		switch x := val.(type) {
		case float64:
			v = x
		case int:
			v = float64(x)
		default:
			continue
		}
		if k == "usd" {
			btcUSD = v
		}
		if k == fiatCode {
			btcFiat = v
		}
	}
	if btcUSD <= 0 || btcFiat <= 0 {
		return 0, false
	}
	return btcUSD / btcFiat, true
}

// getAssetPriceInFiat returns the spot price of an asset (type + symbol) in the given fiat.
// usdPerFiat is from getUSDPerFiat for converting commodity/bond USD into user fiat.
func getAssetPriceInFiat(ctx context.Context, assetType, symbol, fiat string, usdPerFiat float64) (float64, bool) {
	if assetPricer == nil {
		return 0, false
	}
	return assetPricer.GetAssetPriceInFiat(ctx, assetType, symbol, fiat, usdPerFiat)
}

// PortfolioItemWithValue adds value_fiat to a portfolio item for API responses.
type PortfolioItemWithValue struct {
	PortfolioItem
	ValueFiat *float64 `json:"value_fiat,omitempty"` // nil when rate unavailable or non-crypto
}

// computePortfolioValuation enriches portfolio items with value_fiat using the unified asset pricer (crypto, commodity, bond).
// valuationCurrency and usdPerFiat (from getUSDPerFiat) are used for conversion. Missing rate data: value_fiat is nil; total is sum of known values only.
func computePortfolioValuation(p *Portfolio, valuationCurrency string, usdPerFiat float64) (total *float64, items []PortfolioItemWithValue) {
	items = make([]PortfolioItemWithValue, 0, len(p.Items))
	var sum float64
	hasAny := false
	for i := range p.Items {
		it := p.Items[i]
		withVal := PortfolioItemWithValue{PortfolioItem: it}
		assetType := strings.ToLower(strings.TrimSpace(it.Type))
		symbol := pricing.NormalizeAssetSymbol(it.Type, it.Symbol)
		price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat)
		if ok && price >= 0 {
			v := it.Amount * price
			withVal.ValueFiat = &v
			sum += v
			hasAny = true
		}
		items = append(items, withVal)
	}
	if hasAny {
		total = &sum
	}
	return total, items
}

// PortfolioWithValuation is a portfolio plus valuation in user's preferred fiat for API responses.
type PortfolioWithValuation struct {
	ID                string                   `json:"id"`
	Username          string                   `json:"username"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Created           time.Time                `json:"created"`
	Updated           time.Time                `json:"updated"`
	ValuationCurrency string                   `json:"valuation_currency,omitempty"`
	TotalValueFiat    *float64                 `json:"total_value_fiat,omitempty"`
	Items             []PortfolioItemWithValue `json:"items"`
}

// healthHandler is a simple liveness probe. It reports basic process health
// and whether core configuration is present, but does not force external
// dependency checks to succeed.
func healthHandler(c *gin.Context) {
	status := "ok"
	details := gin.H{}

	// Basic Redis check (non-fatal)
	if rdb != nil {
		if err := rdb.Ping(ctx).Err(); err != nil {
			status = "degraded"
			details["redis_error"] = err.Error()
		}
	} else {
		status = "degraded"
		details["redis_error"] = "redis client not initialized"
	}

	// Configuration check
	if baseURL == "" || apiKey == "" {
		status = "degraded"
		details["config_error"] = "GETBLOCK_BASE_URL or GETBLOCK_ACCESS_TOKEN not set"
	}

	c.JSON(http.StatusOK, mergeCorrelationID(c, gin.H{
		"status":     status,
		"details":    details,
		"timestamp":  time.Now().Unix(),
		"app_env":    config.GetAppEnv(),
		"version":    "v1",
		"api_prefix": "/api/v1",
	}))
}

// readinessHandler is a readiness probe. It checks core dependencies such as
// Redis and (optionally) the external GetBlock API. If these checks fail, the
// endpoint returns 503 so orchestrators can avoid routing traffic.
func readinessHandler(c *gin.Context) {
	if rdb == nil {
		c.JSON(http.StatusServiceUnavailable, mergeCorrelationID(c, gin.H{"status": "not_ready", "error": "redis client not initialized"}))
		return
	}

	// Redis must be reachable
	if err := rdb.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, mergeCorrelationID(c, gin.H{"status": "not_ready", "error": fmt.Sprintf("redis ping failed: %v", err)}))
		return
	}

	// Optional shallow external API check, controlled via configuration
	checkExternal := appConfig != nil && appConfig.ReadyCheckExternal
	if checkExternal {
		if baseURL == "" || apiKey == "" {
			c.JSON(http.StatusServiceUnavailable, mergeCorrelationID(c, gin.H{"status": "not_ready", "error": "missing GETBLOCK_* env for external readiness check"}))
			return
		}
		// Perform a lightweight external call with a short timeout
		client := resty.New().SetTimeout(3 * time.Second).SetRetryCount(0)
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("x-api-key", apiKey).
			SetBody(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "readiness",
				"method":  "getblockcount",
				"params":  []interface{}{},
			}).
			Post(baseURL)
		if err != nil || resp.StatusCode() >= 400 {
			msg := "external API readiness check failed"
			if err != nil {
				msg = fmt.Sprintf("%s: %v", msg, err)
			} else {
				msg = fmt.Sprintf("%s: %s", msg, resp.Status())
			}
			c.JSON(http.StatusServiceUnavailable, mergeCorrelationID(c, gin.H{"status": "not_ready", "error": msg}))
			return
		}
	}

	c.JSON(http.StatusOK, mergeCorrelationID(c, gin.H{"status": "ready", "timestamp": time.Now().Unix()}))
}

var (
	// baseURL and apiKey are loaded strictly from environment variables.
	// For production, they must be set via GETBLOCK_BASE_URL and GETBLOCK_ACCESS_TOKEN.
	// Tests may override these globals directly.
	baseURL string
	apiKey  string
	// httpClient is injectable for tests; production code uses a default resty client.
	httpClient = resty.New().
			SetTimeout(10 * time.Second).
			SetRetryCount(3)
	// blockchainClient is the pluggable blockchain data provider.
	blockchainClient blockchain.RPCClient
	// pricingClient is the pluggable pricing/FX provider.
	pricingClient pricing.Client
	// assetPricer unifies crypto, commodity, and bond pricing for portfolio valuation.
	assetPricer pricing.AssetPricer
	// newsService fetches and caches contextual financial news.
	newsService *news.Service
	// emailService sends templated emails for onboarding/alerts.
	emailService   *email.Service
	emailTemplates *email.Templates
	// appConfig holds the parsed application configuration.
	appConfig *config.Config
)

// APIError represents a standardized error payload returned by the API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SetHTTPClient allows tests or other packages to replace the internal HTTP client used for API calls.
func SetHTTPClient(c *resty.Client) {
	if c != nil {
		httpClient = c
	}
}

// SetBlockchainClient allows tests to inject a mock blockchain RPC client.
func SetBlockchainClient(c blockchain.RPCClient) {
	blockchainClient = c
}

// clientCryptoAdapter adapts pricing.Client to CryptoPriceFetcher (bitcoin only) for tests.
type clientCryptoAdapter struct{ c pricing.Client }

func (a *clientCryptoAdapter) GetCryptoPriceInFiat(ctx context.Context, coinID, fiat string) (float64, bool) {
	if a.c == nil {
		return 0, false
	}
	rates, err := a.c.GetMultiCurrencyRatesIn(ctx, []string{fiat})
	if err != nil {
		return 0, false
	}
	coin, ok := rates[coinID].(map[string]interface{})
	if !ok {
		return 0, false
	}
	val, ok := coin[fiat]
	if !ok {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return v, v >= 0
	case int:
		return float64(v), v >= 0
	default:
		return 0, false
	}
}

// SetPricingClient allows tests to inject a mock pricing client. Also sets assetPricer so portfolio valuation works.
func SetPricingClient(c pricing.Client) {
	pricingClient = c
	if c == nil {
		assetPricer = nil
		return
	}
	if cg, ok := c.(pricing.CryptoPriceFetcher); ok {
		assetPricer = &pricing.CompositePricer{Crypto: cg, Commodity: &pricing.StaticCommoditySource{}, Bond: &pricing.StaticBondSource{PricePer100: pricing.DefaultBondPrices()}}
	} else {
		assetPricer = &pricing.CompositePricer{Crypto: &clientCryptoAdapter{c: c}, Commodity: &pricing.StaticCommoditySource{}, Bond: &pricing.StaticBondSource{PricePer100: pricing.DefaultBondPrices()}}
	}
}

// correlationIDMiddleware propagates or generates a correlation ID (X-Correlation-ID / X-Request-ID),
// sets response headers, gin context keys, and Sentry tags for end-to-end tracing.
func correlationIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := correlation.FromHeaders(c.Request.Header)
		if rid == "" {
			rid = correlation.NewID()
		}
		c.Set("correlation_id", rid)
		c.Set("request_id", rid) // legacy alias
		c.Header(correlation.HeaderCorrelationID, rid)
		c.Header(correlation.HeaderRequestID, rid)
		if hub := sentrygin.GetHubFromContext(c); hub != nil {
			hub.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetTag("correlation_id", rid)
				scope.SetTag("request_id", rid)
			})
		}
		c.Next()
	}
}

// sentryUserScopeMiddleware attaches client metadata and optional authenticated user to Sentry scopes.
func sentryUserScopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if hub := sentrygin.GetHubFromContext(c); hub != nil {
			hub.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetTag("client_ip", c.ClientIP())
				if ua := c.GetHeader("User-Agent"); ua != "" {
					if len(ua) > 512 {
						ua = ua[:512]
					}
					scope.SetTag("user_agent", ua)
				}
			})
		}
		if sid, err := c.Cookie("session_id"); err == nil && sid != "" {
			if username, ok := validateSession(sid); ok {
				if hub := sentrygin.GetHubFromContext(c); hub != nil {
					hub.ConfigureScope(func(scope *sentry.Scope) {
						scope.SetUser(sentry.User{ID: username, Username: username})
						if u, exists := getUser(username); exists {
							scope.SetTag("user_role", u.Role)
						}
					})
				}
			}
		}
		c.Next()
	}
}

func sentryRouteForContext(c *gin.Context) string {
	if r := c.FullPath(); r != "" {
		return r
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func sentryLevelForHTTPStatus(status int) sentry.Level {
	if status >= 500 {
		return sentry.LevelError
	}
	if status >= 400 {
		return sentry.LevelWarning
	}
	return sentry.LevelInfo
}

// captureSentryException records err on the request hub with tags and HTTP context.
func captureSentryException(c *gin.Context, err error, status int, extraTags map[string]string) {
	if err == nil {
		return
	}
	apply := func(scope *sentry.Scope) {
		for k, v := range extraTags {
			if k != "" && v != "" {
				scope.SetTag(k, v)
			}
		}
		if rid, exists := c.Get("correlation_id"); exists {
			if rs, ok := rid.(string); ok && rs != "" {
				scope.SetTag("correlation_id", rs)
			}
		}
		if rid, exists := c.Get("request_id"); exists {
			if rs, ok := rid.(string); ok && rs != "" {
				scope.SetTag("request_id", rs)
			}
		}
		route := sentryRouteForContext(c)
		if route != "" {
			scope.SetTag("route", route)
		}
		scope.SetTag("http.status_code", strconv.Itoa(status))
		scope.SetLevel(sentryLevelForHTTPStatus(status))
		scope.SetContext("http", map[string]interface{}{
			"method":      c.Request.Method,
			"path":        route,
			"status_code": status,
		})
	}
	hub := sentrygin.GetHubFromContext(c)
	if hub == nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			apply(scope)
			sentry.CaptureException(err)
		})
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		apply(scope)
		hub.CaptureException(err)
	})
}

// defaultErrorCode derives a generic error code from an HTTP status code.
func defaultErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "error"
	}
}

// mergeCorrelationID adds correlation_id to a JSON body when the request has one (for client/operator tracing).
func mergeCorrelationID(c *gin.Context, body gin.H) gin.H {
	if body == nil {
		body = gin.H{}
	}
	if cid, ok := c.Get("correlation_id"); ok {
		if s, ok := cid.(string); ok && s != "" {
			body["correlation_id"] = s
		}
	}
	return body
}

// errorResponse writes a structured error response.
func errorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, mergeCorrelationID(c, gin.H{"error": APIError{
		Code:    code,
		Message: message,
	}}))
}

// abortErrorResponse aborts the request with a structured error response.
func abortErrorResponse(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, mergeCorrelationID(c, gin.H{"error": APIError{
		Code:    code,
		Message: message,
	}}))
}

// handleError captures an error with Sentry and returns a standardized payload.
func handleError(c *gin.Context, err error, status int) {
	captureSentryException(c, err, status, map[string]string{"source": "handleError"})
	errorResponse(c, status, defaultErrorCode(status), err.Error())
}

func isValidAddress(address string) bool {
	// Bitcoin addresses are usually 26-90 characters long and start with specific characters
	if len(address) < 26 || len(address) > 90 {
		return false
	}

	// Check for valid address prefix
	validPrefixes := []string{"1", "3", "bc1"}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(address, prefix) {
			return true
		}
	}
	return false
}

func isValidTransactionID(txID string) bool {
	// Transaction IDs are 64-character hex strings
	if len(txID) != 64 {
		return false
	}
	// Verify hex characters using regex
	matched, _ := regexp.MatchString("^[0-9a-fA-F]{64}$", txID)
	return matched
}

func isValidBlockHeight(blockHeight string) bool {
	// A simple check to see if the blockHeight string can be converted to an integer
	_, err := strconv.Atoi(blockHeight)
	return err == nil
}

// updated to use rdb Redis client
func getNetworkStatus() (map[string]interface{}, error) {
	cacheKey := "network:status"
	cached, err := rdb.Get(context.Background(), cacheKey).Result()
	if err == nil {
		var data map[string]interface{}
		if json.Unmarshal([]byte(cached), &data) == nil {
			metrics.RecordNetworkStatus(true)
			return data, nil
		}
	}
	metrics.RecordNetworkStatus(false)
	ctx := context.Background()

	// Fetch block height
	blockCountResp, err := callBlockchain(ctx, "getblockcount", []interface{}{})
	if err != nil {
		return nil, err
	}
	var blockCountData map[string]interface{}
	if err := json.Unmarshal(blockCountResp.Body(), &blockCountData); err != nil {
		return nil, err
	}
	blockHeight, ok := blockCountData["result"].(float64)
	if !ok {
		return nil, errors.New("invalid block height response")
	}

	// Fetch difficulty
	difficultyResp, err := callBlockchain(ctx, "getdifficulty", []interface{}{})
	if err != nil {
		return nil, err
	}
	var difficultyData map[string]interface{}
	if err := json.Unmarshal(difficultyResp.Body(), &difficultyData); err != nil {
		return nil, err
	}
	difficulty, ok := difficultyData["result"].(float64)
	if !ok {
		return nil, errors.New("invalid difficulty response")
	}

	// Fetch network hash rate
	hashRateResp, err := callBlockchain(ctx, "getnetworkhashps", []interface{}{})
	if err != nil {
		return nil, err
	}
	var hashRateData map[string]interface{}
	if err := json.Unmarshal(hashRateResp.Body(), &hashRateData); err != nil {
		return nil, err
	}
	raw, exists := hashRateData["result"]
	if !exists {
		return nil, errors.New("missing hash rate in response")
	}
	hashRate, ok := raw.(float64)
	if !ok {
		return nil, errors.New("invalid hash rate response")
	}

	result := map[string]interface{}{
		"block_height": blockHeight,
		"difficulty":   difficulty,
		"hash_rate":    hashRate,
	}
	resultJSON, _ := json.Marshal(result)
	err = rdb.Set(context.Background(), cacheKey, resultJSON, 1*time.Minute).Err()
	if err != nil {
		logging.WithComponent(logging.ComponentNetwork).WithError(err).WithField(logging.FieldEvent, "cache_set_failed").Warn("redis set failed in getNetworkStatus")
	}
	return result, nil
}

// updated to use rdb Redis client
func getAddressDetails(address string) (map[string]interface{}, error) {
	cacheKey := "address:" + address
	cached, err := rdb.Get(context.Background(), cacheKey).Result()
	if err == nil {
		var data map[string]interface{}
		if json.Unmarshal([]byte(cached), &data) == nil {
			metrics.RecordRPCAddressCache(true)
			return data, nil
		}
	}
	metrics.RecordRPCAddressCache(false)

	response, err := callBlockchain(context.Background(), "getaddressinfo", []interface{}{address})
	if err != nil {
		return nil, err
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(response.Body(), &responseData); err != nil {
		return nil, err
	}
	result, ok := responseData["result"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid address details response")
	}

	resultJSON, _ := json.Marshal(result)
	rdb.Set(context.Background(), cacheKey, resultJSON, 1*time.Minute) // Cache for 1 minute
	return result, nil
}

// updated to use rdb Redis client
func getTransactionDetails(txID string) (map[string]interface{}, error) {
	cacheKey := "tx:" + txID
	cached, err := rdb.Get(context.Background(), cacheKey).Result()
	if err == nil {
		var data map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(cached), &data); unmarshalErr == nil {
			metrics.RecordRPCTxCache(true)
			return data, nil
		} else {
			metrics.RecordRPCTxCache(false)
			return nil, &InvalidCachedJSONError{TxID: txID, Err: unmarshalErr}
		}
	}
	metrics.RecordRPCTxCache(false)

	response, err := callBlockchain(context.Background(), "getrawtransaction", []interface{}{txID, 1})
	if err != nil {
		return nil, err
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(response.Body(), &responseData); err != nil {
		return nil, err
	}
	result, ok := responseData["result"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid transaction details response")
	}

	resultJSON, _ := json.Marshal(result)
	rdb.Set(context.Background(), cacheKey, resultJSON, 5*time.Minute) // Cache for 5 minutes
	return result, nil
}

// updated to use rdb Redis client
func getBlockDetails(blockHeight string) (map[string]interface{}, error) {
	cacheKey := "block:" + blockHeight
	cached, err := rdb.Get(context.Background(), cacheKey).Result()
	if err == nil {
		var data map[string]interface{}
		if json.Unmarshal([]byte(cached), &data) == nil {
			metrics.RecordRPCBlockCache(true)
			return data, nil
		}
	}
	metrics.RecordRPCBlockCache(false)

	height, _ := strconv.Atoi(blockHeight)
	response, err := callBlockchain(context.Background(), "getblockbyheight", []interface{}{height, 1})
	if err != nil {
		return nil, err
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(response.Body(), &responseData); err != nil {
		return nil, err
	}
	result, ok := responseData["result"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid block details response")
	}

	resultJSON, _ := json.Marshal(result)
	rdb.Set(context.Background(), cacheKey, resultJSON, 5*time.Minute) // Cache for 5 minutes
	return result, nil
}

// collectMetrics collects historical metrics for charts
func blockchairRequest(method string, params []interface{}) (*resty.Response, error) {
	if baseURL == "" || apiKey == "" {
		return nil, errors.New("missing required environment variables")
	}

	// Generate a unique ID for this request
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}

	response, err := httpClient.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("x-api-key", apiKey).
		SetBody(payload).
		Post(baseURL)

	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if response.StatusCode() >= 400 {
		return nil, fmt.Errorf("API error: %s", response.Status())
	}

	return response, nil
}

// callBlockchain prefers blockchainClient (GetBlock-compatible JSON-RPC). When nil,
// falls back to blockchairRequest using baseURL/apiKey/httpClient (legacy test path).
func callBlockchain(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
	if blockchainClient != nil {
		return blockchainClient.Call(ctx, method, params)
	}
	return blockchairRequest(method, params)
}

// Redis key for historical BTC price in a fiat currency (e.g. btc_price_history:usd).
func btcPriceHistoryKey(currency string) string {
	return "btc_price_history:" + strings.ToLower(currency)
}

const btcPriceHistoryMaxPoints = 8640 // ~30 days at 5-min interval

// collectMetrics collects historical metrics for charts, including multi-currency FX for portfolio performance.
func collectMetrics() {
	if rdb == nil {
		return
	}
	cid := correlation.NewID()
	jobLog := logging.WithComponent(logging.ComponentBackground).WithFields(log.Fields{
		logging.FieldCorrelationID: cid,
		logging.FieldEvent:         "metrics_collect",
	})
	jobLog.Debug("metrics collection run started")
	defer metrics.RecordMetricsJob()
	ctx := context.Background()
	now := float64(time.Now().Unix())

	// Collect Bitcoin price in multiple fiats for consistent portfolio charts over time
	if pricingClient != nil {
		rates, err := pricingClient.GetMultiCurrencyRatesIn(ctx, pricing.DefaultFiatCurrencies)
		if err != nil {
			jobLog.WithError(err).Warn("multi-currency rates fetch failed; using USD fallback if available")
			// Fallback: at least store USD
			if usd, err2 := pricingClient.GetBTCUSD(ctx); err2 == nil {
				s := strconv.FormatFloat(usd, 'f', -1, 64)
				rdb.ZAdd(ctx, "btc_price_history", redis.Z{Score: now, Member: s})
				rdb.ZAdd(ctx, btcPriceHistoryKey("usd"), redis.Z{Score: now, Member: s})
				rdb.ZRemRangeByRank(ctx, "btc_price_history", 0, -(btcPriceHistoryMaxPoints + 1))
				rdb.ZRemRangeByRank(ctx, btcPriceHistoryKey("usd"), 0, -(btcPriceHistoryMaxPoints + 1))
			}
		} else {
			btc, _ := rates["bitcoin"].(map[string]interface{})
			for _, c := range pricing.DefaultFiatCurrencies {
				if btc == nil {
					break
				}
				var price float64
				switch v := btc[c].(type) {
				case float64:
					price = v
				case int:
					price = float64(v)
				default:
					continue
				}
				key := btcPriceHistoryKey(c)
				rdb.ZAdd(ctx, key, redis.Z{Score: now, Member: strconv.FormatFloat(price, 'f', -1, 64)})
				rdb.ZRemRangeByRank(ctx, key, 0, -(btcPriceHistoryMaxPoints + 1))
			}
			// Legacy key for backward compatibility (USD)
			if btc != nil {
				if v, ok := btc["usd"].(float64); ok {
					rdb.ZAdd(ctx, "btc_price_history", redis.Z{Score: now, Member: strconv.FormatFloat(v, 'f', -1, 64)})
					rdb.ZRemRangeByRank(ctx, "btc_price_history", 0, -(btcPriceHistoryMaxPoints + 1))
				}
			}
		}
	}

	// Get mempool size
	mempoolResp, err := callBlockchain(context.Background(), "getmempoolinfo", []interface{}{})
	if err == nil {
		var mempoolData map[string]interface{}
		_ = json.Unmarshal(mempoolResp.Body(), &mempoolData)
		if result, ok := mempoolData["result"].(map[string]interface{}); ok {
			if size, ok := result["size"].(float64); ok {
				rdb.ZAdd(context.Background(), "mempool_size", redis.Z{Score: now, Member: size})
			}
		}
	}

	// Get latest blocks for block times and tx volume
	blocksResp, err := callBlockchain(context.Background(), "getblockchaininfo", []interface{}{})
	if err == nil {
		var chainData map[string]interface{}
		_ = json.Unmarshal(blocksResp.Body(), &chainData)
		if result, ok := chainData["result"].(map[string]interface{}); ok {
			if heightF, ok := result["blocks"].(float64); ok {
				height := int(heightF)
				// Get last 10 blocks
				blockTimes := []int64{}
				txCounts := []float64{}
				for i := 0; i < 10; i++ {
					h := height - i
					if h < 0 {
						break
					}
					blockResp, err := callBlockchain(context.Background(), "getblockhash", []interface{}{h})
					if err != nil {
						continue
					}
					var hashData map[string]interface{}
					_ = json.Unmarshal(blockResp.Body(), &hashData)
					if hash, ok := hashData["result"].(string); ok {
						blockDetailResp, err := callBlockchain(context.Background(), "getblock", []interface{}{hash})
						if err != nil {
							continue
						}
						var blockData map[string]interface{}
						_ = json.Unmarshal(blockDetailResp.Body(), &blockData)
						if result, ok := blockData["result"].(map[string]interface{}); ok {
							if t, ok := result["time"].(float64); ok {
								blockTimes = append(blockTimes, int64(t))
							}
							if txs, ok := result["tx"].([]interface{}); ok {
								txCounts = append(txCounts, float64(len(txs)))
							}
						}
					}
				}
				// Calculate average block time
				if len(blockTimes) > 1 {
					var totalTime int64 = 0
					for i := 1; i < len(blockTimes); i++ {
						// previous minus current
						totalTime += blockTimes[i-1] - blockTimes[i]
					}
					avgBlockTime := float64(totalTime) / float64(len(blockTimes)-1)
					rdb.ZAdd(context.Background(), "block_times", redis.Z{Score: now, Member: avgBlockTime})
				}
				// Sum tx volume
				if len(txCounts) > 0 {
					totalTx := float64(0)
					for _, c := range txCounts {
						totalTx += c
					}
					rdb.ZAdd(context.Background(), "tx_volume", redis.Z{Score: now, Member: totalTx})
				}
			}
		}
	}
}
