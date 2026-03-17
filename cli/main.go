package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultAPIURL   = "http://localhost:8080"
	pollInterval    = 1 * time.Second
	pollTimeout     = 120 * time.Second
	requestTimeout  = 60 * time.Second
)

// --- API types ---

type scrapeRequest struct {
	URL          string `json:"url"`
	UseBrowser   bool   `json:"useBrowser,omitempty"`
	GenerateJson bool   `json:"generateJson,omitempty"`
}

type batchScrapeRequest struct {
	URLs         []string `json:"urls"`
	UseBrowser   bool     `json:"useBrowser,omitempty"`
	GenerateJson bool     `json:"generateJson,omitempty"`
}

type jobResponse struct {
	ID            string           `json:"id"`
	Status        string           `json:"status"`
	URL           string           `json:"url,omitempty"`
	JobType       string           `json:"jobType"`
	HTML          *string          `json:"html,omitempty"`
	CleanedHTML   *string          `json:"cleanedHtml,omitempty"`
	Markdown      *string          `json:"markdown,omitempty"`
	GeneratedJson *generatedJSON   `json:"generatedJson,omitempty"`
	Cached        *bool            `json:"cached,omitempty"`
	Error         *string          `json:"error,omitempty"`
	CreatedAt     string           `json:"createdAt,omitempty"`
	CompletedAt   *string          `json:"completedAt,omitempty"`
	DurationMs    *int             `json:"durationMs,omitempty"`
}

type generatedJSON struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type batchJobResponse struct {
	ID          string        `json:"id"`
	Status      string        `json:"status"`
	JobType     string        `json:"jobType"`
	URLs        []string      `json:"urls,omitempty"`
	Results     []batchResult `json:"results,omitempty"`
	CreatedAt   string        `json:"createdAt,omitempty"`
	CompletedAt *string       `json:"completedAt,omitempty"`
	DurationMs  *int          `json:"durationMs,omitempty"`
}

