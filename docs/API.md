# AnakinScraper API Reference

Base URL: `http://localhost:8080` (default)

## Authentication

All endpoints except `/health` require an API key. Pass it via any of these headers:

```
X-API-Key: sk_live_abc123
Authorization: Bearer sk_live_abc123
Api-Key: sk_live_abc123
```

### Create an API Key

```bash
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key"}'
```

Response:

```json
{
  "id": "key-uuid",
  "name": "my-key",
  "key": "sk_live_abc123...",
  "createdAt": "2025-01-15T10:00:00Z"
}
```

The full key is shown only once. Store it securely.

---

## Endpoints

### Health Check

```
GET /health
```

No authentication required.

```bash
curl http://localhost:8080/health
```

Response:

```json
{"status": "ok", "redis": true, "service": "anakinscraper"}
```

---

### Scrape a Single URL

```
POST /v1/url-scraper
```

Submit a scrape job for a single URL. Returns immediately with a job ID. Poll the job result endpoint to get the scraped content.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/url-scraper \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "country": "us",
    "forceFresh": false,
    "useBrowser": false,
    "generateJson": false
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to scrape |
| `country` | string | `"us"` | Proxy country code |
| `forceFresh` | bool | `false` | Bypass cache |
| `useBrowser` | bool | `false` | Force browser rendering (skip HTTP handler) |
| `generateJson` | bool | `false` | Extract structured JSON via AI (requires Gemini API key) |

**Response (pending):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "url": "https://example.com",
  "jobType": "url_scraper"
}
```

---

### Get Job Result

```
GET /v1/url-scraper/:id
```

Retrieve the current state of a scrape job.

```bash
curl http://localhost:8080/v1/url-scraper/550e8400-e29b-41d4-a716-446655440000 \
  -H "X-API-Key: YOUR_API_KEY"
```

**Response (completed):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "url": "https://example.com",
  "jobType": "url_scraper",
  "html": "<html>...</html>",
  "cleanedHtml": "<main>...</main>",
  "markdown": "# Page Title\n\nContent here...",
  "generatedJson": null,
  "cached": false,
  "createdAt": "2025-01-15T10:30:00Z",
  "completedAt": "2025-01-15T10:30:03Z",
  "durationMs": 3421
}
```

**Response (failed):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "url": "https://example.com",
  "error": "all handlers failed: HTTP 403, browser timeout"
}
```

**Status lifecycle:** `pending` -> `processing` -> `completed` | `failed`

---

### Batch Scrape

```
POST /v1/url-scraper/batch
```

Scrape up to 10 URLs in a single request. Returns a batch job ID. Poll for results.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/url-scraper/batch \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "urls": [
      "https://example.com/page-1",
      "https://example.com/page-2",
      "https://example.com/page-3"
    ],
    "country": "us",
    "useBrowser": false,
    "generateJson": false
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `urls` | string[] | **required** | 1 to 10 URLs to scrape |
| `country` | string | `"us"` | Proxy country code |
| `useBrowser` | bool | `false` | Force browser rendering |
| `generateJson` | bool | `false` | Extract structured JSON |

**Response (pending):**

```json
{
  "id": "batch-job-uuid",
  "status": "pending",
  "jobType": "batch_url_scraper",
  "urls": ["url1", "url2", "url3"]
}
```

**Poll for results:**

```bash
curl http://localhost:8080/v1/url-scraper/batch/BATCH_JOB_UUID \
  -H "X-API-Key: YOUR_API_KEY"
```

**Response (completed):**

```json
{
  "id": "batch-job-uuid",
  "status": "completed",
  "jobType": "batch_url_scraper",
  "urls": ["url1", "url2", "url3"],
  "results": [
    {
      "index": 0,
      "url": "https://example.com/page-1",
      "status": "completed",
      "html": "...",
      "cleanedHtml": "...",
      "markdown": "...",
      "cached": false,
      "durationMs": 1234
    },
    {
      "index": 1,
      "url": "https://example.com/page-2",
      "status": "completed",
      "html": "...",
      "cleanedHtml": "...",
      "markdown": "...",
      "cached": false,
      "durationMs": 987
    }
  ],
  "createdAt": "...",
  "completedAt": "...",
  "durationMs": 5678
}
```

---

### Discover URLs (Map)

```
POST /v1/map
```

Discover links on a page. Useful for building a list of URLs to scrape.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/map \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "includeSubdomains": false,
    "limit": 100,
    "search": "blog"
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Starting URL |
| `includeSubdomains` | bool | `false` | Include links to subdomains |
| `limit` | int | `100` | Max links to return (max: 5000) |
| `search` | string | -- | Filter links containing this string |

**Poll for results:**

```bash
curl http://localhost:8080/v1/map/MAP_JOB_UUID \
  -H "X-API-Key: YOUR_API_KEY"
