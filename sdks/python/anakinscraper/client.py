import time

import requests

from .exceptions import (
    AnakinScraperError,
    AuthenticationError,
    JobFailedError,
    RateLimitError,
)
from .models import BatchScrapeResult, CrawlResult, MapResult, ScrapeResult


class AnakinScraper:
    """Python SDK client for AnakinScraper.

    Args:
        api_key: Your AnakinScraper API key.
        base_url: Base URL of the AnakinScraper API.
        timeout: Maximum seconds to wait when polling for job completion.
        poll_interval: Seconds between poll requests.
    """

    def __init__(
        self,
        api_key: str,
        base_url: str = "http://localhost:8080",
        timeout: float = 120,
        poll_interval: float = 1.0,
    ):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.poll_interval = poll_interval
        self._session = requests.Session()
        self._session.headers.update(
            {
                "X-API-Key": self.api_key,
                "Content-Type": "application/json",
            }
        )

    # ------------------------------------------------------------------ #
    #  Public methods
    # ------------------------------------------------------------------ #

    def scrape(
        self,
        url: str,
        country: str = "us",
        force_fresh: bool = False,
        use_browser: bool = False,
        generate_json: bool = False,
    ) -> ScrapeResult:
        """Submit a scrape job and poll until it completes or fails.

        Returns a ``ScrapeResult`` with the scraped content.
        Raises ``TimeoutError`` if the job does not finish within *timeout* seconds.
        """
        job_id = self.scrape_async(
            url,
            country=country,
            force_fresh=force_fresh,
            use_browser=use_browser,
            generate_json=generate_json,
        )
        return self._poll_job(job_id)

    def scrape_async(
        self,
        url: str,
        country: str = "us",
        force_fresh: bool = False,
        use_browser: bool = False,
        generate_json: bool = False,
    ) -> str:
        """Submit a scrape job without waiting for it to finish.

        Returns the job ID that can later be passed to ``get_job()``.
        """
        payload = {
            "url": url,
            "country": country,
            "forceFresh": force_fresh,
            "useBrowser": use_browser,
            "generateJson": generate_json,
        }
        data = self._post("/v1/url-scraper", payload)
        return data["id"]

    def get_job(self, job_id: str) -> ScrapeResult:
        """Fetch the current state of a scrape job by its ID."""
        data = self._get(f"/v1/url-scraper/{job_id}")
        return self._to_scrape_result(data)

    def scrape_batch(
        self,
        urls: list,
        country: str = "us",
        use_browser: bool = False,
        generate_json: bool = False,
    ) -> BatchScrapeResult:
        """Submit a batch scrape job and poll until it completes or fails.

        Accepts up to 10 URLs.  Returns a ``BatchScrapeResult``.
        """
        payload = {
            "urls": urls,
            "country": country,
            "useBrowser": use_browser,
            "generateJson": generate_json,
        }
        data = self._post("/v1/url-scraper/batch", payload)
        job_id = data["id"]

        deadline = time.monotonic() + self.timeout
        while time.monotonic() < deadline:
            data = self._get(f"/v1/url-scraper/batch/{job_id}")
            status = data.get("status", "")
            if status in ("completed", "failed"):
                return self._to_batch_result(data)
            time.sleep(self.poll_interval)

        raise TimeoutError(
            f"Batch job {job_id} did not complete within {self.timeout}s"
        )

    def map(
        self,
        url: str,
        include_subdomains: bool = False,
        limit: int = 100,
        search: str = None,
    ) -> MapResult:
        """Discover links on a URL.

        Returns a ``MapResult`` containing the discovered links.
        """
        payload = {
            "url": url,
            "includeSubdomains": include_subdomains,
            "limit": limit,
        }
        if search is not None:
            payload["search"] = search

        data = self._post("/v1/map", payload)
        job_id = data["id"]

        deadline = time.monotonic() + self.timeout
        while time.monotonic() < deadline:
            data = self._get(f"/v1/map/{job_id}")
            status = data.get("status", "")
            if status in ("completed", "failed"):
                return MapResult(
                    id=data["id"],
                    status=data["status"],
                    url=data.get("url", url),
                    links=data.get("links", []),
                    total_links=data.get("totalLinks", 0),
                )
            time.sleep(self.poll_interval)

        raise TimeoutError(
            f"Map job {job_id} did not complete within {self.timeout}s"
        )

    def crawl(
        self,
        url: str,
        max_pages: int = 10,
        include_patterns: list = None,
        exclude_patterns: list = None,
        country: str = "us",
    ) -> CrawlResult:
        """Start a multi-page crawl and poll until it completes or fails.

        Returns a ``CrawlResult`` with per-page results.
        """
        payload = {
            "url": url,
            "maxPages": max_pages,
            "country": country,
        }
        if include_patterns is not None:
            payload["includePatterns"] = include_patterns
        if exclude_patterns is not None:
            payload["excludePatterns"] = exclude_patterns

        data = self._post("/v1/crawl", payload)
        job_id = data["id"]

        deadline = time.monotonic() + self.timeout
        while time.monotonic() < deadline:
            data = self._get(f"/v1/crawl/{job_id}")
            status = data.get("status", "")
            if status in ("completed", "failed"):
                return CrawlResult(
                    id=data["id"],
                    status=data["status"],
                    url=data.get("url", url),
                    total_pages=data.get("totalPages", 0),
                    completed_pages=data.get("completedPages", 0),
                    results=data.get("results"),
                )
            time.sleep(self.poll_interval)

        raise TimeoutError(
            f"Crawl job {job_id} did not complete within {self.timeout}s"
        )

    # ------------------------------------------------------------------ #
    #  Internal helpers
    # ------------------------------------------------------------------ #

    def _post(self, path: str, payload: dict) -> dict:
        resp = self._session.post(f"{self.base_url}{path}", json=payload)
        self._check_response(resp)
        return resp.json()

    def _get(self, path: str) -> dict:
        resp = self._session.get(f"{self.base_url}{path}")
        self._check_response(resp)
        return resp.json()

    def _check_response(self, resp: requests.Response) -> None:
        if resp.status_code == 401:
            raise AuthenticationError(
                "Invalid or missing API key",
                status_code=401,
                error_code="unauthorized",
            )
        if resp.status_code == 429:
            raise RateLimitError(
                "Rate limit exceeded",
                status_code=429,
                error_code="rate_limited",
            )
        if resp.status_code >= 400:
            try:
                body = resp.json()
                message = body.get("message", body.get("error", resp.text))
                error_code = body.get("error")
            except ValueError:
                message = resp.text
                error_code = None
            raise AnakinScraperError(
                message,
                status_code=resp.status_code,
                error_code=error_code,
            )

    def _poll_job(self, job_id: str) -> ScrapeResult:
        deadline = time.monotonic() + self.timeout
        while time.monotonic() < deadline:
            result = self.get_job(job_id)
            if result.status == "completed":
                return result
            if result.status == "failed":
                raise JobFailedError(
                    result.error or f"Job {job_id} failed",
                    error_code="job_failed",
                )
            time.sleep(self.poll_interval)

        raise TimeoutError(
            f"Job {job_id} did not complete within {self.timeout}s"
        )

    @staticmethod
    def _to_scrape_result(data: dict) -> ScrapeResult:
        return ScrapeResult(
            id=data["id"],
            status=data["status"],
            url=data.get("url", ""),
            html=data.get("html"),
            cleaned_html=data.get("cleanedHtml"),
            markdown=data.get("markdown"),
            cached=data.get("cached"),
            error=data.get("error"),
            duration_ms=data.get("durationMs"),
        )

    @staticmethod
    def _to_batch_result(data: dict) -> BatchScrapeResult:
        results = []
        for item in data.get("results", []):
            results.append(
                ScrapeResult(
                    id=item.get("id", data["id"]),
                    status=item.get("status", ""),
                    url=item.get("url", ""),
                    html=item.get("html"),
                    cleaned_html=item.get("cleanedHtml"),
                    markdown=item.get("markdown"),
                    cached=item.get("cached"),
                    error=item.get("error"),
                    duration_ms=item.get("durationMs"),
                )
            )
        return BatchScrapeResult(
            id=data["id"],
            status=data["status"],
            urls=data.get("urls", []),
            results=results,
            duration_ms=data.get("durationMs"),
        )
