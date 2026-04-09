package server

import (
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/retrybudget"
	"github.com/gin-gonic/gin"
)

// inboundRetryBudgetMiddleware attaches an outbound HTTP attempt budget to the Gin request context when
// OUTBOUND_HTTP_INBOUND_ATTEMPT_BUDGET > 0. Handlers must pass c.Request.Context() to downstream calls for it to apply.
func inboundRetryBudgetMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || cfg.OutboundHTTPInboundAttemptBudget <= 0 {
			c.Next()
			return
		}
		ctx := retrybudget.WithAttemptBudget(c.Request.Context(), cfg.OutboundHTTPInboundAttemptBudget)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
