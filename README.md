# AnakinScraper OSS

Open-source web scraping engine. Turn any website into clean markdown or structured data.

Self-host with a single command. No cloud dependencies.

## Features

- **URL scraping** — fetch any page, get back clean HTML + markdown
- **Batch scraping** — scrape up to 10 URLs in one request
- **Site mapping** — discover all links on a page
- **Multi-page crawl** — crawl a site with include/exclude patterns
- **Handler chain** — fast HTTP fetch with automatic fallback to full browser rendering
- **HTML to Markdown** — intelligent content extraction with boilerplate removal
- **Self-contained** — just PostgreSQL + one Go binary + Playwright browser. No Redis, no AWS, no message queues

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose

### Start

```bash
git clone https://github.com/AnakinAI/anakinscraper-oss.git
cd anakinscraper-oss
make up
```

That's it. Three containers start:

| Service | Port | Description |
|---------|------|-------------|
| Server | 8080 | REST API + worker pool |
| Browser Service | 9222 | Playwright Chromium (WebSocket) |
| PostgreSQL | 5432 | Job storage |

### Scrape a URL

```bash
# Submit
curl -X POST http://localhost:8080/v1/url-scraper \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Response: {"id": "job-uuid", "status": "pending", ...}

# Poll for result
curl http://localhost:8080/v1/url-scraper/JOB_UUID

# Response includes: html, cleanedHtml, markdown, durationMs
```

No API keys. No auth headers. Just JSON.

## Architecture

```
                    ┌─────────────────┐
                    │   Your App      │
                    │  (SDK / cURL)   │
                    └────────┬────────┘
                             │ HTTP
                             ▼
                    ┌─────────────────┐
                    │     Server      │
                    │   (Go/Fiber)    │    Handler Chain:
                    │   API + Workers │    1. HTTP (fast, no JS)
                    │   Port 8080     │    2. Browser (Playwright)
                    └────┬───────┬───┘
                         │       │
              ┌──────────┘       └──────────┐
              ▼                             ▼
        ┌──────────┐               ┌──────────────┐
        │PostgreSQL │               │   Browser    │
        │  jobs +   │               │   Service    │
        │  results  │               │  (Playwright)│
        └──────────┘               └──────────────┘
```

The server is a single Go binary. API handlers accept requests, insert jobs into PostgreSQL, and push them to an in-process worker pool via Go channels. Workers execute the handler chain (HTTP fetch → browser fallback), convert HTML to markdown, and write results back to the database. No external queues or object storage.

## API Reference

### Health Check

```
GET /health
```

```json
{"status": "ok", "database": true, "service": "anakinscraper"}
```

### Scrape a Single URL

```
POST /v1/url-scraper
```

