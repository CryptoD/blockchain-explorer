package server

import (
	"strings"
	"testing"
)

// Table-driven tests for validation helpers (ROADMAP task 18).

func TestIsStrongPassword_Table(t *testing.T) {
	tests := []struct {
		password string
		want     bool
	}{
		{"Str0ngOk", true},
		{"abc12345", true},
		{"Password1", true},
		{"a1bbbbbb", true},
		{"", false},
		{"short1", false}, // len < 8
		{"NoDigits!", false},
		{"12345678", false},                  // no letter
		{"abcdefgh", false},                  // no digit
		{strings.Repeat("a", 129), false},    // > 128
		{strings.Repeat("a", 7) + "1", true}, // exactly 8
		{strings.Repeat("a", 127) + "1", true},
	}
	for _, tt := range tests {
		got := isStrongPassword(tt.password)
		if got != tt.want {
			t.Errorf("isStrongPassword(%q) = %v, want %v", tt.password, got, tt.want)
		}
	}
}

func TestValidateWatchlistEntry_Table(t *testing.T) {
	longSymbol := strings.Repeat("s", maxEntrySymbolLen+1)
	longAddr := strings.Repeat("1", maxEntryAddressLen+1)

	tests := []struct {
		name    string
		entry   WatchlistEntry
		wantErr string // substring of error, empty if nil
	}{
		{
			name: "symbol_ok",
			entry: WatchlistEntry{
				Type: "symbol", Symbol: "bitcoin",
			},
		},
		{
			name: "address_ok",
			entry: WatchlistEntry{
				Type: "address", Address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			},
		},
		{
			name: "type_uppercase_normalized",
			entry: WatchlistEntry{
				Type: "SYMBOL", Symbol: "eth",
			},
		},
		{
			name:    "invalid_type",
			entry:   WatchlistEntry{Type: "nft", Symbol: "x"},
			wantErr: "type must be symbol or address",
		},
		{
			name:    "symbol_missing",
			entry:   WatchlistEntry{Type: "symbol", Symbol: ""},
			wantErr: "symbol must be",
		},
		{
			name:    "symbol_too_long",
			entry:   WatchlistEntry{Type: "symbol", Symbol: longSymbol},
			wantErr: "symbol must be",
		},
		{
			name:    "address_missing",
			entry:   WatchlistEntry{Type: "address", Address: ""},
			wantErr: "address must be",
		},
		{
			name:    "address_too_long",
			entry:   WatchlistEntry{Type: "address", Address: longAddr},
			wantErr: "address must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.entry
			err := validateWatchlistEntry(0, &e)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
