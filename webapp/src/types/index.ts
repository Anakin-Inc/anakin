export interface HealthResponse {
  status: string;
  database: boolean;
  service: string;
}

export interface ScrapeRequest {
  url: string;
  country?: string;
  forceFresh?: boolean;
  useBrowser?: boolean;
  generateJson?: boolean;
}

export interface BatchScrapeRequest {
  urls: string[];
  country?: string;
  useBrowser?: boolean;
  generateJson?: boolean;
}

export interface GeneratedJsonResponse {
  status: string;
  data?: unknown;
}

export interface JobResponse {
  id: string;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  url?: string;
  jobType: string;
  html?: string | null;
  cleanedHtml?: string | null;
  markdown?: string | null;
  generatedJson?: GeneratedJsonResponse | null;
  cached?: boolean | null;
  error?: string | null;
  createdAt?: string;
  completedAt?: string | null;
  durationMs?: number | null;
}

export interface BatchJobResponse {
  id: string;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  jobType: string;
  urls?: string[];
  results?: BatchResult[];
  createdAt?: string;
  completedAt?: string | null;
  durationMs?: number | null;
}

export interface BatchResult {
  index: number;
  url: string;
  status: string;
  html?: string | null;
  cleanedHtml?: string | null;
  markdown?: string | null;
  generatedJson?: GeneratedJsonResponse | null;
  cached?: boolean | null;
  error?: string | null;
  durationMs?: number | null;
}

export interface DomainConfig {
  id: number;
  domain: string;
  isEnabled: boolean;
  matchSubdomains: boolean;
  priority: number;
  handlerChain: string[];
  requestTimeoutMs: number;
  maxRetries: number;
  minContentLength: number;
  failurePatterns: string[];
  requiredPatterns: string[];
  customHeaders: Record<string, string>;
  customUserAgent: string;
  proxyUrl: string;
  blocked: boolean;
  blockedReason: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProxyScore {
  proxyUrl: string;
  targetHost: string;
  alpha: number;
  beta: number;
  score: number;
  totalRequests: number;
  avgLatencyMs: number;
  lastUpdated: string;
}

export interface ProxyScoresResponse {
  proxies?: string[];
  scores: Record<string, ProxyScore[]>;
}

export interface ErrorResponse {
  error: string;
  message: string;
}

export type TrackedJob = {
  id: string;
  url: string;
  type: 'single' | 'batch';
  status: 'pending' | 'processing' | 'completed' | 'failed';
  createdAt: string;
  urls?: string[];
};
