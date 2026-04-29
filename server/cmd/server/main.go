// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/config"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/domain"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/gemini"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/http/router"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/processor"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setLogLevel(cfg.LogLevel)

	// Storage: PostgreSQL if DATABASE_URL is set, otherwise in-memory
	var (
		jobStore store.JobStore
		db       *sql.DB
	)

	if cfg.DatabaseURL != "" {
		db, err = sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			slog.Error("failed to open database connection", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			slog.Error("failed to ping database", "error", err)
			os.Exit(1)
		}
		slog.Info("connected to PostgreSQL")
		jobStore = store.NewPostgresStore(db)
	} else {
		slog.Info("no DATABASE_URL — using in-memory storage (jobs won't persist across restarts)")
		jobStore = store.NewMemoryStore()
	}

	// Build handler chain: HTTP -> Browser -> [API fallback]
	httpHandler := handler.NewHTTPHandler(cfg.BrowserTimeout, cfg.ProxyURL)
	browserHandler := handler.NewBrowserHandler(cfg.BrowserWSURL, cfg.BrowserTimeout, cfg.BrowserLoadWait)
	handlers := []handler.ScrapingHandler{httpHandler, browserHandler}

	// Anakin.io API fallback (optional — set ANAKIN_API_KEY to enable)
	if cfg.AnakinAPIKey != "" {
		handlers = append(handlers, handler.NewAnakinHandler(cfg.AnakinAPIKey))
		slog.Info("anakin.io API handler enabled as chain fallback")
	}

	chain := handler.NewChain(handlers)
	slog.Info("handler chain initialized", "handlers", chain.HandlerNames())

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Domain config cache (requires PostgreSQL)
	var domainCache *domain.Cache
	if db != nil {
		domainRepo := domain.NewRepository(db)
		domainCache = domain.NewCache(domainRepo)
		domainCache.Start(bgCtx)
		slog.Info("domain config cache started")
	}

	// Proxy pool (optional — only if PROXY_URLS is configured and DB is available)
	var proxyPool *proxy.Pool
	if len(cfg.ProxyURLs) > 0 && db != nil {
		proxyPool = proxy.NewPool(db, cfg.ProxyURLs)
		proxyPool.Start(bgCtx)
	}

	// Gemini client (optional — enables generateJson)
	geminiClient := gemini.NewClient(cfg.GeminiAPIKey)

	// Telemetry (anonymous usage data)
	tel := telemetry.New(db, cfg.TelemetryEnabled, cfg.TelemetryURL,
		cfg.GeminiAPIKey != "", len(cfg.ProxyURLs))

	// Create processor and worker pool
	proc := processor.NewProcessor(jobStore, chain, domainCache, proxyPool, geminiClient, tel)
	pool := worker.NewPool(proc, cfg.WorkerPoolSize, cfg.JobBufferSize, cfg.JobTimeout)
	pool.Start(bgCtx)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "AnakinScraper",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    10 * 1024 * 1024,
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "${time} ${status} ${method} ${path} ${latency}\n",
		TimeFormat: time.RFC3339,
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-API-Key,Api-Key",
	}))

	// Rate limiting (per-IP). Controlled via RATE_LIMIT (requests per minute). 0 = disabled.
	if cfg.RateLimit > 0 {
		app.Use(limiter.New(limiter.Config{
			Max:        cfg.RateLimit,
			Expiration: 60 * time.Second,
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":   "rate_limited",
					"message": "rate limit exceeded",
				})
			},
		}))
	}

	// Setup routes
	router.Setup(app, jobStore, db, pool, proxyPool, tel)

	// Startup banner
	fmt.Println("")
	fmt.Println("━━━ AnakinScraper OSS v0.1.0 ━━━")
	fmt.Printf("  API:     http://localhost:%s\n", cfg.Port)
	if db == nil {
		fmt.Println("  Storage: in-memory (set DATABASE_URL for persistence)")
	}
	fmt.Println("  Docs:    https://github.com/Anakin-Inc/anakinscraper-oss")
	fmt.Println("  Hosted:  https://anakin.io (geo-proxies, caching, search, research)")
	if cfg.TelemetryEnabled {
		fmt.Println("  Telemetry: anonymous usage data enabled (disable: TELEMETRY=off)")
	}
	fmt.Println("")

	// Start server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		slog.Info("starting server", "port", cfg.Port, "workers", cfg.WorkerPoolSize,
			"proxy_pool", len(cfg.ProxyURLs) > 0, "storage", storageMode(db))
		if err := app.Listen(addr); err != nil {
			slog.Error("server error", "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("shutting down", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new HTTP requests
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	// Drain worker pool (finish in-flight jobs)
	bgCancel()
	pool.Drain()

	// Cleanup
	tel.Stop()
	if domainCache != nil {
		domainCache.Stop()
	}
	if proxyPool != nil {
		proxyPool.Stop()
	}
	browserHandler.Stop()

	slog.Info("server stopped")
}

func storageMode(db *sql.DB) string {
	if db != nil {
		return "postgresql"
	}
	return "memory"
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"error":   "internal_error",
		"message": err.Error(),
	})
}

func setLogLevel(level string) {
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
