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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
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

var rdb = redis.NewClient(&redis.Options{
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
				log.WithFields(log.Fields{"ip": ip, "export": prefix}).Warn("Export rate limit exceeded (IP)")
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
					log.WithFields(log.Fields{"username": username, "export": prefix}).Warn("Export rate limit exceeded (user)")
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
		log.WithFields(log.Fields{"ip": ip, "export": prefix}).Warn("Export rate limit exceeded (in-memory)")
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
			log.WithFields(log.Fields{"username": username, "export": prefix}).Warn("Export rate limit exceeded (in-memory)")
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
	log.WithFields(fields).Info("Large or intensive export request")
}

// User struct definition
type User struct {
	Username string    `json:"username"`
	Password string    `json:"-"`    // Hashed password, never sent in JSON
	Role     string    `json:"role"` // "admin" or "user"
	Created  time.Time `json:"created"`
}

type PortfolioItem struct {
	Type    string  `json:"type"` // "stock", "crypto", "bond", "commodity"
	Address string  `json:"address"`
	Label   string  `json:"label"`
	Amount  float64 `json:"amount"`
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
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Asset       string     `json:"asset"` // e.g., "bitcoin"
	TargetPrice float64    `json:"target_price"`
	Currency    string     `json:"currency"`  // e.g., "usd"
	Condition   string     `json:"condition"` // "above" or "below"
	IsActive    bool       `json:"is_active"`
	TriggeredAt *time.Time `json:"triggered_at"`
	Created     time.Time  `json:"created"`
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
			log.WithError(err).Warn("Failed to store CSRF token in Redis")
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
			log.WithError(err).WithField("username", username).Warn("Failed to load user from Redis")
			continue
		}

		var user User
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			log.WithError(err).WithField("username", username).Warn("Failed to unmarshal user from Redis")
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
		log.WithError(err).Warn("Failed to load users from Redis")
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
			log.Fatal("ADMIN_USERNAME and ADMIN_PASSWORD must be set in non-development environments")
		}
		if !isStrongPassword(adminPassword) {
			log.Fatal("ADMIN_PASSWORD must be 8-128 characters and include at least one letter and one digit in non-development environments")
		}
	}

	if _, exists := users[adminUsername]; !exists {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			log.WithError(err).Fatal("Failed to hash default admin password")
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
			log.WithError(err).Warn("Failed to save default admin to Redis")
		}

		if appEnv == "development" && (os.Getenv("ADMIN_USERNAME") == "" || os.Getenv("ADMIN_PASSWORD") == "") {
			log.WithField("env", appEnv).Warn("Default admin credentials (admin/admin123) are being used in development")
		}
		log.WithFields(log.Fields{
			"username": adminUsername,
			"env":      appEnv,
		}).Info("Default admin user initialized")
	}
}

