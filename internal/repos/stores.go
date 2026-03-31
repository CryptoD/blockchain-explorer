package repos

import (
	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
)

// Stores holds all domain repositories sharing one Redis client.
type Stores struct {
	RDB       redisstore.Client
	Portfolio *PortfolioRepo
	Watchlist *WatchlistRepo
	Session   *SessionRepo
	User      *UserRepo
	Feedback  *FeedbackRepo
	Admin     *AdminRepo
}

// NewStores wires repositories for the given Redis client (may be nil in tests).
func NewStores(rdb redisstore.Client) *Stores {
	return &Stores{
		RDB:       rdb,
		Portfolio: &PortfolioRepo{RDB: rdb},
		Watchlist: &WatchlistRepo{RDB: rdb},
		Session:   &SessionRepo{RDB: rdb},
		User:      &UserRepo{RDB: rdb},
		Feedback:  &FeedbackRepo{RDB: rdb},
		Admin:     &AdminRepo{RDB: rdb},
	}
}
