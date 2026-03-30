import { useState, useEffect } from 'react';
import { useParams, useSearchParams, Link } from 'react-router-dom';
import { api } from '../api/client';
import { StatusBadge } from '../components/StatusBadge';
import { CodeBlock } from '../components/CodeBlock';
import type { JobResponse, BatchJobResponse } from '../types';

export function JobDetail() {
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const isBatch = searchParams.get('type') === 'batch';

  const [job, setJob] = useState<JobResponse | null>(null);
  const [batchJob, setBatchJob] = useState<BatchJobResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'markdown' | 'html' | 'cleaned' | 'json'>('markdown');
  const [selectedBatchIndex, setSelectedBatchIndex] = useState(0);

  useEffect(() => {
    if (!id) return;

    const fetchJob = async () => {
      try {
        if (isBatch) {
          const result = await api.getBatchJob(id);
          setBatchJob(result);
        } else {
          const result = await api.getJob(id);
          setJob(result);
        }
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : 'Failed to load job');
      } finally {
        setLoading(false);
      }
    };

    fetchJob();

    // Poll if still active
    const timer = setInterval(async () => {
      try {
        if (isBatch) {
          const result = await api.getBatchJob(id);
          setBatchJob(result);
          if (result.status === 'completed' || result.status === 'failed') {
            clearInterval(timer);
          }
        } else {
          const result = await api.getJob(id);
          setJob(result);
          if (result.status === 'completed' || result.status === 'failed') {
            clearInterval(timer);
          }
        }
      } catch {
        // keep polling
      }
    }, 2000);

    return () => clearInterval(timer);
  }, [id, isBatch]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-zinc-500">Loading...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <Link to="/jobs" className="text-sm text-zinc-500 hover:text-zinc-300">
          &larr; Back to Jobs
        </Link>
        <div className="card p-6 border-red-500/30">
          <p className="text-red-400">{error}</p>
        </div>
      </div>
    );
  }

  // Batch job view
  if (isBatch && batchJob) {
    const selectedResult = batchJob.results?.[selectedBatchIndex];
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Link to="/jobs" className="text-sm text-zinc-500 hover:text-zinc-300">
            &larr; Back
          </Link>
          <h1 className="text-2xl font-bold text-zinc-100">Batch Job</h1>
          <StatusBadge status={batchJob.status} />
        </div>

        <div className="grid grid-cols-3 gap-4">
          <InfoCard label="Job ID" value={batchJob.id} mono />
          <InfoCard label="URLs" value={`${batchJob.urls?.length || 0} URLs`} />
          <InfoCard
            label="Duration"
            value={batchJob.durationMs ? `${batchJob.durationMs}ms` : '-'}
          />
        </div>

        {/* URL selector */}
        {batchJob.results && batchJob.results.length > 0 && (
          <>
            <div className="space-y-2">
              <label className="label">Select URL</label>
              <div className="grid gap-2">
                {batchJob.results.map((r, i) => (
                  <button
                    key={i}
                    onClick={() => setSelectedBatchIndex(i)}
                    className={`card p-3 text-left flex items-center gap-3 transition-colors ${
                      selectedBatchIndex === i
                        ? 'border-emerald-500/50 bg-emerald-500/5'
                        : 'hover:border-zinc-700'
                    }`}
                  >
                    <StatusBadge status={r.status} />
                    <span className="text-sm text-zinc-300 truncate font-mono flex-1">
                      {r.url}
                    </span>
                    {r.durationMs && (
                      <span className="text-xs text-zinc-500">{r.durationMs}ms</span>
                    )}
                  </button>
                ))}
              </div>
            </div>

            {selectedResult && selectedResult.status === 'completed' && (
              <ResultTabs
                markdown={selectedResult.markdown}
                html={selectedResult.html}
                cleanedHtml={selectedResult.cleanedHtml}
                generatedJson={selectedResult.generatedJson}
                activeTab={activeTab}
                onTabChange={setActiveTab}
              />
            )}

            {selectedResult && selectedResult.error && (
              <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
                <p className="text-sm text-red-400">{selectedResult.error}</p>
              </div>
            )}
          </>
        )}
      </div>
    );
  }

  // Single job view
  if (job) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Link to="/jobs" className="text-sm text-zinc-500 hover:text-zinc-300">
            &larr; Back
          </Link>
          <h1 className="text-2xl font-bold text-zinc-100">Job Detail</h1>
          <StatusBadge status={job.status} />
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <InfoCard label="Job ID" value={job.id} mono />
          <InfoCard label="URL" value={job.url || '-'} mono />
          <InfoCard
            label="Duration"
            value={job.durationMs ? `${job.durationMs}ms` : '-'}
          />
          <InfoCard
            label="Created"
            value={job.createdAt ? new Date(job.createdAt).toLocaleString() : '-'}
          />
        </div>

        {job.error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
            <p className="text-sm text-red-400">{job.error}</p>
          </div>
        )}

        {job.status === 'completed' && (
          <ResultTabs
            markdown={job.markdown}
            html={job.html}
            cleanedHtml={job.cleanedHtml}
            generatedJson={job.generatedJson}
            activeTab={activeTab}
            onTabChange={setActiveTab}
          />
        )}
      </div>
    );
  }

  return null;
}

function InfoCard({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="card p-3">
      <p className="text-xs text-zinc-500 mb-1">{label}</p>
      <p className={`text-sm text-zinc-200 truncate ${mono ? 'font-mono' : ''}`}>
        {value}
      </p>
    </div>
  );
}

function ResultTabs({
  markdown,
  html,
  cleanedHtml,
  generatedJson,
  activeTab,
  onTabChange,
}: {
  markdown?: string | null;
  html?: string | null;
  cleanedHtml?: string | null;
  generatedJson?: { status: string; data?: unknown } | null;
  activeTab: 'markdown' | 'html' | 'cleaned' | 'json';
  onTabChange: (tab: 'markdown' | 'html' | 'cleaned' | 'json') => void;
}) {
  const tabs = [
    { key: 'markdown' as const, label: 'Markdown', hasContent: !!markdown },
    { key: 'html' as const, label: 'Raw HTML', hasContent: !!html },
    { key: 'cleaned' as const, label: 'Cleaned HTML', hasContent: !!cleanedHtml },
    { key: 'json' as const, label: 'JSON', hasContent: !!generatedJson },
  ].filter((t) => t.hasContent);

  return (
    <div className="space-y-4">
      <div className="flex gap-1 border-b border-zinc-800">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => onTabChange(tab.key)}
            className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
              activeTab === tab.key
                ? 'border-emerald-400 text-emerald-400'
                : 'border-transparent text-zinc-500 hover:text-zinc-300'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'markdown' && markdown && (
        <CodeBlock code={markdown} language="markdown" maxHeight="600px" />
      )}
      {activeTab === 'html' && html && (
        <CodeBlock code={html} language="html" maxHeight="600px" />
      )}
      {activeTab === 'cleaned' && cleanedHtml && (
        <CodeBlock code={cleanedHtml} language="html" maxHeight="600px" />
      )}
      {activeTab === 'json' && generatedJson && (
        <CodeBlock
          code={JSON.stringify(generatedJson, null, 2)}
          language="json"
          maxHeight="600px"
        />
      )}
    </div>
  );
}
