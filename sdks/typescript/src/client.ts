import { AnakinScraperError } from './errors';
import type { AnakinScraperOptions, ScrapeResult, BatchScrapeResult, MapResult, CrawlResult } from './types';

export class AnakinScraper {
  private apiKey: string;
  private baseUrl: string;
  private timeout: number;
  private pollInterval: number;

  constructor(options: AnakinScraperOptions) {
    this.apiKey = options.apiKey;
    this.baseUrl = (options.baseUrl || 'http://localhost:8080').replace(/\/$/, '');
    this.timeout = options.timeout || 120000;
    this.pollInterval = options.pollInterval || 1000;
  }

  async scrape(
    url: string,
    options?: { country?: string; forceFresh?: boolean; useBrowser?: boolean; generateJson?: boolean }
  ): Promise<ScrapeResult> {
    const job = await this.request<ScrapeResult>('POST', '/v1/url-scraper', {
      url,
      country: options?.country || 'us',
      forceFresh: options?.forceFresh || false,
      useBrowser: options?.useBrowser || false,
      generateJson: options?.generateJson || false,
    });
    return this.pollUntilDone<ScrapeResult>(`/v1/url-scraper/${job.id}`);
  }

  async scrapeAsync(
    url: string,
    options?: { country?: string; forceFresh?: boolean; useBrowser?: boolean; generateJson?: boolean }
  ): Promise<string> {
    const job = await this.request<ScrapeResult>('POST', '/v1/url-scraper', {
      url,
      country: options?.country || 'us',
      forceFresh: options?.forceFresh || false,
      useBrowser: options?.useBrowser || false,
      generateJson: options?.generateJson || false,
    });
    return job.id;
  }

  async getJob(jobId: string): Promise<ScrapeResult> {
    return this.request<ScrapeResult>('GET', `/v1/url-scraper/${jobId}`);
  }

  async scrapeBatch(
    urls: string[],
    options?: { country?: string; useBrowser?: boolean; generateJson?: boolean }
  ): Promise<BatchScrapeResult> {
    const job = await this.request<BatchScrapeResult>('POST', '/v1/url-scraper/batch', {
      urls,
      country: options?.country || 'us',
      useBrowser: options?.useBrowser || false,
      generateJson: options?.generateJson || false,
    });
    return this.pollUntilDone<BatchScrapeResult>(`/v1/url-scraper/${job.id}`);
  }

  async map(
    url: string,
    options?: { includeSubdomains?: boolean; limit?: number; search?: string }
  ): Promise<MapResult> {
    const job = await this.request<MapResult>('POST', '/v1/map', {
      url,
      includeSubdomains: options?.includeSubdomains || false,
      limit: options?.limit || 100,
      search: options?.search,
    });
    return this.pollUntilDone<MapResult>(`/v1/map/${job.id}`);
  }

  async crawl(
    url: string,
    options?: { maxPages?: number; includePatterns?: string[]; excludePatterns?: string[]; country?: string }
  ): Promise<CrawlResult> {
    const job = await this.request<CrawlResult>('POST', '/v1/crawl', {
      url,
      maxPages: options?.maxPages || 10,
      includePatterns: options?.includePatterns,
      excludePatterns: options?.excludePatterns,
      country: options?.country || 'us',
    });
    return this.pollUntilDone<CrawlResult>(`/v1/crawl/${job.id}`);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const options: RequestInit = {
      method,
      headers: {
        'X-API-Key': this.apiKey,
        'Content-Type': 'application/json',
      },
    };

    if (body && method !== 'GET') {
      options.body = JSON.stringify(body);
    }

    const response = await fetch(url, options);

    if (!response.ok) {
      const errorBody = await response.json().catch(() => ({}));
      throw new AnakinScraperError(
        errorBody.message || `HTTP ${response.status}`,
        response.status,
        errorBody.error
      );
    }

    return response.json() as Promise<T>;
  }

  private async pollUntilDone<T extends { status: string; error?: string }>(path: string): Promise<T> {
    const deadline = Date.now() + this.timeout;

    while (Date.now() < deadline) {
      const result = await this.request<T>('GET', path);

      if (result.status === 'completed') {
        return result;
      }

      if (result.status === 'failed') {
        throw new AnakinScraperError(result.error || 'Job failed', undefined, 'job_failed');
      }

      await new Promise((resolve) => setTimeout(resolve, this.pollInterval));
    }

    throw new AnakinScraperError('Polling timeout exceeded', undefined, 'timeout');
  }
}
