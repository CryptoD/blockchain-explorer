// Entry point for the HTTP API and web UI. Application logic lives in package server.
// Do not import domain packages (e.g. internal/news) here; compose them only in internal/server.
package main

import (
	"os"

	"github.com/CryptoD/blockchain-explorer/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		os.Exit(1)
	}
}
