export type AnakinScraperOptions = {
  apiKey: string;
  baseUrl?: string;
  timeout?: number;
  pollInterval?: number;
};

export type ScrapeResult = {
  id: string;
  status: string;
  url: string;
  html?: string;
  cleanedHtml?: string;
  markdown?: string;
  cached?: boolean;
  error?: string;
  durationMs?: number;
};

export type BatchScrapeResult = {
  id: string;
  status: string;
  urls: string[];
  results?: ScrapeResult[];
  durationMs?: number;
};

export type MapResult = {
  id: string;
  status: string;
  url: string;
  links?: string[];
  totalLinks?: number;
};

export type CrawlResult = {
  id: string;
  status: string;
  url: string;
  totalPages?: number;
  completedPages?: number;
  results?: Array<{
    url: string;
    status: string;
    markdown?: string;
    error?: string;
    durationMs?: number;
  }>;
};
