# Changelog

## Unreleased

### Security
- **SSRF guard missed Alibaba Cloud instance metadata** — `netguard.Blocked` relied on `net.IP.IsPrivate`, which covers only RFC 1918 and `fc00::/7`, so `100.100.100.200` (Alibaba Cloud's metadata endpoint) and the rest of RFC 6598 shared address space were treated as public and scraped on request. `192.0.0.0/24`, `198.18.0.0/15` and `240.0.0.0/4` (including the `255.255.255.255` broadcast address) were open for the same reason. All four ranges are now blocked at both the request boundary and dial time (`server/internal/netguard/netguard.go`)

### Fixed
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
