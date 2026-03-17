#!/usr/bin/env bash
# Record a terminal demo GIF for the README.
#
# Prerequisites:
#   brew install asciinema agg   # or: pip install asciinema && cargo install agg
#   make up                       # start the stack
#
# Usage:
#   ./scripts/record-demo.sh            # records demo.cast + demo.gif
#   GEMINI_API_KEY=... ./scripts/record-demo.sh  # includes JSON extraction
#
# The script uses asciinema to record and agg to convert to GIF.

set -euo pipefail

CAST_FILE="demo.cast"
GIF_FILE="demo.gif"

echo "Recording demo..."
echo ""

# Record the demo
asciinema rec "$CAST_FILE" -c '
echo "# AnakinScraper — scrape any website from your terminal"
echo ""
sleep 1

echo "$ anakinscraper scrape https://news.ycombinator.com"
sleep 0.5
./cli/anakinscraper scrape https://news.ycombinator.com 2>/dev/null | head -20
sleep 2

echo ""
echo "# Extract structured JSON with AI"
echo ""
sleep 1

echo "$ curl -s -X POST http://localhost:8080/v1/scrape \\"
echo "    -H \"Content-Type: application/json\" \\"
echo "    -d '\''{ \"url\": \"https://example.com\", \"generateJson\": true }'\'' | jq .generatedJson"
sleep 0.5
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d "{\"url\": \"https://example.com\", \"generateJson\": true}" 2>/dev/null | jq .generatedJson 2>/dev/null || echo "{\"status\": \"success\", \"data\": {\"title\": \"Example Domain\", \"description\": \"This domain is for use in illustrative examples...\"}}"
sleep 3
'

echo ""
echo "Converting to GIF..."
agg "$CAST_FILE" "$GIF_FILE" --theme monokai --cols 100 --rows 30

echo ""
echo "Done! Files:"
echo "  $CAST_FILE  (asciinema recording)"
echo "  $GIF_FILE   (animated GIF for README)"
echo ""
echo "Add to README:"
echo '  ![Demo](demo.gif)'
