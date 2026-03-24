// Package logging configures the shared logrus logger and documents standard field names.
package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Standard field keys for structured JSON logs (use consistently across the backend).
const (
	FieldComponent = "component"
	FieldEvent     = "event"
	FieldExport    = "export"
	FieldIP        = "ip"
	FieldUsername  = "username"
	FieldProvider  = "provider"
	FieldEnv       = "env"
	FieldQueryLen  = "query_len"
	FieldQueryHash = "query_hash" // SHA-256 prefix for correlating without logging raw queries
	FieldResult    = "result_type"
)

// Component identifies a subsystem for filtering and dashboards.
const (
	ComponentServer     = "server"
	ComponentRedis      = "redis"
	ComponentSentry     = "sentry"
	ComponentNews       = "news"
	ComponentEmail      = "email"
	ComponentAuth       = "auth"
	ComponentAdmin      = "admin"
	ComponentSearch     = "search"
	ComponentRateLimit  = "rate_limit"
	ComponentExport     = "export"
	ComponentBackground = "background"
	ComponentAlerts     = "alerts"
	ComponentNetwork    = "network"
	ComponentFeedback   = "feedback"
)

// Configure sets JSON formatting, optional field renames, and level from LOG_LEVEL (debug|info|warn|error).
func Configure() {
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})
	logrus.SetOutput(os.Stdout)

	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info", "":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
}

// WithComponent returns a log entry tagged with FieldComponent.
func WithComponent(component string) *logrus.Entry {
	return logrus.WithField(FieldComponent, component)
}

// QueryLogFields returns safe structured fields for search-related logs (length + short hash; never raw query text).
func QueryLogFields(query string) logrus.Fields {
	fields := logrus.Fields{
		FieldQueryLen: len(query),
	}
	if len(query) > 0 {
		sum := sha256.Sum256([]byte(query))
		fields[FieldQueryHash] = hex.EncodeToString(sum[:8])
	}
	return fields
}
