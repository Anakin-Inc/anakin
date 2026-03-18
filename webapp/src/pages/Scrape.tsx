import { useState, useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { StatusBadge } from '../components/StatusBadge';
import { CodeBlock } from '../components/CodeBlock';
import type { JobResponse, TrackedJob } from '../types';

type Mode = 'sync' | 'async' | 'batch';

export function Scrape() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [mode, setMode] = useState<Mode>('sync');
  const [url, setUrl] = useState(searchParams.get('url') || '');
  const [batchUrls, setBatchUrls] = useState('');
  const [useBrowser, setUseBrowser] = useState(false);
  const [generateJson, setGenerateJson] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<JobResponse | null>(null);
  const [polling, setPolling] = useState(false);
  const [activeTab, setActiveTab] = useState<'markdown' | 'html' | 'cleaned' | 'json'>('markdown');

  // Load URL from search params
  useEffect(() => {
    const u = searchParams.get('url');
    if (u) setUrl(u);
  }, [searchParams]);

  // Poll for async job result
  useEffect(() => {
    if (!polling || !result?.id) return;
    if (result.status === 'completed' || result.status === 'failed') {
      setPolling(false);
      return;
    }

    const timer = setInterval(async () => {
      try {
        const job = await api.getJob(result.id);
        setResult(job);
        if (job.status === 'completed' || job.status === 'failed') {
          setPolling(false);
          updateTrackedJob(result.id, job.status);
        }
      } catch {
        // keep polling
      }
    }, 1500);

    return () => clearInterval(timer);
  }, [polling, result?.id, result?.status]);

  function trackJob(job: JobResponse) {
    const tracked: TrackedJob = {
      id: job.id,
      url: job.url || url,
      type: 'single',
      status: job.status,
      createdAt: job.createdAt || new Date().toISOString(),
    };
    const jobs: TrackedJob[] = JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]');
    jobs.unshift(tracked);
    localStorage.setItem('anakinscraper_jobs', JSON.stringify(jobs.slice(0, 100)));
  }

  function updateTrackedJob(id: string, status: string) {
    const jobs: TrackedJob[] = JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]');
    const idx = jobs.findIndex((j) => j.id === id);
    if (idx !== -1) {
      jobs[idx].status = status as TrackedJob['status'];
      localStorage.setItem('anakinscraper_jobs', JSON.stringify(jobs));
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setResult(null);
    setLoading(true);

    try {
      if (mode === 'batch') {
        const urls = batchUrls
          .split('\n')
          .map((u) => u.trim())
          .filter(Boolean);
        if (urls.length === 0) throw new Error('Enter at least one URL');
        if (urls.length > 10) throw new Error('Maximum 10 URLs per batch');

        const batchJob = await api.scrapeBatch({ urls, useBrowser, generateJson });
        // Track and navigate to jobs page
        const tracked: TrackedJob = {
          id: batchJob.id,
          url: urls[0],
          type: 'batch',
          status: batchJob.status,
          createdAt: batchJob.createdAt || new Date().toISOString(),
          urls,
        };
        const jobs: TrackedJob[] = JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]');
        jobs.unshift(tracked);
        localStorage.setItem('anakinscraper_jobs', JSON.stringify(jobs.slice(0, 100)));
        navigate(`/jobs/${batchJob.id}?type=batch`);
        return;
      }

      const data = { url: url.trim(), useBrowser, generateJson };

      if (mode === 'sync') {
        const job = await api.scrapeSync(data);
        setResult(job);
        trackJob(job);
      } else {
        const job = await api.scrapeAsync(data);
        setResult(job);
        trackJob(job);
        setPolling(true);
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-zinc-100">Scrape</h1>
        <p className="text-zinc-500 mt-1">Submit a scraping job</p>
      </div>

      {/* Mode Selector */}
      <div className="flex gap-1 bg-zinc-900 rounded-lg p-1 w-fit border border-zinc-800">
        {(['sync', 'async', 'batch'] as Mode[]).map((m) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${
              mode === m
                ? 'bg-zinc-700 text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-300'
            }`}
          >
            {m === 'sync' ? 'Synchronous' : m === 'async' ? 'Asynchronous' : 'Batch'}
          </button>
        ))}
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} className="card p-6 space-y-4">
        {mode === 'batch' ? (
          <div>
            <label className="label">URLs (one per line, max 10)</label>
            <textarea
              value={batchUrls}
              onChange={(e) => setBatchUrls(e.target.value)}
              placeholder={"https://example.com/page-1\nhttps://example.com/page-2\nhttps://example.com/page-3"}
              className="input min-h-[120px] resize-y"
              required
            />
          </div>
        ) : (
          <div>
            <label className="label">URL</label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com"
              className="input"
              required
            />
          </div>
        )}

        <div className="flex flex-wrap gap-6">
          <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
            <input
              type="checkbox"
              checked={useBrowser}
              onChange={(e) => setUseBrowser(e.target.checked)}
              className="rounded bg-zinc-800 border-zinc-600 text-emerald-500 focus:ring-emerald-500/50"
            />
            Force browser rendering
          </label>
          <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
            <input
              type="checkbox"
              checked={generateJson}
              onChange={(e) => setGenerateJson(e.target.checked)}
              className="rounded bg-zinc-800 border-zinc-600 text-emerald-500 focus:ring-emerald-500/50"
            />
            Extract structured JSON
          </label>
        </div>

        <div className="flex items-center gap-3 pt-2">
          <button
            type="submit"
            disabled={loading || polling}
            className="btn-primary"
          >
            {loading ? 'Submitting...' : polling ? 'Polling...' : mode === 'sync' ? 'Scrape (wait for result)' : mode === 'async' ? 'Submit & Poll' : 'Submit Batch'}
          </button>
          {mode === 'sync' && (
            <span className="text-xs text-zinc-500">30s timeout</span>
          )}
        </div>
      </form>

      {/* Error */}
      {error && (
        <div className="card p-4 border-red-500/30">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Result */}
      {result && (
        <div className="card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <h2 className="text-lg font-semibold text-zinc-100">Result</h2>
              <StatusBadge status={result.status} />
            </div>
            {result.durationMs && (
              <span className="text-sm text-zinc-500">{result.durationMs}ms</span>
            )}
          </div>

          {result.error && (
            <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
              <p className="text-sm text-red-400">{result.error}</p>
            </div>
          )}

          {result.status === 'completed' && (
            <>
              {/* Tabs */}
              <div className="flex gap-1 border-b border-zinc-800 pb-0">
                {(['markdown', 'html', 'cleaned', 'json'] as const).map((tab) => {
                  const hasContent =
                    tab === 'markdown'
                      ? result.markdown
                      : tab === 'html'
                      ? result.html
                      : tab === 'cleaned'
                      ? result.cleanedHtml
                      : result.generatedJson;
                  if (!hasContent && tab === 'json') return null;
                  return (
                    <button
                      key={tab}
                      onClick={() => setActiveTab(tab)}
                      className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                        activeTab === tab
                          ? 'border-emerald-400 text-emerald-400'
                          : 'border-transparent text-zinc-500 hover:text-zinc-300'
                      }`}
                    >
                      {tab === 'markdown'
                        ? 'Markdown'
                        : tab === 'html'
                        ? 'Raw HTML'
                        : tab === 'cleaned'
                        ? 'Cleaned HTML'
                        : 'JSON'}
                    </button>
                  );
                })}
              </div>

              {/* Content */}
              <div>
                {activeTab === 'markdown' && result.markdown && (
                  <CodeBlock code={result.markdown} language="markdown" maxHeight="500px" />
                )}
                {activeTab === 'html' && result.html && (
                  <CodeBlock code={result.html} language="html" maxHeight="500px" />
                )}
                {activeTab === 'cleaned' && result.cleanedHtml && (
                  <CodeBlock code={result.cleanedHtml} language="html" maxHeight="500px" />
                )}
                {activeTab === 'json' && result.generatedJson && (
                  <CodeBlock
                    code={JSON.stringify(result.generatedJson, null, 2)}
                    language="json"
                    maxHeight="500px"
                  />
                )}
              </div>
            </>
          )}

          <div className="flex gap-4 text-xs text-zinc-500">
            <span>ID: {result.id}</span>
            {result.createdAt && <span>Created: {new Date(result.createdAt).toLocaleString()}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
