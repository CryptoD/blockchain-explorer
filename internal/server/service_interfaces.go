package server

import (
	"context"

	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/news"
)

// Domain service interfaces (ROADMAP task 4): handlers depend on these for testability.
// Defaults are wired in init via ResetDefaultServices; production assigns *news.Service and *email.Service in Run.
// The aggregated graph lives in Dependencies (deps.go), built via NewDependencies and applied in Run (task 8).

// ExplorerService covers blockchain search and network status used by explorer HTTP handlers.
type ExplorerService interface {
	SearchBlockchain(ctx context.Context, query string) (kind string, data map[string]interface{}, err error)
	GetNetworkStatus(ctx context.Context) (map[string]interface{}, error)
}

// AuthService covers session-backed login, registration, and CSRF/session teardown.
type AuthService interface {
	AuthenticateUser(username, password string) (User, bool)
	CreateSession(username string) (sessionID string, err error)
	CreateOrUpdateCSRFToken(sessionID string) (token string, err error)
	DestroySession(sessionID string)
	CreateUser(username, password, role, email string) error
}

// PortfolioService persists user portfolios in Redis (keys portfolio:{user}:{id}).
type PortfolioService interface {
	ListPortfolios(ctx context.Context, username string) ([]Portfolio, error)
	GetPortfolio(ctx context.Context, username, id string) (*Portfolio, error)
	SavePortfolio(ctx context.Context, p *Portfolio) error
	DeletePortfolio(ctx context.Context, username, id string) error
}

// WatchlistService persists user watchlists (keys watchlist:{user}:{id}).
type WatchlistService interface {
	ListWatchlists(ctx context.Context, username string) ([]Watchlist, error)
	GetWatchlist(ctx context.Context, username, id string) (*Watchlist, error)
	CountWatchlists(ctx context.Context, username string) (int, error)
	SaveWatchlist(ctx context.Context, w *Watchlist) error
	DeleteWatchlist(ctx context.Context, username, id string) error
}

// AlertService runs background price-alert evaluation (delegates to existing evaluator).
type AlertService interface {
	EvaluateAll(ctx context.Context)
}

// AdminService covers admin-only Redis introspection and cache operations.
type AdminService interface {
	RedisMemoryInfo(ctx context.Context) string
	ActiveRateLimitEntries(ctx context.Context) int
	ListCacheKeys(ctx context.Context) ([]string, error)
	DeleteCacheKeys(ctx context.Context, keys []string) error
}

// FeedbackService stores anonymous feedback blobs in Redis.
type FeedbackService interface {
	Store(ctx context.Context, name, email, message, clientIP string) error
}

// NewsAppService is the application-facing contract for contextual news (implemented by *news.Service).
type NewsAppService interface {
	Get(ctx context.Context, cacheKey, query string, limit int) ([]news.Article, bool, bool, error)
	ProviderName() string
}

// EmailAppService is the application-facing contract for queued email (implemented by *email.Service).
type EmailAppService interface {
	Enabled() bool
	Enqueue(msg email.Message) bool
}

var (
	explorerSvc  ExplorerService  = &defaultExplorerService{}
	authSvc      AuthService      = &defaultAuthService{}
	portfolioSvc PortfolioService = &defaultPortfolioService{}
	watchlistSvc WatchlistService = &defaultWatchlistService{}
	alertSvc     AlertService     = &defaultAlertService{}
	adminSvc     AdminService     = &defaultAdminService{}
	feedbackSvc  FeedbackService  = &defaultFeedbackService{}
	newsService  NewsAppService   // set in Run when configured
	emailService EmailAppService  // set in Run when configured
)

// SetExplorerService replaces the explorer service (tests).
func SetExplorerService(s ExplorerService) { explorerSvc = s }

// SetAuthService replaces the auth service (tests).
func SetAuthService(s AuthService) { authSvc = s }

// SetPortfolioService replaces the portfolio service (tests).
func SetPortfolioService(s PortfolioService) { portfolioSvc = s }

// SetWatchlistService replaces the watchlist service (tests).
func SetWatchlistService(s WatchlistService) { watchlistSvc = s }

// SetAlertService replaces the alert background service (tests).
func SetAlertService(s AlertService) { alertSvc = s }

// SetAdminService replaces the admin service (tests).
func SetAdminService(s AdminService) { adminSvc = s }

// SetFeedbackService replaces the feedback service (tests).
func SetFeedbackService(s FeedbackService) { feedbackSvc = s }

// SetNewsService replaces the news service (tests).
func SetNewsService(s NewsAppService) { newsService = s }

// SetEmailAppService replaces the email service (tests).
func SetEmailAppService(s EmailAppService) { emailService = s }