```

**Response (completed):**

```json
{
  "id": "map-job-uuid",
  "status": "completed",
  "url": "https://example.com",
  "links": [
    "https://example.com/blog/post-1",
    "https://example.com/blog/post-2",
    "https://example.com/about"
  ],
  "totalLinks": 42
}
```

---

### Multi-Page Crawl

```
POST /v1/crawl
```

Crawl multiple pages starting from a URL. The crawler follows links on the page, respecting the include/exclude patterns.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/crawl \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "maxPages": 10,
    "includePatterns": ["/blog/**"],
    "excludePatterns": ["/admin/**"],
    "country": "us"
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Starting URL |
| `maxPages` | int | `10` | Max pages to crawl (max: 100) |
| `includePatterns` | string[] | -- | Glob patterns for URLs to include |
| `excludePatterns` | string[] | -- | Glob patterns for URLs to exclude |
| `country` | string | `"us"` | Proxy country code |
| `useBrowser` | bool | `false` | Force browser rendering |

**Poll for results:**

```bash
curl http://localhost:8080/v1/crawl/CRAWL_JOB_UUID \
  -H "X-API-Key: YOUR_API_KEY"
```

**Response (completed):**

```json
{
  "id": "crawl-job-uuid",
  "status": "completed",
  "url": "https://example.com",
  "totalPages": 10,
  "completedPages": 8,
  "results": [
    {
      "url": "https://example.com/",
      "status": "completed",
      "markdown": "# Home\n\nWelcome...",
      "durationMs": 2100
    },
    {
      "url": "https://example.com/blog/post-1",
      "status": "completed",
      "markdown": "# Blog Post 1\n\n...",
      "durationMs": 1800
    },
    {
      "url": "https://example.com/blog/post-2",
      "status": "failed",
      "error": "HTTP 404",
      "durationMs": 350
    }
  ]
}
```

---

## Rate Limits

| Endpoint | Limit |
|----------|-------|
| `POST /v1/url-scraper` | 60 requests/min per user |
| `POST /v1/url-scraper/batch` | 30 requests/min per user |
| `POST /v1/map` | 30 requests/min per user |
| `POST /v1/crawl` | 10 requests/min per user |

When rate-limited, the API returns `429 Too Many Requests`:

```bash
curl -s -w "\n%{http_code}" -X POST http://localhost:8080/v1/url-scraper \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

```json
{
  "error": "rate_limited",
  "message": "Too many requests. Please slow down."
}
```

---

## Error Codes

All error responses follow this format:

```json
{
  "error": "error_code",
  "message": "Human-readable description"
}
```

| HTTP Status | Error Code | Description |
|-------------|------------|-------------|
| 400 | `invalid_url` | Malformed or missing URL |
| 400 | `invalid_request` | Invalid request body or parameters |
| 401 | `unauthorized` | Missing or invalid API key |
| 429 | `rate_limited` | Too many requests -- slow down |
| 500 | `internal_error` | Unexpected server error |

### Job-level errors

Jobs can also fail after being accepted. When polling a job, check the `status` field:

| Status | Meaning |
|--------|---------|
| `pending` | Job is queued, waiting for a worker |
| `processing` | Worker is actively scraping the URL |
| `completed` | Scrape succeeded -- results are available |
| `failed` | Scrape failed -- check the `error` field for details |

Common job failure reasons:

| Error | Description |
|-------|-------------|
| `all handlers failed` | Both HTTP and browser handlers failed to scrape the page |
| `timeout` | Page load exceeded the configured timeout |
| `blocked` | The target site blocked the request (403/captcha) |
| `invalid_url` | URL could not be resolved or connected to |

---

## Polling Pattern

All mutation endpoints (scrape, batch, map, crawl) return immediately with a job ID. Poll the corresponding GET endpoint until `status` is `completed` or `failed`.

Recommended polling strategy:

1. Wait 1 second after submitting
2. Poll every 1-2 seconds
3. Set a maximum timeout (120 seconds recommended)
4. Handle `failed` status gracefully

```bash
# 1. Submit
JOB_ID=$(curl -s -X POST http://localhost:8080/v1/url-scraper \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq -r '.id')

echo "Job ID: $JOB_ID"

# 2. Poll
while true; do
  RESULT=$(curl -s http://localhost:8080/v1/url-scraper/$JOB_ID \
    -H "X-API-Key: YOUR_API_KEY")

  STATUS=$(echo $RESULT | jq -r '.status')
  echo "Status: $STATUS"

  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    echo $RESULT | jq .
    break
  fi

  sleep 1
done
```

Or use the SDKs, which handle polling automatically:

```python
from anakinscraper import AnakinScraper

client = AnakinScraper(api_key="sk_live_...", base_url="http://localhost:8080")
result = client.scrape("https://example.com")  # blocks until done
print(result.markdown)
```

```typescript
import { AnakinScraper } from 'anakinscraper';

const client = new AnakinScraper({ apiKey: 'sk_live_...', baseUrl: 'http://localhost:8080' });
const result = await client.scrape('https://example.com'); // awaits until done
console.log(result.markdown);
```
