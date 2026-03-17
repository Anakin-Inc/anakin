#!/usr/bin/env bash
# Batch scrape example — scrape multiple URLs in one request.
# Usage: ./batch.sh

set -euo pipefail

BASE_URL="${ANAKIN_BASE_URL:-http://localhost:8080}"

URLS='["https://example.com", "https://httpbin.org/html", "https://jsonplaceholder.typicode.com/posts/1"]'

echo "Batch scraping 3 URLs ..."
echo ""

# Submit batch job
RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/url-scraper/batch" \
  -H "Content-Type: application/json" \
  -d "{\"urls\": ${URLS}}")

JOB_ID=$(echo "$RESPONSE" | jq -r '.id')
echo "Batch job ID: ${JOB_ID}"

# Poll until complete
while true; do
  RESULT=$(curl -s "${BASE_URL}/v1/url-scraper/batch/${JOB_ID}")
  STATUS=$(echo "$RESULT" | jq -r '.status')

  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    echo ""
    echo "$RESULT" | jq '.results[] | {url, status, durationMs}'
    break
  fi

  echo "  Status: ${STATUS} ..."
  sleep 1
done
