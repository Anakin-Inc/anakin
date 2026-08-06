import type {
  HealthResponse,
  ScrapeRequest,
  JobResponse,
  BatchScrapeRequest,
  BatchJobResponse,
  DomainConfig,
  ProxyScoresResponse,
} from '../types';

const BASE = '/api';

// Set VITE_API_KEY when the server runs with API_KEY. Required for domain config writes.
// WARNING: `vite build` inlines this into the public JS bundle — localhost dev only. On a deployed dashboard anyone can read the server's API_KEY via view-source.
const API_KEY = import.meta.env.VITE_API_KEY as string | undefined;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(API_KEY ? { 'X-API-Key': API_KEY } : {}),
      ...init?.headers,
    },
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let body: Record<string, unknown>;
  try {
    body = JSON.parse(text);
  } catch {
    if (!res.ok) throw new Error(`API unreachable (HTTP ${res.status})`);
    throw new Error('Invalid JSON response from server');
  }

  if (!res.ok) {
    throw new Error((body.message as string) || (body.error as string) || `HTTP ${res.status}`);
  }
  return body as T;
}

export const api = {
  health: () => request<HealthResponse>('/health'),

  // Sync scrape
  scrapeSync: (data: ScrapeRequest) =>
    request<JobResponse>('/v1/scrape', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Async scrape
  scrapeAsync: (data: ScrapeRequest) =>
    request<JobResponse>('/v1/url-scraper', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Get job
  getJob: (id: string) => request<JobResponse>(`/v1/url-scraper/${id}`),

  // Batch scrape
  scrapeBatch: (data: BatchScrapeRequest) =>
    request<BatchJobResponse>('/v1/url-scraper/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Get batch job
  getBatchJob: (id: string) =>
    request<BatchJobResponse>(`/v1/url-scraper/batch/${id}`),

  // Domain configs
  getDomainConfigs: () => request<DomainConfig[]>('/v1/domain-configs'),

  getDomainConfig: (domain: string) =>
    request<DomainConfig>(`/v1/domain-configs/${encodeURIComponent(domain)}`),

  createDomainConfig: (data: Partial<DomainConfig>) =>
    request<DomainConfig>('/v1/domain-configs', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateDomainConfig: (domain: string, data: Partial<DomainConfig>) =>
    request<DomainConfig>(`/v1/domain-configs/${encodeURIComponent(domain)}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteDomainConfig: (domain: string) =>
    request<void>(`/v1/domain-configs/${encodeURIComponent(domain)}`, {
      method: 'DELETE',
    }),

  // Proxy scores
  getProxyScores: () => request<ProxyScoresResponse>('/v1/proxy/scores'),
};
