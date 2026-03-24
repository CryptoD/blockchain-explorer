// Package metrics registers Prometheus-compatible metrics and Gin middleware.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, normalized route, and status.",
		},
		[]string{"method", "handler", "status"},
	)

	httpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "handler"},
	)

	cacheEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "explorer_cache_events_total",
			Help: "Cache-related events: layer (subsystem) and outcome.",
		},
		[]string{"layer", "outcome"},
	)

	backgroundJobRuns = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "explorer_background_job_runs_total",
			Help: "Background job loop iterations by job name.",
		},
		[]string{"job"},
	)

	backgroundJobErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "explorer_background_job_errors_total",
			Help: "Background job errors by job and error class.",
		},
		[]string{"job", "class"},
	)

	prefetchLastOKUnix = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "explorer_prefetch_last_success_unixtime",
			Help: "Unix timestamp of the last successful prefetch tick (blocks and txs OK).",
		},
	)

	alertEvalDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "explorer_alert_eval_duration_seconds",
			Help:    "Duration of a full price-alert evaluation cycle.",
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		},
	)

	alertEvalTriggered = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "explorer_alert_eval_triggered_total",
			Help: "Cumulative number of price alerts triggered.",
		},
	)
)

// Handler returns the Prometheus scrape handler (default registry).
func Handler() http.Handler {
	return promhttp.Handler()
}

// Middleware records request counts and latencies. Skips the /metrics path.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if len(path) > 256 {
			path = path[:256]
		}
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())
		httpRequests.WithLabelValues(method, path, status).Inc()
		httpDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}

// TokenAuthMiddleware requires Authorization: Bearer <token> or X-Metrics-Token when token is non-empty.
func TokenAuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		if constantTimeEqual(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "), token) {
			c.Next()
			return
		}
		if constantTimeEqual(c.GetHeader("X-Metrics-Token"), token) {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// RecordSearchETag records HTTP-level ETag cache hits (304) vs full responses.
func RecordSearchETag(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("search_etag", outcome).Inc()
}

// RecordNews records news cache: fresh Redis hit, stale fallback, or fetch from provider.
func RecordNews(cached, stale bool) {
	switch {
	case cached && !stale:
		cacheEvents.WithLabelValues("news", "fresh").Inc()
	case cached && stale:
		cacheEvents.WithLabelValues("news", "stale").Inc()
	default:
		cacheEvents.WithLabelValues("news", "fetch").Inc()
	}
}

// RecordRates records Redis hit vs miss for /api/rates.
func RecordRates(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("rates", outcome).Inc()
}

// RecordNetworkStatus records Redis hit vs miss for network status aggregation.
func RecordNetworkStatus(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("network_status", outcome).Inc()
}

// RecordRPCTxCache records tx detail Redis cache.
func RecordRPCTxCache(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("rpc_tx", outcome).Inc()
}

// RecordRPCAddressCache records address detail Redis cache.
func RecordRPCAddressCache(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("rpc_address", outcome).Inc()
}

// RecordRPCBlockCache records block detail Redis cache.
func RecordRPCBlockCache(hit bool) {
	outcome := "miss"
	if hit {
		outcome = "hit"
	}
	cacheEvents.WithLabelValues("rpc_block", outcome).Inc()
}

// RecordPrefetchTick records one prefetch loop iteration.
func RecordPrefetchTick(blocksErr, txsErr error) {
	backgroundJobRuns.WithLabelValues("prefetch").Inc()
	if blocksErr != nil {
		backgroundJobErrors.WithLabelValues("prefetch", "blocks").Inc()
	}
	if txsErr != nil {
		backgroundJobErrors.WithLabelValues("prefetch", "transactions").Inc()
	}
	if blocksErr == nil && txsErr == nil {
		prefetchLastOKUnix.Set(float64(time.Now().Unix()))
	}
}

// RecordMetricsJob records one metrics chart collection run.
func RecordMetricsJob() {
	backgroundJobRuns.WithLabelValues("metrics_charts").Inc()
}

// RecordAlertEval records one alert evaluation cycle.
func RecordAlertEval(elapsed time.Duration, triggered int) {
	backgroundJobRuns.WithLabelValues("price_alerts").Inc()
	alertEvalDuration.Observe(elapsed.Seconds())
	if triggered > 0 {
		alertEvalTriggered.Add(float64(triggered))
	}
}
