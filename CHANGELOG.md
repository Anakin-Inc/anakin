# Changelog

## Unreleased

### Fixed
- **Domain config writes reported success for config that was never applied** — an invalid regex in `failurePatterns`/`requiredPatterns` was accepted with `201`, listed by `GET`, and then silently skipped by the detector, so failure detection was off with no error and no log line; `PUT /v1/domain-configs/:domain` for a domain that had no config returned `200` with the body echoed back while the `UPDATE` matched no row. Writes are now validated and return `400`, an update against a missing domain returns `404`, and a stored pattern that fails to compile is logged instead of dropped (`server/internal/domain/validate.go`, `server/internal/http/handlers/domain_config.go`)
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
