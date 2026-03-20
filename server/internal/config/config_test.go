package config

import (
	"os"
	"testing"
	"time"
)

// clearConfigEnvVars unsets all env vars that Load() reads, to ensure test isolation.
func clearConfigEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{
		"PORT", "DATABASE_URL", "BROWSER_WS_URL", "BROWSER_TIMEOUT",
		"BROWSER_LOAD_WAIT", "JOB_TIMEOUT", "MAX_JOB_RETRIES",
		"WORKER_POOL_SIZE", "JOB_BUFFER_SIZE", "PROXY_URL", "PROXY_URLS",
		"GEMINI_API_KEY", "TELEMETRY", "TELEMETRY_URL", "LOG_LEVEL",
	}
	for _, v := range vars {
		t.Setenv(v, "")
		os.Unsetenv(v)
	}
}

func TestLoad(t *testing.T) {
	t.Run("succeeds with DATABASE_URL set", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/testdb" {
			t.Errorf("expected DatabaseURL to match, got: %q", cfg.DatabaseURL)
		}
	})

	t.Run("fails when DATABASE_URL is missing", func(t *testing.T) {
		clearConfigEnvVars(t)
		// DATABASE_URL is not set

		cfg, err := Load()
		if err == nil {
			t.Fatal("expected error when DATABASE_URL is missing, got nil")
		}
		if cfg != nil {
			t.Error("expected nil config on error")
		}
		if err.Error() != "DATABASE_URL is required" {
			t.Errorf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("default values are applied", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Port != "8080" {
			t.Errorf("expected default Port '8080', got: %q", cfg.Port)
		}
		if cfg.BrowserWSURL != "ws://localhost:9222/camoufox" {
			t.Errorf("expected default BrowserWSURL, got: %q", cfg.BrowserWSURL)
		}
		if cfg.BrowserTimeout != 60*time.Second {
			t.Errorf("expected default BrowserTimeout 60s, got: %v", cfg.BrowserTimeout)
		}
		if cfg.BrowserLoadWait != 2*time.Second {
			t.Errorf("expected default BrowserLoadWait 2s, got: %v", cfg.BrowserLoadWait)
		}
		if cfg.JobTimeout != 120*time.Second {
			t.Errorf("expected default JobTimeout 120s, got: %v", cfg.JobTimeout)
		}
		if cfg.MaxJobRetries != 3 {
			t.Errorf("expected default MaxJobRetries 3, got: %d", cfg.MaxJobRetries)
		}
		if cfg.WorkerPoolSize != 5 {
			t.Errorf("expected default WorkerPoolSize 5, got: %d", cfg.WorkerPoolSize)
		}
		if cfg.JobBufferSize != 100 {
			t.Errorf("expected default JobBufferSize 100, got: %d", cfg.JobBufferSize)
		}
		if cfg.LogLevel != "INFO" {
			t.Errorf("expected default LogLevel 'INFO', got: %q", cfg.LogLevel)
		}
		if cfg.ProxyURL != "" {
			t.Errorf("expected empty ProxyURL by default, got: %q", cfg.ProxyURL)
		}
		if cfg.ProxyURLs != nil {
			t.Errorf("expected nil ProxyURLs by default, got: %v", cfg.ProxyURLs)
		}
		if cfg.GeminiAPIKey != "" {
			t.Errorf("expected empty GeminiAPIKey by default, got: %q", cfg.GeminiAPIKey)
		}
		if !cfg.TelemetryEnabled {
			t.Error("expected TelemetryEnabled=true by default")
		}
		if cfg.TelemetryURL != "" {
			t.Errorf("expected empty TelemetryURL by default, got: %q", cfg.TelemetryURL)
		}
	})

	t.Run("custom values via env vars", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://custom:pass@db:5432/mydb")
		t.Setenv("PORT", "9090")
		t.Setenv("BROWSER_WS_URL", "ws://remote:9222/browser")
		t.Setenv("BROWSER_TIMEOUT", "30")
		t.Setenv("BROWSER_LOAD_WAIT", "5")
		t.Setenv("JOB_TIMEOUT", "300")
		t.Setenv("MAX_JOB_RETRIES", "10")
		t.Setenv("WORKER_POOL_SIZE", "20")
		t.Setenv("JOB_BUFFER_SIZE", "500")
		t.Setenv("PROXY_URL", "http://proxy:8080")
		t.Setenv("PROXY_URLS", "http://proxy1:8080, http://proxy2:8080, http://proxy3:8080")
		t.Setenv("GEMINI_API_KEY", "test-api-key-123")
		t.Setenv("LOG_LEVEL", "DEBUG")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Port != "9090" {
			t.Errorf("expected Port '9090', got: %q", cfg.Port)
		}
		if cfg.DatabaseURL != "postgres://custom:pass@db:5432/mydb" {
			t.Errorf("expected custom DatabaseURL, got: %q", cfg.DatabaseURL)
		}
		if cfg.BrowserWSURL != "ws://remote:9222/browser" {
			t.Errorf("expected custom BrowserWSURL, got: %q", cfg.BrowserWSURL)
		}
		if cfg.BrowserTimeout != 30*time.Second {
			t.Errorf("expected BrowserTimeout 30s, got: %v", cfg.BrowserTimeout)
		}
		if cfg.BrowserLoadWait != 5*time.Second {
			t.Errorf("expected BrowserLoadWait 5s, got: %v", cfg.BrowserLoadWait)
		}
		if cfg.JobTimeout != 300*time.Second {
			t.Errorf("expected JobTimeout 300s, got: %v", cfg.JobTimeout)
		}
		if cfg.MaxJobRetries != 10 {
			t.Errorf("expected MaxJobRetries 10, got: %d", cfg.MaxJobRetries)
		}
		if cfg.WorkerPoolSize != 20 {
			t.Errorf("expected WorkerPoolSize 20, got: %d", cfg.WorkerPoolSize)
		}
		if cfg.JobBufferSize != 500 {
			t.Errorf("expected JobBufferSize 500, got: %d", cfg.JobBufferSize)
		}
		if cfg.ProxyURL != "http://proxy:8080" {
			t.Errorf("expected ProxyURL 'http://proxy:8080', got: %q", cfg.ProxyURL)
		}
		if len(cfg.ProxyURLs) != 3 {
			t.Fatalf("expected 3 ProxyURLs, got: %d (%v)", len(cfg.ProxyURLs), cfg.ProxyURLs)
		}
		if cfg.ProxyURLs[0] != "http://proxy1:8080" {
			t.Errorf("expected first ProxyURL 'http://proxy1:8080', got: %q", cfg.ProxyURLs[0])
		}
		if cfg.ProxyURLs[1] != "http://proxy2:8080" {
			t.Errorf("expected second ProxyURL 'http://proxy2:8080', got: %q", cfg.ProxyURLs[1])
		}
		if cfg.ProxyURLs[2] != "http://proxy3:8080" {
			t.Errorf("expected third ProxyURL 'http://proxy3:8080', got: %q", cfg.ProxyURLs[2])
		}
		if cfg.GeminiAPIKey != "test-api-key-123" {
			t.Errorf("expected GeminiAPIKey 'test-api-key-123', got: %q", cfg.GeminiAPIKey)
		}
		if cfg.LogLevel != "DEBUG" {
			t.Errorf("expected LogLevel 'DEBUG', got: %q", cfg.LogLevel)
		}
	})

	t.Run("invalid integer env vars fall back to defaults", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("MAX_JOB_RETRIES", "not-a-number")
		t.Setenv("WORKER_POOL_SIZE", "abc")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.MaxJobRetries != 3 {
			t.Errorf("expected default MaxJobRetries 3 for invalid input, got: %d", cfg.MaxJobRetries)
		}
		if cfg.WorkerPoolSize != 5 {
			t.Errorf("expected default WorkerPoolSize 5 for invalid input, got: %d", cfg.WorkerPoolSize)
		}
	})

	t.Run("invalid duration env vars fall back to defaults", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("BROWSER_TIMEOUT", "not-a-number")
		t.Setenv("JOB_TIMEOUT", "xyz")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.BrowserTimeout != 60*time.Second {
			t.Errorf("expected default BrowserTimeout for invalid input, got: %v", cfg.BrowserTimeout)
		}
		if cfg.JobTimeout != 120*time.Second {
			t.Errorf("expected default JobTimeout for invalid input, got: %v", cfg.JobTimeout)
		}
	})

	t.Run("PROXY_URLS with empty entries are filtered out", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("PROXY_URLS", "http://a:8080,, ,http://b:8080,")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(cfg.ProxyURLs) != 2 {
			t.Fatalf("expected 2 ProxyURLs after filtering, got: %d (%v)", len(cfg.ProxyURLs), cfg.ProxyURLs)
		}
		if cfg.ProxyURLs[0] != "http://a:8080" {
			t.Errorf("expected first ProxyURL 'http://a:8080', got: %q", cfg.ProxyURLs[0])
		}
		if cfg.ProxyURLs[1] != "http://b:8080" {
			t.Errorf("expected second ProxyURL 'http://b:8080', got: %q", cfg.ProxyURLs[1])
		}
	})

	t.Run("TELEMETRY=off disables telemetry", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("TELEMETRY", "off")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TelemetryEnabled {
			t.Error("expected TelemetryEnabled=false when TELEMETRY=off")
		}
	})

	t.Run("TELEMETRY=false disables telemetry", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("TELEMETRY", "false")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TelemetryEnabled {
			t.Error("expected TelemetryEnabled=false when TELEMETRY=false")
		}
	})

	t.Run("custom TELEMETRY_URL", func(t *testing.T) {
		clearConfigEnvVars(t)
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("TELEMETRY_URL", "https://custom.example.com/telemetry")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TelemetryURL != "https://custom.example.com/telemetry" {
			t.Errorf("expected custom TelemetryURL, got: %q", cfg.TelemetryURL)
		}
	})
}
