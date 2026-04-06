package server

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/CryptoD/blockchain-explorer/internal/email"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/news"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/CryptoD/blockchain-explorer/internal/sentryutil"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// listenAddr returns HTTP_LISTEN_ADDR if set, else ":APP_PORT", else ":8080".
func listenAddr() string {
	if a := strings.TrimSpace(os.Getenv("HTTP_LISTEN_ADDR")); a != "" {
		return a
	}
	if p := strings.TrimSpace(os.Getenv("APP_PORT")); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	return ":8080"
}

// Run loads configuration, wires dependencies, registers routes, and blocks serving HTTP until shutdown.
func Run() error {
	logging.Configure()

	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		logging.WithComponent(logging.ComponentServer).WithError(err).Fatal("Failed to load configuration")
	}
	appConfig = cfg
	httpClient = newRestyClientForConfig(cfg)

	appEnv := cfg.AppEnv
	if appEnv == "development" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
		logging.FieldEvent: "startup",
		logging.FieldEnv:   appEnv,
	}).Info("Application starting")

	// Initialize GetBlock settings from configuration.
	baseURL = cfg.GetBlockBaseURL
	apiKey = cfg.GetBlockAccessToken

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		logging.WithComponent(logging.ComponentRedis).WithError(err).WithField(logging.FieldEvent, "ping_failed").Warn("redis ping failed before startup")
	}

	// Initialize Sentry
	_ = pong
	if cfg.SentryDSN != "" {
		if initErr := sentryutil.Init(cfg); initErr != nil {
			logging.WithComponent(logging.ComponentSentry).WithError(initErr).Fatal("sentry.Init failed")
		}
		logging.WithComponent(logging.ComponentSentry).WithFields(log.Fields{
			"traces_sample_rate": cfg.SentryTracesSampleRate,
			"error_sample_rate":  cfg.SentryErrorSampleRate,
		}).Info("sentry initialized")
	} else {
		logging.WithComponent(logging.ComponentSentry).Warn("sentry disabled: SENTRY_DSN not set")
	}

	r := gin.Default()

	r.Use(securityHeadersMiddleware(cfg))
	r.Use(responseCompressionMiddleware(cfg))
	r.Use(sentrygin.New(sentrygin.Options{Repanic: true, Timeout: 2 * time.Second}))
	r.Use(correlationIDMiddleware())
	r.Use(sentryUserScopeMiddleware())
	r.Use(requestBodyLimitsMiddleware(cfg))
	if cfg.MetricsEnabled {
		r.Use(metrics.Middleware())
	}
	r.Use(rateLimitMiddleware)
	r.Use(csrfMiddleware)

	// Initialize Redis client (pool size, max active conns, and timeouts from config / env).
	rdb = redis.NewClient(redisOptionsFromConfig(cfg))
	appRepos = repos.NewStores(rdb)

	// Configure Redis for LRU eviction
	rdb.ConfigSet(ctx, "maxmemory", "100mb")
	rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")

	// Initialize pluggable service clients
	blockchainClient = blockchain.NewGetBlockRPCClient(baseURL, apiKey, httpClient)
	cgClient := pricing.NewCoinGeckoClient(httpClient)
	ratesTTL := time.Duration(cfg.RatesCacheTTLSeconds) * time.Second
	if ratesTTL <= 0 {
		ratesTTL = 60 * time.Second
	}
	pricingClient = pricing.NewCachingClient(cgClient, ratesTTL)
	cryptoFetcher := pricing.NewCachingCryptoFetcher(cgClient, ratesTTL)
	assetPricer = &pricing.CompositePricer{
		Crypto:    cryptoFetcher,
		Commodity: &pricing.StaticCommoditySource{},
		Bond:      &pricing.StaticBondSource{PricePer100: pricing.DefaultBondPrices()},
	}

	// Initialize news service (provider + Redis cache). Wiring is encapsulated in internal/news.
	if cfg.NewsProvider == "" {
		cfg.NewsProvider = "thenewsapi"
	}
	if s := news.NewServiceFromConfig(cfg, rdb, httpClient); s != nil {
		newsService = s
	} else {
		logging.WithComponent(logging.ComponentNews).WithField(logging.FieldProvider, cfg.NewsProvider).Warn("news provider not configured; news endpoints will be unavailable")
	}

	// Initialize email service (SMTP)
	if cfg.EmailProvider == "" {
		cfg.EmailProvider = "smtp"
	}
	if cfg.EmailProvider == "smtp" && strings.TrimSpace(cfg.EmailFrom) != "" && strings.TrimSpace(cfg.SMTPHost) != "" {
		sender := &email.SMTPSender{
			Host:        cfg.SMTPHost,
			Port:        cfg.SMTPPort,
			Username:    cfg.SMTPUsername,
			Password:    cfg.SMTPPassword,
			UseSTARTTLS: cfg.SMTPStartTLS,
			SkipVerify:  cfg.SMTPSkipVerify,
		}
		emailService = email.NewService(sender, email.Address{Email: cfg.EmailFrom, Name: cfg.EmailFromName})
		emailTemplates = email.NewTemplates(cfg.AppBaseURL)
		emailLog := logging.WithComponent(logging.ComponentEmail).WithFields(log.Fields{
			logging.FieldProvider: sender.Name(),
			"from_configured":     strings.TrimSpace(cfg.EmailFrom) != "",
		})
		if cfg.SMTPSkipVerify {
			emailLog.WithFields(log.Fields{
				"smtp_skip_verify":        true,
				logging.FieldEnv:          cfg.AppEnv,
				"smtp_starttls":           cfg.SMTPStartTLS,
				"security_event":          "smtp_tls_verification_disabled",
				"security_recommendation": "use a CA-issued cert or install the dev CA; never set SMTP_SKIP_VERIFY outside development",
			}).Warn("SECURITY: SMTP TLS certificate verification is disabled (SMTP_SKIP_VERIFY=true). Email is vulnerable to MITM. Allowed only for APP_ENV=development with self-signed or local CAs.")
		}
		emailLog.Info("email service initialized")
	} else {
		logging.WithComponent(logging.ComponentEmail).WithField(logging.FieldProvider, cfg.EmailProvider).Warn("email service not configured; emails disabled")
	}

	// Initialize default admin user
	initializeDefaultAdmin()

	applyDependencies(NewDependencies(
		cfg,
		rdb,
		appRepos,
		httpClient,
		blockchainClient,
		pricingClient,
		assetPricer,
		emailTemplates,
		newsService,
		emailService,
	))

	registerStaticRoutes(r)
	registerHealthAndMetricsRoutes(r, cfg)
	registerAPIV1Routes(r)
	registerLegacyAPIRoutes(r)

	// Start background job to prefetch latest blocks and transactions
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			// Initial run and every tick
			func() {
				cid := correlation.NewID()
				jobLog := logging.WithComponent(logging.ComponentBackground).WithFields(log.Fields{
					logging.FieldCorrelationID: cid,
					logging.FieldEvent:         "prefetch_tick",
				})
				const numBlocks = 5
				const numTxs = 10
				blocks, blocksErr := fetchLatestBlocks(numBlocks)
				if blocksErr == nil {
					blocksJSON, _ := json.Marshal(blocks)
					rdb.Set(context.Background(), "latest_blocks", blocksJSON, 5*time.Minute)
				} else {
					jobLog.WithError(blocksErr).WithField(logging.FieldEvent, "prefetch_blocks_failed").Error("failed to prefetch latest blocks")
				}
				txs, txsErr := fetchLatestTransactions(numBlocks, numTxs)
				if txsErr == nil {
					txsJSON, _ := json.Marshal(txs)
					rdb.Set(context.Background(), "latest_transactions", txsJSON, 5*time.Minute)
				} else {
					jobLog.WithError(txsErr).WithField(logging.FieldEvent, "prefetch_txs_failed").Error("failed to prefetch latest transactions")
				}
				if blocksErr == nil && txsErr == nil {
					jobLog.WithField(logging.FieldEvent, "prefetch_tick_ok").Debug("prefetch tick completed")
				}
				metrics.RecordPrefetchTick(blocksErr, txsErr)
			}()
			<-ticker.C
		}
	}()

	// Start background job to collect metrics for charts
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			collectMetrics()
			<-ticker.C
		}
	}()

	// Start background job to evaluate price alerts
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			alertSvc.EvaluateAll(context.Background())
			<-ticker.C
		}
	}()

	if cfg.SentryDSN != "" {
		defer sentry.Flush(2 * time.Second)
	}

	addr := listenAddr()
	logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
		logging.FieldEvent: "listen",
		"addr":             addr,
	}).Info("HTTP server listening")

	if err := r.Run(addr); err != nil {
		logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
			logging.FieldEvent: "server_exit",
			"error":            err.Error(),
		}).Error("HTTP server stopped with error")
		return err
	}
	return nil
}
