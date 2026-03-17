# AnakinScraper OSS

Open-source web scraping engine. Turn any website into clean markdown or structured data.

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.11-3776AB.svg)](https://python.org)

## Features

- **Single URL scraping** — fetch any page and get back clean HTML, markdown, or structured JSON
- **Batch scraping** — scrape up to 10 URLs in a single request
- **Handler chain** — automatic fallback from fast HTTP fetch to full browser rendering
- **Browser automation** — Playwright-based browser service for JavaScript-heavy sites
- **HTML → Markdown** — intelligent content extraction with boilerplate removal
- **JSON extraction** — optional AI-powered structured data extraction (requires Gemini API key)
- **Caching** — Redis cache with configurable TTL to avoid redundant fetches
- **Rate limiting** — per-user token bucket rate limiting
- **API key auth** — SHA256-hashed API key authentication
- **Self-hostable** — single `docker compose up` to run the full stack

## Architecture

```
                    ┌─────────────────┐
                    │   Your App      │
                    │  (SDK / cURL)   │
                    └────────┬────────┘
                             │ HTTP (X-API-Key)
                             ▼
                    ┌─────────────────┐
                    │   Go API        │
                    │   (Fiber v2)    │
                    │   Port 8080     │
                    └────────┬────────┘
                             │ SQS
                             ▼
                    ┌─────────────────┐
                    │ Scraper Worker  │     Handler Chain:
                    │   (Go)          │     1. HTTP (fast, no JS)
                    │                 │──── 2. Browser (Playwright)
                    └────┬───┬───┬────┘
                         │   │   │
              ┌──────────┘   │   └──────────┐
              ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │PostgreSQL │  │  Redis   │  │    S3    │
        │ metadata  │  │  cache   │  │ storage  │
        └──────────┘  └──────────┘  └──────────┘

        Browser Service (Playwright, port 9222) ◄── WebSocket
```

**Storage tiers:**
- **PostgreSQL** — job metadata (ID, URL, status, timestamps, S3 path)
- **Redis** — full response cache (24h TTL, configurable)
- **S3** — permanent storage for HTML, markdown, and JSON results

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- 4 GB RAM minimum

### 1. Clone and configure

```bash
git clone https://github.com/AnakinAI/anakinscraper-oss.git
cd anakinscraper-oss
cp .env.example .env
```

### 2. Start the stack

```bash
docker compose up -d
```

This starts all services:

| Service | Port | Description |
|---------|------|-------------|
| API | 8080 | REST API |
| Scraper Worker | — | Background job processor |
| Browser Service | 9222 | Playwright browser (WebSocket) |
| PostgreSQL | 5432 | Job metadata |
| Redis | 6379 | Response cache |
| LocalStack | 4566 | S3 + SQS (local AWS emulation) |

### 3. Initialize queues and buckets

```bash
./scripts/setup-localstack.sh
```

### 4. Create an API key

```bash
# Generate a key (outputs the full key — save it, shown only once)
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key"}'
```

### 5. Scrape a URL

```bash
# Submit a scrape job
curl -X POST http://localhost:8080/v1/url-scraper \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Response: {"id": "job-uuid", "status": "pending", ...}

# Poll for results
curl http://localhost:8080/v1/url-scraper/JOB_UUID \
  -H "X-API-Key: YOUR_API_KEY"

# Response includes: html, cleanedHtml, markdown, status, durationMs
```

## API Reference

### Authentication

All endpoints except `/health` require an API key via one of these headers:

```
X-API-Key: sk_live_abc123
Authorization: Bearer sk_live_abc123
Api-Key: sk_live_abc123
```

### Endpoints

#### Health Check

```
GET /health
```

Returns service status. No auth required.

```json
{"status": "ok", "redis": true, "service": "anakinscraper"}
```

#### Scrape a Single URL

```
POST /v1/url-scraper
```

**Request:**

```json
{
  "url": "https://example.com",
  "country": "us",
  "forceFresh": false,
  "useBrowser": false,
  "generateJson": false
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to scrape |
| `country` | string | `"us"` | Proxy country code |
| `forceFresh` | bool | `false` | Bypass cache |
| `useBrowser` | bool | `false` | Force browser rendering (skip HTTP handler) |
| `generateJson` | bool | `false` | Extract structured JSON via AI (requires Gemini API key) |

**Response (pending):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "url": "https://example.com",
  "jobType": "url_scraper"
}
```

#### Get Job Result

```
GET /v1/url-scraper/:id
```

**Response (completed):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "url": "https://example.com",
  "jobType": "url_scraper",
  "html": "<html>...</html>",
  "cleanedHtml": "<main>...</main>",
  "markdown": "# Page Title\n\nContent here...",
  "generatedJson": null,
  "cached": false,
  "createdAt": "2025-01-15T10:30:00Z",
  "completedAt": "2025-01-15T10:30:03Z",
  "durationMs": 3421
}
```

**Status values:** `pending` → `processing` → `completed` | `failed`

#### Batch Scrape

```
POST /v1/url-scraper/batch
```

**Request:**

```json
{
  "urls": [
    "https://example.com/page-1",
    "https://example.com/page-2",
    "https://example.com/page-3"
  ],
  "country": "us",
  "useBrowser": false,
  "generateJson": false
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `urls` | string[] | **required** | 1–10 URLs to scrape |
| `country` | string | `"us"` | Proxy country code |
| `useBrowser` | bool | `false` | Force browser rendering |
| `generateJson` | bool | `false` | Extract structured JSON |

**Response (completed):**

```json
{
  "id": "batch-job-uuid",
  "status": "completed",
  "jobType": "batch_url_scraper",
  "urls": ["url1", "url2", "url3"],
  "results": [
    {
      "index": 0,
      "url": "https://example.com/page-1",
      "status": "completed",
      "html": "...",
      "cleanedHtml": "...",
      "markdown": "...",
      "cached": false,
      "durationMs": 1234
    }
  ],
  "createdAt": "...",
  "completedAt": "...",
  "durationMs": 5678
}
```

#### Discover URLs (Map)

```
POST /v1/map
```

**Request:**

```json
{
  "url": "https://example.com",
  "includeSubdomains": false,
  "limit": 100,
  "search": "blog"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Starting URL |
| `includeSubdomains` | bool | `false` | Include links to subdomains |
| `limit` | int | `100` | Max links to return (max: 5000) |
| `search` | string | — | Filter links containing this string |

**Response (completed):**

```json
{
  "id": "map-job-uuid",
  "status": "completed",
  "url": "https://example.com",
  "links": ["https://example.com/blog/post-1", "..."],
  "totalLinks": 42
}
```

#### Multi-Page Crawl

```
POST /v1/crawl
```

**Request:**

```json
{
  "url": "https://example.com",
  "maxPages": 10,
  "includePatterns": ["/blog/**"],
  "excludePatterns": ["/admin/**"],
  "country": "us"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Starting URL |
| `maxPages` | int | `10` | Max pages to crawl (max: 100) |
| `includePatterns` | string[] | — | Glob patterns for URLs to include |
| `excludePatterns` | string[] | — | Glob patterns for URLs to exclude |
| `country` | string | `"us"` | Proxy country code |
| `useBrowser` | bool | `false` | Force browser rendering |

**Response (completed):**

```json
{
  "id": "crawl-job-uuid",
  "status": "completed",
  "url": "https://example.com",
  "totalPages": 10,
  "completedPages": 8,
  "results": [
    {
      "url": "https://example.com/page",
      "status": "completed",
      "markdown": "# Page\n\n...",
      "durationMs": 2100
    }
  ]
}
```

### Rate Limits

| Endpoint | Limit |
|----------|-------|
| `POST /v1/url-scraper` | 60/min per user |
| `POST /v1/url-scraper/batch` | 30/min per user |
| `POST /v1/map` | 30/min per user |
| `POST /v1/crawl` | 10/min per user |

Rate-limited responses return `429 Too Many Requests`.

### Error Responses

```json
{
  "error": "invalid_url",
  "message": "The provided URL is not valid"
}
```

| Status | Error | Description |
|--------|-------|-------------|
| 400 | `invalid_url` | Malformed or missing URL |
| 401 | `unauthorized` | Missing or invalid API key |
| 429 | `rate_limited` | Too many requests |
| 500 | `internal_error` | Server error |

## Handler Chain

The scraper worker processes each job through a chain of handlers, falling back to the next one on failure:

```
┌──────────────┐     ┌──────────────────┐
│ HTTP Handler │────▶│ Browser Handler  │
│  (fast, no   │fail │  (Playwright +   │
│   JS render) │     │   full browser)  │
└──────────────┘     └──────────────────┘
```

### HTTP Handler

- Direct HTTP fetch using Chrome TLS fingerprint impersonation
- Fast (~200ms) and cheap — no browser overhead
- Handles static HTML sites, APIs, and simple server-rendered pages
- Proxy support with automatic fallback

### Browser Handler

- Full Playwright browser via WebSocket connection to Browser Service
- Renders JavaScript, waits for dynamic content, handles SPAs
- Smart waiting: detects JS frameworks (React, Vue), waits for network idle
- Auto-scrolls for lazy-loaded content
- Proxy support with automatic fallback to direct connection

### Extending the Chain

Implement the `ScrapingHandler` interface to add custom handlers:

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *Request) bool
    Scrape(ctx context.Context, req *Request) (*Result, error)
    IsHealthy() bool
}
```

Register your handler in the chain at startup to control fallback priority. See [examples/custom-handler/](examples/custom-handler/) for a working example.

## Configuration

### API Service

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API server port |
| `DATABASE_URL` | — | PostgreSQL connection string (required) |
| `REDIS_URL` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | — | Redis password |
| `REDIS_DB` | `7` | Redis database number |
| `JWT_SECRET` | — | JWT signing key (min 32 chars) |
| `S3_BUCKET` | — | S3 bucket for result storage |
| `S3_REGION` | `us-east-1` | S3 region |
| `S3_ACCESS_KEY` | — | AWS access key |
| `S3_SECRET_KEY` | — | AWS secret key |
| `S3_ENDPOINT` | — | Custom S3 endpoint (LocalStack, MinIO) |
| `S3_FORCE_PATH_STYLE` | `false` | Use path-style S3 URLs |
| `SQS_ENDPOINT` | — | Custom SQS endpoint (LocalStack) |
| `SQS_ACCESS_KEY` | — | AWS access key for SQS |
| `SQS_SECRET_KEY` | — | AWS secret key for SQS |
| `URL_SCRAPER_JOB_QUEUE_NAME` | — | SQS queue for scrape jobs |
| `STATUS_UPDATE_QUEUE_NAME` | `request-status-processor-queue` | SQS queue for status updates |
| `CACHE_TTL` | `5m` | API response cache duration |
| `JOB_RESULT_TTL` | `24h` | How long results stay in Redis |
| `WORKER_POOL_SIZE` | `5` | Concurrent status update workers |

### Scraper Worker

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | — | PostgreSQL connection string (required) |
| `REDIS_URL` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | — | Redis password |
| `REDIS_DB` | `7` | Redis database number |
| `SQS_QUEUE_NAME` | — | SQS queue to consume jobs from (required) |
| `SQS_REGION` | `us-east-1` | SQS region |
| `SQS_ENDPOINT` | — | Custom SQS endpoint (LocalStack) |
| `SQS_MAX_MESSAGES` | `5` | Max messages per SQS poll |
| `SQS_WAIT_TIME` | `20` | Long-poll wait time (seconds) |
| `STATUS_UPDATE_QUEUE_NAME` | `request-status-processor-queue` | Queue for status updates |
| `BROWSER_WS_URL` | `ws://localhost:9222/camoufox` | Browser service WebSocket URL |
| `BROWSER_TIMEOUT` | `60` | Page navigation timeout (seconds) |
| `BROWSER_LOAD_WAIT` | `2` | Wait after page load (seconds) |
| `S3_BUCKET` | — | S3 bucket for result storage |
| `S3_REGION` | `us-east-1` | S3 region |
| `S3_ENDPOINT` | — | Custom S3 endpoint |
| `S3_FORCE_PATH_STYLE` | `false` | Use path-style S3 URLs |
| `S3_ACCESS_KEY` | — | AWS access key |
| `S3_SECRET_KEY` | — | AWS secret key |
| `JOB_TIMEOUT` | `120` | Max job duration (seconds) |
| `MAX_JOB_RETRIES` | `3` | Max retries before dead-letter queue |
| `WORKER_POOL_SIZE` | `5` | Concurrent scrape workers |
| `JOB_RESULT_TTL` | `86400` | Result cache TTL (seconds) |
| `GEMINI_API_KEY` | — | Google Gemini key (for JSON extraction) |
| `LOG_LEVEL` | `INFO` | Log level (DEBUG, INFO, WARN, ERROR) |

