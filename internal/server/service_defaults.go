package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/CryptoD/blockchain-explorer/internal/repos"
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
	if appRepos == nil || appRepos.Portfolio == nil {
		return nil, errors.New("redis unavailable")
	}
	raw, err := appRepos.Portfolio.ListJSON(ctx, username)
	if err != nil {
		if errors.Is(err, repos.ErrNotConfigured) {
			return nil, errors.New("redis unavailable")
		}
		return nil, err
	}
	out := make([]Portfolio, 0, len(raw))
	for _, b := range raw {
		var p Portfolio
		if err := json.Unmarshal(b, &p); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (defaultPortfolioService) GetPortfolio(ctx context.Context, username, id string) (*Portfolio, error) {
	if appRepos == nil || appRepos.Portfolio == nil {
		return nil, errors.New("redis unavailable")
	}
	data, err := appRepos.Portfolio.Get(ctx, username, id)
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var p Portfolio
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (defaultPortfolioService) SavePortfolio(ctx context.Context, p *Portfolio) error {
	if appRepos == nil || appRepos.Portfolio == nil {
		return errors.New("redis unavailable")
	}
	return appRepos.Portfolio.Save(ctx, p.Username, p.ID, p)
}

func (defaultPortfolioService) DeletePortfolio(ctx context.Context, username, id string) error {
	if appRepos == nil || appRepos.Portfolio == nil {
		return errors.New("redis unavailable")
	}
	return appRepos.Portfolio.Delete(ctx, username, id)
}

type defaultWatchlistService struct{}

func (defaultWatchlistService) ListWatchlists(ctx context.Context, username string) ([]Watchlist, error) {
	if appRepos == nil || appRepos.Watchlist == nil {
		return nil, errors.New("redis unavailable")
	}
	raw, err := appRepos.Watchlist.ListJSON(ctx, username)
	if err != nil {
		if errors.Is(err, repos.ErrNotConfigured) {
			return nil, errors.New("redis unavailable")
		}
		return nil, err
	}
	out := make([]Watchlist, 0, len(raw))
	for _, b := range raw {
		var w Watchlist
		if err := json.Unmarshal(b, &w); err == nil {
			out = append(out, w)
		}
	}
	return out, nil
}

func (defaultWatchlistService) GetWatchlist(ctx context.Context, username, id string) (*Watchlist, error) {
	if appRepos == nil || appRepos.Watchlist == nil {
		return nil, errors.New("redis unavailable")
	}
	data, err := appRepos.Watchlist.Get(ctx, username, id)
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var w Watchlist
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (defaultWatchlistService) CountWatchlists(ctx context.Context, username string) (int, error) {
	if appRepos == nil || appRepos.Watchlist == nil {
		return 0, errors.New("redis unavailable")
	}
	n, err := appRepos.Watchlist.Count(ctx, username)
	if err != nil && errors.Is(err, repos.ErrNotConfigured) {
		return 0, errors.New("redis unavailable")
	}
	return n, err
}

func (defaultWatchlistService) SaveWatchlist(ctx context.Context, w *Watchlist) error {
	if appRepos == nil || appRepos.Watchlist == nil {
		return errors.New("redis unavailable")
	}
	return appRepos.Watchlist.Save(ctx, w.Username, w.ID, w)
}

func (defaultWatchlistService) DeleteWatchlist(ctx context.Context, username, id string) error {
	if appRepos == nil || appRepos.Watchlist == nil {
		return errors.New("redis unavailable")
	}
	return appRepos.Watchlist.Delete(ctx, username, id)
}

type defaultAlertService struct{}

func (defaultAlertService) EvaluateAll(ctx context.Context) {
	_ = ctx
	evaluatePriceAlerts()
}

type defaultAdminService struct{}

func (defaultAdminService) RedisMemoryInfo(ctx context.Context) string {
	if appRepos == nil || appRepos.Admin == nil {
		return ""
	}
	return appRepos.Admin.MemoryInfo(ctx)
}

func (defaultAdminService) ActiveRateLimitEntries(ctx context.Context) int {
	_ = ctx
	return getActiveRateLimitCount()
}

func (defaultAdminService) ListCacheKeys(ctx context.Context) ([]string, error) {
	if appRepos == nil || appRepos.Admin == nil {
		return nil, errors.New("redis unavailable")
	}
	keys, err := appRepos.Admin.ListAllKeys(ctx)
	if err != nil && errors.Is(err, repos.ErrNotConfigured) {
		return nil, errors.New("redis unavailable")
	}
	return keys, err
}

func (defaultAdminService) DeleteCacheKeys(ctx context.Context, keys []string) error {
	if appRepos == nil || appRepos.Admin == nil {
		return errors.New("redis unavailable")
	}
	err := appRepos.Admin.DeleteKeys(ctx, keys)
	if err != nil && errors.Is(err, repos.ErrNotConfigured) {
		return errors.New("redis unavailable")
	}
	return err
}

type defaultFeedbackService struct{}

func (defaultFeedbackService) Store(ctx context.Context, name, emailAddr, message, clientIP string) error {
	if appRepos == nil || appRepos.Feedback == nil {
		return errors.New("redis unavailable")
	}
	err := appRepos.Feedback.Store(ctx, name, emailAddr, message, clientIP)
	if err != nil && errors.Is(err, repos.ErrNotConfigured) {
		return errors.New("redis unavailable")
	}
	return err
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
