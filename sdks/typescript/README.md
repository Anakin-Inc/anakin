# AnakinScraper TypeScript SDK

Official TypeScript/JavaScript client for the AnakinScraper API.

## Install

```bash
npm install anakinscraper
```

## Usage

```typescript
import { AnakinScraper } from 'anakinscraper';

const client = new AnakinScraper({
  apiKey: 'sk_test_local_development_key_12345',
  baseUrl: 'http://localhost:8080',
});

// Scrape a single URL
const result = await client.scrape('https://example.com');
console.log(result.markdown);

// Batch scrape
const batch = await client.scrapeBatch([
  'https://example.com/page-1',
  'https://example.com/page-2',
]);

// Discover URLs
const map = await client.map('https://example.com', { limit: 50 });
console.log(map.links);

// Crawl
const crawl = await client.crawl('https://example.com', { maxPages: 5 });
```

## License

MIT