**Smart waiting (optional tuning):**

| Variable | Default | Description |
|----------|---------|-------------|
| `WAIT_STRATEGY` | `adaptive` | Wait strategy type |
| `MAX_WAIT_TIME` | `15` | Max wait time (seconds) |
| `STABILITY_INTERVAL` | `500` | DOM stability check interval (ms) |
| `STABILITY_CHECKS` | `2` | Number of stability checks |
| `ENABLE_FRAMEWORK_DETECT` | `true` | Detect React/Vue/Angular for smarter waits |
| `NETWORK_QUIET_WINDOW` | `1500` | Network idle window (ms) |

### Browser Service

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `9222` | WebSocket port |
| `WS_PATH` | `camoufox` | WebSocket path |
| `HEALTH_CHECK_PORT` | `8080` | Health check HTTP port |
| `HEADLESS` | `true` | Run browser headless |
| `PROXY_SERVER` | — | Proxy server URL |
| `PROXY_USERNAME` | — | Proxy username |
| `PROXY_PASSWORD` | — | Proxy password |
| `PROXY_COUNTRY` | `us` | Default proxy country |

### Proxy Configuration

Proxies are optional. Without a proxy, all requests come from your server's IP.

To add a proxy, set these variables on the **browser service** and/or **scraper worker**:

