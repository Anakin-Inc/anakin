package models

import "time"

// Job statuses
const (
	JobStatusPending    = "pending"
	JobStatusProcessing = "processing"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
)

// Job types
const (
	JobTypeURLScraper      = "url_scraper"
	JobTypeBatchURLScraper = "batch_url_scraper"
	JobTypeMap             = "map"
	JobTypeCrawl           = "crawl"
)

// --- API request/response types ---

// ScrapeRequest is the request body for single URL scraping.
type ScrapeRequest struct {
	URL          string `json:"url"`
	Country      string `json:"country,omitempty"`
	ForceFresh   bool   `json:"forceFresh,omitempty"`
	UseBrowser   bool   `json:"useBrowser,omitempty"`
	GenerateJson bool   `json:"generateJson,omitempty"`
}

// BatchScrapeRequest is the request body for batch URL scraping.
type BatchScrapeRequest struct {
	URLs         []string `json:"urls"`
	Country      string   `json:"country,omitempty"`
	UseBrowser   bool     `json:"useBrowser,omitempty"`
	GenerateJson bool     `json:"generateJson,omitempty"`
}

// MapRequest is the request body for site mapping.
type MapRequest struct {
	URL               string `json:"url"`
	IncludeSubdomains bool   `json:"includeSubdomains,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Search            string `json:"search,omitempty"`
	UseBrowser        bool   `json:"useBrowser,omitempty"`
}

// CrawlRequest is the request body for site crawling.
type CrawlRequest struct {
	URL             string   `json:"url"`
	MaxPages        int      `json:"maxPages,omitempty"`
	IncludePatterns []string `json:"includePatterns,omitempty"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
	Country         string   `json:"country,omitempty"`
	UseBrowser      bool     `json:"useBrowser,omitempty"`
}

// JobResponse is the response for a single scraping job.
type JobResponse struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	URL         string  `json:"url,omitempty"`
	JobType     string  `json:"jobType"`
	HTML        *string `json:"html,omitempty"`
	CleanedHTML *string `json:"cleanedHtml,omitempty"`
	Markdown    *string `json:"markdown,omitempty"`
	Cached      *bool   `json:"cached,omitempty"`
	Error       *string `json:"error,omitempty"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	CompletedAt *string `json:"completedAt,omitempty"`
	DurationMs  *int    `json:"durationMs,omitempty"`
}

// BatchJobResponse is the response for a batch scraping job.
type BatchJobResponse struct {
	ID          string        `json:"id"`
	Status      string        `json:"status"`
	JobType     string        `json:"jobType"`
	URLs        []string      `json:"urls,omitempty"`
	Results     []BatchResult `json:"results,omitempty"`
	CreatedAt   string        `json:"createdAt,omitempty"`
	CompletedAt *string       `json:"completedAt,omitempty"`
	DurationMs  *int          `json:"durationMs,omitempty"`
}

// BatchResult is the result for a single URL within a batch job.
type BatchResult struct {
	Index       int     `json:"index"`
	URL         string  `json:"url"`
	Status      string  `json:"status"`
	HTML        *string `json:"html,omitempty"`
	CleanedHTML *string `json:"cleanedHtml,omitempty"`
	Markdown    *string `json:"markdown,omitempty"`
	Cached      *bool   `json:"cached,omitempty"`
	Error       *string `json:"error,omitempty"`
	DurationMs  *int    `json:"durationMs,omitempty"`
}

// MapResponse is the response for a site mapping job.
type MapResponse struct {
	ID          string   `json:"id"`
	Status      string   `json:"status"`
	URL         string   `json:"url"`
	Links       []string `json:"links,omitempty"`
	TotalLinks  int      `json:"totalLinks,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	CompletedAt *string  `json:"completedAt,omitempty"`
	DurationMs  *int     `json:"durationMs,omitempty"`
}

// CrawlResponse is the response for a site crawling job.
type CrawlResponse struct {
	ID             string        `json:"id"`
	Status         string        `json:"status"`
	URL            string        `json:"url"`
	TotalPages     int           `json:"totalPages,omitempty"`
	CompletedPages int           `json:"completedPages,omitempty"`
	Results        []CrawlResult `json:"results,omitempty"`
	CreatedAt      string        `json:"createdAt,omitempty"`
	CompletedAt    *string       `json:"completedAt,omitempty"`
	DurationMs     *int          `json:"durationMs,omitempty"`
}

// CrawlResult is the result for a single page within a crawl job.
type CrawlResult struct {
	URL        string  `json:"url"`
	Status     string  `json:"status"`
	Markdown   *string `json:"markdown,omitempty"`
	Error      *string `json:"error,omitempty"`
	DurationMs *int    `json:"durationMs,omitempty"`
}

// ErrorResponse is a standard JSON error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// --- Internal job types ---

// JobMessage is the message passed from API handlers to the worker pool.
type JobMessage struct {
	JobID       string `json:"jobId"`
	URL         string `json:"url"`
	JobType     string `json:"jobType"`
	Country     string `json:"country"`
	ForceFresh  bool   `json:"forceFresh"`
	UseBrowser  bool   `json:"useBrowser"`
	ParentJobID string `json:"parentJobId,omitempty"`
	// Map options
	IncludeSubdomains bool   `json:"includeSubdomains,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Search            string `json:"search,omitempty"`
	// Crawl options
	MaxPages        int      `json:"maxPages,omitempty"`
	IncludePatterns []string `json:"includePatterns,omitempty"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

// HandlerRequest holds parameters passed to the scraping handler chain.
type HandlerRequest struct {
	JobID      string
	URL        string
	Country    string
	UseBrowser bool
	ProxyURL   string
	Timeout    time.Duration
}

// ScrapeResult holds the output from a scraping handler.
type ScrapeResult struct {
	HTML        string
	CleanedHTML string
	Markdown    string
	StatusCode  int
	DurationMs  int
	Handler     string
	Cached      bool
}

// JobStatusResponse is the full job result stored in the database.
type JobStatusResponse struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	URL         string  `json:"url"`
	JobType     string  `json:"jobType"`
	HTML        *string `json:"html,omitempty"`
	CleanedHTML *string `json:"cleanedHtml,omitempty"`
	Markdown    *string `json:"markdown,omitempty"`
	Cached      *bool   `json:"cached,omitempty"`
	Error       *string `json:"error,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	CompletedAt *string `json:"completedAt,omitempty"`
	DurationMs  *int    `json:"durationMs,omitempty"`
}

// MapJobResult is the result payload stored for map jobs.
type MapJobResult struct {
	Links []string `json:"links"`
}

// CrawlJobResult is the result payload stored for crawl jobs.
type CrawlJobResult struct {
	TotalPages     int           `json:"totalPages"`
	CompletedPages int           `json:"completedPages"`
	Results        []CrawlResult `json:"results"`
}
