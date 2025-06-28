package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	cache "github.com/patrickmn/go-cache"
)

var ErrNotFound = errors.New("not found")
var appCache = cache.New(5*time.Minute, 10*time.Minute)

func main() {
	r := gin.Default()

	r.Static("/images", "./images")
	r.StaticFile("/bitcoin.html", "bitcoin.html")
	r.StaticFile("/", "index.html")

	r.GET("/api/search", searchHandler)

	r.GET("/bitcoin", func(c *gin.Context) {
		query := c.Query("q")
		c.Redirect(http.StatusFound, "/bitcoin.html?q="+query)
	})

	r.Run(":8080")
}

func searchBlockchain(query string) (string, map[string]interface{}, error) {
	// ... existing code ...
	return "", nil, ErrNotFound
}

// Fix 1: Refactor searchHandler to use Gin's context
func searchHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(400, gin.H{"error": "Missing query parameter"})
		return
	}
	resultType, result, err := searchBlockchain(query)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(404, gin.H{"error": "Not found"})
		} else {
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}
	// Marshal the result to JSON for ETag calculation
	jsonBytes, _ := json.Marshal(result)
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(jsonBytes))
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60")
	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Status(304)
		return
	}
	c.JSON(200, gin.H{"type": resultType, "result": result})
}

var (
	baseURL = getEnvWithDefault("GETBLOCK_BASE_URL", "https://go.getblock.io/eb8cb69423354abb8d5e489adfc54742")
	apiKey  = getEnvWithDefault("GETBLOCK_ACCESS_TOKEN", "eb8cb69423354abb8d5e489adfc54742")
)

func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// handleError standardizes error responses
func handleError(c *gin.Context, err error, status int) {
	c.JSON(status, gin.H{"error": err.Error()})
}

func isValidAddress(address string) bool {
	// Bitcoin addresses are usually 26-35 characters long and start with specific characters
	if len(address) < 26 || len(address) > 35 {
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

func getNetworkStatus() (map[string]interface{}, error) {
	cacheKey := "network:status"
	if cached, found := appCache.Get(cacheKey); found {
		return cached.(map[string]interface{}), nil
	}
	// Example: Fetch latest block count; customize as needed
	params := []interface{}{}
	resp, err := blockchairRequest("getblockcount", params)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}
	appCache.Set(cacheKey, result, 10*time.Second) // Short TTL for fast-changing data
	return result, nil
}

func getAddressDetails(address string) (map[string]interface{}, error) {
	cacheKey := "address:" + address
	if cached, found := appCache.Get(cacheKey); found {
		if data, ok := cached.(map[string]interface{}); ok {
			return data, nil
		}
	}

	response, err := blockchairRequest("getaddressinfo", []interface{}{address})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, err
	}

	appCache.Set(cacheKey, result, 1*time.Minute) // Cache for 1 minute
	return result, nil
}

func getTransactionDetails(txID string) (map[string]interface{}, error) {
	cacheKey := "tx:" + txID
	if cached, found := appCache.Get(cacheKey); found {
		if data, ok := cached.(map[string]interface{}); ok {
			return data, nil
		}
	}

	response, err := blockchairRequest("getrawtransaction", []interface{}{txID, 1})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, err
	}

	appCache.Set(cacheKey, result, 5*time.Minute) // Cache for 5 minutes
	return result, nil
}

func getBlockDetails(blockHeight string) (map[string]interface{}, error) {
	cacheKey := "block:" + blockHeight
	if cached, found := appCache.Get(cacheKey); found {
		if data, ok := cached.(map[string]interface{}); ok {
			return data, nil
		}
	}

	height, _ := strconv.Atoi(blockHeight)
	response, err := blockchairRequest("getblockbyheight", []interface{}{height, 1})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, err
	}

	appCache.Set(cacheKey, result, 5*time.Minute) // Cache for 5 minutes
	return result, nil
}

func blockchairRequest(method string, params []interface{}) (*resty.Response, error) {
	if baseURL == "" || apiKey == "" {
		return nil, errors.New("missing required environment variables")
	}

	client := resty.New().
		SetTimeout(10 * time.Second).
		SetRetryCount(3)

	// Generate a unique ID for this request
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}

	response, err := client.R().
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
