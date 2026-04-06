package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apperrors"
	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

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
	return pricing.USDPerUnitOfFiatFromBTCRates(rates, fiatCode)
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
			SetTimeout(30 * time.Second).
			SetRetryCount(3)
	// blockchainClient is the pluggable blockchain data provider (nil uses baseURL/apiKey/httpClient).
	blockchainClient blockchain.Blockchain
	// pricingClient is the pluggable pricing/FX provider.
	pricingClient pricing.Client
	// assetPricer unifies crypto, commodity, and bond pricing for portfolio valuation.
	assetPricer pricing.AssetPricer
	// emailTemplates is set in Run when SMTP is configured (see also EmailAppService in service_interfaces.go).
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

// SetBlockchainClient allows tests to inject a mock blockchain JSON-RPC client.
func SetBlockchainClient(c blockchain.Blockchain) {
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

// errorResponseFrom maps domain errors to stable API codes without leaking raw errors to clients.
func errorResponseFrom(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var apiErr *apperrors.Error
	if errors.As(err, &apiErr) {
		errorResponse(c, apiErr.Status, apiErr.Code, apiErr.Message)
		return
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		errorResponse(c, http.StatusNotFound, apperrors.CodeNotFound, "Not found")
		return
	}
	errorResponse(c, http.StatusInternalServerError, apperrors.CodeInternal, "An internal error occurred")
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
	msg := err.Error()
	if status >= http.StatusInternalServerError {
		msg = "An internal error occurred"
	}
	errorResponse(c, status, defaultErrorCode(status), msg)
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

const defaultBlockchainRPCTimeout = 30 * time.Second

func blockchainForCall() blockchain.Blockchain {
	if blockchainClient != nil {
		return blockchainClient
	}
	return blockchain.NewGetBlockRPCClient(baseURL, apiKey, httpClient)
}

func ensureRPCDeadline(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// callBlockchain performs a JSON-RPC call through the configured Blockchain client,
// applies a default deadline when ctx has none, and records Prometheus metrics.
func callBlockchain(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
	bc := blockchainForCall()
	ctx, cancel := ensureRPCDeadline(ctx, defaultBlockchainRPCTimeout)
	defer cancel()
	start := time.Now()
	resp, err := bc.Call(ctx, method, params)
	metrics.RecordBlockchainRPC(method, time.Since(start), err)
	return resp, err
}

// Redis key for historical BTC price in a fiat currency (e.g. btc_price_history:usd).
func btcPriceHistoryKey(currency string) string {
	return "btc_price_history:" + strings.ToLower(currency)
}

const btcPriceHistoryMaxPoints = 8640 // ~30 days at 5-min interval

// collectMetrics collects historical metrics for charts, including multi-currency FX for portfolio performance.
