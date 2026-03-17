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
