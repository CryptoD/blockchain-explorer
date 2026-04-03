package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/CryptoD/blockchain-explorer/internal/apperrors"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/gin-gonic/gin"
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
var ErrNotFound = apperrors.ErrNotFound
var ctx = context.Background()

var rdb redisstore.Client = redis.NewClient(&redis.Options{
	Addr: fmt.Sprintf("%s:%d",
		config.GetEnvWithDefault("REDIS_HOST", "localhost"),
		config.GetEnvIntWithDefault("REDIS_PORT", 6379)),
	DB: 0, // use default DB
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

// WatchlistEntry is one row in a watchlist: a symbol or address plus optional tags, notes, and group label.
// Exactly one of Symbol or Address should be set; Type is "symbol" or "address".
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
	if appRepos != nil && appRepos.Session != nil && rdb != nil {
		_ = appRepos.Session.SetSession(ctx, sessionID, username, 24*time.Hour)
	}

	return sessionID, nil
}

// validateSession checks if a session is valid
func validateSession(sessionID string) (string, bool) {
	// Check Redis first
	if appRepos != nil && appRepos.Session != nil && rdb != nil {
		if username, err := appRepos.Session.GetSessionUsername(ctx, sessionID); err == nil && username != "" {
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

	if appRepos != nil && appRepos.Session != nil && rdb != nil {
		_ = appRepos.Session.DeleteSession(ctx, sessionID)
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

	if appRepos != nil && appRepos.Session != nil && rdb != nil {
		if err := appRepos.Session.SetCSRF(ctx, sessionID, token, 24*time.Hour); err != nil {
			logging.WithComponent(logging.ComponentAuth).WithError(err).Warn("Failed to store CSRF token in Redis")
		}
	}

	return token, nil
}

// getCSRFTokenForSession retrieves the CSRF token associated with a session.
func getCSRFTokenForSession(sessionID string) (string, error) {
	if appRepos != nil && appRepos.Session != nil && rdb != nil {
		if val, err := appRepos.Session.GetCSRF(ctx, sessionID); err == nil && val != "" {
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
	if rdb == nil || appRepos == nil || appRepos.User == nil {
		return nil // No Redis, use in-memory only
	}

	keys, err := appRepos.User.ListUserKeys(ctx)
	if err != nil {
		return err
	}

	userMutex.Lock()
	defer userMutex.Unlock()

	for _, key := range keys {
		username, ok := repos.UsernameFromUserKey(key)
		if !ok {
			continue
		}
		data, err := appRepos.User.Get(ctx, username)
		if err != nil {
			if err == redis.Nil {
				continue
			}
			logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to load user from Redis")
			continue
		}

		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to unmarshal user from Redis")
			continue
		}

		users[username] = user
	}

	return nil
}

// saveUserToRedis saves a user to Redis
func saveUserToRedis(user User) error {
	if rdb == nil || appRepos == nil || appRepos.User == nil {
		return nil // No Redis, use in-memory only
	}

	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return appRepos.User.Save(ctx, user.Username, data) // No expiration
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

	var adminUsername, adminPassword string
	if appConfig != nil {
		adminUsername = appConfig.AdminUsername
		adminPassword = appConfig.AdminPassword
	} else {
		adminUsername = strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
		adminPassword = os.Getenv("ADMIN_PASSWORD")
	}

	if appEnv == "development" {
		if adminUsername == "" {
			adminUsername = "admin"
		}
		if adminPassword == "" {
			adminPassword = "admin123"
		}
	}
	// Non-development admin requirements are enforced by config.Validate() at startup.

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

	_ = saveUserToRedis(user)

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