// createUser adds a new user to the system
func createUser(username, password, role string) error {
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

// userProfileHandler returns the profile of the authenticated user
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
				log.WithError(err).Warn("Redis rate limit per IP failed, falling back to in-memory limiter")
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
				log.WithError(err).Warn("Redis rate limit per user failed, falling back to in-memory limiter")
			}
		}

		if exceeded {
			log.WithFields(log.Fields{
				"ip":       ip,
				"username": username,
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
		log.WithField("ip", ip).Warn("Rate limit exceeded (in-memory)")
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
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)

	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}
	appConfig = cfg

	appEnv := cfg.AppEnv
	log.WithField("env", appEnv).Info("Starting Bitcoin Explorer server")

	// Initialize GetBlock settings from configuration.
	baseURL = cfg.GetBlockBaseURL
	apiKey = cfg.GetBlockAccessToken

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.WithField("redis", "ping").Warnf("Redis ping failed: %v", err)
	}

	// Initialize Sentry
	_ = pong
	if cfg.SentryDSN != "" {
		if initErr := sentry.Init(sentry.ClientOptions{
			Dsn: cfg.SentryDSN,
			// Set traces sample rate to 1.0 to capture 100% of transactions for performance monitoring.
			TracesSampleRate: 1.0,
		}); initErr != nil {
			log.WithError(initErr).Fatal("sentry.Init failed")
		}
		defer sentry.Flush(2 * time.Second)
		log.Info("Sentry initialized successfully")
	} else {
		log.Warn("SENTRY_DSN not set, Sentry not initialized")
	}

	r := gin.Default()

	r.Use(sentrygin.New(sentrygin.Options{}))
	r.Use(rateLimitMiddleware)
	r.Use(csrfMiddleware)

	log.Info("Starting Bitcoin Explorer server")

	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: cfg.RedisHost + ":6379",
	})

	// Configure Redis for LRU eviction
	rdb.ConfigSet(ctx, "maxmemory", "100mb")
	rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")

	// Initialize pluggable service clients
	blockchainClient = blockchain.NewGetBlockRPCClient(baseURL, apiKey, httpClient)
	pricingClient = pricing.NewCoinGeckoClient(httpClient)

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
	r.StaticFile("/symbols", "symbols.html")

	// Health and readiness endpoints
	r.GET("/healthz", healthHandler)
	r.GET("/readyz", readinessHandler)

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

		// Authentication routes
		apiV1.POST("/login", loginHandler)
		apiV1.POST("/logout", logoutHandler)
		apiV1.POST("/register", registerHandler)

		// User routes (require authentication)
		userV1 := apiV1.Group("/user")
		userV1.Use(authMiddleware)
		{
			userV1.GET("/profile", userProfileHandler)
			userV1.GET("/portfolios", listPortfoliosHandler)
			userV1.GET("/portfolios/export", exportPortfoliosHandler)
			userV1.GET("/portfolios/:id/export/csv", exportPortfolioCSVHandler)
			userV1.GET("/portfolios/:id/export/pdf", exportPortfolioPDFHandler)
			userV1.POST("/portfolios", createPortfolioHandler)
			userV1.PUT("/portfolios/:id", updatePortfolioHandler)
			userV1.DELETE("/portfolios/:id", deletePortfolioHandler)
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
		user.GET("/portfolios", listPortfoliosHandler)
		user.GET("/portfolios/export", exportPortfoliosHandler)
		user.GET("/portfolios/:id/export/csv", exportPortfolioCSVHandler)
		user.GET("/portfolios/:id/export/pdf", exportPortfolioPDFHandler)
		user.POST("/portfolios", createPortfolioHandler)
		user.PUT("/portfolios/:id", updatePortfolioHandler)
		user.DELETE("/portfolios/:id", deletePortfolioHandler)
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
		log.Info("Starting background prefetch job")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			// Initial run and every tick
			func() {
				const numBlocks = 5
				const numTxs = 10
				blocks, err := fetchLatestBlocks(numBlocks)
				if err == nil {
					blocksJSON, _ := json.Marshal(blocks)
					rdb.Set(context.Background(), "latest_blocks", blocksJSON, 5*time.Minute)
				} else {
					log.WithError(err).Error("Failed to prefetch latest blocks")
				}
				txs, err := fetchLatestTransactions(numBlocks, numTxs)
				if err == nil {
					txsJSON, _ := json.Marshal(txs)
					rdb.Set(context.Background(), "latest_transactions", txsJSON, 5*time.Minute)
				} else {
					log.WithError(err).Error("Failed to prefetch latest transactions")
				}
				log.Info("Prefetched latest blocks and transactions")
			}()
			<-ticker.C
		}
	}()

	// Start background job to collect metrics for charts
	go func() {
		log.Info("Starting background metrics collection job")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			collectMetrics()
			log.Info("Collected metrics for charts")
			<-ticker.C
		}
	}()

	defer sentry.Flush(2 * time.Second)

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
		log.WithField("username", loginReq.Username).Warn("Failed login attempt")
		return
	}

	sessionID, err := createSession(loginReq.Username)
	if err != nil {
		log.WithError(err).Error("Failed to create session")
		errorResponse(c, http.StatusInternalServerError, "session_creation_failed", "Failed to create session")
		return
	}

	csrfToken, err := createOrUpdateCSRFToken(sessionID)
	if err != nil {
		log.WithError(err).Error("Failed to create CSRF token")
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
	log.WithFields(log.Fields{
		"username": loginReq.Username,
		"role":     user.Role,
	}).Info("User logged in successfully")
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
	err := createUser(registerReq.Username, registerReq.Password, "user")
	if err != nil {
		if err.Error() == "user already exists" {
			errorResponse(c, http.StatusConflict, "username_taken", "Username already exists")
		} else {
			log.WithError(err).Error("Failed to create user")
			errorResponse(c, http.StatusInternalServerError, "user_creation_failed", "Failed to create user")
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
	log.WithField("username", registerReq.Username).Info("New user registered")
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
		log.WithError(err).Error("Failed to marshal feedback data")
		errorResponse(c, http.StatusInternalServerError, "feedback_processing_failed", "Failed to process feedback")
		return
	}

	err = rdb.Set(ctx, feedbackKey, jsonData, 30*24*time.Hour).Err() // Store for 30 days
	if err != nil {
		log.WithError(err).Error("Failed to store feedback in Redis")
		errorResponse(c, http.StatusInternalServerError, "feedback_save_failed", "Failed to save feedback")
		return
	}

	log.WithFields(log.Fields{
		"name":  feedbackReq.Name,
		"email": feedbackReq.Email,
		"message": func() string {
			if len(feedbackReq.Message) > 100 {
				return feedbackReq.Message[:100]
			}
			return feedbackReq.Message
		}(),
	}).Info("Feedback received")

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
			log.WithError(err).Warn("Failed to scan rate limit keys from Redis")
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

		log.WithField("username", username).Info("Cache cleared by admin")
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

	c.JSON(http.StatusOK, gin.H{
		"data": paginated,
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
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"symbol", "type", "address", "amount", "value", "portfolio_created", "portfolio_updated"})
	createdStr := p.Created.UTC().Format(time.RFC3339)
	updatedStr := p.Updated.UTC().Format(time.RFC3339)
	for _, item := range p.Items {
		amountStr := strconv.FormatFloat(item.Amount, 'f', -1, 64)
		_ = w.Write([]string{
			item.Label,
			item.Type,
			item.Address,
			amountStr,
			amountStr, // value (quantity; replace with amount*price if pricing available)
			createdStr,
			updatedStr,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.WithError(err).Error("CSV export write failed")
	}
}

// generatePortfolioPDF writes a short portfolio summary report (overall value, allocations by type, positions table) to w.
func generatePortfolioPDF(p *Portfolio, w io.Writer) error {
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

	// Overall value (sum of amounts) and meta
	var total float64
	for _, item := range p.Items {
		total += item.Amount
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Summary", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total (quantity): %s  |  Positions: %d  |  Created: %s  |  Updated: %s",
		formatFloat(total), len(p.Items),
		p.Created.UTC().Format("2006-01-02"),
		p.Updated.UTC().Format("2006-01-02")), "", 0, "L", false, 0, "")
	pdf.Ln(12)

	// Allocations by asset type
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
	if total == 0 {
		total = 1
	}
	for _, t := range []string{"crypto", "stock", "bond", "commodity", "other"} {
		amt, ok := typeAlloc[t]
		if !ok || amt == 0 {
			continue
		}
		pct := amt / total * 100
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

	// Positions table
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Positions", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	posColW := []float64{45, 25, 70, 35}
	posHeaders := []string{"Label", "Type", "Address", "Amount"}
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
		pdf.Ln(-1)
	}
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 5, "Performance history is not available in this report. Value above is total quantity (amounts).", "", 0, "L", false, 0, "")

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
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := generatePortfolioPDF(&p, c.Writer); err != nil {
		log.WithError(err).Error("portfolio PDF export failed")
		errorResponse(c, http.StatusInternalServerError, "pdf_generation_failed", "Failed to generate PDF")
		return
	}
}

const exportVersion = "1.0"

// CSV export limits to prevent abuse and control memory/RPC load.
const (
	maxBlockRangeExport   = 500   // max blocks in one blocks CSV export (end_height - start_height + 1)
	maxBlockRowsExport   = 2000  // max rows for blocks CSV
	maxTxBlockRangeExport = 100   // max block range when exporting transactions (each block may have many txs)
	maxTxRowsExport      = 5000  // max transaction rows per CSV export
	defaultBlockRows     = 500
	defaultTxRows        = 1000
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
		log.WithError(w.Error()).Error("blocks CSV export write failed")
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
		log.WithError(w.Error()).Error("transactions CSV export write failed")
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
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp": time.Now().UTC().Format(time.RFC3339),
			"export_version":   exportVersion,
			"endpoint":         "portfolios",
		},
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"data": paginated,
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
	log.WithField("query", query).Info("Search request received")
	if query == "" {
		log.Warn("Search request with empty query")
		errorResponse(c, http.StatusBadRequest, "missing_query", "Missing query parameter")
		return
	}
	if len(query) > 100 {
		log.WithField("query", query).Warn("Search request query too long")
		errorResponse(c, http.StatusBadRequest, "query_too_long", "Query too long")
		return
	}
	resultType, result, err := searchBlockchain(query)
	if err != nil {
		log.WithFields(log.Fields{"query": query, "error": err}).Error("Search failed")
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
		log.WithError(err).Error("Failed to marshal search response")
		errorResponse(c, http.StatusInternalServerError, "marshal_failed", "Failed to marshal response")
		return
	}
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(jsonBytes))
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60")
	if match := c.GetHeader("If-None-Match"); match == etag {
		log.WithField("query", query).Info("Search cache hit")
		c.Status(304)
		return
	}
	log.WithFields(log.Fields{"query": query, "type": resultType}).Info("Search successful")
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
	Type        string  `json:"type"`        // "crypto", "stock", "commodity", etc.
	Category    string  `json:"category"`    // e.g., "layer1", "defi", "nft", "payment"
	MarketCap   float64 `json:"market_cap"`
	Price       float64 `json:"price"`
	Volume24h   float64 `json:"volume_24h"`
	Change24h   float64 `json:"change_24h"`  // percentage change
	Rank        int     `json:"rank"`
	IsActive    bool    `json:"is_active"`
	ListedSince int64   `json:"listed_since"` // timestamp
}

