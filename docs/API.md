# AnakinScraper API Reference

Base URL: `http://localhost:8080`

## Authentication

None. AnakinScraper OSS is designed for self-hosting and does not require authentication. All endpoints are open.

## Rate Limiting

None. You are responsible for managing load on your own infrastructure.

---

## Endpoints

### Health Check

```
GET /health
```

Returns the service health status. Always accessible.

```bash
curl http://localhost:8080/health
```

**Response:**

```json
{
  "status": "ok",
  "database": true,
  "service": "anakinscraper"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always `"ok"` |
| `database` | bool | Whether the PostgreSQL connection is healthy |
| `service` | string | Always `"anakinscraper"` |

---

### Scrape a URL (Synchronous)

```
POST /v1/scrape
```

Submit a scrape job and wait for the result. The server holds the connection open for up to 30 seconds. If the job completes within that window, the full result is returned directly. If not, a `408` timeout error is returned.

This is the simplest way to scrape a page when you do not want to deal with polling.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "useBrowser": false,
    "generateJson": false
  }'
```

**Request body:** Same as [POST /v1/url-scraper](#scrape-a-url-async).

**Response (completed):** Same shape as [GET /v1/url-scraper/:id](#get-job-result) when status is `completed`.

**Response (timeout):**

```json
{
  "error": "timeout",
  "message": "Job did not complete within 30 seconds. Use the async endpoint and poll for results."
}
```

---

### Scrape a URL (Async)

```
POST /v1/url-scraper
```

Submit a scrape job for a single URL. Returns immediately with a job ID. Poll [GET /v1/url-scraper/:id](#get-job-result) to retrieve the result.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/url-scraper \
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
| `url` | string | **required** | The URL to scrape. Must use `http` or `https` scheme. |
| `country` | string | `""` | Proxy country code. Reserved for future use (no-op in OSS; available on hosted [anakin.io](https://anakin.io)). |
| `forceFresh` | bool | `false` | Bypass cache. Reserved for future use (no-op in OSS; available on hosted [anakin.io](https://anakin.io)). |
| `useBrowser` | bool | `false` | Force browser rendering via Camoufox. When `false`, the HTTP handler is tried first and the browser is used as a fallback. |
| `generateJson` | bool | `false` | Extract structured JSON from the page using Gemini. Requires the `GEMINI_API_KEY` environment variable to be set on the server. |

**Response (`201 Created`):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "url": "https://example.com",
  "jobType": "url_scraper",
  "createdAt": "2025-01-15T10:30:00Z"
}
```

---

### Get Job Result

```
GET /v1/url-scraper/:id
```

Retrieve the current state of a scrape job.

```bash
curl http://localhost:8080/v1/url-scraper/550e8400-e29b-41d4-a716-446655440000
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
  "jobType": "url_scraper",
  "error": "all handlers failed: HTTP 403, browser timeout",
  "createdAt": "2025-01-15T10:30:00Z"
}
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Job UUID |
| `status` | string | One of: `pending`, `processing`, `completed`, `failed` |
| `url` | string | The URL that was scraped |
| `jobType` | string | `"url_scraper"` |
| `html` | string or null | Raw HTML of the page (only when `completed`) |
| `cleanedHtml` | string or null | Cleaned HTML with boilerplate removed (only when `completed`) |
| `markdown` | string or null | Page content converted to Markdown (only when `completed`) |
| `generatedJson` | object or null | AI-extracted structured data (only when `generateJson` was `true`) |
| `cached` | bool or null | Whether the result was served from cache (only when `completed`) |
| `error` | string or null | Error message (only when `failed`) |
| `createdAt` | string | ISO 8601 timestamp |
| `completedAt` | string or null | ISO 8601 timestamp (only when `completed`) |
| `durationMs` | int or null | Total processing time in milliseconds (only when `completed`) |

**`generatedJson` shape** (when present):

```json
{
  "status": "success",
  "data": { "title": "Example", "price": "$9.99", "..." : "..." }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `"success"` or `"failed"` |
| `data` | object | Extracted structured data (only when `status` is `"success"`) |

---

### Batch Scrape

```
POST /v1/url-scraper/batch
```

Submit up to 10 URLs for scraping in a single request. Returns a batch job ID. Poll [GET /v1/url-scraper/batch/:id](#get-batch-result) for results.

**Request:**

```bash
curl -X POST http://localhost:8080/v1/url-scraper/batch \
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
| `country` | string | `""` | Proxy country code. Reserved for future use (no-op in OSS). |
| `useBrowser` | bool | `false` | Force browser rendering for all URLs |
| `generateJson` | bool | `false` | Extract structured JSON for all URLs (requires `GEMINI_API_KEY`) |

**Response (`201 Created`):**

```json
{
  "id": "batch-job-uuid",
  "status": "pending",
  "jobType": "batch_url_scraper",
  "urls": [
    "https://example.com/page-1",
    "https://example.com/page-2",
    "https://example.com/page-3"
  ],
  "createdAt": "2025-01-15T10:30:00Z"
}
```

---

### Get Batch Result

```
GET /v1/url-scraper/batch/:id
```

Retrieve the current state of a batch scrape job, including per-URL results.

```bash
curl http://localhost:8080/v1/url-scraper/batch/BATCH_JOB_UUID
```

**Response (completed):**

```json
{
  "id": "batch-job-uuid",
  "status": "completed",
  "jobType": "batch_url_scraper",
  "urls": [
    "https://example.com/page-1",
    "https://example.com/page-2"
  ],
  "results": [
    {
      "index": 0,
      "url": "https://example.com/page-1",
      "status": "completed",
      "html": "...",
      "cleanedHtml": "...",
      "markdown": "...",
      "generatedJson": null,
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
      "generatedJson": null,
      "cached": false,
      "durationMs": 987
    }
  ],
  "createdAt": "2025-01-15T10:30:00Z",
  "completedAt": "2025-01-15T10:30:08Z",
  "durationMs": 5678
}
```

The batch `status` is derived from the child jobs: it remains `pending` if any child is pending, `processing` if any child is processing, and becomes `completed` once all children finish.

---

### List Domain Configs

```
GET /v1/domain-configs
```

Return all per-domain scraping configurations.

```bash
curl http://localhost:8080/v1/domain-configs
```

**Response (`200 OK`):**

```json
[
  {
    "id": 1,
    "domain": "example.com",
    "isEnabled": true,
    "matchSubdomains": false,
    "priority": 0,
    "handlerChain": ["http", "browser"],
    "requestTimeoutMs": 30000,
    "maxRetries": 2,
    "minContentLength": 0,
    "failurePatterns": [],
    "requiredPatterns": [],
    "customHeaders": {},
    "customUserAgent": "",
    "proxyUrl": "",
    "blocked": false,
    "blockedReason": "",
    "notes": "",
    "createdAt": "2025-01-15T10:00:00Z",
    "updatedAt": "2025-01-15T10:00:00Z"
  }
]
```

Returns an empty array `[]` if no configs exist.

---

### Create Domain Config

```
POST /v1/domain-configs
```

Create a new per-domain configuration. Domain configs let you customize handler chain order, timeouts, retries, custom headers, and more for specific domains.

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "example.com",
    "isEnabled": true,
    "matchSubdomains": true,
    "handlerChain": ["browser", "http"],
    "requestTimeoutMs": 45000,
    "maxRetries": 3,
    "customHeaders": {
      "Accept-Language": "en-US"
    }
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `domain` | string | **required** | The domain name (e.g. `"example.com"`) |
| `isEnabled` | bool | `false` | Whether the config is active |
| `matchSubdomains` | bool | `false` | Apply to all subdomains of the domain |
| `priority` | int | `0` | Higher priority configs take precedence |
| `handlerChain` | string[] | `["http", "browser"]` | Ordered list of scraping handlers to try |
| `requestTimeoutMs` | int | `30000` | Per-request timeout in milliseconds |
| `maxRetries` | int | `2` | Maximum retry attempts |
| `minContentLength` | int | `0` | Minimum acceptable content length (bytes) |
| `failurePatterns` | string[] | `[]` | Regex patterns that indicate a failed scrape |
| `requiredPatterns` | string[] | `[]` | Regex patterns that must appear in successful content |
| `customHeaders` | object | `{}` | Extra HTTP headers to send |
| `customUserAgent` | string | `""` | Override the default User-Agent |
| `proxyUrl` | string | `""` | Force a specific proxy for this domain |
| `blocked` | bool | `false` | Mark domain as blocked (skips scraping) |
| `blockedReason` | string | `""` | Reason the domain is blocked |
| `notes` | string | `""` | Free-text notes |

**Response (`201 Created`):** The created config object (same shape as the list response items).

---

### Get Domain Config

```
GET /v1/domain-configs/:domain
```

Get the config for a specific domain.

```bash
curl http://localhost:8080/v1/domain-configs/example.com
```

**Response (`200 OK`):** A single domain config object.

**Response (`404 Not Found`):**

```json
{
  "error": "not_found",
  "message": "Domain config not found"
}
```

---

### Update Domain Config

```
PUT /v1/domain-configs/:domain
```

Update an existing domain config. Send the full config body; fields not included will be set to their zero values.

```bash
curl -X PUT http://localhost:8080/v1/domain-configs/example.com \
  -H "Content-Type: application/json" \
  -d '{
    "isEnabled": true,
    "handlerChain": ["browser"],
    "requestTimeoutMs": 60000,
    "maxRetries": 5
  }'
```

**Response (`200 OK`):** The updated config object.

---

### Delete Domain Config

```
DELETE /v1/domain-configs/:domain
```

Delete a domain config.

```bash
curl -X DELETE http://localhost:8080/v1/domain-configs/example.com
```

**Response:** `204 No Content`

---

### View Proxy Scores

```
GET /v1/proxy/scores
```

View the current Thompson Sampling scores for all proxies, grouped by target host. Useful for monitoring which proxies perform best for which sites.

```bash
curl http://localhost:8080/v1/proxy/scores
```

**Response (`200 OK`):**

```json
{
  "scores": {
    "example.com": [
      {
        "proxyUrl": "http://proxy1:8080",
        "targetHost": "example.com",
        "alpha": 15,
        "beta": 3,
        "score": 0.833,
        "totalRequests": 18,
        "avgLatencyMs": 1200,
        "lastUpdated": "2025-01-15T10:30:00Z"
      }
    ]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `proxyUrl` | string | The proxy URL |
| `targetHost` | string | The target domain |
| `alpha` | int | Beta distribution success parameter |
| `beta` | int | Beta distribution failure parameter |
| `score` | float | Current win rate estimate (`alpha / (alpha + beta)`) |
| `totalRequests` | int | Total requests routed through this proxy for this host |
| `avgLatencyMs` | int | Exponential moving average latency in milliseconds |
| `lastUpdated` | string | ISO 8601 timestamp of last score update |

If no proxies are configured, the response is:

```json
{
  "proxies": [],
  "scores": {}
}
```

---

## Job Status Lifecycle

```
pending  -->  processing  -->  completed
                           \-> failed
```

| Status | Meaning |
|--------|---------|
| `pending` | Job is queued, waiting for a worker |
| `processing` | A worker is actively scraping the URL |
| `completed` | Scrape succeeded -- results are available |
| `failed` | Scrape failed -- check the `error` field for details |

Common failure reasons:

| Error | Description |
|-------|-------------|
| `all handlers failed` | Both HTTP and browser handlers failed |
| `timeout` | Page load exceeded the configured timeout |
| `blocked` | The target site blocked the request (403/captcha) |
| `invalid_url` | URL could not be resolved or connected to |

---

## Error Responses

All errors follow this format:

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
| 404 | `not_found` | Job or resource not found |
| 408 | `timeout` | Sync scrape did not complete in time (sync endpoint only) |
| 500 | `internal_error` | Unexpected server error |

---

## Polling Pattern

The async endpoints (`POST /v1/url-scraper`, `POST /v1/url-scraper/batch`) return immediately with a job ID. Poll the corresponding GET endpoint until `status` is `completed` or `failed`.

Recommended strategy:

1. Wait 1 second after submitting
2. Poll every 1--2 seconds
3. Set a client-side timeout (60--120 seconds recommended)
4. Handle `failed` status gracefully

```bash
# 1. Submit
JOB_ID=$(curl -s -X POST http://localhost:8080/v1/url-scraper \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq -r '.id')

echo "Job ID: $JOB_ID"

# 2. Poll
while true; do
  RESULT=$(curl -s http://localhost:8080/v1/url-scraper/$JOB_ID)
  STATUS=$(echo $RESULT | jq -r '.status')
  echo "Status: $STATUS"

  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    echo $RESULT | jq .
    break
  fi

  sleep 1
done
```

Alternatively, use `POST /v1/scrape` to avoid polling entirely -- it blocks until the job finishes or times out at 30 seconds.
