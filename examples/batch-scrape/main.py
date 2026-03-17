"""Batch scrape example - fetch multiple URLs in one request."""
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "../../sdks/python"))

from anakinscraper import AnakinScraper

client = AnakinScraper(
    api_key=os.environ.get("ANAKIN_API_KEY", "sk_test_local_development_key_12345"),
    base_url=os.environ.get("ANAKIN_BASE_URL", "http://localhost:8080"),
)

urls = [
    "https://example.com",
    "https://httpbin.org/html",
    "https://jsonplaceholder.typicode.com/posts/1",
]

print(f"Batch scraping {len(urls)} URLs...")
result = client.scrape_batch(urls)
print(f"Status: {result.status}")
print(f"Duration: {result.duration_ms}ms")
print()

for r in result.results:
    print(f"[{r.status}] {r.url}")
    if r.markdown:
        preview = r.markdown[:100].replace("\n", " ")
        print(f"  {preview}...")
    if r.error:
        print(f"  Error: {r.error}")
    print()
