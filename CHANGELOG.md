# Changelog

## Unreleased

### Fixed
- **Disabled domain configs are now inert** — `isEnabled: false` still blocked the domain and still enforced `failurePatterns`/`requiredPatterns`/`minContentLength`, because those branches never consulted the field. Disabled configs are now dropped when the cache indexes them, so they also no longer shadow an enabled parent domain (`server/internal/domain/cache.go`)
- **Domain config `priority` is now applied** — it was loaded from the database and then discarded, so the nearest matching ancestor always won. The highest priority among matching configs now wins, with specificity as the tie-break (`server/internal/domain/cache.go`)
- **Domain config `requestTimeoutMs` is now applied** — it was copied onto the handler request and never read, so every domain used the process-wide timeout. It now bounds the individual handler attempt via a per-attempt context in the chain (`server/internal/handler/chain.go`, `server/internal/processor/processor.go`)

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
