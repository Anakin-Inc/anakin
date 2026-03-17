# Basic Scrape Example

Scrape a single URL and print the markdown output.

## Prerequisites

Start the stack first:

```bash
make up
```

## Run

```bash
# Default URL (example.com)
python main.py

# Custom URL
python main.py https://news.ycombinator.com
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ANAKIN_API_KEY` | `sk_test_local_development_key_12345` | API key |
| `ANAKIN_BASE_URL` | `http://localhost:8080` | API URL |
