# Handler Chain

The handler chain is the core of AnakinScraper. Each scrape job goes through a sequence of handlers. If one fails, the next one tries automatically.

```
HTTP Handler ──fail──▶ Browser Handler ──fail──▶ API Handler ──fail──▶ Settlement ──fail──▶ ERROR
(fast, ~200ms)         (Camoufox, ~2-5s)         (external, optional)   (paid per scrape)
```

## Built-in Handlers

### HTTP Handler

**Name:** `http`

Direct HTTP GET with a browser user-agent. Fastest option (~200ms). Handles static HTML, server-rendered pages, and most websites that don't require JavaScript.

- Skipped when `useBrowser: true` is set in the request
- Uses a realistic Chrome user-agent by default
- Supports custom headers and user-agent per request (via domain configs)
- Supports per-request proxy override
- Response body limited to 10MB
- Returns error on HTTP 4xx/5xx

**When it fails:** JavaScript-heavy SPAs, pages behind Cloudflare/DataDome, sites that detect non-browser requests.

### Browser Handler

**Name:** `browser`

Connects to [Camoufox](https://github.com/daijro/camoufox) (anti-detect Firefox) over WebSocket via the Playwright protocol. Full JavaScript rendering with realistic browser fingerprints.

- Waits for `networkidle` state (no pending network requests)
- Optional extra wait time after page load (`BROWSER_LOAD_WAIT`, default 2s)
- Not headless Chrome — Camoufox is a modified Firefox that resists fingerprinting
- Requires the browser service to be running (Docker or `python server.py`)
- If the browser service is unavailable, this handler is marked unhealthy and skipped

**When it fails:** Sites with advanced anti-bot (DataDome, PerimeterX), geo-restricted content, IP-banned ranges.

### API Handler

**Name:** configurable (default: `anakin` for the built-in handler)

Delegates to an external scraping API. This is the last resort — when your local HTTP and browser handlers both fail, the API handler sends the URL to a third-party service that has managed proxies, geo-targeting, and better anti-detection.

- Only active when its API key is set (e.g. `ANAKIN_API_KEY`)
- Sends a POST request with `{"url": "..."}` to the configured endpoint
- Authenticates via configurable header (default: `X-API-Key`)
- Parses JSON response for `html`, `cleanedHtml`, `markdown` fields
- Falls back to treating the entire response body as HTML if JSON parsing fails
- Response body limited to 10MB

**Built-in:** `anakin.io` handler, activated by setting `ANAKIN_API_KEY`.

### Settlement Handler

**Name:** `xno`

Settles each scrape individually through an [x402](https://x402.org) facilitator, so a
caller pays per request instead of holding a prepaid contract. This is the last
resort: it only runs when every other handler has failed, and it is intended for
callers that hold crypto but not an API key.

- Only active when `XNO_FACILITATOR_URL` is set (see the table below)
- Requires the caller to present a signed payment in the `X-PAYMENT` header
- Terms — network, amount, asset, pay-to address — are all operator-configured
- **The server never holds a wallet key.** Callers sign with their own wallet and
  the operator configures only a receiving address, so enabling this does not add
  a custodial wallet to the deployment.

#### Request flow

A facilitator verifies and settles payments. It does not fetch pages, so the
scrape itself is performed locally:

```
1. Caller sends X-PAYMENT: <base64 x402 payment payload>
2. Handler POSTs the payload to the facilitator's /verify
3. Handler performs the scrape (via the API handler if one is configured,
   otherwise the HTTP handler)
4. Handler POSTs the payload to the facilitator's /settle
5. Handler returns the ScrapeResult
```

A request that reaches this handler without a usable payment fails with a
`*PaymentRequiredError` naming the terms, so the caller knows what to pay.

#### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `XNO_FACILITATOR_URL` | — | Facilitator base URL. Enables the handler when set |
| `XNO_NETWORK` | — | Facilitator's network identifier (e.g. `nano`, `base-sepolia`) |
| `XNO_PAY_TO` | — | Address that receives payment |
| `XNO_AMOUNT` | — | Price of one scrape in the asset's base units |
| `XNO_ASSET` | — | Asset identifier, in the facilitator's convention |
| `XNO_ASSET_NAME` | — | Human-readable asset label for logs and errors |
| `XNO_SCHEME` | `exact` | x402 payment scheme |
| `XNO_RESOURCE` | `/v1/scrape` | Resource identifier published in the terms |
| `XNO_SETTLE` | `true` | Set to `off` to verify payments without settling them |
| `XNO_TIMEOUT` | `15` | Per-facilitator-call timeout (seconds) |

`XNO_AMOUNT` is an integer in the asset's smallest unit: `"1 XNO"` is `1000000`
rawNano, and `"$0.01"` of USDC is `10000`.

When `ANAKIN_API_KEY` is also set, the payment buys the external API call rather
than a plain fetch.

#### Network support

The handler is not specific to Nano — scheme, network, asset and pay-to address
are all configuration. It settles on whatever network the configured facilitator
supports, so the same code path works for an XNO facilitator or an EVM one.
Confirm that a facilitator supports your network before enabling this: the server
does not settle payments itself and cannot fall back to another network.

#### Replay protection

A facilitator keeps reporting a captured authorization as valid until it expires,
so validity alone does not prevent replay. This handler records each authorization
nonce and refuses a second use, and rejects authorizations whose validity window
exceeds the replay-cache TTL. Payments are verified before the scrape runs, and
settlement failures do not return page content.

Because responses are job records rather than synchronous HTTP responses, a
payment failure surfaces as a `failed` job whose `error` names the terms —
not as a `402` on the submitting request. See [API.md](API.md).

## How the Chain Works

```
for each handler in chain:
    1. Is this handler allowed? (check AllowedHandlers from domain config)
    2. Can it handle this request? (e.g., HTTP handler skips useBrowser requests)
    3. Is it healthy? (e.g., browser handler checks if Camoufox is reachable)
    4. Try scraping
       ├── Success → return result, stop chain
       └── Failure → log warning, try next handler
```

If all handlers fail, the job returns the last error. A `*PaymentRequiredError`
from the settlement handler stops the retry loop rather than re-running the
earlier handlers — the caller has to pay, not retry.

## Per-Domain Handler Selection

Domain configs can override which handlers are used:

```json
{"domain": "spa-app.com", "handlerChain": ["browser"]}
```

This skips the HTTP handler entirely. The chain only tries handlers listed in `handlerChain`.

See [domain-configs.md](domain-configs.md) for full details.

## Writing a Custom Handler

Implement the `ScrapingHandler` interface:

```go
type ScrapingHandler interface {
    Name() string
    CanHandle(ctx context.Context, req *HandlerRequest) bool
    Scrape(ctx context.Context, req *HandlerRequest) (*ScrapeResult, error)
    IsHealthy() bool
}
```

### Method Reference

| Method | Purpose | Return |
|--------|---------|--------|
| `Name()` | Identifier used in logs and domain config `handlerChain` | e.g. `"my-handler"` |
| `CanHandle()` | Should this handler attempt this request? | `false` to skip |
| `IsHealthy()` | Is the handler operational? | `false` to skip with warning |
| `Scrape()` | Perform the scrape | `*ScrapeResult` or `error` |

### ScrapeResult

```go
type ScrapeResult struct {
    HTML        string  // raw HTML (required)
    CleanedHTML string  // cleaned HTML (optional — converter will generate if empty)
    Markdown    string  // markdown (optional — converter will generate if empty)
    StatusCode  int     // HTTP status code
    DurationMs  int     // set automatically by the chain
    Handler     string  // set automatically by the chain
    Cached      bool    // true if result was served from cache
}
```

Only `HTML` is required. The server will convert HTML to markdown automatically if `Markdown` is empty.

### HandlerRequest

```go
type HandlerRequest struct {
    JobID           string
    URL             string
    Country         string
    UseBrowser      bool
    ProxyURL        string
    Timeout         time.Duration
    AllowedHandlers []string          // from domain config handlerChain
    CustomHeaders   map[string]string // from domain config
    CustomUserAgent string            // from domain config
    PaymentHeader   string            // raw x402 payment from the caller
}
```

### Example: Adding a Third-Party API Handler

The easiest way to add a new external service is to use the built-in `APIHandler` with custom config:

```go
// In main.go:
if cfg.ExternalAPIKey != "" {
    handlers = append(handlers, handler.NewAPIHandler(handler.APIHandlerConfig{
        Name:       "my-scraping-service",
        APIURL:     "https://api.example.com/v1/scrape",
        APIKey:     cfg.ExternalAPIKey,
        AuthHeader: "X-Api-Key",
        Timeout:    30 * time.Second,
    }))
}
```

Then add to `config.go`:

```go
ExternalAPIKey: os.Getenv("EXTERNAL_API_KEY"),
```

The handler only activates when the env var is set. No impact on users who don't use it.

### Example: Fully Custom Handler

For services with non-standard APIs, implement `ScrapingHandler` directly:

```go
package handler

type MyHandler struct {
    apiKey string
    client *http.Client
}

func NewMyHandler(apiKey string) *MyHandler {
    return &MyHandler{
        apiKey: apiKey,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (h *MyHandler) Name() string                                    { return "my-handler" }
func (h *MyHandler) CanHandle(_ context.Context, _ *HandlerRequest) bool { return h.apiKey != "" }
func (h *MyHandler) IsHealthy() bool                                 { return h.apiKey != "" }

func (h *MyHandler) Scrape(ctx context.Context, req *HandlerRequest) (*ScrapeResult, error) {
    // Your custom logic here
    // Return &ScrapeResult{HTML: html, StatusCode: 200}, nil
}
```

Register in `main.go`:

```go
if cfg.MyAPIKey != "" {
    handlers = append(handlers, handler.NewMyHandler(cfg.MyAPIKey))
}
```

### Auth Pattern

API keys always follow this pattern:

1. **Environment variable** → `MY_SERVICE_API_KEY`
2. **Config struct** → `config.go` reads it: `MyServiceAPIKey: os.Getenv("MY_SERVICE_API_KEY")`
3. **Constructor** → handler receives the key: `NewMyHandler(cfg.MyServiceAPIKey)`
4. **Guard** → `CanHandle()` and `IsHealthy()` return `false` when key is empty

This keeps secrets out of code, makes handlers testable, and ensures unused handlers have zero overhead.

See [examples/custom-handler/](../examples/custom-handler/) for a working example with a local file-based handler.
