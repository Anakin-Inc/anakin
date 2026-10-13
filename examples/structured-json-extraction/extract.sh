#!/usr/bin/env bash
# Structured JSON extraction example — scrape a page with a data table and
# return it as JSON via generateJson (requires GEMINI_API_KEY on the server).
# Usage: ./extract.sh [URL]

set -euo pipefail

BASE_URL="${ANAKIN_BASE_URL:-http://localhost:8080}"
URL="${1:-https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html}"

echo "Extracting structured JSON from ${URL} ..."
echo ""

curl -s -X POST "${BASE_URL}/v1/scrape" \
  -H "Content-Type: application/json" \
  -d "{\"url\": \"${URL}\", \"generateJson\": true}" | jq .generatedJson
