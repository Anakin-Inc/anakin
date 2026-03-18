import { useState, useEffect, useCallback } from 'react';
import { Link } from 'react-router-dom';
import { StatusBadge } from '../components/StatusBadge';
import { api } from '../api/client';
import type { TrackedJob } from '../types';

export function Jobs() {
  const [jobs, setJobs] = useState<TrackedJob[]>([]);
  const [filter, setFilter] = useState<string>('all');

  const loadJobs = useCallback(() => {
    const raw = localStorage.getItem('anakinscraper_jobs');
    if (raw) {
      setJobs(JSON.parse(raw));
    }
  }, []);

  useEffect(() => {
    loadJobs();
  }, [loadJobs]);

  // Refresh active job statuses
  useEffect(() => {
    const activeJobs = jobs.filter(
      (j) => j.status === 'pending' || j.status === 'processing'
    );
    if (activeJobs.length === 0) return;

    const timer = setInterval(async () => {
      const updated = [...jobs];
      let changed = false;

      for (const job of activeJobs) {
        try {
          if (job.type === 'batch') {
            const result = await api.getBatchJob(job.id);
            const idx = updated.findIndex((j) => j.id === job.id);
            if (idx !== -1 && updated[idx].status !== result.status) {
              updated[idx] = { ...updated[idx], status: result.status };
              changed = true;
            }
          } else {
            const result = await api.getJob(job.id);
            const idx = updated.findIndex((j) => j.id === job.id);
            if (idx !== -1 && updated[idx].status !== result.status) {
              updated[idx] = { ...updated[idx], status: result.status };
              changed = true;
            }
          }
        } catch {
          // skip
        }
      }

      if (changed) {
        setJobs(updated);
        localStorage.setItem('anakinscraper_jobs', JSON.stringify(updated));
      }
    }, 2000);

    return () => clearInterval(timer);
  }, [jobs]);

  const removeJob = (id: string) => {
    const updated = jobs.filter((j) => j.id !== id);
    setJobs(updated);
    localStorage.setItem('anakinscraper_jobs', JSON.stringify(updated));
  };

  const clearAll = () => {
    setJobs([]);
    localStorage.removeItem('anakinscraper_jobs');
  };

  const filtered =
    filter === 'all' ? jobs : jobs.filter((j) => j.status === filter);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-zinc-100">Jobs</h1>
          <p className="text-zinc-500 mt-1">
            Track your scraping jobs ({jobs.length} total)
          </p>
        </div>
        {jobs.length > 0 && (
          <button onClick={clearAll} className="btn-danger text-sm">
            Clear All
          </button>
        )}
      </div>

      {/* Filters */}
      <div className="flex gap-2">
        {['all', 'pending', 'processing', 'completed', 'failed'].map((f) => {
          const count =
            f === 'all' ? jobs.length : jobs.filter((j) => j.status === f).length;
          return (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                filter === f
                  ? 'bg-zinc-700 text-zinc-100'
                  : 'text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800/50'
              }`}
            >
              {f.charAt(0).toUpperCase() + f.slice(1)}
              {count > 0 && (
                <span className="ml-1.5 text-xs text-zinc-500">({count})</span>
              )}
            </button>
          );
        })}
      </div>

      {/* Job List */}
      {filtered.length === 0 ? (
        <div className="card p-12 text-center">
          <p className="text-zinc-500">
            {jobs.length === 0
              ? 'No jobs yet. Submit a scrape request to get started.'
              : 'No jobs match the current filter.'}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map((job) => (
            <div
              key={job.id}
              className="card p-4 flex items-center gap-4 hover:border-zinc-700 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-3 mb-1">
                  <StatusBadge status={job.status} />
                  <span className="text-xs text-zinc-500 font-mono">
                    {job.type === 'batch' ? 'BATCH' : 'SINGLE'}
                  </span>
                </div>
                <p className="text-sm text-zinc-300 truncate font-mono">
                  {job.url}
                  {job.urls && job.urls.length > 1 && (
                    <span className="text-zinc-500">
                      {' '}
                      +{job.urls.length - 1} more
                    </span>
                  )}
                </p>
                <p className="text-xs text-zinc-600 mt-1">
                  {job.id} &middot;{' '}
                  {new Date(job.createdAt).toLocaleString()}
                </p>
              </div>

              <div className="flex items-center gap-2 shrink-0">
                <Link
                  to={`/jobs/${job.id}${job.type === 'batch' ? '?type=batch' : ''}`}
                  className="btn-secondary text-sm py-1.5"
                >
                  View
                </Link>
                <button
                  onClick={() => removeJob(job.id)}
                  className="p-1.5 text-zinc-600 hover:text-zinc-400 transition-colors"
                  title="Remove"
                >
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M4 4l8 8M12 4l-8 8" />
                  </svg>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
