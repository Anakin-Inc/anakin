# AnakinScraper CLI

A command-line interface for the AnakinScraper web scraping API. Scrape websites from your terminal with a single command.

## Installation

```bash
go install github.com/AnakinAI/anakinscraper-oss/cli@latest
```

The binary will be installed as `cli`. To rename it to `anakinscraper`:

```bash
mv $(go env GOPATH)/bin/cli $(go env GOPATH)/bin/anakinscraper
```

Or build from source:

```bash
git clone https://github.com/AnakinAI/anakinscraper-oss.git
cd anakinscraper-oss/cli
go build -o anakinscraper .
```

## Configuration

The CLI connects to your AnakinScraper server. Set the API URL via environment variable or flag:

```bash
# Environment variable (recommended)
export ANAKINSCRAPER_API_URL=http://localhost:8080

# Or per-command flag
anakinscraper scrape --api-url http://localhost:8080 https://example.com
```

Default: `http://localhost:8080`

## Commands

### scrape -- Scrape a single URL

Performs a synchronous scrape and prints the result to stdout.

```bash
# Basic scrape (outputs markdown)
anakinscraper scrape https://example.com

# Output raw HTML instead
anakinscraper scrape --format html https://example.com

# Get the full JSON response
anakinscraper scrape --json https://example.com

# Force browser rendering
anakinscraper scrape --browser https://example.com

# Extract structured JSON (requires GEMINI_API_KEY on the server)
anakinscraper scrape --extract https://example.com
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output the full JSON response |
| `--extract` | Enable AI JSON extraction (`generateJson=true`) and print extracted data |
| `--browser` | Force browser rendering via Camoufox |
| `--format` | Output field: `markdown` (default), `html`, or `json` |
| `--api-url` | Override the API base URL |

### batch -- Batch scrape multiple URLs

Submits multiple URLs for scraping, then polls until all results are ready.

```bash
# Scrape multiple pages
anakinscraper batch https://example.com/page-1 https://example.com/page-2 https://example.com/page-3

# Get JSON output for the whole batch
anakinscraper batch --json https://example.com/a https://example.com/b

# Force browser for all URLs
anakinscraper batch --browser https://example.com/a https://example.com/b
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output the full JSON response for the batch |
| `--extract` | Enable AI JSON extraction for all URLs |
| `--browser` | Force browser rendering for all URLs |
| `--format` | Output field: `markdown` (default), `html`, or `json` |
| `--api-url` | Override the API base URL |

### health -- Check server health

```bash
anakinscraper health
```

Output:

```
Service:  anakinscraper
Status:   ok
Database: connected
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--api-url` | Override the API base URL |

## Examples

### Pipe markdown to a file

```bash
anakinscraper scrape https://example.com > page.md
```

### Scrape and process with jq

```bash
anakinscraper scrape --json https://example.com | jq '.markdown'
```

### Extract structured data

```bash
anakinscraper scrape --extract https://example.com/product | jq '.price'
```

### Batch scrape and save all results

```bash
anakinscraper batch --json \
  https://example.com/page-1 \
  https://example.com/page-2 \
  https://example.com/page-3 \
  > results.json
```

### Use in a shell script

```bash
#!/bin/bash
urls=("https://example.com/a" "https://example.com/b" "https://example.com/c")

for url in "${urls[@]}"; do
  echo "--- $url ---"
  anakinscraper scrape "$url"
  echo
done
```

## Requirements

- Go 1.22 or later
- A running AnakinScraper server (see the main [README](../README.md) for setup)
