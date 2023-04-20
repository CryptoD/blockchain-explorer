package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

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
	// Make an API call to Blockchair to get information about the Bitcoin address
	apiUrl := fmt.Sprintf("https://api.blockchair.com/bitcoin/dashboards/address/%s", query)
	resp, err := http.Get(apiUrl)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", nil, err
	}

	return "address", data, nil
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

const blockchairBaseURL = "https://api.blockchair.com/bitcoin"

func isValidAddress(address string) bool {
	// Bitcoin addresses are usually 26-35 characters long.
	if len(address) >= 26 && len(address) <= 35 {
		return true
	}
	return false
}

func getAddressDetails(address string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/dashboards/address/%s", blockchairBaseURL, address)
	response, err := blockchairRequest(url)

	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

	return result, nil
}

func isValidTransactionID(txID string) bool {
	// Bitcoin transaction IDs are 64 characters long hexadecimal strings.
	if len(txID) == 64 {
		return true
	}
	return false
}

func getTransactionDetails(txID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/dashboards/transaction/%s", blockchairBaseURL, txID)
	response, err := blockchairRequest(url)

	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

	return result, nil
}

func isValidBlockHeight(blockHeight string) bool {
	// A simple check to see if the blockHeight string can be converted to an integer
	_, err := strconv.Atoi(blockHeight)
	return err == nil
}

func getBlockDetails(blockHeight string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/dashboards/block/%s", blockchairBaseURL, blockHeight)
	response, err := blockchairRequest(url)

	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(response.Body(), &result)

	return result, nil
}

func blockchairRequest(url string) (*resty.Response, error) {
	client := resty.New()

	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		Get(url)

	return response, err
}