```bash
# Browser service (browser-based scraping)
PROXY_SERVER=http://proxy.example.com:9000
PROXY_USERNAME=user
PROXY_PASSWORD=pass

# Scraper worker (HTTP-based scraping)
PROXY_URL=http://proxy.example.com:9000
PROXY_USERNAME=user
PROXY_PASSWORD=pass
```

Any HTTP/SOCKS5 proxy works. Residential proxies are recommended for sites with anti-bot protection.

## Self-Hosting

### With Docker Compose (recommended)

The included `docker-compose.yml` runs the full stack with LocalStack emulating AWS services (S3 + SQS).

```bash
# Start everything
docker compose up -d

# Initialize LocalStack (creates queues + buckets)
./scripts/setup-localstack.sh

# Check service health
curl http://localhost:8080/health
```

**Infrastructure created by `setup-localstack.sh`:**

| Resource | Name | Purpose |
|----------|------|---------|
| S3 Bucket | `anakinscraper` | Result storage |
| SQS Queue | `url-scraper-task-queue` | Job queue (visibility: 120s, retention: 24h) |
| SQS Queue | `request-status-processor-queue` | Status updates (visibility: 60s) |
| SQS Queue | `scraper-jobs-dlq` | Dead-letter queue for failed jobs |

### With Real AWS

