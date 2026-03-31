package server

import (
	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/go-resty/resty/v2"
)

// Dependencies aggregates runtime wiring for the HTTP server (light DI).
// External boundaries are interface-typed (blockchain RPC, pricing, asset valuation).
// Built in Run via NewDependencies; legacy package globals mirror these fields until
// handlers are migrated. Prefer passing Dependencies into new code instead of adding globals.
type Dependencies struct {
	Config *config.Config
	Redis  redisstore.Client
	Repos  *repos.Stores
	HTTP   *resty.Client

	Blockchain blockchain.RPCClient
	Pricing    pricing.Client
	Assets     pricing.AssetPricer

	EmailTemplates *email.Templates

	Explorer  ExplorerService
	Auth      AuthService
	Portfolio PortfolioService
	Watchlist WatchlistService
	Alert     AlertService
	Admin     AdminService
	Feedback  FeedbackService
	News      NewsAppService
	Email     EmailAppService
}

var appDeps *Dependencies

// NewDependencies wires the application graph from validated config, Redis, repositories,
// and interface-typed clients. Domain services default to the current package-level
// implementations (explorerSvc, authSvc, …); pass news/email from Run when configured.
func NewDependencies(
	cfg *config.Config,
	redisClient redisstore.Client,
	stores *repos.Stores,
	httpClient *resty.Client,
	bc blockchain.RPCClient,
	pc pricing.Client,
	assets pricing.AssetPricer,
	emailTmpl *email.Templates,
	news NewsAppService,
	emailApp EmailAppService,
) *Dependencies {
	return &Dependencies{
		Config:         cfg,
		Redis:          redisClient,
		Repos:          stores,
		HTTP:           httpClient,
		Blockchain:     bc,
		Pricing:        pc,
		Assets:         assets,
		EmailTemplates: emailTmpl,
		Explorer:       explorerSvc,
		Auth:           authSvc,
		Portfolio:      portfolioSvc,
		Watchlist:      watchlistSvc,
		Alert:          alertSvc,
		Admin:          adminSvc,
		Feedback:       feedbackSvc,
		News:           news,
		Email:          emailApp,
	}
}

// applyDependencies sets the aggregated graph and keeps legacy globals in sync for existing handlers.
func applyDependencies(d *Dependencies) {
	appDeps = d
	if d == nil {
		return
	}
	appConfig = d.Config
	rdb = d.Redis
	appRepos = d.Repos
	httpClient = d.HTTP
	blockchainClient = d.Blockchain
	pricingClient = d.Pricing
	assetPricer = d.Assets
	emailTemplates = d.EmailTemplates
	if d.Config != nil {
		baseURL = d.Config.GetBlockBaseURL
		apiKey = d.Config.GetBlockAccessToken
	}
	explorerSvc = d.Explorer
	authSvc = d.Auth
	portfolioSvc = d.Portfolio
	watchlistSvc = d.Watchlist
	alertSvc = d.Alert
	adminSvc = d.Admin
	feedbackSvc = d.Feedback
	newsService = d.News
	emailService = d.Email
}

// GetDependencies returns the graph built in Run, or nil before Run or in tests that never call applyDependencies.
func GetDependencies() *Dependencies {
	return appDeps
}
