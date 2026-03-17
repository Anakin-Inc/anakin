# Custom Handler Example

Demonstrates how to extend the AnakinScraper handler chain with your own scraping handler.

This example implements a `CachedHTMLHandler` that serves pre-fetched HTML from a local directory, falling back to the built-in HTTP handler for uncached URLs.

## How It Works

1. Implement the `ScrapingHandler` interface (4 methods: `Name`, `CanHandle`, `Scrape`, `IsHealthy`)
2. Register your handler in the chain — position determines fallback priority
3. The chain tries handlers in order until one succeeds

## Run

```bash
# From the scraper-service directory (needed for Go module dependencies)
cd scraper-service

# Optional: create a cached page to test the custom handler
mkdir -p cached-pages
echo "<html><body>Cached!</body></html>" > cached-pages/example.com.html

# Run the example
go run ../examples/custom-handler/main.go https://example.com
```

## The Interface

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *ScrapeRequest) bool
    Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResult, error)
    IsHealthy() bool
}
```

| Method | Purpose |
|--------|---------|
| `Name()` | Handler identifier for logging and metrics |
| `CanHandle()` | Return `true` if this handler should attempt the request |
| `Scrape()` | Perform the scraping — return result or error to try next handler |
| `IsHealthy()` | Health check — unhealthy handlers are skipped |

## Ideas for Custom Handlers

- **API handler** — call a site's JSON API instead of scraping HTML
- **Sitemap handler** — parse `sitemap.xml` for structured data
- **Archive handler** — fetch from Wayback Machine when the live site is down
- **Headless Chrome handler** — use Chrome DevTools Protocol instead of Playwright