```json
{
  "url": "https://example.com",
  "country": "us",
  "forceFresh": false,
  "useBrowser": false
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to scrape |
| `country` | string | `"us"` | Proxy country code (if proxy configured) |
| `forceFresh` | bool | `false` | Bypass any caching |
| `useBrowser` | bool | `false` | Skip HTTP handler, go straight to browser |

Returns a job ID. Poll `GET /v1/url-scraper/:id` for the result.

**Response (completed):**

```json
{
  "id": "550e8400-...",
  "status": "completed",
  "url": "https://example.com",
  "html": "<html>...</html>",
  "cleanedHtml": "<main>...</main>",
  "markdown": "# Page Title\n\nContent...",
  "cached": false,
  "durationMs": 1234
}
```

**Status lifecycle:** `pending` → `processing` → `completed` | `failed`

### Batch Scrape

```
POST /v1/url-scraper/batch
```

```json
{
  "urls": ["https://example.com/page-1", "https://example.com/page-2"],
  "useBrowser": false
}
```

Poll `GET /v1/url-scraper/batch/:id` for results. Max 10 URLs per batch.

### Discover URLs (Map)

```
POST /v1/map
```

```json
{
  "url": "https://example.com",
  "includeSubdomains": false,
  "limit": 100,
  "search": "blog"
}
```

Poll `GET /v1/map/:id`. Returns a list of discovered links.

### Multi-Page Crawl

```
POST /v1/crawl
```

```json
{
  "url": "https://example.com",
  "maxPages": 10,
  "includePatterns": ["/blog/**"],
  "excludePatterns": ["/admin/**"]
}
```

Poll `GET /v1/crawl/:id`. Returns per-page markdown results.

### Error Responses

```json
{"error": "invalid_url", "message": "URL must use http or https scheme"}
```

| Status | Error | Description |
|--------|-------|-------------|
| 400 | `invalid_url` | Malformed or missing URL |
| 400 | `invalid_request` | Bad request body |
| 404 | `not_found` | Job not found |
| 500 | `internal_error` | Server error |

## Handler Chain

Each scrape job goes through the handler chain. On failure, it falls back to the next handler:

```
HTTP Handler (fast, ~200ms) ──fail──▶ Browser Handler (Playwright, full JS)
```

**HTTP Handler** — direct HTTP GET with a browser user-agent. Handles static HTML, server-rendered pages. No browser overhead.

**Browser Handler** — connects to the Playwright browser service over WebSocket. Full JavaScript rendering, network-idle detection. Handles SPAs, lazy-loaded content, JS-rendered pages.

### Extending the Chain

Implement the `ScrapingHandler` interface:

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *HandlerRequest) bool
    Scrape(ctx context.Context, req *HandlerRequest) (*ScrapeResult, error)
    IsHealthy() bool
}
```

See [examples/custom-handler/](examples/custom-handler/).

## Configuration

All configuration via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DATABASE_URL` | — | PostgreSQL connection string (**required**) |
| `BROWSER_WS_URL` | `ws://localhost:9222/playwright` | Browser service WebSocket URL |
| `BROWSER_TIMEOUT` | `60` | Page navigation timeout (seconds) |
| `BROWSER_LOAD_WAIT` | `2` | Extra wait after page load (seconds) |
| `WORKER_POOL_SIZE` | `5` | Concurrent scrape workers |
| `JOB_BUFFER_SIZE` | `100` | Job queue buffer size |
| `JOB_TIMEOUT` | `120` | Max job duration (seconds) |
| `PROXY_URL` | — | HTTP proxy for the HTTP handler |
| `LOG_LEVEL` | `INFO` | Log level (DEBUG, INFO, WARN, ERROR) |

## Project Structure

```
anakinscraper-oss/
├── server/                     # Go server (API + workers)
│   ├── cmd/server/             # Entry point
│   └── internal/
│       ├── config/             # Environment config
│       ├── models/             # Data types
│       ├── worker/             # Channel-based worker pool
│       ├── handler/            # Scraping handlers (HTTP, Browser)
│       ├── converter/          # HTML → Markdown
│       ├── processor/          # Job processing + link extraction
│       └── http/
│           ├── handlers/       # API request handlers
│           └── router/         # Route registration
├── browser-service/            # Python Playwright server
├── examples/                   # Usage examples
├── docker-compose.yml          # Full stack (3 containers)
├── scripts/init-db.sql         # Database schema
└── .env.example                # Config template
```

## Development

### Running Locally (without Docker)

**Prerequisites:** Go 1.22+, Python 3.11+, PostgreSQL

```bash
# Terminal 1: PostgreSQL (if not running)
docker compose up postgres -d

# Terminal 2: Browser Service
cd browser-service && pip install -r requirements.txt && python server.py

# Terminal 3: Server
cd server && DATABASE_URL="postgres://postgres:postgres@localhost:5432/anakinscraper?sslmode=disable" go run cmd/server/main.go
```

### Running Tests

```bash
cd server && go test ./...
```

### Building

```bash
cd server && go build -o server ./cmd/server
```

## License

[AGPL-3.0](LICENSE)
