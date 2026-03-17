#!/usr/bin/env bash
# Basic scrape example — fetch a single URL and print the markdown.
# Usage: ./scrape.sh [URL]

set -euo pipefail

BASE_URL="${ANAKIN_BASE_URL:-http://localhost:8080}"
URL="${1:-https://example.com}"

echo "Scraping ${URL} ..."
echo ""

curl -s -X POST "${BASE_URL}/v1/scrape" \
  -H "Content-Type: application/json" \
  -d "{\"url\": \"${URL}\"}" | jq .
