// Package sentryutil configures the Sentry SDK from application config.
package sentryutil

import (
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/getsentry/sentry-go"
)

// Init configures sentry-go when SENTRY_DSN / cfg.SentryDSN is set. No-op when DSN is empty.
func Init(cfg *config.Config) error {
	if cfg == nil || strings.TrimSpace(cfg.SentryDSN) == "" {
		return nil
	}

	env := strings.TrimSpace(cfg.SentryEnvironment)
	if env == "" {
		env = cfg.AppEnv
	}

	traces := cfg.SentryTracesSampleRate
	if traces < 0 {
		traces = 0
	}
	if traces > 1 {
		traces = 1
	}

	errRate := cfg.SentryErrorSampleRate
	if errRate <= 0 {
		errRate = 1.0
	}
	if errRate > 1 {
		errRate = 1
	}

	return sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Environment:      env,
		Release:          strings.TrimSpace(cfg.SentryRelease),
		AttachStacktrace: true,
		SampleRate:       errRate,
		EnableTracing:    traces > 0,
		TracesSampleRate: traces,
		BeforeSend:       scrubEvent,
	})
}

func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	if event.Request == nil {
		return event
	}
	event.Request.Cookies = ""
	if len(event.Request.Headers) == 0 {
		return event
	}
	for k := range event.Request.Headers {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "cookie", "authorization", "x-metrics-token", "x-csrf-token":
			delete(event.Request.Headers, k)
		}
	}
	return event
}
