package news

import (
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/go-resty/resty/v2"
	"github.com/redis/go-redis/v9"
)

// NewServiceFromConfig builds a [Service] when TheNewsAPI is enabled, or returns nil.
func NewServiceFromConfig(cfg *config.Config, rdb redis.Cmdable, httpClient *resty.Client) *Service {
	if cfg == nil || rdb == nil || httpClient == nil {
		return nil
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.NewsProvider))
	if provider == "" {
		provider = "thenewsapi"
	}
	if provider != "thenewsapi" || strings.TrimSpace(cfg.TheNewsAPIToken) == "" {
		return nil
	}
	prov := &TheNewsAPIProvider{
		BaseURL:           cfg.TheNewsAPIBaseURL,
		Token:             cfg.TheNewsAPIToken,
		Client:            httpClient,
		DefaultLanguage:   cfg.TheNewsAPIDefaultLanguage,
		DefaultLocale:     cfg.TheNewsAPIDefaultLocale,
		DefaultCategories: cfg.TheNewsAPIDefaultCategories,
	}
	return &Service{
		Provider: prov,
		Cache:    &RedisCache{RDB: rdb},
		FreshTTL: time.Duration(cfg.NewsCacheTTLSeconds) * time.Second,
		StaleTTL: time.Duration(cfg.NewsStaleTTLSeconds) * time.Second,
	}
}
