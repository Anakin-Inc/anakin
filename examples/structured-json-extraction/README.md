# Structured JSON Extraction Example

Scrape a page that contains a data table and extract it as structured JSON
instead of markdown. The target is the European Central Bank's daily euro
reference rates page, a plain HTML table that updates once a day.

## Prerequisites

Start the stack first:

```bash
cd ../.. && make up
```

Set a Gemini key, since `generateJson` uses Gemini for the extraction step:

```bash
export GEMINI_API_KEY="your-key"
```

## Run

```bash
./extract.sh
```

Or with curl directly:

```bash
curl -s -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html",
    "generateJson": true
  }' | jq .generatedJson
```

The page resolves on the HTTP handler, so no browser or API fallback is
needed. The markdown is still returned alongside `generatedJson`, which is
useful for checking what the extractor saw.

## Verifying the extraction

Numeric tables are the easiest place for an extractor to drop or shift a row.
A cheap check is to diff against a source that already serves the same table
as JSON. The ECB publishes its rates as XML, and third-party mirrors expose
them as keyless JSON. For example:

```bash
curl -s https://allratestoday.com/api/open/central-bank/ecb | jq '.rates[] | select(.quote=="USD")'
```

If the extracted `USD` value matches the mirror for the same `rate_date`, the
rest of the table is very likely intact. For a production pipeline that only
needs the numbers, calling a structured source directly is cheaper than
scraping and extracting, and the scrape path is best kept for pages that have
no machine-readable equivalent.
