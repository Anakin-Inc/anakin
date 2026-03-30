import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import type { HealthResponse } from '../types';

export function Dashboard() {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [quickUrl, setQuickUrl] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    api
      .health()
      .then(setHealth)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const handleQuickScrape = (e: React.FormEvent) => {
    e.preventDefault();
    if (quickUrl.trim()) {
      navigate(`/scrape?url=${encodeURIComponent(quickUrl.trim())}`);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-zinc-100">Dashboard</h1>
        <p className="text-zinc-500 mt-1">Monitor your AnakinScraper instance</p>
      </div>

      {/* Health Status */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <HealthCard
          label="Service Status"
          value={loading ? 'Checking...' : error ? 'Unreachable' : 'Online'}
          status={loading ? 'loading' : error ? 'error' : 'ok'}
        />
        <HealthCard
          label="Database"
          value={
            loading
              ? 'Checking...'
              : error
              ? 'Unknown'
              : health?.database
              ? 'Connected'
              : 'Disconnected'
          }
          status={
            loading
              ? 'loading'
              : error
              ? 'error'
              : health?.database
              ? 'ok'
              : 'error'
          }
        />
        <HealthCard
          label="Service Name"
          value={health?.service || 'anakinscraper'}
          status="neutral"
        />
      </div>

      {error && (
        <div className="card p-4 border-red-500/30">
          <p className="text-red-400 text-sm">
            Could not connect to the API at <code className="text-red-300">/api</code>.
            Make sure the server is running on port 8080.
          </p>
        </div>
      )}

      {/* Quick Scrape */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-zinc-100 mb-4">Quick Scrape</h2>
        <form onSubmit={handleQuickScrape} className="flex gap-3">
          <input
            type="url"
            placeholder="https://example.com"
            value={quickUrl}
            onChange={(e) => setQuickUrl(e.target.value)}
            className="input flex-1"
            required
          />
          <button type="submit" className="btn-primary whitespace-nowrap">
            Scrape URL
          </button>
        </form>
      </div>

      {/* Info Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="card p-6">
          <h3 className="text-sm font-medium text-zinc-400 mb-3">API Endpoints</h3>
          <div className="space-y-2 text-sm font-mono">
            <EndpointRow method="POST" path="/v1/scrape" desc="Sync scrape" />
            <EndpointRow method="POST" path="/v1/url-scraper" desc="Async scrape" />
            <EndpointRow method="POST" path="/v1/url-scraper/batch" desc="Batch scrape" />
            <EndpointRow method="GET" path="/v1/domain-configs" desc="List configs" />
            <EndpointRow method="GET" path="/v1/proxy/scores" desc="Proxy stats" />
          </div>
        </div>

        <div className="card p-6">
          <h3 className="text-sm font-medium text-zinc-400 mb-3">Quick Start</h3>
          <pre className="bg-zinc-950 border border-zinc-800 rounded-lg p-4 text-sm text-zinc-300 overflow-x-auto">
{`curl -X POST http://localhost:8080/v1/scrape \\
  -H "Content-Type: application/json" \\
  -d '{"url": "https://example.com"}'`}
          </pre>
        </div>
      </div>
    </div>
  );
}

function HealthCard({
  label,
  value,
  status,
}: {
  label: string;
  value: string;
  status: 'ok' | 'error' | 'loading' | 'neutral';
}) {
  const dot =
    status === 'ok'
      ? 'bg-emerald-400'
      : status === 'error'
      ? 'bg-red-400'
      : status === 'loading'
      ? 'bg-yellow-400 animate-pulse'
      : 'bg-zinc-500';

  return (
    <div className="card p-4">
      <p className="text-xs text-zinc-500 uppercase tracking-wider mb-2">{label}</p>
      <div className="flex items-center gap-2">
        <span className={`w-2 h-2 rounded-full ${dot}`} />
        <span className="text-lg font-semibold text-zinc-100">{value}</span>
      </div>
    </div>
  );
}

function EndpointRow({ method, path, desc }: { method: string; path: string; desc: string }) {
  const color = method === 'POST' ? 'text-emerald-400' : 'text-blue-400';
  return (
    <div className="flex items-center gap-3">
      <span className={`${color} w-12 text-xs font-bold`}>{method}</span>
      <span className="text-zinc-300">{path}</span>
      <span className="text-zinc-600 text-xs ml-auto">{desc}</span>
    </div>
  );
}
