import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { ProxyScoresResponse, ProxyScore } from '../types';

export function ProxyScores() {
  const [data, setData] = useState<ProxyScoresResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedHost, setExpandedHost] = useState<string | null>(null);

  const fetchScores = async () => {
    try {
      const result = await api.getProxyScores();
      setData(result);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load proxy scores');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchScores();
    const timer = setInterval(fetchScores, 10000);
    return () => clearInterval(timer);
  }, []);

  const allScores: ProxyScore[] = data?.scores
    ? Object.values(data.scores).flat()
    : [];

  const hosts = data?.scores ? Object.keys(data.scores).sort() : [];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-zinc-500">Loading proxy scores...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-zinc-100">Proxy Scores</h1>
          <p className="text-zinc-500 mt-1">
            Thompson Sampling proxy performance
          </p>
        </div>
        <button onClick={fetchScores} className="btn-secondary text-sm">
          Refresh
        </button>
      </div>

      {error && (
        <div className="card p-4 border-red-500/30">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Summary Stats */}
      {allScores.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <StatCard
            label="Total Proxies"
            value={String(new Set(allScores.map((s) => s.proxyUrl)).size)}
          />
          <StatCard label="Target Hosts" value={String(hosts.length)} />
          <StatCard
            label="Total Requests"
            value={String(allScores.reduce((sum, s) => sum + s.totalRequests, 0))}
          />
          <StatCard
            label="Avg Score"
            value={
              allScores.length > 0
                ? (allScores.reduce((sum, s) => sum + s.score, 0) / allScores.length).toFixed(3)
                : '-'
            }
          />
        </div>
      )}

      {/* No Data */}
      {hosts.length === 0 && !error && (
        <div className="card p-12 text-center">
          <p className="text-zinc-500">
            No proxy scores yet. Configure proxies via <code className="text-zinc-300">PROXY_URLS</code> environment variable and start scraping to see performance data.
          </p>
        </div>
      )}

      {/* Scores by Host */}
      {hosts.map((host) => {
        const scores = data!.scores[host].sort((a, b) => b.score - a.score);
        const isExpanded = expandedHost === host;

        return (
          <div key={host} className="card overflow-hidden">
            <button
              onClick={() => setExpandedHost(isExpanded ? null : host)}
              className="w-full px-5 py-4 flex items-center justify-between hover:bg-zinc-800/50 transition-colors cursor-pointer"
            >
              <div className="flex items-center gap-3">
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  className={`transition-transform ${isExpanded ? 'rotate-90' : ''}`}
                >
                  <path d="M6 4l4 4-4 4" />
                </svg>
                <span className="font-mono text-sm font-medium text-zinc-200">
                  {host}
                </span>
                <span className="text-xs text-zinc-500">
                  {scores.length} {scores.length === 1 ? 'proxy' : 'proxies'}
                </span>
              </div>
              <div className="flex items-center gap-4 text-xs text-zinc-500">
                <span>
                  Best:{' '}
                  <span className="text-emerald-400">
                    {(scores[0]?.score * 100).toFixed(1)}%
                  </span>
                </span>
                <span>
                  Requests:{' '}
                  <span className="text-zinc-300">
                    {scores.reduce((sum, s) => sum + s.totalRequests, 0)}
                  </span>
                </span>
              </div>
            </button>

            {isExpanded && (
              <div className="border-t border-zinc-800">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-xs text-zinc-500 uppercase tracking-wider">
                      <th className="text-left px-5 py-3 font-medium">Proxy</th>
                      <th className="text-right px-5 py-3 font-medium">Score</th>
                      <th className="text-right px-5 py-3 font-medium">Win Rate</th>
                      <th className="text-right px-5 py-3 font-medium">Alpha / Beta</th>
                      <th className="text-right px-5 py-3 font-medium">Requests</th>
                      <th className="text-right px-5 py-3 font-medium">Avg Latency</th>
                      <th className="text-right px-5 py-3 font-medium">Last Updated</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-800">
                    {scores.map((score, i) => (
                      <tr
                        key={`${score.proxyUrl}-${score.targetHost}`}
                        className="hover:bg-zinc-800/30"
                      >
                        <td className="px-5 py-3 font-mono text-zinc-300 truncate max-w-[250px]">
                          {score.proxyUrl}
                        </td>
                        <td className="px-5 py-3 text-right">
                          <ScoreBar score={score.score} rank={i} total={scores.length} />
                        </td>
                        <td className="px-5 py-3 text-right text-zinc-200">
                          {(score.score * 100).toFixed(1)}%
                        </td>
                        <td className="px-5 py-3 text-right text-zinc-400 font-mono">
                          {score.alpha} / {score.beta}
                        </td>
                        <td className="px-5 py-3 text-right text-zinc-300">
                          {score.totalRequests}
                        </td>
                        <td className="px-5 py-3 text-right text-zinc-300">
                          {score.avgLatencyMs}ms
                        </td>
                        <td className="px-5 py-3 text-right text-zinc-500">
                          {new Date(score.lastUpdated).toLocaleString()}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="card p-4">
      <p className="text-xs text-zinc-500 uppercase tracking-wider mb-1">{label}</p>
      <p className="text-2xl font-bold text-zinc-100">{value}</p>
    </div>
  );
}

function ScoreBar({ score, rank, total }: { score: number; rank: number; total: number }) {
  const color =
    rank === 0 && total > 1
      ? 'bg-emerald-400'
      : score >= 0.7
      ? 'bg-emerald-500/60'
      : score >= 0.4
      ? 'bg-yellow-500/60'
      : 'bg-red-500/60';

  return (
    <div className="flex items-center gap-2 justify-end">
      <div className="w-16 h-1.5 bg-zinc-800 rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full ${color}`}
          style={{ width: `${score * 100}%` }}
        />
      </div>
    </div>
  );
}
