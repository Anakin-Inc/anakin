-- Scrape requests table
CREATE TABLE IF NOT EXISTS scrape_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type VARCHAR(50) NOT NULL,
    url TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    country VARCHAR(10) DEFAULT 'us',
    payload TEXT,
    force_fresh BOOLEAN DEFAULT false,
    cached BOOLEAN DEFAULT false,
    html_length INTEGER DEFAULT 0,
    success BOOLEAN DEFAULT true,
    error TEXT,
    result TEXT,
    duration_ms INTEGER,
    parent_job_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_scrape_requests_status ON scrape_requests(status);
CREATE INDEX IF NOT EXISTS idx_scrape_requests_parent ON scrape_requests(parent_job_id);

-- Domain configs table
CREATE TABLE IF NOT EXISTS domain_configs (
    id SERIAL PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    match_subdomains BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 0,
    handler_chain TEXT NOT NULL DEFAULT 'http,browser',
    request_timeout_ms INTEGER NOT NULL DEFAULT 30000,
    max_retries INTEGER NOT NULL DEFAULT 2,
    min_content_length INTEGER NOT NULL DEFAULT 0,
    failure_patterns TEXT NOT NULL DEFAULT '',
    required_patterns TEXT NOT NULL DEFAULT '',
    custom_headers TEXT NOT NULL DEFAULT '{}',
    custom_user_agent TEXT,
    proxy_url TEXT,
    blocked BOOLEAN NOT NULL DEFAULT false,
    blocked_reason TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_domain_configs_domain ON domain_configs(domain);
CREATE INDEX IF NOT EXISTS idx_domain_configs_enabled ON domain_configs(is_enabled);

-- Proxy scores table (Thompson Sampling)
CREATE TABLE IF NOT EXISTS proxy_scores (
    proxy_url TEXT NOT NULL,
    target_host VARCHAR(255) NOT NULL,
    alpha INTEGER NOT NULL DEFAULT 1,
    beta INTEGER NOT NULL DEFAULT 1,
    score FLOAT NOT NULL DEFAULT 0.5,
    total_requests INTEGER NOT NULL DEFAULT 0,
    avg_latency_ms INTEGER NOT NULL DEFAULT 0,
    last_updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (proxy_url, target_host)
);

-- Telemetry instance identity (persists across container restarts)
CREATE TABLE IF NOT EXISTS telemetry_instance (
    id SERIAL PRIMARY KEY,
    instance_id UUID NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scrape_requests_created ON scrape_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_scrape_requests_url ON scrape_requests(url);
CREATE INDEX IF NOT EXISTS idx_proxy_scores_host ON proxy_scores(target_host);
