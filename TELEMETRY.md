# Telemetry

AnakinScraper collects anonymous usage data to help us understand how the project is used, prioritize improvements, and catch issues. Telemetry is **enabled by default** and can be disabled at any time.

## Disable telemetry

Set the `TELEMETRY` environment variable to `off`:

```bash
TELEMETRY=off
```

Or in your `.env` file:

```
TELEMETRY=off
```

When disabled, no data is collected or sent. The server logs `telemetry disabled` on startup.

## What is collected

Every hour, the server sends a single HTTP POST to `https://telemetry.anakin.io/v1/collect` containing:

| Field | Example | Description |
|-------|---------|-------------|
| `instance_id` | `550e8400-...` | Random UUID generated on first boot. Not derived from hardware or user identity. |
| `version` | `0.1.0` | Server version |
| `uptime_hours` | `168.5` | Hours since server started |
| `endpoints.scrape_sync` | `142` | Count of sync scrape requests since last report |
| `endpoints.scrape_async` | `38` | Count of async scrape requests |
| `endpoints.scrape_batch` | `5` | Count of batch scrape requests |
| `handlers.http` | `160` | Jobs handled by HTTP handler |
| `handlers.browser` | `25` | Jobs handled by browser handler |
| `status.success` | `170` | Successful jobs |
| `status.failed` | `15` | Failed jobs |
| `duration.under_1s` | `120` | Jobs completing in <1 second |
| `duration.from_1s_to_5s` | `50` | Jobs completing in 1-5 seconds |
| `duration.from_5s_to_30s` | `15` | Jobs completing in 5-30 seconds |
| `duration.over_30s` | `0` | Jobs completing in >30 seconds |
| `features.gemini_enabled` | `true` | Whether Gemini API key is configured |
| `features.proxy_pool_size` | `3` | Number of proxies in the pool |

## What is NOT collected

- URLs you scrape
- Page content (HTML, markdown, JSON)
- IP addresses (not stored on the receiver)
- Domain configurations
- API keys or credentials
- Proxy URLs
- Error messages or stack traces
- Any personally identifiable information (PII)

## Transparency

You can see exactly what will be sent at any time by querying:

```bash
curl http://localhost:8080/v1/telemetry/status | jq .
```

This returns the current telemetry state including the next payload that will be sent.

## How it works

1. On first boot, a random UUID is generated and stored in PostgreSQL (the `telemetry_instance` table).
2. As jobs are processed, counters are incremented using atomic operations (zero overhead).
3. Every hour, counters are snapshotted, reset, and sent as a single HTTP POST.
4. If the telemetry endpoint is unreachable, the batch is dropped. No data is queued or retried beyond the current cycle.
5. Telemetry runs in a background goroutine and never affects scraping performance.

## Source code

The complete telemetry implementation is in [`server/internal/telemetry/telemetry.go`](server/internal/telemetry/telemetry.go). It's a single file — easy to audit.