Replace LocalStack with real AWS services:

1. Create an S3 bucket and SQS queues (same names as above)
2. Create an IAM user with S3 + SQS permissions
3. Update `.env` with real AWS credentials:

```bash
S3_ENDPOINT=          # Remove (uses real S3)
SQS_ENDPOINT=         # Remove (uses real SQS)
S3_ACCESS_KEY=AKIA...
S3_SECRET_KEY=...
SQS_ACCESS_KEY=AKIA...
SQS_SECRET_KEY=...
S3_BUCKET=your-bucket-name
S3_REGION=us-east-1
SQS_REGION=us-east-1
```

### With MinIO (S3-compatible)

If you prefer self-hosted S3:

```bash
S3_ENDPOINT=http://minio:9000
S3_FORCE_PATH_STYLE=true
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
```

## SDKs

Official client SDKs (MIT-licensed, separate repos):

### Python

```bash
pip install anakinscraper
```

```python
from anakinscraper import AnakinScraper

client = AnakinScraper(api_key="sk_live_...", base_url="http://localhost:8080")

# Single URL
result = client.scrape("https://example.com")
print(result.markdown)

# Batch
results = client.scrape_batch([
    "https://example.com/page-1",
    "https://example.com/page-2",
])
for r in results:
    print(r.url, r.markdown[:100])
```

### TypeScript / JavaScript

```bash
npm install anakinscraper
```

