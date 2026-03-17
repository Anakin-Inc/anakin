# AnakinScraper Python SDK

Python client for the [AnakinScraper](https://github.com/AnakinAI/anakinscraper-oss) API.

## Installation

```bash
pip install anakinscraper
```

Or install from source:

```bash
git clone https://github.com/AnakinAI/anakinscraper-py.git
cd anakinscraper-py
pip install -e .
```

## Usage

### Single URL

```python
from anakinscraper import AnakinScraper

client = AnakinScraper(api_key="sk_live_...", base_url="http://localhost:8080")

result = client.scrape("https://example.com")
print(result.markdown)
print(f"Completed in {result.duration_ms}ms (cached: {result.cached})")
```

### Batch Scrape

```python
result = client.scrape_batch([
    "https://example.com/page-1",
    "https://example.com/page-2",
    "https://example.com/page-3",
])

for r in result.results:
    print(f"[{r.status}] {r.url}: {r.markdown[:80]}...")
```

### Async Submit (no polling)

```python
job_id = client.scrape_async("https://example.com")
# ... do other work ...
result = client.get_job(job_id)
```

### Map (discover links)

```python
result = client.map("https://example.com", limit=50, search="blog")
print(f"Found {result.total_links} links")
for link in result.links:
    print(link)
```

### Crawl (multi-page)

```python
result = client.crawl(
    "https://example.com",
    max_pages=10,
    include_patterns=["/blog/**"],
)
for page in result.results:
    print(f"{page['url']}: {page['status']}")
```

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `api_key` | **required** | Your API key |
| `base_url` | `http://localhost:8080` | API base URL |
| `timeout` | `120` | Max seconds to wait for job completion |
| `poll_interval` | `1.0` | Seconds between poll requests |

## Error Handling

```python
from anakinscraper.exceptions import (
    AnakinScraperError,
    AuthenticationError,
    RateLimitError,
    JobFailedError,
)

try:
    result = client.scrape("https://example.com")
except AuthenticationError:
    print("Invalid API key")
except RateLimitError:
    print("Too many requests, slow down")
except JobFailedError as e:
    print(f"Scrape failed: {e}")
except TimeoutError:
    print("Job did not complete in time")
```

## License

MIT
