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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/config"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/domain"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/gemini"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/handler"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/http/router"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/processor"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/proxy"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/worker"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setLogLevel(cfg.LogLevel)

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.DatabaseURL)
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

	// Build handler chain: HTTP -> Browser
	httpHandler := handler.NewHTTPHandler(cfg.BrowserTimeout, cfg.ProxyURL)
	browserHandler := handler.NewBrowserHandler(cfg.BrowserWSURL, cfg.BrowserTimeout, cfg.BrowserLoadWait)
	chain := handler.NewChain([]handler.ScrapingHandler{httpHandler, browserHandler})
	slog.Info("handler chain initialized", "handlers", chain.HandlerNames())

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Domain config cache
	domainRepo := domain.NewRepository(db)
	domainCache := domain.NewCache(domainRepo)
	domainCache.Start(bgCtx)
	slog.Info("domain config cache started")

	// Proxy pool (optional — only if PROXY_URLS is configured)
	var proxyPool *proxy.Pool
	if len(cfg.ProxyURLs) > 0 {
		proxyPool = proxy.NewPool(db, cfg.ProxyURLs)
		proxyPool.Start(bgCtx)
	}

	// Gemini client (optional — enables generateJson)
	geminiClient := gemini.NewClient(cfg.GeminiAPIKey)

	// Create processor and worker pool
	proc := processor.NewProcessor(db, chain, domainCache, proxyPool, geminiClient)
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

	// Setup routes
	router.Setup(app, db, pool, proxyPool)

	// Start server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		slog.Info("starting server", "port", cfg.Port, "workers", cfg.WorkerPoolSize,
			"proxy_pool", len(cfg.ProxyURLs) > 0)
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
	domainCache.Stop()
	if proxyPool != nil {
		proxyPool.Stop()
	}
	browserHandler.Stop()

	slog.Info("server stopped")
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
