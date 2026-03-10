package config

import (
	"os"
	"strconv"
	"strings"
)

// GetEnvWithDefault returns the value of the environment variable named by key,
// or defaultValue if the variable is not set or empty.
func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetEnvIntWithDefault reads an environment variable and parses it as int,
// returning defaultValue if unset or invalid.
func GetEnvIntWithDefault(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	if v, err := strconv.Atoi(valStr); err == nil {
		return v
	}
	return defaultValue
}

// GetAppEnv returns the current application environment, defaulting to "development".
func GetAppEnv() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		return "development"
	}
	return strings.ToLower(env)
}

// UseSecureCookies determines whether cookies should be marked Secure.
// Priority:
// - If SECURE_COOKIES is set to a truthy value, always use secure cookies.
// - Otherwise, use secure cookies for any non-development APP_ENV.
func UseSecureCookies() bool {
	if val := strings.ToLower(os.Getenv("SECURE_COOKIES")); val != "" {
		return val == "1" || val == "true" || val == "yes"
	}
	return GetAppEnv() != "development"
}

