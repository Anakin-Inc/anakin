# Changelog

## v0.1.0 (2026-03-18)

Initial open-source release.

### Features
- **Sync scraping** — `POST /v1/scrape` returns results directly (30s timeout)
- **Async scraping** — `POST /v1/url-scraper` with polling
- **Batch scraping** — up to 10 URLs in one request
- **Handler chain** — HTTP fetch with automatic fallback to Camoufox anti-detect browser
- **Structured JSON extraction** — Gemini 2.5 Flash AI extraction (bring your own API key)
- **Domain configs** — per-domain handler selection, timeouts, retries, content validation, custom headers
- **Proxy auto-selection** — Thompson Sampling picks the best proxy per domain
- **HTML to Markdown** — intelligent content extraction with boilerplate removal

### Tools
- **CLI** — `anakinscraper scrape/batch/health` commands
- **MCP server** — Claude Desktop, Cursor, VS Code, Windsurf integration
- **OpenClaw skill** — AI agent integration

### Infrastructure
- Single Go binary + Camoufox browser + PostgreSQL
- Docker Compose for one-command deployment
- GitHub Actions CI
- 35 unit tests