// SearchFilters represents the filter parameters for symbol search
type SearchFilters struct {
	Types      []string `json:"types"`       // Filter by symbol types
	Categories []string `json:"categories"`  // Filter by categories
	MinPrice   float64  `json:"min_price"`   // Minimum price filter
	MaxPrice   float64  `json:"max_price"`   // Maximum price filter
	MinMarketCap float64 `json:"min_market_cap"`
	MaxMarketCap float64 `json:"max_market_cap"`
	IsActive   *bool    `json:"is_active"`   // Filter by active status
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
	log.WithField("query", query).Info("Advanced search request received")

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

	log.WithFields(log.Fields{
		"query":      query,
		"results":    len(paginatedResults),
		"total":      total,
		"page":       pagination.Page,
		"sort_by":    sort.Field,
		"sort_dir":   sort.Direction,
	}).Info("Advanced search completed")

	c.JSON(http.StatusOK, gin.H{
		"data":       paginatedResults,
		"pagination": gin.H{
			"page":       pagination.Page,
			"page_size":  pagination.PageSize,
			"total":      total,
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
	// Get last 100 points for each metric
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

func priceHistoryHandler(c *gin.Context) {
	if rdb == nil {
		handleError(c, errors.New("redis not available"), http.StatusInternalServerError)
		return
	}

	// Get history for the last 24 hours (288 data points at 5-minute intervals)
	history, err := rdb.ZRevRangeWithScores(ctx, "btc_price_history", 0, 287).Result()
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

	c.JSON(http.StatusOK, result)
}

func ratesHandler(c *gin.Context) {
	cacheKey := "btc:rates"
	ctx := context.Background()

	// Try cache first
	if rdb != nil {
		cached, err := rdb.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var data map[string]interface{}
			if unmarshalErr := json.Unmarshal([]byte(cached), &data); unmarshalErr == nil {
				c.JSON(http.StatusOK, data)
				return
			}
			// If unmarshalling fails, fall through to fetch fresh data
		}
	}

	// Fetch from pricing provider
	if pricingClient == nil {
		handleError(c, errors.New("pricing client not initialized"), http.StatusInternalServerError)
		return
	}

	rates, err := pricingClient.GetMultiCurrencyRates(ctx)
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	// Cache for 5 minutes if Redis is available
	if rdb != nil {
		if ratesJSON, err := json.Marshal(rates); err == nil {
			_ = rdb.Set(ctx, cacheKey, ratesJSON, 5*time.Minute).Err()
		}
	}

	c.JSON(http.StatusOK, rates)
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

	c.JSON(http.StatusOK, gin.H{
		"status":     status,
		"details":    details,
		"timestamp":  time.Now().Unix(),
		"app_env":    config.GetAppEnv(),
		"version":    "v1",
		"api_prefix": "/api/v1",
	})
}

// readinessHandler is a readiness probe. It checks core dependencies such as
// Redis and (optionally) the external GetBlock API. If these checks fail, the
// endpoint returns 503 so orchestrators can avoid routing traffic.
func readinessHandler(c *gin.Context) {
	if rdb == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "redis client not initialized"})
		return
	}

	// Redis must be reachable
	if err := rdb.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": fmt.Sprintf("redis ping failed: %v", err)})
		return
	}

	// Optional shallow external API check, controlled via configuration
	checkExternal := appConfig != nil && appConfig.ReadyCheckExternal
	if checkExternal {
		if baseURL == "" || apiKey == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "missing GETBLOCK_* env for external readiness check"})
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
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": msg})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready", "timestamp": time.Now().Unix()})
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

