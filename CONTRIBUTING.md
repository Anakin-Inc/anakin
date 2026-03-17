# Contributing to AnakinScraper OSS

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

```bash
# Clone the repo
git clone https://github.com/AnakinAI/anakinscraper-oss.git
cd anakinscraper-oss

# Start infrastructure
docker compose up postgres redis localstack -d
bash scripts/setup-localstack.sh

# Run API (terminal 1)
cd api && go run cmd/api/main.go

# Run Scraper Worker (terminal 2)
cd scraper-service && go run cmd/scraper/main.go

# Run Browser Service (terminal 3)
cd browser-service && python server.py
```

A default API key is created automatically: `sk_test_local_development_key_12345`

## Code Style

- **Go**: Run `gofmt` before committing. Follow standard Go conventions.
- **Python**: Follow PEP 8. Use type hints.
- **TypeScript**: Use strict mode. Run `npm run build` to check types.

## Adding a New Scraping Handler

1. Create a new file in `scraper-service/internal/handler/`
2. Implement the `ScrapingHandler` interface:

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *ScrapeRequest) bool
    Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResult, error)
    IsHealthy() bool
}
```

3. Register your handler in the chain in `cmd/scraper/main.go`
4. The handler's position in the chain determines fallback priority

## Running Tests

```bash
cd api && go test ./...
cd scraper-service && go test ./...
```

## Submitting Changes

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes and add tests
4. Run tests to verify nothing broke
5. Commit with a descriptive message
6. Open a pull request against `main`

## Reporting Issues

Open an issue on GitHub with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Environment details (OS, Go/Python version, Docker version)

## License

By contributing, you agree that your contributions will be licensed under AGPL-3.0 (engine) or MIT (SDKs).
