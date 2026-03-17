# Batch Scrape Example

Scrape multiple URLs in a single API request (up to 10).

## Prerequisites

Start the stack first:

```bash
cd ../.. && make up
```

## Run

```bash
./batch.sh
```

Or with curl directly:

```bash
# Submit batch job
JOB_ID=$(curl -s -X POST http://localhost:8080/v1/url-scraper/batch \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://example.com", "https://httpbin.org/html"]}' | jq -r '.id')

echo "Batch job: $JOB_ID"

# Poll for results
sleep 3
curl -s http://localhost:8080/v1/url-scraper/batch/$JOB_ID | jq '.results[] | {url, status}'
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ANAKIN_BASE_URL` | `http://localhost:8080` | API URL |
