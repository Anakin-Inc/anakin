// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the server.
type Config struct {
	Port string

	// Database
	DatabaseURL string

	// Browser (Playwright WebSocket)
	BrowserWSURL    string
	BrowserTimeout  time.Duration
	BrowserLoadWait time.Duration

	// Job Processing
	JobTimeout     time.Duration
	MaxJobRetries  int
	WorkerPoolSize int
	JobBufferSize  int

	// Proxy (optional)
	ProxyURL  string
	ProxyURLs []string // Pool of proxy URLs for auto-selection

	// Gemini (optional — enables generateJson)
	GeminiAPIKey string

	// Anakin.io API handler (optional — fallback when local handlers fail)
	AnakinAPIKey string

	// Telemetry (anonymous usage data — opt-out via TELEMETRY=off)
	TelemetryEnabled bool
	TelemetryURL     string

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnvOrDefault("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		BrowserWSURL:     getEnvOrDefault("BROWSER_WS_URL", "ws://localhost:9222/camoufox"),
		BrowserTimeout:   getDurationEnv("BROWSER_TIMEOUT", 60*time.Second),
		BrowserLoadWait:  getDurationEnv("BROWSER_LOAD_WAIT", 2*time.Second),
		JobTimeout:       getDurationEnv("JOB_TIMEOUT", 120*time.Second),
		MaxJobRetries:    getIntEnv("MAX_JOB_RETRIES", 3),
		WorkerPoolSize:   getIntEnv("WORKER_POOL_SIZE", 5),
		JobBufferSize:    getIntEnv("JOB_BUFFER_SIZE", 100),
		ProxyURL:         os.Getenv("PROXY_URL"),
		ProxyURLs:        getStringSliceEnv("PROXY_URLS"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		AnakinAPIKey:     os.Getenv("ANAKIN_API_KEY"),
		TelemetryEnabled: getBoolEnvDefault("TELEMETRY", true),
		TelemetryURL:     os.Getenv("TELEMETRY_URL"),
		LogLevel:         getEnvOrDefault("LOG_LEVEL", "INFO"),
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getStringSliceEnv(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var result []string
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func getBoolEnvDefault(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	switch strings.ToLower(v) {
	case "false", "off", "0", "no":
		return false
	default:
		return true
	}
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}
