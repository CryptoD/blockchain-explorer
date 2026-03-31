// Package repos centralizes Redis key prefixes and builders for the explorer (task 6).
package repos

import (
	"fmt"
	"strings"
)

// Key prefixes (single source of truth for stringly-typed Redis keys).
const (
	PrefixPortfolio      = "portfolio:"
	PrefixWatchlist      = "watchlist:"
	PrefixUser           = "user:"
	PrefixSession        = "session:"
	PrefixCSRF           = "csrf:"
	PrefixAlert          = "alert:"
	PrefixNotification   = "notification:"
	PrefixFeedback       = "feedback:"
	PrefixAddressCache   = "address:"
	PrefixTxCache        = "tx:"
	PrefixBlockCache     = "block:"
	PrefixNetworkStatus  = "network:status"
	PrefixRatesBTC       = "rates:btc"
	PrefixLatestBlocks   = "latest_blocks"
	PrefixLatestTxs      = "latest_transactions"
	PrefixUserKeyPattern = "user:*"
	PrefixSessionPattern = "session:*"
)

// PortfolioKey returns portfolio:{username}:{id}.
func PortfolioKey(username, id string) string {
	return PrefixPortfolio + username + ":" + id
}

// WatchlistKey returns watchlist:{username}:{id}.
func WatchlistKey(username, id string) string {
	return PrefixWatchlist + username + ":" + id
}

// UserKey returns user:{username}.
func UserKey(username string) string {
	return PrefixUser + username
}

// SessionKey returns session:{sessionID}.
func SessionKey(sessionID string) string {
	return PrefixSession + sessionID
}

// CSRFKey returns csrf:{sessionID}.
func CSRFKey(sessionID string) string {
	return PrefixCSRF + sessionID
}

// UsernameFromUserKey strips the "user:" prefix from a full Redis key.
func UsernameFromUserKey(key string) (string, bool) {
	if !strings.HasPrefix(key, PrefixUser) {
		return "", false
	}
	return strings.TrimPrefix(key, PrefixUser), true
}

// FeedbackKey returns a time-bucket feedback key (matches prior server behavior).
func FeedbackKey(unixSec int64) string {
	return fmt.Sprintf("%s%d", PrefixFeedback, unixSec)
}
