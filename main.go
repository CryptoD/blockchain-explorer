package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
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
	Addr: getEnvWithDefault("REDIS_HOST", "localhost") + ":6379",
	DB:   0, // use default DB
})

// Rate limiting variables
var rateLimitCount = make(map[string]int)
var rateLimitReset = make(map[string]time.Time)
var rateLimitMutex sync.Mutex

// User struct definition
type User struct {
	Username string    `json:"username"`
	Password string    `json:"-"`    // Hashed password, never sent in JSON
	Role     string    `json:"role"` // "admin" or "user"
	Created  time.Time `json:"created"`
}

type PortfolioItem struct {
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
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Asset       string    `json:"asset"`       // e.g., "bitcoin"
	TargetPrice float64   `json:"target_price"`
	Currency    string    `json:"currency"`     // e.g., "usd"
	Condition   string    `json:"condition"`    // "above" or "below"
	IsActive    bool      `json:"is_active"`
	TriggeredAt *time.Time `json:"triggered_at"`
	Created     time.Time `json:"created"`
}

// User authentication variables
var (
	users     = make(map[string]User) // username -> User
	userMutex sync.RWMutex
	// In-memory session store as a fallback
	sessionStore = make(map[string]string)
	sessionMutex sync.RWMutex
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

	if rdb != nil {
		_ = rdb.Del(ctx, "session:"+sessionID).Err()
	}
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

// initializeDefaultAdmin creates the default admin user if it doesn't exist
func initializeDefaultAdmin() {
	// First try to load existing users from Redis
	if err := loadUsersFromRedis(); err != nil {
		log.WithError(err).Warn("Failed to load users from Redis")
	}

	userMutex.Lock()
	defer userMutex.Unlock()

	adminUsername := getEnvWithDefault("ADMIN_USERNAME", "admin")
	if _, exists := users[adminUsername]; !exists {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(getEnvWithDefault("ADMIN_PASSWORD", "admin123")), bcrypt.DefaultCost)
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

		log.Info("Default admin user initialized")
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found in session"})
		return
	}

	user, exists := getUser(username.(string))
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "User profile not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// authMiddleware checks for valid authentication
func authMiddleware(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		c.Abort()
		return
	}

	username, valid := validateSession(sessionID)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid session"})
		c.Abort()
		return
	}

	user, exists := getUser(username)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
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
			c.JSON(http.StatusForbidden, gin.H{"error": "Role information missing"})
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid role format"})
			c.Abort()
			return
		}

		if userRole != requiredRole && userRole != "admin" { // Admin can access everything
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware limits requests to 10 per minute per IP
func rateLimitMiddleware(c *gin.Context) {
	ip := c.ClientIP()
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()
	if reset, ok := rateLimitReset[ip]; ok && now.After(reset) {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(time.Minute)
	}
	if _, ok := rateLimitCount[ip]; !ok {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(time.Minute)
	}
	rateLimitCount[ip]++
	if rateLimitCount[ip] > 10 {
		log.WithField("ip", ip).Warn("Rate limit exceeded")
		c.JSON(429, gin.H{"error": "Too many requests"})
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

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.WithField("redis", "ping").Warnf("Redis ping failed: %v", err)
	}

	// Initialize Sentry
	_ = pong
	sentryDSN := getEnvWithDefault("SENTRY_DSN", "")
	if sentryDSN != "" {
		if initErr := sentry.Init(sentry.ClientOptions{
			Dsn: sentryDSN,
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

	log.Info("Starting Bitcoin Explorer server")

	// Initialize Redis client
	redisHost := getEnvWithDefault("REDIS_HOST", "localhost")
	rdb = redis.NewClient(&redis.Options{
		Addr: redisHost + ":6379",
	})

	// Configure Redis for LRU eviction
	rdb.ConfigSet(ctx, "maxmemory", "100mb")
	rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")

	// Initialize default admin user
	initializeDefaultAdmin()

	// Serve static assets: images plus specific static files
	r.Static("/images", "./images")
	// Serve built Tailwind CSS
	r.Static("/dist", "./dist")
	r.StaticFile("/bitcoin.html", "bitcoin.html")
	r.StaticFile("/", "index.html")
	r.StaticFile("/admin", "admin.html")

	r.GET("/api/search", searchHandler)

	r.GET("/api/autocomplete", autocompleteHandler)
	r.GET("/api/metrics", metricsHandler)
	r.GET("/api/network-status", networkStatusHandler)

	r.GET("/api/rates", ratesHandler)
	r.GET("/api/price-history", priceHistoryHandler)

	r.POST("/api/feedback", feedbackHandler)

	// Price alert routes
	priceAlerts := r.Group("/api/price-alerts")
	priceAlerts.Use(authMiddleware)
	{
		priceAlerts.GET("", listPriceAlertsHandler)
		priceAlerts.POST("", createPriceAlertHandler)
		priceAlerts.DELETE("/:id", deletePriceAlertHandler)
	}
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

	go func() {
		log.Info("Starting background price alert worker")
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			checkPriceAlerts()
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	user, authenticated := authenticateUser(loginReq.Username, loginReq.Password)
	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		log.WithField("username", loginReq.Username).Warn("Failed login attempt")
		return
	}

	sessionID, err := createSession(loginReq.Username)
	if err != nil {
		log.WithError(err).Error("Failed to create session")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.SetCookie("session_id", sessionID, 86400, "/", "", false, true) // 24 hours
	c.JSON(http.StatusOK, gin.H{
		"message":  "Login successful",
		"username": loginReq.Username,
		"role":     user.Role,
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

	c.SetCookie("session_id", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logout successful"})
}

// registerHandler handles user registration
func registerHandler(c *gin.Context) {
	var registerReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email"` // Optional for now
	}

	if err := c.ShouldBindJSON(&registerReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Basic validation
	if len(registerReq.Username) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username must be at least 3 characters"})
		return
	}
	if len(registerReq.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 6 characters"})
		return
	}

	// Create user with default "user" role
	err := createUser(registerReq.Username, registerReq.Password, "user")
	if err != nil {
		if err.Error() == "user already exists" {
			c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		} else {
			log.WithError(err).Error("Failed to create user")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Store feedback in Redis with timestamp
	feedbackKey := fmt.Sprintf("feedback:%d", time.Now().Unix())
	feedbackData := map[string]interface{}{
		"name":      feedbackReq.Name,
		"email":     feedbackReq.Email,
		"message":   feedbackReq.Message,
		"timestamp": time.Now().Format(time.RFC3339),
		"ip":        c.ClientIP(),
	}

	jsonData, err := json.Marshal(feedbackData)
	if err != nil {
		log.WithError(err).Error("Failed to marshal feedback data")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process feedback"})
		return
	}

	err = rdb.Set(ctx, feedbackKey, jsonData, 30*24*time.Hour).Err() // Store for 30 days
	if err != nil {
		log.WithError(err).Error("Failed to store feedback in Redis")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save feedback"})
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
	rateLimitMutex.Lock()
	activeLimits := len(rateLimitCount)
	rateLimitMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"user":               username,
		"role":               role,
		"redis_memory":       info,
		"active_rate_limits": activeLimits,
		"timestamp":          time.Now().Unix(),
	})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get cache keys"})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get cache stats"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total_keys": len(keys),
			"keys":       keys,
		})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action. Use 'clear' or 'stats'"})
	}
}

// Portfolio management handlers

// listPortfoliosHandler returns all portfolios for the authenticated user
func listPortfoliosHandler(c *gin.Context) {
	username, _ := c.Get("username")

	keys, err := rdb.Keys(ctx, "portfolio:"+username.(string)+":*").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch portfolios"})
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

	c.JSON(http.StatusOK, portfolios)
}

// createPortfolioHandler creates a new portfolio
func createPortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")

	var p Portfolio
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	p.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	p.Username = username.(string)
	p.Created = time.Now()
	p.Updated = time.Now()

	data, _ := json.Marshal(p)
	err := rdb.Set(ctx, "portfolio:"+p.Username+":"+p.ID, data, 0).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save portfolio"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	key := "portfolio:" + username.(string) + ":" + portfolioID
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Portfolio not found"})
		return
	}

	var p Portfolio
	json.Unmarshal([]byte(data), &p)

	// Update fields
	p.Name = updateReq.Name
	p.Description = updateReq.Description
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete portfolio"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Portfolio deleted successfully"})
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
		c.JSON(400, gin.H{"error": "Missing query parameter"})
		return
	}
	if len(query) > 100 {
		log.WithField("query", query).Warn("Search request query too long")
		c.JSON(400, gin.H{"error": "Query too long"})
		return
	}
	resultType, result, err := searchBlockchain(query)
	if err != nil {
		log.WithFields(log.Fields{"query": query, "error": err}).Error("Search failed")
		if err == ErrNotFound {
			c.JSON(404, gin.H{"error": "Not found"})
		} else {
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}
	// Marshal the result to JSON for ETag calculation
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		log.WithError(err).Error("Failed to marshal search response")
		c.JSON(500, gin.H{"error": "Failed to marshal response"})
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

	// Fetch from CoinGecko API
	resp, err := httpClient.R().
		SetHeader("Accept", "application/json").
		Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,eur,gbp,jpy")
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	var rates map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rates); err != nil {
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

var (
	baseURL = getEnvWithDefault("GETBLOCK_BASE_URL", "https://go.getblock.io/eb8cb69423354abb8d5e489adfc54742")
	apiKey  = getEnvWithDefault("GETBLOCK_ACCESS_TOKEN", "eb8cb69423354abb8d5e489adfc54742")
	// httpClient is injectable for tests; production code uses a default resty client
	httpClient = resty.New().
			SetTimeout(10 * time.Second).
			SetRetryCount(3)
)

// SetHTTPClient allows tests or other packages to replace the internal HTTP client used for API calls.
func SetHTTPClient(c *resty.Client) {
	if c != nil {
		httpClient = c
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// handleError standardizes error responses
func handleError(c *gin.Context, err error, status int) {
	sentry.CaptureException(err)
	c.JSON(status, gin.H{"error": err.Error()})
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
	// Fetch block height
	blockCountResp, err := blockchairRequest("getblockcount", []interface{}{})
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
	difficultyResp, err := blockchairRequest("getdifficulty", []interface{}{})
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
	hashRateResp, err := blockchairRequest("getnetworkhashps", []interface{}{})
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
	fmt.Println("Setting cache for", cacheKey)
	err = rdb.Set(context.Background(), cacheKey, resultJSON, 1*time.Minute).Err()
	if err != nil {
		fmt.Println("Redis set error:", err)
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

	response, err := blockchairRequest("getaddressinfo", []interface{}{address})
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

	response, err := blockchairRequest("getrawtransaction", []interface{}{txID, 1})
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
	response, err := blockchairRequest("getblockbyheight", []interface{}{height, 1})
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

// collectMetrics collects historical metrics for charts
func collectMetrics() {
	// Use a float64 timestamp for Redis scores
	now := float64(time.Now().Unix())

	// Collect Bitcoin price for history
	resp, err := httpClient.R().
		SetHeader("Accept", "application/json").
		Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd")
	if err == nil {
		var rates map[string]map[string]float64
		if err := json.Unmarshal(resp.Body(), &rates); err == nil {
			if btc, ok := rates["bitcoin"]; ok {
				if usd, ok := btc["usd"]; ok {
					rdb.ZAdd(context.Background(), "btc_price_history", redis.Z{Score: now, Member: usd})
					// Keep only last 30 days of 5-minute data (roughly 8640 points)
					rdb.ZRemRangeByRank(context.Background(), "btc_price_history", 0, -8641)
				}
			}
		}
	}

	// Get mempool size
	mempoolResp, err := blockchairRequest("getmempoolinfo", []interface{}{})
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
	blocksResp, err := blockchairRequest("getblockchaininfo", []interface{}{})
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
					blockResp, err := blockchairRequest("getblockhash", []interface{}{h})
					if err != nil {
						continue
					}
					var hashData map[string]interface{}
					_ = json.Unmarshal(blockResp.Body(), &hashData)
					if hash, ok := hashData["result"].(string); ok {
						blockDetailResp, err := blockchairRequest("getblock", []interface{}{hash})
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

// Price alert handlers and worker

func listPriceAlertsHandler(c *gin.Context) {
	username, _ := c.Get("username")

	keys, err := rdb.Keys(ctx, fmt.Sprintf("alerts:%s:*", username)).Result()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	alerts := []PriceAlert{}
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var alert PriceAlert
		if err := json.Unmarshal([]byte(data), &alert); err == nil {
			alerts = append(alerts, alert)
		}
	}

	c.JSON(http.StatusOK, alerts)
}

func createPriceAlertHandler(c *gin.Context) {
	username, _ := c.Get("username")

	var req struct {
		Asset       string  `json:"asset" binding:"required"`
		TargetPrice float64 `json:"target_price" binding:"required"`
		Currency    string  `json:"currency" binding:"required"`
		Condition   string  `json:"condition" binding:"required"` // "above" or "below"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if req.Condition != "above" && req.Condition != "below" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Condition must be 'above' or 'below'"})
		return
	}

	alertID := fmt.Sprintf("%d", time.Now().UnixNano())
	alert := PriceAlert{
		ID:          alertID,
		Username:    username.(string),
		Asset:       req.Asset,
		TargetPrice: req.TargetPrice,
		Currency:    strings.ToLower(req.Currency),
		Condition:   req.Condition,
		IsActive:    true,
		Created:     time.Now(),
	}

	data, _ := json.Marshal(alert)
	err := rdb.Set(ctx, fmt.Sprintf("alerts:%s:%s", username, alertID), data, 0).Err()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusCreated, alert)
}

func deletePriceAlertHandler(c *gin.Context) {
	username, _ := c.Get("username")
	alertID := c.Param("id")

	err := rdb.Del(ctx, fmt.Sprintf("alerts:%s:%s", username, alertID)).Err()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert deleted"})
}

func checkPriceAlerts() {
	log.Debug("Checking price alerts")

	// Get Bitcoin price
	resp, err := httpClient.R().
		SetHeader("Accept", "application/json").
		Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,eur,gbp,jpy")

	if err != nil {
		log.WithError(err).Error("Failed to fetch prices for alerts")
		return
	}

	var rates map[string]map[string]float64
	if err := json.Unmarshal(resp.Body(), &rates); err != nil {
		log.WithError(err).Error("Failed to unmarshal prices for alerts")
		return
	}

	btcRates := rates["bitcoin"]

	// Get all alert keys
	keys, err := rdb.Keys(ctx, "alerts:*:*").Result()
	if err != nil {
		log.WithError(err).Error("Failed to list alert keys")
		return
	}

	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var alert PriceAlert
		if err := json.Unmarshal([]byte(data), &alert); err != nil {
			continue
		}

		if !alert.IsActive {
			continue
		}

		currentPrice, ok := btcRates[alert.Currency]
		if !ok {
			continue
		}

		triggered := false
		if alert.Condition == "above" && currentPrice >= alert.TargetPrice {
			triggered = true
		} else if alert.Condition == "below" && currentPrice <= alert.TargetPrice {
			triggered = true
		}

		if triggered {
			now := time.Now()
			alert.IsActive = false
			alert.TriggeredAt = &now

			// Update in Redis
			updatedData, _ := json.Marshal(alert)
			rdb.Set(ctx, key, updatedData, 0)

			// Log notification (proper notification system could involve email/webhook/push)
			log.WithFields(log.Fields{
				"username":     alert.Username,
				"asset":        alert.Asset,
				"target":       alert.TargetPrice,
				"current":      currentPrice,
				"currency":     alert.Currency,
				"condition":    alert.Condition,
			}).Info("Price Alert Triggered!")
		}
	}
}
