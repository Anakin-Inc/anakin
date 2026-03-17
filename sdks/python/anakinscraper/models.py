from dataclasses import dataclass
from typing import Optional


@dataclass
class ScrapeResult:
    id: str
    status: str
    url: str
    html: Optional[str] = None
    cleaned_html: Optional[str] = None
    markdown: Optional[str] = None
    cached: Optional[bool] = None
    error: Optional[str] = None
    duration_ms: Optional[int] = None


@dataclass
class BatchScrapeResult:
    id: str
    status: str
    urls: list
    results: list  # list of ScrapeResult
    duration_ms: Optional[int] = None


@dataclass
class MapResult:
    id: str
    status: str
    url: str
    links: list
    total_links: int = 0


@dataclass
class CrawlResult:
    id: str
    status: str
    url: str
    total_pages: int = 0
    completed_pages: int = 0
    results: list = None
