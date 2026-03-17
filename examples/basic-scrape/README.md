# Basic Scrape Example

Scrape a single URL using the synchronous `/v1/scrape` endpoint.

## Prerequisites

Start the stack first:

```bash
cd ../.. && make up
```

## Run

```bash
# Default URL (example.com)
./scrape.sh

# Custom URL
./scrape.sh https://news.ycombinator.com
```

Or with curl directly:

```bash
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq .
```

## With Structured JSON Extraction

Set `GEMINI_API_KEY` and pass `generateJson: true`:

```bash
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "generateJson": true}' | jq .generatedJson
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ANAKIN_BASE_URL` | `http://localhost:8080` | API URL |
