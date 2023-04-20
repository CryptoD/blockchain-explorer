package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

func main() {
	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.File("index.html")
	})

	router.Run(":8080")

	// Connect to Ethereum node
	client, err := ethclient.Dial("https://delicate-twilight-silence.discover.quiknode.pro/d07e2d37b61844a0677b85210c8074b063e3b3d8/")
	if err != nil {
		log.Fatal(err)
	}

	router.GET("/balance/:address", func(c *gin.Context) {
		address := c.Param("address")

		// Validate Ethereum address
		if !common.IsHexAddress(address) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid address"})
			return
		}

		account := common.HexToAddress(address)

		// Get account balance
		balance, err := client.BalanceAt(c, account, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"balance": fmt.Sprintf("%d", balance)})
	})

	router.Run(":3000") // Listen on port 3000
}

func searchBlockchain(query string) (string, map[string]interface{}, error) {
	// Check if the query is an address, transaction ID, or block height
	if isValidAddress(query) {
		data, err := getAddressDetails(query)
		return "address", data, err
	} else if isValidTransactionID(query) {
		data, err := getTransactionDetails(query)
		return "transaction", data, err
	} else if isValidBlockHeight(query) {
		data, err := getBlockDetails(query)
		return "block", data, err
	} else {
		return "", nil, errors.New("invalid search query")
	}
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
