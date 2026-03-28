package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type defaultExplorerService struct{}

func (defaultExplorerService) SearchBlockchain(ctx context.Context, query string) (string, map[string]interface{}, error) {
	_ = ctx
	return searchBlockchain(query)
}

func (defaultExplorerService) GetNetworkStatus(ctx context.Context) (map[string]interface{}, error) {
	_ = ctx
	return getNetworkStatus()
}

type defaultAuthService struct{}

func (defaultAuthService) AuthenticateUser(username, password string) (User, bool) {
	return authenticateUser(username, password)
}

func (defaultAuthService) CreateSession(username string) (string, error) {
	return createSession(username)
}

func (defaultAuthService) CreateOrUpdateCSRFToken(sessionID string) (string, error) {
	return createOrUpdateCSRFToken(sessionID)
}

func (defaultAuthService) DestroySession(sessionID string) {
	destroySession(sessionID)
}

func (defaultAuthService) CreateUser(username, password, role, email string) error {
	return createUser(username, password, role, email)
}

type defaultPortfolioService struct{}

func (defaultPortfolioService) ListPortfolios(ctx context.Context, username string) ([]Portfolio, error) {
	if rdb == nil {
		return nil, errors.New("redis unavailable")
	}
	keys, err := rdb.Keys(ctx, "portfolio:"+username+":*").Result()
	if err != nil {
		return nil, err
	}
	out := make([]Portfolio, 0, len(keys))
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var p Portfolio
		if err := json.Unmarshal([]byte(data), &p); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (defaultPortfolioService) GetPortfolio(ctx context.Context, username, id string) (*Portfolio, error) {
	if rdb == nil {
		return nil, errors.New("redis unavailable")
	}
	key := "portfolio:" + username + ":" + id
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var p Portfolio
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (defaultPortfolioService) SavePortfolio(ctx context.Context, p *Portfolio) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, "portfolio:"+p.Username+":"+p.ID, data, 0).Err()
}

func (defaultPortfolioService) DeletePortfolio(ctx context.Context, username, id string) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	return rdb.Del(ctx, "portfolio:"+username+":"+id).Err()
}

type defaultWatchlistService struct{}

func (defaultWatchlistService) ListWatchlists(ctx context.Context, username string) ([]Watchlist, error) {
	if rdb == nil {
		return nil, errors.New("redis unavailable")
	}
	keys, err := rdb.Keys(ctx, watchlistKey(username, "*")).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Watchlist, 0, len(keys))
	for _, key := range keys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var w Watchlist
		if err := json.Unmarshal([]byte(data), &w); err == nil {
			out = append(out, w)
		}
	}
	return out, nil
}

func (defaultWatchlistService) GetWatchlist(ctx context.Context, username, id string) (*Watchlist, error) {
	if rdb == nil {
		return nil, errors.New("redis unavailable")
	}
	key := watchlistKey(username, id)
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var w Watchlist
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (defaultWatchlistService) CountWatchlists(ctx context.Context, username string) (int, error) {
	return getWatchlistCount(ctx, username)
}

func (defaultWatchlistService) SaveWatchlist(ctx context.Context, w *Watchlist) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	key := watchlistKey(w.Username, w.ID)
	return rdb.Set(ctx, key, data, 0).Err()
}

func (defaultWatchlistService) DeleteWatchlist(ctx context.Context, username, id string) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	return rdb.Del(ctx, watchlistKey(username, id)).Err()
}

type defaultAlertService struct{}

func (defaultAlertService) EvaluateAll(ctx context.Context) {
	_ = ctx
	evaluatePriceAlerts()
}

type defaultAdminService struct{}

func (defaultAdminService) RedisMemoryInfo(ctx context.Context) string {
	if rdb == nil {
		return ""
	}
	return rdb.Info(ctx, "memory").Val()
}

func (defaultAdminService) ActiveRateLimitEntries(ctx context.Context) int {
	_ = ctx
	return getActiveRateLimitCount()
}

func (defaultAdminService) ListCacheKeys(ctx context.Context) ([]string, error) {
	if rdb == nil {
		return nil, errors.New("redis unavailable")
	}
	return rdb.Keys(ctx, "*").Result()
}

func (defaultAdminService) DeleteCacheKeys(ctx context.Context, keys []string) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	if len(keys) == 0 {
		return nil
	}
	return rdb.Del(ctx, keys...).Err()
}

type defaultFeedbackService struct{}

func (defaultFeedbackService) Store(ctx context.Context, name, emailAddr, message, clientIP string) error {
	if rdb == nil {
		return errors.New("redis unavailable")
	}
	feedbackKey := fmt.Sprintf("feedback:%d", time.Now().Unix())
	feedbackData := map[string]interface{}{
		"name":      name,
		"email":     emailAddr,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
		"ip":        clientIP,
	}
	jsonData, err := json.Marshal(feedbackData)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, feedbackKey, jsonData, 30*24*time.Hour).Err()
}

// ResetDefaultServices restores production default implementations for all domain services.
// Call from tests (e.g. TestMain) after swapping mocks.
func ResetDefaultServices() {
	explorerSvc = &defaultExplorerService{}
	authSvc = &defaultAuthService{}
	portfolioSvc = &defaultPortfolioService{}
	watchlistSvc = &defaultWatchlistService{}
	alertSvc = &defaultAlertService{}
	adminSvc = &defaultAdminService{}
	feedbackSvc = &defaultFeedbackService{}
}
