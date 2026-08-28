# Changelog

## Unreleased

### Fixed
- **Relative links on pages with `<base href>`** — the markdown converter resolved relative `href`/`src` values against the page URL and ignored the document's `<base href>`, so every link and image scraped from a page that sets one came out pointing at the wrong host or path (`server/internal/converter/markdown.go`)
- **Proxy latency tracking** — the average latency EMA now seeds with the first observed sample instead of blending it against a zero baseline, which had caused a proxy's first request to be recorded at ~20% of its true latency (`server/internal/proxy/pool.go`)

## v0.1.1 (2026-03-20)

### Added
- **Anonymous telemetry** — event-level usage telemetry with hourly batch reporting (opt-out: `TELEMETRY=off`). See [TELEMETRY.md](TELEMETRY.md)
- **Transparency endpoint** — `GET /v1/telemetry/status` shows exactly what telemetry data will be sent
- **Startup banner notice** — telemetry status displayed on server boot
- **Privacy documentation** — `TELEMETRY.md` details what is and isn't collected

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
- **CLI** — `anakin scrape/batch/health` commands ([anakin-cli](https://github.com/Anakin-Inc/anakin-cli))
- **Web dashboard** — React UI for scraping, job tracking, domain configs, proxy monitoring
- **OpenClaw skill** — AI agent integration

### Infrastructure
- Single Go binary + Camoufox browser + PostgreSQL
- Docker Compose for one-command deployment
- GitHub Actions CI
- 35 unit tests
