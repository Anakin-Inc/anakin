# Domain Configs

Domain configs let you define per-domain scraping strategies. When a URL is scraped, the server checks if a matching domain config exists and applies its settings — handler selection, timeouts, retries, content validation, custom headers, and more.

## Quick Example

Block a domain:

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "example.com",
    "blocked": true,
    "blockedReason": "Terms of service"
  }'
```

Force browser-only scraping with failure detection:

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "linkedin.com",
    "handlerChain": ["browser"],
    "requestTimeoutMs": 45000,
    "maxRetries": 3,
    "failurePatterns": ["captcha", "Sign in to continue", "authwall"],
    "requiredPatterns": ["<main", "experience"],
    "minContentLength": 5000
  }'
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/domain-configs` | List all configs |
| `GET` | `/v1/domain-configs/:domain` | Get config for a domain |
| `POST` | `/v1/domain-configs` | Create a new config |
| `PUT` | `/v1/domain-configs/:domain` | Update an existing config |
| `DELETE` | `/v1/domain-configs/:domain` | Delete a config |

**Note:** Domain configs require PostgreSQL (`DATABASE_URL`). In zero-config/memory mode, these endpoints return `503`.

## Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `domain` | string | **required** | Domain name (e.g. `linkedin.com`) |
| `isEnabled` | bool | `true` | Whether this config is active |
| `matchSubdomains` | bool | `true` | Match `www.linkedin.com`, `mobile.linkedin.com`, etc. |
| `priority` | int | `0` | Higher priority configs match first |
| `handlerChain` | string[] | `["http","browser"]` | Which handlers to try, in order |
| `requestTimeoutMs` | int | `30000` | Timeout per handler attempt (ms) |
| `maxRetries` | int | `2` | Number of retry attempts on failure |
| `minContentLength` | int | `0` | Minimum HTML bytes — below this triggers retry |
| `failurePatterns` | string[] | `[]` | Regex patterns — if ANY match the HTML, it's a failure |
| `requiredPatterns` | string[] | `[]` | Regex patterns — at least ONE must match or it's a failure |
| `customHeaders` | object | `{}` | Extra HTTP headers (e.g. `{"Cookie": "..."}`) |
| `customUserAgent` | string | `""` | Override the default user-agent |
| `proxyUrl` | string | `""` | Force a specific proxy for this domain |
| `blocked` | bool | `false` | Block all scraping for this domain |
| `blockedReason` | string | `""` | Reason shown in error message |
| `notes` | string | `""` | Free-text notes for your team |

## Failure Detection

Failure detection catches cases where a page loads successfully (HTTP 200) but the content is wrong — a CAPTCHA page, a login wall, or an empty shell.

### How It Works

After a handler returns HTML, the detector runs three checks in order:

```
HTML from handler
  │
  ▼
1. Content length check
   len(html) < minContentLength?  ──yes──▶ FAIL (retry)
  │ no
  ▼
2. Failure pattern check
   Any failurePattern matches?    ──yes──▶ FAIL (retry)
  │ no
  ▼
3. Required pattern check
   At least one requiredPattern    ──no───▶ FAIL (retry)
   matches?
  │ yes
  ▼
SUCCESS — content is valid
```

On failure, the job retries with the next handler in the chain (up to `maxRetries`).

### Failure Patterns

Regex patterns that indicate the page content is wrong. If **any** pattern matches, the scrape is considered failed.

```json
{
  "failurePatterns": [
    "captcha",
    "Please verify you are a human",
    "Access Denied",
    "cf-browser-verification",
    "Just a moment\\.\\.\\."
  ]
}
```

Common failure patterns:
- `captcha` — CAPTCHA challenges
- `Access Denied` — Cloudflare/WAF blocks
- `Sign in to continue` — Login walls
- `cf-browser-verification` — Cloudflare interstitial
- `<title>403` — Forbidden pages that return 200

### Required Patterns

Regex patterns where **at least one** must match for the content to be valid. Use this to verify the page actually loaded the expected content.

```json
{
  "requiredPatterns": [
    "<article",
    "class=\"content\"",
    "<main"
  ]
}
```

If none of the required patterns match, the scrape fails and retries.

### Minimum Content Length

Simple byte-length check. Pages that are too short are likely error pages.

```json
{
  "minContentLength": 5000
}
```

A CAPTCHA page might be 2KB. A real product page is 50KB+. Set the threshold accordingly.

## Handler Chain Selection

By default, every domain uses `["http", "browser"]` — try HTTP first, fall back to the browser. You can customize this per domain:

```json
{"handlerChain": ["browser"]}
```
Skip HTTP, go straight to browser. Use for JavaScript-heavy SPAs.

```json
{"handlerChain": ["http"]}
```
HTTP only. Use for static sites where browser overhead is unnecessary.

```json
{"handlerChain": ["http", "browser", "anakin"]}
```
Full chain with API fallback. Requires `ANAKIN_API_KEY` to be set.

```json
{"handlerChain": ["anakin"]}
```
Skip local handlers entirely, delegate to the external API. Useful for domains that are always blocked locally.

## Domain Matching

When a URL is scraped, the domain is matched against configs:

1. **Exact match** — `linkedin.com` matches `linkedin.com`
2. **Subdomain match** — `www.linkedin.com` matches `linkedin.com` (if `matchSubdomains: true`)
3. **Priority** — if multiple configs could match, higher `priority` wins

Subdomain matching walks up the domain tree:
- URL: `https://www.jobs.linkedin.com/posting/123`
- Tries: `www.jobs.linkedin.com` → `jobs.linkedin.com` → `linkedin.com`
- First match with `matchSubdomains: true` wins

## Examples

### E-commerce site with content validation

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "amazon.com",
    "handlerChain": ["http", "browser"],
    "maxRetries": 3,
    "failurePatterns": ["captcha", "robot check", "Sorry, we just need to make sure"],
    "requiredPatterns": ["<div id=\"dp\"", "a]productTitle"],
    "minContentLength": 10000,
    "customHeaders": {"Accept-Language": "en-US"}
  }'
```

### News site with custom user-agent

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "nytimes.com",
    "handlerChain": ["browser"],
    "customUserAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "requestTimeoutMs": 60000,
    "failurePatterns": ["Subscribe to continue", "gateway-content"]
  }'
```

### Block a domain

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "internal-app.company.com",
    "blocked": true,
    "blockedReason": "Internal site — do not scrape"
  }'
```

## Cache

Domain configs are cached in memory and refreshed from PostgreSQL every 60 seconds. After creating or updating a config, it may take up to 60 seconds to take effect.
