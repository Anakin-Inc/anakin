# AnakinScraper OSS

[![CI](https://github.com/Anakin-Inc/anakinscraper-oss/actions/workflows/ci.yml/badge.svg)](https://github.com/Anakin-Inc/anakinscraper-oss/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.11-3776AB?logo=python&logoColor=white)](https://python.org)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)](docker-compose.yml)
[![React](https://img.shields.io/badge/Web_UI-React_19-61DAFB?logo=react&logoColor=white)](webapp/)

The open-source web scraping API for AI. Turn any website into LLM-ready markdown or structured data.

Self-host with a single command. No cloud dependencies. Powers RAG pipelines, AI agents, and data extraction at scale.

```bash
git clone https://github.com/Anakin-Inc/anakinscraper-oss.git && cd anakinscraper-oss && make up

# Scrape any website — one curl, full result:
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq .markdown
```

## Features

- **Sync + async API** — `POST /v1/scrape` returns the result directly; `/v1/url-scraper` for async with polling
- **Batch scraping** — scrape up to 10 URLs in one request
- **Handler chain** — fast HTTP fetch with automatic fallback to [Camoufox](https://github.com/daijro/camoufox) anti-detect browser
- **Domain configs** — per-domain scraping strategies: handler selection, timeouts, retries, content validation, custom headers, domain blocking
- **Proxy auto-select** — [Thompson Sampling](https://en.wikipedia.org/wiki/Thompson_sampling) picks the best proxy per domain from a pool, learning from success/failure
- **Structured JSON extraction** — use Gemini AI to extract structured data from any page (bring your own API key)
- **HTML to Markdown** — intelligent content extraction with boilerplate removal
- **Web dashboard** — built-in React UI for scraping, job tracking, domain config management, and proxy monitoring
- **Zero-config mode** — run with just Go, no PostgreSQL needed. Or use Docker for the full stack
- **Self-contained** — no Redis, no AWS, no message queues. Optional PostgreSQL for persistence

## Quick Start (no Docker, no database)

Just Go 1.25+. Two commands:

```bash
cd server && go run cmd/server/main.go

# In another terminal:
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq .markdown
```

Jobs are stored in memory (lost on restart). For persistence, set `DATABASE_URL`. For JavaScript-heavy sites, add the browser service via Docker.

## Self-Host (Docker — full stack)

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose

### Start

```bash
git clone https://github.com/Anakin-Inc/anakinscraper-oss.git
cd anakinscraper-oss
make up
```

That's it. Three containers start:

| Service | Port | Description |
|---------|------|-------------|
| Server | 8080 | REST API + worker pool |
| Browser Service | 9222 | Camoufox anti-detect browser (WebSocket) |
| PostgreSQL | 5432 | Job storage |

### Web Dashboard

A built-in web UI is included for visual scraping, job tracking, and configuration:

```bash
cd webapp && npm install && npm run dev
```

Open [http://localhost:3000](http://localhost:3000) — the dashboard proxies API calls to the server on port 8080.

Pages: **Dashboard** (health + quick scrape) | **Scrape** (sync/async/batch with live results) | **Jobs** (tracked history with status filters) | **Domain Configs** (CRUD with handler chain management) | **Proxy Scores** (Thompson Sampling performance)

### Scrape a URL

**Synchronous** (recommended for getting started):

```bash
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq .
```

One request, full result back. No polling. Timeout: 30 seconds.

**Asynchronous** (for long-running scrapes):

```bash
# Submit
curl -s -X POST http://localhost:8080/v1/url-scraper \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Poll for result
curl -s http://localhost:8080/v1/url-scraper/JOB_UUID | jq .
```

**With AI-powered JSON extraction** (requires `GEMINI_API_KEY`):

```bash
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "generateJson": true}' | jq .generatedJson
```

No API keys required for the scraper itself. Just JSON in, results out.

## Architecture

```
                    ┌─────────────────┐
                    │   Your App      │
                    │   (cURL / CLI)  │
                    └────────┬────────┘
                             │ HTTP
                             ▼
                    ┌─────────────────┐         ┌──────────┐
                    │     Server      │────────▶│  Gemini  │
                    │   (Go/Fiber)    │ optional│  (JSON)  │
                    │   Port 8080     │         └──────────┘
                    └──┬──────┬───┬──┘
                       │      │   │
            ┌──────────┘      │   └────────────┐
            ▼                 ▼                 ▼
      ┌──────────┐   ┌──────────────┐   ┌──────────────┐
      │ Storage  │   │   Browser    │   │ API Handler  │
      │ Postgres │   │   Service    │   │ (anakin.io   │
      │ or memory│   │  (Camoufox)  │   │  or custom)  │
      │(optional)│   │  (optional)  │   │  (optional)  │
      └──────────┘   └──────────────┘   └──────────────┘
```

The server is a single Go binary that runs with zero dependencies. Optionally add PostgreSQL for persistence, the browser service for JavaScript-heavy sites, and API handlers for hard-to-scrape sites. Workers execute the handler chain (HTTP → browser → API fallback), convert HTML to markdown, and optionally extract structured JSON via Gemini.

## API Reference

See [docs/API.md](docs/API.md) for the complete API reference. Quick overview:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/scrape` | **Sync** — scrape a URL and get the result back directly (30s timeout) |
| `POST` | `/v1/url-scraper` | **Async** — submit a scrape job, returns job ID |
| `GET` | `/v1/url-scraper/:id` | Poll for async job result |
| `POST` | `/v1/url-scraper/batch` | Batch scrape up to 10 URLs |
| `GET` | `/v1/url-scraper/batch/:id` | Poll for batch result |
| `POST` | `/v1/domain-configs` | Create a per-domain scraping config |
| `GET` | `/v1/domain-configs` | List all domain configs |
| `GET` | `/v1/proxy/scores` | View proxy Thompson Sampling scores |
| `GET` | `/v1/telemetry/status` | View telemetry state and next payload ([details](TELEMETRY.md)) |
| `GET` | `/health` | Health check |

### Request Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to scrape |
| `useBrowser` | bool | `false` | Skip HTTP handler, go straight to browser |
| `generateJson` | bool | `false` | Extract structured JSON via Gemini AI (requires `GEMINI_API_KEY`) |

### Response

```json
{
  "id": "550e8400-...",
  "status": "completed",
  "url": "https://example.com",
  "html": "<html>...</html>",
  "cleanedHtml": "<main>...</main>",
  "markdown": "# Page Title\n\nContent...",
  "generatedJson": {
    "status": "success",
    "data": {"title": "Page Title", "content": "..."}
  },
  "durationMs": 1234
}
```

`generatedJson` is only present when `generateJson: true` and `GEMINI_API_KEY` is configured.

## Handler Chain

Each scrape job goes through the handler chain. On failure, it falls back to the next handler:

```
HTTP Handler (fast, ~200ms) ──fail──▶ Browser Handler (Camoufox) ──fail──▶ API Handler (optional)
```

**HTTP Handler** — direct HTTP GET with a browser user-agent. Handles static HTML, server-rendered pages. No browser overhead.

**Browser Handler** — connects to [Camoufox](https://github.com/daijro/camoufox) (anti-detect Firefox) over WebSocket via Playwright protocol. Full JavaScript rendering, network-idle detection, realistic browser fingerprints. Handles SPAs, lazy-loaded content, and sites with anti-bot protection.

**API Handler** (optional) — delegates to an external scraping API when local handlers fail. Set `ANAKIN_API_KEY` to enable the built-in [anakin.io](https://anakin.io) fallback for hard-to-scrape sites (Cloudflare, DataDome, etc.). See [Adding Custom API Handlers](#adding-custom-api-handlers) below.

### Adding Custom API Handlers

The API handler pattern makes it easy to integrate any third-party scraping service as a chain fallback. The built-in anakin.io handler (`server/internal/handler/api.go`) is a working example — copy and modify it for your provider:

1. **Copy** `api.go` to `my_provider.go`
2. **Add a constructor** like `NewAnakinHandler` — set your provider's URL, auth header name, and response format
3. **Register** in `main.go`:
   ```go
   if cfg.MyProviderAPIKey != "" {
       handlers = append(handlers, handler.NewAPIHandler(handler.APIHandlerConfig{
           Name:       "my-provider",
           APIURL:     "https://api.my-provider.com/scrape",
           APIKey:     cfg.MyProviderAPIKey,
           AuthHeader: "Authorization",  // or "X-API-Key", "Bearer", etc.
       }))
   }
   ```
4. **Add env var** to `config.go`: `MyProviderAPIKey: os.Getenv("MY_PROVIDER_API_KEY")`

API keys always come from environment variables — never hardcoded. The handler only activates when its key is set.

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
| `DATABASE_URL` | — | PostgreSQL connection string (optional — uses in-memory storage when not set) |
| `BROWSER_WS_URL` | `ws://localhost:9222/camoufox` | Browser service WebSocket URL |
| `BROWSER_TIMEOUT` | `60` | Page navigation timeout (seconds) |
| `BROWSER_LOAD_WAIT` | `2` | Extra wait after page load (seconds) |
| `WORKER_POOL_SIZE` | `5` | Concurrent scrape workers |
| `JOB_BUFFER_SIZE` | `100` | Job queue buffer size |
| `JOB_TIMEOUT` | `120` | Max job duration (seconds) |
| `PROXY_URL` | — | Default HTTP proxy for the HTTP handler |
| `PROXY_URLS` | — | Comma-separated proxy pool for auto-selection (Thompson Sampling) |
| `GEMINI_API_KEY` | — | Google Gemini API key for structured JSON extraction ([get one free](https://aistudio.google.com/apikey)) |
| `ANAKIN_API_KEY` | — | [anakin.io](https://anakin.io) API key — enables hosted API as chain fallback for hard-to-scrape sites |
| `LOG_LEVEL` | `INFO` | Log level (DEBUG, INFO, WARN, ERROR) |
| `TELEMETRY` | `on` | Anonymous usage telemetry (`off` to disable — see [TELEMETRY.md](TELEMETRY.md)) |
| `TELEMETRY_URL` | — | Custom telemetry endpoint (defaults to `https://telemetry.anakin.io/v1/collect`) |
| `DISABLE_HOSTED_HINTS` | — | Set to `true` to suppress hosted service tips in error messages |

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
│       ├── gemini/             # Gemini AI JSON extraction
│       ├── domain/             # Domain configs + failure detection
│       ├── store/              # Job storage (PostgreSQL or in-memory)
│       ├── proxy/              # Proxy pool + Thompson Sampling
│       ├── telemetry/          # Anonymous usage telemetry
│       ├── processor/          # Job processing
│       └── http/
│           ├── handlers/       # API request handlers
│           └── router/         # Route registration
├── browser-service/            # Camoufox anti-detect browser server
├── webapp/                     # React web dashboard (Vite + Tailwind)
├── openclaw-skill/             # OpenClaw skill wrapper
├── examples/                   # Usage examples
├── docker-compose.yml          # Full stack (3 containers)
├── scripts/init-db.sql         # Database schema
└── .env.example                # Config template
```

## Development

### Running Locally (without Docker)

**Minimal (Go only):**

```bash
cd server && go run cmd/server/main.go
```

No database, no browser service. HTTP handler scrapes static sites. Jobs stored in memory.

**Full local stack (Go + Python + PostgreSQL):**

```bash
# Terminal 1: PostgreSQL
docker compose up postgres -d

# Terminal 2: Browser Service (for JS-heavy sites)
cd browser-service && pip install -r requirements.txt && python server.py

# Terminal 3: Server with persistence
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

## CLI

Use the [Anakin CLI](https://github.com/Anakin-Inc/anakin-cli) to scrape from your terminal. It works against both self-hosted and the [hosted API](https://anakin.io):

```bash
# Install
pip install anakin-cli

# Scrape via your local instance (no API key needed)
anakin scrape "https://example.com" --api-url http://localhost:8080

# Or set it as your default
export ANAKIN_API_URL="http://localhost:8080"
anakin scrape "https://example.com"

# Extract structured JSON
anakin scrape "https://example.com" --format json --api-url http://localhost:8080

# Batch scrape
anakin scrape-batch "https://example.com" "https://httpbin.org/html" --api-url http://localhost:8080
```

The same CLI also supports **AI web search** and **deep research** via the hosted API — [get a free API key](https://anakin.io/dashboard) to unlock those features.

See the [anakin-cli repo](https://github.com/Anakin-Inc/anakin-cli) for full usage.

## Integrations

### OpenClaw Skill

Use AnakinScraper as an [OpenClaw](https://openclaw.ai) skill:

```bash
cp -r openclaw-skill ~/.openclaw/workspace/skills/anakinscraper
```

See [openclaw-skill/SKILL.md](openclaw-skill/SKILL.md).

## Self-Hosted vs Hosted

This repo gives you the full scraping engine. [anakin.io](https://anakin.io) adds the infrastructure you'd otherwise build yourself:

| Feature | Self-Hosted (this repo) | Hosted ([anakin.io](https://anakin.io)) |
|---------|------------------------|----------------------------------------|
| Sync + async scraping | Yes | Yes |
| Batch scraping | Yes | Yes |
| Anti-detect browser | Yes | Yes |
| Structured JSON extraction | Yes (bring your own Gemini key) | Yes (built-in) |
| Domain configs | Yes | Yes |
| Proxy auto-selection | Yes (bring your own proxies) | Yes (195 countries included) |
| Geo-targeted proxies | — | 195 countries |
| AI web search | — | Yes |
| Deep agentic research | — | Yes |
| Zero infrastructure | — | Yes |

**Already self-hosting?** Switch to hosted with one line — same API, same CLI:

```bash
anakin login --api-key "ak-xxx"   # get your key at anakin.io/dashboard
anakin scrape "https://example.com"  # now routes through hosted
```

[Try it free →](https://anakin.io)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[AGPL-3.0](LICENSE) — free to self-host. If you modify the code and offer it as a hosted service, you must open-source your changes.

---

Built by [Anakin-Inc](https://anakin.io). If you find this useful, give us a star!
