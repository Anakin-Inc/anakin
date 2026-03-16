# AnakinScraper OSS

Open-source web scraping engine. Turn any website into clean markdown or structured data.

## Architecture

```
API (Go Fiber) → Queue (SQS/Redis) → Scraper Worker → Browser Service (Playwright)
                                                    → Results: S3 + Redis cache
```

## Services

| Service | Language | Description |
|---------|----------|-------------|
| `api/` | Go | REST API with auth, rate limiting, job management |
| `scraper-service/` | Go | Background worker with handler chain (HTTP → Browser) |
| `browser-service/` | Python | Playwright browser with anti-detection basics |

## TODO

### Phase 1: Core Engine
- [ ] Sanitized Go API (Fiber) — health, scrape, batch, job status endpoints
- [ ] Auth middleware (API key via SHA256 hash lookup)
- [ ] Rate limiting middleware (token bucket per user)
- [ ] Job repository (PostgreSQL metadata, S3 content, Redis cache)
- [ ] SQS producer/consumer framework
- [ ] S3 storage client

### Phase 2: Scraper Worker
- [ ] Handler chain pattern (interface, registry, chain orchestrator)
- [ ] HTTP handler (direct fetch, simple proxy support)
- [ ] Browser handler (Playwright WebSocket client, basic wait strategies)
- [ ] HTML → Markdown converter
- [ ] Job processor (SQS consume → handler chain → store result → status update)
- [ ] Graceful shutdown

### Phase 3: Browser Service
- [ ] Playwright browser server (WebSocket)
- [ ] Health check server
- [ ] Watchdog with exponential backoff restart
- [ ] Basic proxy configuration
- [ ] Docker container

### Phase 4: Infrastructure
- [ ] `docker-compose.yml` — full self-host stack (API + Scraper + Browser + PostgreSQL + Redis + LocalStack)
- [ ] `.env.example` files per service
- [ ] Dockerfile per service
- [ ] `SELF_HOST.md` — setup guide
- [ ] `CONTRIBUTING.md`
- [ ] LocalStack setup script (SQS queues, S3 buckets)

### Phase 5: SDKs (MIT license, separate repos)
- [ ] `anakinscraper-py` — Python SDK
- [ ] `anakinscraper-js` — TypeScript SDK
- [ ] `anakinscraper-go` — Go SDK

### Phase 6: Examples & Docs
- [ ] Basic scrape example (single URL → markdown)
- [ ] Batch scrape example (multiple URLs)
- [ ] Custom handler example (extend the chain)
- [ ] API reference docs

## What's NOT Included (Cloud-Only)

- Intelligent proxy selection (Thompson Sampling)
- Domain-specific failure detection & strategies
- Cookie warming
- Real device handler (Mac Safari/Chrome)
- Agentic search pipeline
- Credit system & billing
- Anti-detection tuning (Camoufox config, CapSolver)
- Production autoscaling configs
- Telemetry-driven optimization
- Admin dashboard & analytics

## License

- Engine: AGPL-3.0
- SDKs: MIT