// SetPricingClient allows tests to inject a mock pricing client.
func SetPricingClient(c pricing.Client) {
	pricingClient = c
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

// errorResponse writes a structured error response.
func errorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": APIError{
		Code:    code,
		Message: message,
	}})
}

// abortErrorResponse aborts the request with a structured error response.
func abortErrorResponse(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": APIError{
		Code:    code,
		Message: message,
	}})
}

// handleError captures an error with Sentry and returns a standardized payload.
func handleError(c *gin.Context, err error, status int) {
	sentry.CaptureException(err)
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
			return data, nil
		}
	}
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
		log.WithError(err).WithField("cache_key", cacheKey).Warn("Redis set error in getNetworkStatus")
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
			return data, nil
		}
	}

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
			return data, nil
		} else {
			return nil, &InvalidCachedJSONError{TxID: txID, Err: unmarshalErr}
		}
	}

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
			return data, nil
		}
	}

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

// callBlockchain is a small helper that prefers the configured blockchainClient
// when available, and falls back to the legacy blockchairRequest helper
// otherwise. This keeps tests that exercise blockchairRequest working while
// allowing production to inject a richer client implementation.
func callBlockchain(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
	if blockchainClient != nil {
		return blockchainClient.Call(ctx, method, params)
	}
	return blockchairRequest(method, params)
}

// collectMetrics collects historical metrics for charts
func collectMetrics() {
	// Use a float64 timestamp for Redis scores
	now := float64(time.Now().Unix())

	// Collect Bitcoin price for history
	if pricingClient != nil {
		if usd, err := pricingClient.GetBTCUSD(context.Background()); err == nil {
			rdb.ZAdd(context.Background(), "btc_price_history", redis.Z{Score: now, Member: usd})
			// Keep only last 30 days of 5-minute data (roughly 8640 points)
			rdb.ZRemRangeByRank(context.Background(), "btc_price_history", 0, -8641)
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
