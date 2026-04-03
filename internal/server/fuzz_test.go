package server

// Fuzz tests for blockchain string validators (ROADMAP task 23).
// Run one target at a time: go test ./internal/server -fuzz=FuzzIsValidTransactionID -fuzztime=30s
//
//	go test ./internal/server -fuzz=FuzzIsValidAddress -fuzztime=30s
//	go test ./internal/server -fuzz=FuzzIsValidBlockHeight -fuzztime=30s

import (
	"strings"
	"testing"
)

func FuzzIsValidTransactionID(f *testing.F) {
	f.Add(strings.Repeat("a", 64))
	f.Fuzz(func(t *testing.T, txid string) {
		_ = isValidTransactionID(txid)
	})
}

func FuzzIsValidAddress(f *testing.F) {
	f.Add("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa")
	f.Fuzz(func(t *testing.T, addr string) {
		_ = isValidAddress(addr)
	})
}

func FuzzIsValidBlockHeight(f *testing.F) {
	f.Add("12345")
	f.Fuzz(func(t *testing.T, s string) {
		_ = isValidBlockHeight(s)
	})
}
