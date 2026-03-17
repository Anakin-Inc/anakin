"""Basic scrape example - fetch a single URL and print the markdown."""
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "../../sdks/python"))

from anakinscraper import AnakinScraper

client = AnakinScraper(
    api_key=os.environ.get("ANAKIN_API_KEY", "sk_test_local_development_key_12345"),
    base_url=os.environ.get("ANAKIN_BASE_URL", "http://localhost:8080"),
)

url = sys.argv[1] if len(sys.argv) > 1 else "https://example.com"

print(f"Scraping {url}...")
result = client.scrape(url)
print(f"Status: {result.status}")
print(f"Duration: {result.duration_ms}ms")
print(f"Cached: {result.cached}")
print()
print(result.markdown)
