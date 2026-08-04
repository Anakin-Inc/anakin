# Changelog

## Unreleased

### Fixed
- **Batch result ordering without a database** — `MemoryStore.GetChildJobs` returned children in Go's randomised map order, so polling a batch twice gave different orders and each result's `index` identified nothing. It now returns creation order, matching `PostgresStore` (`server/internal/store/memory.go`)
- **Job status without a database** — `MemoryStore.CreateJob` left `status` empty while `PostgresStore` starts jobs at `pending`. No branch of `UpdateParentBatchStatus` counts an empty status, so a freshly submitted batch was reported `completed` before any child had run (`server/internal/store/memory.go`)
- **In-memory store grows without bound** — `MemoryStore` never removed anything, and each completed job retains the page HTML, cleaned HTML and markdown, so the DB-less mode grew until the process was OOM-killed. It now evicts the oldest jobs once `MEMORY_STORE_MAX_JOBS` is exceeded (`server/internal/store/memory.go`)

### Added
- **`MEMORY_STORE_MAX_JOBS`** — bounds retention of the in-memory store used when `DATABASE_URL` is unset (default 500). Set to `0` to restore unbounded retention

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
