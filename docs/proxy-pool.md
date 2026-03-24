# Proxy Pool

AnakinScraper uses [Thompson Sampling](https://en.wikipedia.org/wiki/Thompson_sampling) to automatically select the best proxy for each domain. Instead of round-robin or random selection, the system learns which proxies work best for which sites — in real time.

## Setup

Provide a comma-separated list of proxy URLs:

```bash
PROXY_URLS="http://proxy1:8080,http://proxy2:8080,socks5://proxy3:1080"
```

That's it. The system starts learning immediately. No configuration per proxy, no manual scoring.

**Requirements:** PostgreSQL (`DATABASE_URL`) is needed to persist proxy scores across restarts. Without it, scores start fresh each time.

## How It Works

### Thompson Sampling

Each proxy maintains a score per target domain, modeled as a [Beta distribution](https://en.wikipedia.org/wiki/Beta_distribution) with two parameters:

- **Alpha** — number of successes (starts at 1)
- **Beta** — number of failures (starts at 1)

When selecting a proxy for a domain:

1. Sample a random value from each proxy's Beta(alpha, beta) distribution
2. Pick the proxy with the highest sampled value
3. This naturally balances exploitation (use what works) with exploration (try alternatives)

```
Proxy A: Alpha=50, Beta=5   → 91% win rate, usually sampled high
Proxy B: Alpha=10, Beta=10  → 50% win rate, sometimes sampled high (exploration)
Proxy C: Alpha=2, Beta=20   → 9% win rate, rarely sampled high
```

Proxy A gets picked most often, but B still gets tried occasionally. If B starts succeeding for a specific domain, its score rises and it gets picked more. C is effectively deprioritized but never fully abandoned.

### Success and Failure Recording

**On success:**
- Alpha += 1
- Average latency updated via exponential moving average (weight: 0.2)
- Score recalculated: `Alpha / (Alpha + Beta)`

**On failure:**
- Beta += 1 (normal failure)
- Beta += 10 (if the site returned 403/blocked — severe penalty)
- If blocked: proxy is temporarily blocked for this domain (5 minutes)
- Score recalculated

### Blocked Proxy Handling

When a proxy gets a 403 (blocked) response for a domain:

1. The proxy receives a severe beta penalty (+10 instead of +1)
2. The proxy is blocked for that specific domain for 5 minutes
3. During the block period, other proxies are selected
4. After 5 minutes, the proxy becomes eligible again (but with a low score)

This prevents wasting requests on a proxy that's clearly banned for a specific site, while allowing recovery if the block is temporary.

## Viewing Scores

```bash
curl http://localhost:8080/v1/proxy/scores | jq .
```

Response:

```json
{
  "linkedin.com": [
    {
      "proxyUrl": "http://proxy1:8080",
      "alpha": 45,
      "beta": 3,
      "score": 0.9375,
      "totalRequests": 48,
      "avgLatencyMs": 1200,
      "lastUpdated": "2026-03-24T10:30:00Z"
    },
    {
      "proxyUrl": "http://proxy2:8080",
      "alpha": 12,
      "beta": 15,
      "score": 0.4444,
      "totalRequests": 27,
      "avgLatencyMs": 3400,
      "lastUpdated": "2026-03-24T09:15:00Z"
    }
  ]
}
```

Scores are grouped by target domain. Each proxy has independent scores per domain — a proxy that's great for LinkedIn might be terrible for Amazon.

### Reading the Scores

| Field | Meaning |
|-------|---------|
| `alpha` | Success count + 1 (prior) |
| `beta` | Failure count + 1 (prior) |
| `score` | Win rate: `alpha / (alpha + beta)`. Range 0-1. |
| `totalRequests` | Total attempts through this proxy for this domain |
| `avgLatencyMs` | Exponential moving average of successful request latency |

**Score interpretation:**
- `0.90+` — excellent proxy for this domain
- `0.70-0.89` — good, generally works
- `0.50-0.69` — mediocre, fails often
- `< 0.50` — poor, mostly fails

## Per-Domain Proxy Override

Domain configs can force a specific proxy, bypassing Thompson Sampling:

```bash
curl -X POST http://localhost:8080/v1/domain-configs \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "geo-restricted-site.com",
    "proxyUrl": "http://us-proxy:8080"
  }'
```

When `proxyUrl` is set in a domain config, Thompson Sampling is skipped for that domain.

## Default Proxy

A single default proxy (not a pool) can be set for the HTTP handler:

```bash
PROXY_URL="http://default-proxy:8080"
```

This is used when no proxy pool is configured and no domain-specific proxy is set. It's simpler but doesn't get Thompson Sampling — just a static proxy for all requests.

## Persistence

Proxy scores are saved to PostgreSQL every 60 seconds:

- **Table:** `proxy_scores`
- **Primary key:** `(proxy_url, target_host)`
- **Upsert:** scores are merged, not replaced
- **Eviction:** scores unused for 24 hours are automatically removed

On server restart, scores are loaded from the database — the system doesn't need to re-learn from scratch.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PROXY_URLS` | — | Comma-separated proxy pool (enables Thompson Sampling) |
| `PROXY_URL` | — | Single default proxy for HTTP handler (no scoring) |

Both are optional. If neither is set, all requests go direct (no proxy).
