# AnakinScraper MCP Server

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io) server that wraps the AnakinScraper REST API. It lets AI assistants use AnakinScraper as a tool for web scraping, data extraction, and batch processing.

Works with: **Claude Desktop**, **Cursor**, **VS Code (Copilot)**, **Windsurf**, and **OpenClaw**.

## Prerequisites

- Node.js 18+
- A running AnakinScraper instance (default: `http://localhost:8080`)

## Installation

```bash
cd mcp-server
npm install
npm run build
```

## Configuration

The server reads one environment variable:

| Variable | Default | Description |
|----------|---------|-------------|
| `ANAKINSCRAPER_API_URL` | `http://localhost:8080` | Base URL of your AnakinScraper instance |

## Tools

The server exposes 5 tools:

| Tool | Description |
|------|-------------|
| `anakinscraper_scrape` | Synchronous scrape. Blocks up to 30s and returns markdown, HTML, and optional JSON. |
| `anakinscraper_scrape_async` | Async scrape. Returns a job ID immediately for polling. |
| `anakinscraper_get_job` | Get the status/result of a scrape job (works for both single and batch jobs). |
| `anakinscraper_batch_scrape` | Submit up to 10 URLs for parallel scraping. Returns a batch job ID. |
| `anakinscraper_extract_json` | Scrape + extract structured JSON in one call (requires GEMINI_API_KEY on server). |

## Client Configuration

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "node",
      "args": ["/absolute/path/to/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

### Cursor

Open **Settings > MCP Servers** and add:

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "node",
      "args": ["/absolute/path/to/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

### VS Code (GitHub Copilot)

Add to `.vscode/mcp.json` in your workspace (or user settings):

```json
{
  "servers": {
    "anakinscraper": {
      "type": "stdio",
      "command": "node",
      "args": ["/absolute/path/to/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

### Windsurf

Open **Settings > MCP** and add a new server:

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "node",
      "args": ["/absolute/path/to/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

### OpenClaw

Add to your OpenClaw configuration:

```json
{
  "mcpServers": {
    "anakinscraper": {
      "command": "node",
      "args": ["/absolute/path/to/mcp-server/dist/index.js"],
      "env": {
        "ANAKINSCRAPER_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

Replace `/absolute/path/to/mcp-server` with the actual path to this directory.

## Examples

Once connected, you can ask your AI assistant things like:

- **"Scrape https://example.com and show me the content"** -- uses `anakinscraper_scrape`
- **"Extract structured data from this product page: https://..."** -- uses `anakinscraper_extract_json`
- **"Scrape these 5 URLs and compare the results"** -- uses `anakinscraper_batch_scrape` + `anakinscraper_get_job`
- **"Start scraping this page in the background and check on it later"** -- uses `anakinscraper_scrape_async` + `anakinscraper_get_job`

## Development

```bash
# Watch mode (rebuild on changes)
npx tsc --watch

# Run directly with ts-node
npx tsx src/index.ts
```

## Troubleshooting

- **"Network error connecting to AnakinScraper"** -- Make sure AnakinScraper is running and `ANAKINSCRAPER_API_URL` is correct.
- **"JSON extraction failed"** -- The `anakinscraper_extract_json` tool requires `GEMINI_API_KEY` to be set on the AnakinScraper server.
- **"Job did not complete within 30 seconds"** -- Use `anakinscraper_scrape_async` instead for slow pages, then poll with `anakinscraper_get_job`.