```typescript
import { AnakinScraper } from 'anakinscraper';

const client = new AnakinScraper({
  apiKey: 'sk_live_...',
  baseUrl: 'http://localhost:8080',
});

// Single URL
const result = await client.scrape('https://example.com');
console.log(result.markdown);

// Batch
const results = await client.scrapeBatch([
  'https://example.com/page-1',
  'https://example.com/page-2',
]);
```

### Go

```bash
go get github.com/AnakinAI/anakinscraper-go
```

```go
client := anakinscraper.New("sk_live_...", anakinscraper.WithBaseURL("http://localhost:8080"))

result, err := client.Scrape(ctx, &anakinscraper.ScrapeRequest{
    URL: "https://example.com",
})
fmt.Println(result.Markdown)
```

## Examples

See the [examples/](examples/) directory:

| Example | Description |
|---------|-------------|
| [basic-scrape/](examples/basic-scrape/) | Scrape a single URL and get markdown |
| [batch-scrape/](examples/batch-scrape/) | Scrape multiple URLs in one request |

## Development

### Project Structure

```
anakinscraper-oss/
├── api/                        # Go REST API (Fiber v2)
│   ├── cmd/api/                # Entry point
│   └── internal/
│       ├── config/             # Environment config
│       ├── http/
│       │   ├── handlers/       # Request handlers
│       │   ├── middleware/     # Auth, rate limiting
│       │   └── router/         # Route registration
│       └── models/             # Data models
├── scraper-service/            # Go background worker
│   ├── cmd/scraper/            # Entry point
│   └── internal/
│       ├── converter/          # HTML → Markdown
│       ├── handler/            # Handler chain (HTTP, Browser)
│       └── processor/          # Job processing loop
├── browser-service/            # Python Playwright server
├── docs/                       # API documentation
├── examples/                   # Usage examples
├── sdks/
│   ├── python/                 # Python SDK
│   └── typescript/             # TypeScript SDK
├── docker-compose.yml          # Full self-host stack
├── scripts/
│   └── setup-localstack.sh     # Initialize SQS queues + S3 buckets
└── .env.example                # Configuration template
```

### Running Locally (without Docker)

**Prerequisites:** Go 1.25+, Python 3.11+, PostgreSQL, Redis

```bash
# Terminal 1: PostgreSQL + Redis (if not already running)
docker compose up postgres redis localstack -d
./scripts/setup-localstack.sh

# Terminal 2: API
cd api && go run cmd/api/main.go

# Terminal 3: Scraper Worker
cd scraper-service && go run cmd/scraper/main.go

# Terminal 4: Browser Service
cd browser-service && python server.py
```

### Running Tests

```bash
# API
cd api && go test ./...

# Scraper Worker
cd scraper-service && go test ./...
```

### Building Docker Images

```bash
# API
docker build -t anakinscraper-api -f api/Dockerfile .

# Scraper Worker
docker build -t anakinscraper-scraper -f scraper-service/Dockerfile .

# Browser Service
docker build -t anakinscraper-browser -f browser-service/Dockerfile .
```

## What's NOT Included (Cloud-Only)

The following features are part of the managed [AnakinScraper Cloud](https://anakin.io) offering and are not in this repo:

- **Intelligent proxy selection** — Thompson Sampling algorithm that learns which proxies work best per domain
- **Domain-specific strategies** — auto-tuned scraping configs per domain based on failure analysis
- **Cookie warming** — pre-navigation cookie setup for sites that require session state
- **Real device handler** — macOS Safari/Chrome scraping for the toughest anti-bot protections
- **Agentic search** — multi-stage AI search pipeline (Perplexity + browser)
- **Credit system & billing** — usage metering, Stripe integration
- **Anti-detection tuning** — Camoufox fingerprint config, CapSolver CAPTCHA solving
- **Interactive browser sessions** — noVNC-based manual login with cookie/storage export
- **Production autoscaling** — ECS auto-scaling configs
- **Telemetry pipeline** — scrape metrics aggregation and optimization
- **Admin dashboard & analytics** — usage dashboards, job monitoring

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Quick overview:**

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Run tests (`cd api && go test ./... && cd ../scraper-service && go test ./...`)
5. Submit a pull request

## License

- **Engine** (api/, scraper-service/, browser-service/): [AGPL-3.0](LICENSE)
- **SDKs** (sdks/): [MIT](sdks/LICENSE)
