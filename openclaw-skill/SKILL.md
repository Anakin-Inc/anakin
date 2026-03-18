---
name: anakinscraper
description: Scrape any website into clean markdown or structured JSON. Anti-detect browser, smart proxy rotation, AI-powered data extraction.
version: 1.0.0
license: AGPL-3.0
user-invocable: true
metadata: {"openclaw":{"requires":{"bins":["node"]},"emoji":"🕷️","homepage":"https://github.com/Anakin-Inc/anakinscraper-oss"}}
---

# AnakinScraper Skill

Scrape any website and get back clean markdown or structured JSON data. Uses an anti-detect browser (Camoufox) to handle JavaScript-heavy sites and anti-bot protection.

## Setup

1. Start AnakinScraper (if not already running):

```bash
git clone https://github.com/Anakin-Inc/anakinscraper-oss.git
cd anakinscraper-oss && make up
```

2. Build the MCP server:

```bash
cd anakinscraper-oss/mcp-server && npm install && npm run build
```

3. Add the MCP server to your OpenClaw config (`~/.openclaw/config.json`):

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "node",
      "args": ["/path/to/anakinscraper-oss/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

## Available Tools

### anakinscraper_scrape
Scrape a single URL synchronously. Returns markdown, cleaned HTML, and optionally structured JSON.

Use this when the user asks to:
- "Scrape this website"
- "Get the content from this URL"
- "Turn this page into markdown"
- "What's on this webpage?"

Parameters:
- `url` (required) — The URL to scrape
- `useBrowser` (optional, default false) — Force browser rendering for JavaScript-heavy sites
- `generateJson` (optional, default false) — Extract structured JSON data using AI

### anakinscraper_extract_json
Scrape a URL and extract structured data as JSON. Best for product pages, articles, listings.

Use this when the user asks to:
- "Extract the product data from this page"
- "Get structured data from this URL"
- "Parse this listing page into JSON"
- "What products are on this page?"

Parameters:
- `url` (required) — The URL to extract data from

### anakinscraper_batch_scrape
Scrape multiple URLs at once (up to 10). Returns results for all URLs.

Use this when the user asks to:
- "Scrape these 5 URLs"
- "Get content from all these pages"
- "Batch scrape this list"

Parameters:
- `urls` (required) — Array of URLs to scrape (max 10)
- `useBrowser` (optional) — Force browser rendering
- `generateJson` (optional) — Extract structured JSON from each page

### anakinscraper_scrape_async
Submit a scrape job asynchronously. Returns a job ID for later polling. Use for pages that take longer than 30 seconds.

Parameters:
- `url` (required) — The URL to scrape
- `useBrowser` (optional) — Force browser rendering
- `generateJson` (optional) — Extract structured JSON

### anakinscraper_get_job
Check the status of an async scrape job. Poll until status is "completed" or "failed".

Parameters:
- `jobId` (required) — The job ID returned by anakinscraper_scrape_async

## Usage Guidelines

1. **Default to `anakinscraper_scrape`** for most requests — it's synchronous and returns results immediately.
2. **Use `anakinscraper_extract_json`** when the user wants structured data (products, articles, listings).
3. **Use `anakinscraper_batch_scrape`** when scraping multiple URLs.
4. **Use `anakinscraper_scrape_async` + `anakinscraper_get_job`** only for pages you know will be slow (complex SPAs, heavy anti-bot sites).
5. **Set `useBrowser: true`** for JavaScript-heavy sites, SPAs, or sites with anti-bot protection.
6. Present the **markdown** field for readable content, **generatedJson.data** for structured data.
