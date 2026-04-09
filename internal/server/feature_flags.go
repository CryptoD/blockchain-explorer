package server

import (
	"context"
	"net/http"

	"github.com/CryptoD/blockchain-explorer/internal/featureflags"
	"github.com/gin-gonic/gin"
)

// featureFlags is set in Run after Redis and config are available; optional Redis keys override env defaults.
var featureFlags *featureflags.Resolver

// SetFeatureFlagsResolver replaces the resolver (tests).
func SetFeatureFlagsResolver(r *featureflags.Resolver) {
	featureFlags = r
}

func newsFeatureAllowed(ctx context.Context) bool {
	if featureFlags != nil {
		return featureFlags.NewsEnabled(ctx)
	}
	if appConfig != nil {
		return appConfig.FeatureNewsEnabled
	}
	return true
}

func priceAlertsFeatureAllowed(ctx context.Context) bool {
	if featureFlags != nil {
		return featureFlags.PriceAlertsEnabled(ctx)
	}
	if appConfig != nil {
		return appConfig.FeaturePriceAlertsEnabled
	}
	return true
}

func rejectIfNewsDisabled(c *gin.Context) bool {
	if newsFeatureAllowed(c.Request.Context()) {
		return false
	}
	errorResponse(c, http.StatusServiceUnavailable, "feature_disabled", "News is temporarily disabled")
	return true
}

func rejectIfPriceAlertsDisabled(c *gin.Context) bool {
	if priceAlertsFeatureAllowed(c.Request.Context()) {
		return false
	}
	errorResponse(c, http.StatusServiceUnavailable, "feature_disabled", "Price alerts are temporarily disabled")
	return true
}
