# Contributing to AnakinScraper OSS

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

```bash
# Clone the repo
git clone https://github.com/Anakin-Inc/anakinscraper-oss.git
cd anakinscraper-oss

# Start full stack (3 containers)
make up

# Or run individually:

# Terminal 1: PostgreSQL
docker compose up postgres -d

# Terminal 2: Browser Service
cd browser-service && pip install -r requirements.txt && python server.py

# Terminal 3: Server
cd server && DATABASE_URL="postgres://postgres:postgres@localhost:5432/anakinscraper?sslmode=disable" go run cmd/server/main.go
```

## Code Style

- **Go**: Run `gofmt` before committing. Follow standard Go conventions.
- **Python**: Follow PEP 8. Use type hints.

## Adding a New Scraping Handler

1. Create a new file in `server/internal/handler/`
2. Implement the `ScrapingHandler` interface:

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *models.HandlerRequest) bool
    Scrape(ctx context.Context, req *models.HandlerRequest) (*ScrapeResult, error)
    IsHealthy() bool
}
```

3. Register your handler in the chain in `server/cmd/server/main.go`
4. The handler's position in the chain determines fallback priority

See [examples/custom-handler/](examples/custom-handler/) for a working example.

## Running Tests

```bash
cd server && go test ./...
```

## Submitting Changes

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes and add tests
4. Run tests to verify nothing broke
5. Commit with a descriptive message
6. Open a pull request against `master`

## Reporting Issues

Open an issue on GitHub with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Environment details (OS, Go/Python version, Docker version)

## License

By contributing, you agree that your contributions will be licensed under AGPL-3.0.
