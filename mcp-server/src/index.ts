#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const API_URL = (process.env.ANAKINSCRAPER_API_URL || "http://localhost:8080").replace(/\/+$/, "");

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

interface ApiError {
  error: string;
  message: string;
}

async function apiRequest(
  method: string,
  path: string,
  body?: Record<string, unknown>,
): Promise<unknown> {
  const url = `${API_URL}${path}`;
  const opts: RequestInit = {
    method,
    headers: { "Content-Type": "application/json" },
  };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(url, opts);
  } catch (err) {
    throw new Error(
      `Network error connecting to AnakinScraper at ${url}: ${err instanceof Error ? err.message : String(err)}`,
    );
  }

  const text = await res.text();

  if (!res.ok) {
    let detail: string;
    try {
      const parsed = JSON.parse(text) as ApiError;
      detail = parsed.message || parsed.error || text;
    } catch {
      detail = text;
    }
    throw new Error(`AnakinScraper API error (HTTP ${res.status}): ${detail}`);
  }

  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

// ---------------------------------------------------------------------------
// MCP Server
// ---------------------------------------------------------------------------

const server = new McpServer({
  name: "anakinscraper",
  version: "1.0.0",
});

// ---- Tool 1: anakinscraper_scrape (synchronous) --------------------------

server.tool(
  "anakinscraper_scrape",
  "Scrape a URL synchronously. Blocks up to 30 seconds and returns the full result including markdown, HTML, and optionally extracted JSON. Use this when you want immediate results without polling.",
  {
    url: z.string().describe("The URL to scrape (must be http or https)"),
    useBrowser: z
      .boolean()
      .optional()
      .default(false)
      .describe("Force browser rendering via Camoufox. When false, HTTP is tried first with browser as fallback."),
    generateJson: z
      .boolean()
      .optional()
      .default(false)
      .describe("Extract structured JSON from the page using Gemini (requires GEMINI_API_KEY on the server)."),
  },
  async ({ url, useBrowser, generateJson }) => {
    try {
      const result = await apiRequest("POST", "/v1/scrape", {
        url,
        useBrowser,
        generateJson,
      });
      return {
        content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
      };
    } catch (err) {
      return {
        isError: true,
        content: [{ type: "text" as const, text: String(err instanceof Error ? err.message : err) }],
      };
    }
  },
);

// ---- Tool 2: anakinscraper_scrape_async ----------------------------------

server.tool(
  "anakinscraper_scrape_async",
  "Submit an async scrape job. Returns immediately with a job ID. Use anakinscraper_get_job to poll for the result. Useful for pages that take longer than 30 seconds to scrape.",
  {
    url: z.string().describe("The URL to scrape (must be http or https)"),
    useBrowser: z
      .boolean()
      .optional()
      .default(false)
      .describe("Force browser rendering via Camoufox. When false, HTTP is tried first with browser as fallback."),
    generateJson: z
      .boolean()
      .optional()
      .default(false)
      .describe("Extract structured JSON from the page using Gemini (requires GEMINI_API_KEY on the server)."),
  },
  async ({ url, useBrowser, generateJson }) => {
    try {
      const result = await apiRequest("POST", "/v1/url-scraper", {
        url,
        useBrowser,
        generateJson,
      });
      return {
        content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
      };
    } catch (err) {
      return {
        isError: true,
        content: [{ type: "text" as const, text: String(err instanceof Error ? err.message : err) }],
      };
    }
  },
);

// ---- Tool 3: anakinscraper_get_job ---------------------------------------

server.tool(
  "anakinscraper_get_job",
  "Get the status and result of a scrape job by its ID. Use this to poll async jobs submitted via anakinscraper_scrape_async or anakinscraper_batch_scrape. Status will be one of: pending, processing, completed, failed.",
  {
    jobId: z.string().describe("The job UUID returned by anakinscraper_scrape_async or anakinscraper_batch_scrape"),
  },
  async ({ jobId }) => {
    try {
      // Try single job first, then batch job
      let result: unknown;
      try {
        result = await apiRequest("GET", `/v1/url-scraper/${encodeURIComponent(jobId)}`);
      } catch (singleErr) {
        // If single job lookup fails with 404, try batch endpoint
        if (singleErr instanceof Error && singleErr.message.includes("404")) {
          result = await apiRequest("GET", `/v1/url-scraper/batch/${encodeURIComponent(jobId)}`);
        } else {
          throw singleErr;
        }
      }
      return {
        content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
      };
    } catch (err) {
      return {
        isError: true,
        content: [{ type: "text" as const, text: String(err instanceof Error ? err.message : err) }],
      };
    }
  },
);

// ---- Tool 4: anakinscraper_batch_scrape ----------------------------------

server.tool(
  "anakinscraper_batch_scrape",
  "Submit a batch of up to 10 URLs for scraping. Returns a batch job ID. Use anakinscraper_get_job to poll for results. All URLs are scraped in parallel on the server.",
  {
    urls: z.array(z.string()).min(1).max(10).describe("Array of 1-10 URLs to scrape"),
    useBrowser: z
      .boolean()
      .optional()
      .default(false)
      .describe("Force browser rendering for all URLs"),
    generateJson: z
      .boolean()
      .optional()
      .default(false)
      .describe("Extract structured JSON from all pages using Gemini (requires GEMINI_API_KEY on the server)."),
  },
  async ({ urls, useBrowser, generateJson }) => {
    try {
      const result = await apiRequest("POST", "/v1/url-scraper/batch", {
        urls,
        useBrowser,
        generateJson,
      });
      return {
        content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
      };
    } catch (err) {
      return {
        isError: true,
        content: [{ type: "text" as const, text: String(err instanceof Error ? err.message : err) }],
      };
    }
  },
);

// ---- Tool 5: anakinscraper_extract_json ----------------------------------

server.tool(
  "anakinscraper_extract_json",
  "Scrape a URL and extract structured JSON data in one call. This is a convenience wrapper that calls the sync scrape endpoint with generateJson=true and returns just the extracted data. Requires GEMINI_API_KEY on the server.",
  {
    url: z.string().describe("The URL to scrape and extract structured data from"),
  },
  async ({ url }) => {
    try {
      const result = (await apiRequest("POST", "/v1/scrape", {
        url,
        useBrowser: false,
        generateJson: true,
      })) as Record<string, unknown>;

      // Extract just the generatedJson.data field if available
      const generatedJson = result.generatedJson as Record<string, unknown> | null | undefined;
      if (generatedJson && generatedJson.status === "success" && generatedJson.data) {
        return {
          content: [{ type: "text" as const, text: JSON.stringify(generatedJson.data, null, 2) }],
        };
      }

      // If generatedJson is not available or failed, return the full result with context
      if (generatedJson && generatedJson.status === "failed") {
        return {
          isError: true,
          content: [
            {
              type: "text" as const,
              text: "JSON extraction failed. The page was scraped successfully but Gemini could not extract structured data. Make sure GEMINI_API_KEY is set on the server.",
            },
          ],
        };
      }

      // Fallback: return the whole response
      return {
        content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
      };
    } catch (err) {
      return {
        isError: true,
        content: [{ type: "text" as const, text: String(err instanceof Error ? err.message : err) }],
      };
    }
  },
);

// ---------------------------------------------------------------------------
// Start
// ---------------------------------------------------------------------------

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("AnakinScraper MCP server running on stdio");
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