type batchResult struct {
	Index         int            `json:"index"`
	URL           string         `json:"url"`
	Status        string         `json:"status"`
	HTML          *string        `json:"html,omitempty"`
	CleanedHTML   *string        `json:"cleanedHtml,omitempty"`
	Markdown      *string        `json:"markdown,omitempty"`
	GeneratedJson *generatedJSON `json:"generatedJson,omitempty"`
	Cached        *bool          `json:"cached,omitempty"`
	Error         *string        `json:"error,omitempty"`
	DurationMs    *int           `json:"durationMs,omitempty"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type healthResponse struct {
	Status   string `json:"status"`
	Database bool   `json:"database"`
	Service  string `json:"service"`
}

// --- helpers ---

func getAPIURL(flagValue string) string {
	if flagValue != "" {
		return strings.TrimRight(flagValue, "/")
	}
	if env := os.Getenv("ANAKINSCRAPER_API_URL"); env != "" {
		return strings.TrimRight(env, "/")
	}
	return defaultAPIURL
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

func doPost(url string, body any) ([]byte, int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling request: %w", err)
	}

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return data, resp.StatusCode, nil
}

func doGet(url string) ([]byte, int, error) {
	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return data, resp.StatusCode, nil
}

// --- output helpers ---

func printJobResult(job *jobResponse, format string, outputJSON bool, extract bool) {
	if outputJSON {
		raw, _ := json.Marshal(job)
		fmt.Println(prettyJSON(raw))
		return
	}

	if extract && job.GeneratedJson != nil {
		if job.GeneratedJson.Status == "success" && len(job.GeneratedJson.Data) > 0 {
			fmt.Println(prettyJSON(job.GeneratedJson.Data))
		} else {
			fmt.Fprintf(os.Stderr, "JSON extraction status: %s\n", job.GeneratedJson.Status)
		}
		return
	}

	switch format {
	case "html":
		if job.CleanedHTML != nil {
			fmt.Println(*job.CleanedHTML)
		} else if job.HTML != nil {
			fmt.Println(*job.HTML)
		}
	case "json":
		if job.GeneratedJson != nil && len(job.GeneratedJson.Data) > 0 {
			fmt.Println(prettyJSON(job.GeneratedJson.Data))
		} else {
			fmt.Fprintln(os.Stderr, "No generated JSON available. Use --extract to enable JSON extraction.")
		}
	default: // "markdown"
		if job.Markdown != nil {
			fmt.Println(*job.Markdown)
		}
	}
}

func printBatchResultItem(r *batchResult, format string, outputJSON bool, extract bool) {
	if outputJSON {
		raw, _ := json.Marshal(r)
		fmt.Println(prettyJSON(raw))
		return
	}

	if r.Status == "failed" {
		errMsg := "unknown error"
		if r.Error != nil {
			errMsg = *r.Error
		}
		fmt.Fprintf(os.Stderr, "[%d] %s -- FAILED: %s\n", r.Index, r.URL, errMsg)
		return
	}

	fmt.Fprintf(os.Stderr, "[%d] %s\n", r.Index, r.URL)

	if extract && r.GeneratedJson != nil {
		if r.GeneratedJson.Status == "success" && len(r.GeneratedJson.Data) > 0 {
			fmt.Println(prettyJSON(r.GeneratedJson.Data))
		} else {
			fmt.Fprintf(os.Stderr, "    JSON extraction status: %s\n", r.GeneratedJson.Status)
		}
		return
	}

	switch format {
	case "html":
		if r.CleanedHTML != nil {
			fmt.Println(*r.CleanedHTML)
		} else if r.HTML != nil {
			fmt.Println(*r.HTML)
		}
	case "json":
		if r.GeneratedJson != nil && len(r.GeneratedJson.Data) > 0 {
			fmt.Println(prettyJSON(r.GeneratedJson.Data))
		} else {
			fmt.Fprintf(os.Stderr, "    No generated JSON available.\n")
		}
	default:
		if r.Markdown != nil {
			fmt.Println(*r.Markdown)
		}
	}
}

// --- commands ---

func cmdScrape(args []string) {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	outputJSON := fs.Bool("json", false, "Output raw JSON response")
	extract := fs.Bool("extract", false, "Enable JSON extraction (generateJson=true)")
	browser := fs.Bool("browser", false, "Force browser rendering")
	format := fs.String("format", "markdown", "Output field: markdown, html, or json")
	apiURL := fs.String("api-url", "", "API base URL (overrides ANAKINSCRAPER_API_URL)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: anakinscraper scrape [flags] <url>\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	url := fs.Arg(0)
	base := getAPIURL(*apiURL)

	req := scrapeRequest{
		URL:          url,
		UseBrowser:   *browser,
		GenerateJson: *extract,
	}

	fmt.Fprintf(os.Stderr, "Scraping %s ...\n", url)

	data, status, err := doPost(base+"/v1/scrape", req)
	if err != nil {
		fatal("request failed: %v", err)
	}

	if status >= 400 {
		var apiErr errorResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			fatal("server error (%d): %s", status, apiErr.Message)
		}
		fatal("server returned HTTP %d: %s", status, string(data))
	}

	var job jobResponse
	if err := json.Unmarshal(data, &job); err != nil {
		fatal("parsing response: %v", err)
	}

	if job.Status == "failed" {
		errMsg := "unknown error"
		if job.Error != nil {
			errMsg = *job.Error
		}
		fatal("scrape failed: %s", errMsg)
	}

	if job.DurationMs != nil {
		fmt.Fprintf(os.Stderr, "Done in %dms\n", *job.DurationMs)
	}

	printJobResult(&job, *format, *outputJSON, *extract)
}

func cmdBatch(args []string) {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	outputJSON := fs.Bool("json", false, "Output raw JSON response")
	extract := fs.Bool("extract", false, "Enable JSON extraction (generateJson=true)")
	browser := fs.Bool("browser", false, "Force browser rendering")
	format := fs.String("format", "markdown", "Output field: markdown, html, or json")
	apiURL := fs.String("api-url", "", "API base URL (overrides ANAKINSCRAPER_API_URL)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: anakinscraper batch [flags] <url1> <url2> ...\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	urls := fs.Args()
	base := getAPIURL(*apiURL)

	req := batchScrapeRequest{
		URLs:         urls,
		UseBrowser:   *browser,
		GenerateJson: *extract,
	}

	fmt.Fprintf(os.Stderr, "Submitting batch of %d URLs ...\n", len(urls))

	data, status, err := doPost(base+"/v1/url-scraper/batch", req)
	if err != nil {
		fatal("request failed: %v", err)
	}

	if status >= 400 {
		var apiErr errorResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			fatal("server error (%d): %s", status, apiErr.Message)
		}
		fatal("server returned HTTP %d: %s", status, string(data))
	}

	var batch batchJobResponse
	if err := json.Unmarshal(data, &batch); err != nil {
		fatal("parsing response: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Batch ID: %s\n", batch.ID)

	// Poll until done
	deadline := time.Now().Add(pollTimeout)
	for batch.Status != "completed" && batch.Status != "failed" {
		if time.Now().After(deadline) {
			fatal("timed out after %v waiting for batch to complete", pollTimeout)
		}

		time.Sleep(pollInterval)

		data, status, err = doGet(fmt.Sprintf("%s/v1/url-scraper/batch/%s", base, batch.ID))
		if err != nil {
			fatal("polling failed: %v", err)
		}

		if status >= 400 {
			var apiErr errorResponse
			if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
				fatal("server error (%d): %s", status, apiErr.Message)
			}
			fatal("server returned HTTP %d while polling", status)
		}

		if err := json.Unmarshal(data, &batch); err != nil {
			fatal("parsing poll response: %v", err)
		}

		fmt.Fprintf(os.Stderr, "Status: %s\n", batch.Status)
	}

	if batch.Status == "failed" {
		fatal("batch job failed")
	}

	if batch.DurationMs != nil {
		fmt.Fprintf(os.Stderr, "Batch completed in %dms\n", *batch.DurationMs)
	}

	// Print results
	if *outputJSON {
		raw, _ := json.Marshal(batch)
		fmt.Println(prettyJSON(raw))
		return
	}

	for i := range batch.Results {
		if i > 0 {
			fmt.Fprintln(os.Stderr, "---")
		}
		printBatchResultItem(&batch.Results[i], *format, false, *extract)
	}
}

func cmdHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	apiURL := fs.String("api-url", "", "API base URL (overrides ANAKINSCRAPER_API_URL)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: anakinscraper health [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	base := getAPIURL(*apiURL)

	data, status, err := doGet(base + "/health")
	if err != nil {
		fatal("request failed: %v", err)
	}

	if status >= 400 {
		fatal("server returned HTTP %d: %s", status, string(data))
	}

	var health healthResponse
	if err := json.Unmarshal(data, &health); err != nil {
		fatal("parsing response: %v", err)
	}

	dbStatus := "connected"
	if !health.Database {
		dbStatus = "disconnected"
	}

	fmt.Printf("Service:  %s\n", health.Service)
	fmt.Printf("Status:   %s\n", health.Status)
	fmt.Printf("Database: %s\n", dbStatus)
}

func usage() {
	fmt.Fprintf(os.Stderr, `AnakinScraper CLI - scrape websites from your terminal

Usage:
  anakinscraper <command> [flags] [arguments]

Commands:
  scrape <url>            Scrape a single URL (synchronous)
  batch <url1> <url2> ... Batch scrape multiple URLs
  health                  Check server health

Environment:
  ANAKINSCRAPER_API_URL   API base URL (default: http://localhost:8080)

Run "anakinscraper <command> -h" for command-specific help.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "scrape":
		cmdScrape(args)
	case "batch":
		cmdBatch(args)
	case "health":
		cmdHealth(args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		usage()
		os.Exit(1)
	}
}
