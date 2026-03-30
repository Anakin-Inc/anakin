# AnakinScraper Web Dashboard

Built-in web UI for AnakinScraper. React 19 + TypeScript + Tailwind CSS + Vite.

## Quick Start

```bash
# Requires the API server running on port 8080
cd webapp
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000). API calls are proxied to `localhost:8080`.

## Pages

| Page | Description |
|------|-------------|
| **Dashboard** | Service health, quick scrape input, API endpoint reference |
| **Scrape** | Sync, async, and batch scraping with live polling and tabbed results (markdown, HTML, cleaned HTML, JSON) |
| **Jobs** | Tracked job history with status filters and auto-refresh |
| **Job Detail** | Full results viewer with metadata and content tabs |
| **Domain Configs** | CRUD management with handler chain reordering, failure patterns, custom headers |
| **Proxy Scores** | Thompson Sampling proxy performance with expandable host groups |

## Build

```bash
npm run build   # Production build → dist/
```

## Tech Stack

- [React 19](https://react.dev) + TypeScript
- [Tailwind CSS 3](https://tailwindcss.com)
- [Vite 8](https://vite.dev)
- [React Router 7](https://reactrouter.com)
