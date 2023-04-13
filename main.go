package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

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

func main2() {
	// Read environment variables
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	// Connect to the PostgreSQL database server
	connectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	router := gin.Default()

	// Your routes and handlers go here

	router.Run(":3000") // Listen on port 3000
}
