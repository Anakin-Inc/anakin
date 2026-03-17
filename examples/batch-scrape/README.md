# Batch Scrape Example

Scrape multiple URLs in a single API request.

## Prerequisites

Start the stack first:

```bash
make up
```

## Run

```bash
python main.py
```

The example scrapes 3 URLs and prints a preview of each result.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ANAKIN_API_KEY` | `sk_test_local_development_key_12345` | API key |
| `ANAKIN_BASE_URL` | `http://localhost:8080` | API URL |
