package main

import (
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
)

var ErrNotFound = errors.New("not found")

func main() {
	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.File("index.html")
	})

	router.GET("/api/search", func(c *gin.Context) {
		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
			return
		}

		resultType, data, err := searchBlockchain(query)
		if err != nil {
			if err == ErrNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}

		response := map[string]interface{}{
			"resultType": resultType,
			"data":       data,
		}

		c.JSON(http.StatusOK, response)
	})

	router.Run(":8080")
}

func searchBlockchain(query string) (string, map[string]interface{}, error) {
	// Validate the query first
	var method string
	var params []interface{}

	// Determine the type of query and set appropriate method
	switch {
	case isValidAddress(query):
		method = "getaddressinfo"
		params = []interface{}{query}
	case isValidTransactionID(query):
		method = "getrawtransaction"
		params = []interface{}{query, 1} // 1 means verbose output
	case isValidBlockHeight(query):
		method = "getblockbyheight"
		height, _ := strconv.Atoi(query)
		params = []interface{}{height, 1} // 1 means verbose output
	default:
		return "", nil, fmt.Errorf("invalid query format")
	}

	// Make JSON-RPC request to GetBlock
	response, err := blockchairRequest(method, params)
	if err != nil {
		return "", nil, err
	}

	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return "", nil, err
	}

	// Check for JSON-RPC error
	if errorObj, ok := result["error"]; ok {
		return "", nil, fmt.Errorf("JSON-RPC error: %v", errorObj)
	}

	// Determine result type based on method
	var resultType string
	switch method {
	case "getaddressinfo":
		resultType = "address"
	case "getrawtransaction":
		resultType = "transaction"
	case "getblockbyheight":
		resultType = "block"
	}

	return resultType, result, nil
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	resultType, data, err := searchBlockchain(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"resultType": resultType,
		"data":       data,
	}

	json.NewEncoder(w).Encode(response)
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

func getAddressDetails(address string) (map[string]interface{}, error) {
	response, err := blockchairRequest("getaddressinfo", []interface{}{address})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

	return result, nil
}

func getTransactionDetails(txID string) (map[string]interface{}, error) {
	response, err := blockchairRequest("getrawtransaction", []interface{}{txID, 1})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

	return result, nil
}

func getBlockDetails(blockHeight string) (map[string]interface{}, error) {
	height, _ := strconv.Atoi(blockHeight)
	response, err := blockchairRequest("getblockbyheight", []interface{}{height, 1})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

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
