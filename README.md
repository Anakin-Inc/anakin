# AnakinScraper OSS

[![CI](https://github.com/AnakinAI/anakinscraper-oss/actions/workflows/ci.yml/badge.svg)](https://github.com/AnakinAI/anakinscraper-oss/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

The open-source web scraping API for AI. Turn any website into LLM-ready markdown or structured data.

Self-host with a single command. No cloud dependencies. Powers RAG pipelines, AI agents, and data extraction at scale.

```bash
git clone https://github.com/AnakinAI/anakinscraper-oss.git && cd anakinscraper-oss && make up

# Scrape any website — one curl, full result:
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq .markdown
```

## Want managed scraping?

[anakin.io](https://anakin.io) gives you everything in this repo plus:

- Geo-proxies in 195 countries
- Built-in caching and rate limiting
- AI-powered structured data extraction at scale
- 99.9% uptime SLA
- Zero infrastructure to manage

[Get started free at anakin.io →](https://anakin.io)

## Features

- **Sync + async API** — `POST /v1/scrape` returns the result directly; `/v1/url-scraper` for async with polling
- **Batch scraping** — scrape up to 10 URLs in one request
- **Handler chain** — fast HTTP fetch with automatic fallback to [Camoufox](https://github.com/daijro/camoufox) anti-detect browser
- **Domain configs** — per-domain scraping strategies: handler selection, timeouts, retries, content validation, custom headers, domain blocking
- **Proxy auto-select** — [Thompson Sampling](https://en.wikipedia.org/wiki/Thompson_sampling) picks the best proxy per domain from a pool, learning from success/failure
- **Structured JSON extraction** — use Gemini AI to extract structured data from any page (bring your own API key)
- **HTML to Markdown** — intelligent content extraction with boilerplate removal
- **Self-contained** — just PostgreSQL + one Go binary + anti-detect browser. No Redis, no AWS, no message queues

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
| Browser Service | 9222 | Camoufox anti-detect browser (WebSocket) |
| PostgreSQL | 5432 | Job storage |

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
                    │   (cURL / SDK)  │
                    └────────┬────────┘
                             │ HTTP
                             ▼
                    ┌─────────────────┐         ┌──────────┐
                    │     Server      │         │  Gemini  │
                    │   (Go/Fiber)    │────────▶│  2.5     │
                    │   API + Workers │ optional│  Flash   │
                    │   Port 8080     │         └──────────┘
                    └────┬───────┬───┘
                         │       │
              ┌──────────┘       └──────────┐
              ▼                             ▼
        ┌──────────┐               ┌──────────────┐
        │PostgreSQL │               │   Browser    │
        │  jobs +   │               │   Service    │
        │  configs  │               │  (Camoufox)  │
        └──────────┘               └──────────────┘
```

The server is a single Go binary. API handlers accept requests, insert jobs into PostgreSQL, and push them to an in-process worker pool via Go channels. Workers execute the handler chain (HTTP fetch → browser fallback), convert HTML to markdown, optionally extract structured JSON via Gemini, and write results back to the database. No external queues or object storage.

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
| `GET` | `/health` | Health check |

### Request Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to scrape |
| `useBrowser` | bool | `false` | Skip HTTP handler, go straight to browser |
| `generateJson` | bool | `false` | Extract structured JSON via Gemini AI (requires `GEMINI_API_KEY`) |
| `country` | string | `"us"` | *Reserved — available in [hosted version](https://anakin.io)* |
| `forceFresh` | bool | `false` | *Reserved — available in [hosted version](https://anakin.io)* |

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
HTTP Handler (fast, ~200ms) ──fail──▶ Browser Handler (Camoufox, anti-detect Firefox)
```

**HTTP Handler** — direct HTTP GET with a browser user-agent. Handles static HTML, server-rendered pages. No browser overhead.

**Browser Handler** — connects to [Camoufox](https://github.com/daijro/camoufox) (anti-detect Firefox) over WebSocket via Playwright protocol. Full JavaScript rendering, network-idle detection, realistic browser fingerprints. Handles SPAs, lazy-loaded content, and sites with anti-bot protection.

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
| `BROWSER_WS_URL` | `ws://localhost:9222/camoufox` | Browser service WebSocket URL |
| `BROWSER_TIMEOUT` | `60` | Page navigation timeout (seconds) |
| `BROWSER_LOAD_WAIT` | `2` | Extra wait after page load (seconds) |
| `WORKER_POOL_SIZE` | `5` | Concurrent scrape workers |
| `JOB_BUFFER_SIZE` | `100` | Job queue buffer size |
| `JOB_TIMEOUT` | `120` | Max job duration (seconds) |
| `PROXY_URL` | — | Default HTTP proxy for the HTTP handler |
| `PROXY_URLS` | — | Comma-separated proxy pool for auto-selection (Thompson Sampling) |
| `GEMINI_API_KEY` | — | Google Gemini API key for structured JSON extraction ([get one free](https://aistudio.google.com/apikey)) |
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
│       ├── gemini/             # Gemini AI JSON extraction
│       ├── domain/             # Domain configs + failure detection
│       ├── proxy/              # Proxy pool + Thompson Sampling
│       ├── processor/          # Job processing
│       └── http/
│           ├── handlers/       # API request handlers
│           └── router/         # Route registration
├── cli/                        # Go CLI tool
├── mcp-server/                 # MCP server for Claude/Cursor/VS Code
├── openclaw-skill/             # OpenClaw skill wrapper
├── browser-service/            # Camoufox anti-detect browser server
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

## CLI

Scrape websites from your terminal:

```bash
# Install
go install github.com/AnakinAI/anakinscraper-oss/cli@latest

# Scrape a URL (prints markdown)
anakinscraper scrape https://example.com

# Extract structured JSON
anakinscraper scrape --extract https://example.com

# Batch scrape
anakinscraper batch https://example.com https://httpbin.org/html

# Full JSON output
anakinscraper scrape --json https://example.com
```

See [cli/README.md](cli/README.md) for full usage.

## Integrations

### MCP Server (Claude, Cursor, VS Code, Windsurf)

Add AnakinScraper as a tool for AI agents:

```bash
npx -y anakinscraper-mcp
```

Or add to your Claude Desktop / Cursor config:

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "npx",
      "args": ["-y", "anakinscraper-mcp"],
      "env": { "ANAKINSCRAPER_API_URL": "http://localhost:8080" }
    }
  }
}
```

See [mcp-server/README.md](mcp-server/README.md) for details.

### OpenClaw Skill

Use AnakinScraper as an [OpenClaw](https://openclaw.ai) skill:

```bash
cp -r openclaw-skill ~/.openclaw/workspace/skills/anakinscraper
```

See [openclaw-skill/SKILL.md](openclaw-skill/SKILL.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[AGPL-3.0](LICENSE) — free to self-host. If you modify the code and offer it as a hosted service, you must open-source your changes.

---

Built by [AnakinAI](https://anakin.io). If you find this useful, give us a star!
