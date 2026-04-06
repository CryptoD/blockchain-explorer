package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/redisstore"
	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
)

const sentryShutdownFlushTimeout = 5 * time.Second

func flushSentryOnShutdown(cfg *config.Config) {
	if cfg == nil || cfg.SentryDSN == "" {
		return
	}
	if ok := sentry.Flush(sentryShutdownFlushTimeout); !ok {
		logging.WithComponent(logging.ComponentSentry).WithField(logging.FieldEvent, "sentry_flush_timeout").Warn("sentry flush did not complete within timeout")
	}
}

func closeRedisPoolOnShutdown(c redisstore.Client, timeout time.Duration) {
	type closer interface {
		Close() error
	}
	cl, ok := c.(closer)
	if !ok || c == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := cl.Close(); err != nil {
			logging.WithComponent(logging.ComponentRedis).WithError(err).WithField(logging.FieldEvent, "redis_close_error").Warn("redis client close returned error")
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logging.WithComponent(logging.ComponentRedis).WithField(logging.FieldEvent, "redis_close_timeout").Warn("redis client close timed out; continuing shutdown")
	}
}

func finishProcessShutdown(cfg *config.Config, cancelBg context.CancelFunc) {
	cancelBg()
	flushSentryOnShutdown(cfg)
	closeRedisPoolOnShutdown(rdb, time.Duration(cfg.RedisCloseTimeoutSeconds)*time.Second)
}

// serveUntilShutdown runs ListenAndServe in a goroutine. On SIGINT/SIGTERM it drains in-flight
// requests with srv.Shutdown (bounded by cfg.ShutdownGraceSeconds), then stops background work,
// flushes Sentry, and closes the Redis pool with a timeout.
func serveUntilShutdown(srv *http.Server, cfg *config.Config, cancelBg context.CancelFunc) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		signal.Stop(quit)
		finishProcessShutdown(cfg, cancelBg)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case sig := <-quit:
		signal.Stop(quit)
		logging.WithComponent(logging.ComponentServer).WithFields(log.Fields{
			logging.FieldEvent: "shutdown_signal",
			"signal":           sig.String(),
		}).Info("graceful shutdown initiated")

		grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), grace)
		shutdownErr := srv.Shutdown(shutdownCtx)
		shutdownCancel()
		if shutdownErr != nil {
			logging.WithComponent(logging.ComponentServer).WithError(shutdownErr).WithField(logging.FieldEvent, "shutdown_drain_error").Warn("HTTP server shutdown did not complete cleanly")
		}
		<-errCh

		finishProcessShutdown(cfg, cancelBg)
		return nil
	}
}
