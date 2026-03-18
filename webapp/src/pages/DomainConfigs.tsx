import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { DomainConfig } from '../types';

type FormData = {
  domain: string;
  isEnabled: boolean;
  matchSubdomains: boolean;
  priority: number;
  handlerChain: string[];
  requestTimeoutMs: number;
  maxRetries: number;
  minContentLength: number;
  failurePatterns: string;
  requiredPatterns: string;
  customHeaders: string;
  customUserAgent: string;
  proxyUrl: string;
  blocked: boolean;
  blockedReason: string;
  notes: string;
};

const emptyForm: FormData = {
  domain: '',
  isEnabled: true,
  matchSubdomains: false,
  priority: 0,
  handlerChain: ['http', 'browser'],
  requestTimeoutMs: 30000,
  maxRetries: 2,
  minContentLength: 0,
  failurePatterns: '',
  requiredPatterns: '',
  customHeaders: '{}',
  customUserAgent: '',
  proxyUrl: '',
  blocked: false,
  blockedReason: '',
  notes: '',
};

export function DomainConfigs() {
  const [configs, setConfigs] = useState<DomainConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingDomain, setEditingDomain] = useState<string | null>(null);
  const [form, setForm] = useState<FormData>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const fetchConfigs = async () => {
    try {
      const data = await api.getDomainConfigs();
      setConfigs(data || []);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load configs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchConfigs();
  }, []);

  const openCreate = () => {
    setForm(emptyForm);
    setEditingDomain(null);
    setFormError(null);
    setShowForm(true);
  };

  const openEdit = (config: DomainConfig) => {
    setForm({
      domain: config.domain,
      isEnabled: config.isEnabled,
      matchSubdomains: config.matchSubdomains,
      priority: config.priority,
      handlerChain: config.handlerChain || ['http', 'browser'],
      requestTimeoutMs: config.requestTimeoutMs,
      maxRetries: config.maxRetries,
      minContentLength: config.minContentLength,
      failurePatterns: (config.failurePatterns || []).join('\n'),
      requiredPatterns: (config.requiredPatterns || []).join('\n'),
      customHeaders: JSON.stringify(config.customHeaders || {}, null, 2),
      customUserAgent: config.customUserAgent || '',
      proxyUrl: config.proxyUrl || '',
      blocked: config.blocked,
      blockedReason: config.blockedReason || '',
      notes: config.notes || '',
    });
    setEditingDomain(config.domain);
    setFormError(null);
    setShowForm(true);
  };

  const handleSave = async () => {
    setSaving(true);
    setFormError(null);

    try {
      let headers: Record<string, string> = {};
      try {
        headers = JSON.parse(form.customHeaders || '{}');
      } catch {
        throw new Error('Custom headers must be valid JSON');
      }

      const payload: Partial<DomainConfig> = {
        domain: form.domain.trim(),
        isEnabled: form.isEnabled,
        matchSubdomains: form.matchSubdomains,
        priority: form.priority,
        handlerChain: form.handlerChain,
        requestTimeoutMs: form.requestTimeoutMs,
        maxRetries: form.maxRetries,
        minContentLength: form.minContentLength,
        failurePatterns: form.failurePatterns.split('\n').map(s => s.trim()).filter(Boolean),
        requiredPatterns: form.requiredPatterns.split('\n').map(s => s.trim()).filter(Boolean),
        customHeaders: headers,
        customUserAgent: form.customUserAgent,
        proxyUrl: form.proxyUrl,
        blocked: form.blocked,
        blockedReason: form.blockedReason,
        notes: form.notes,
      };

      if (editingDomain) {
        await api.updateDomainConfig(editingDomain, payload);
      } else {
        await api.createDomainConfig(payload);
      }

      setShowForm(false);
      await fetchConfigs();
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (domain: string) => {
    if (!confirm(`Delete config for ${domain}?`)) return;
    try {
      await api.deleteDomainConfig(domain);
      await fetchConfigs();
    } catch (err: unknown) {
      alert(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const toggleHandler = (handler: string) => {
    setForm(prev => ({
      ...prev,
      handlerChain: prev.handlerChain.includes(handler)
        ? prev.handlerChain.filter(h => h !== handler)
        : [...prev.handlerChain, handler],
    }));
  };

  const moveHandler = (index: number, direction: -1 | 1) => {
    const chain = [...form.handlerChain];
    const newIndex = index + direction;
    if (newIndex < 0 || newIndex >= chain.length) return;
    [chain[index], chain[newIndex]] = [chain[newIndex], chain[index]];
    setForm(prev => ({ ...prev, handlerChain: chain }));
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-zinc-500">Loading domain configs...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-zinc-100">Domain Configs</h1>
          <p className="text-zinc-500 mt-1">
            Per-domain scraping strategies ({configs.length} configs)
          </p>
        </div>
        <button onClick={openCreate} className="btn-primary">
          Add Config
        </button>
      </div>

      {error && (
        <div className="card p-4 border-red-500/30">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Config List */}
      {configs.length === 0 ? (
        <div className="card p-12 text-center">
          <p className="text-zinc-500">
            No domain configs yet. Add one to customize scraping behavior for specific domains.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {configs.map((config) => (
            <div
              key={config.id}
              className="card p-5 hover:border-zinc-700 transition-colors cursor-pointer"
              onClick={() => openEdit(config)}
            >
              <div className="flex items-start justify-between gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3 mb-2">
                    <h3 className="text-base font-semibold text-zinc-100 font-mono">
                      {config.domain}
                    </h3>
                    <span
                      className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-full border ${
                        config.isEnabled
                          ? 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30'
                          : 'bg-zinc-500/15 text-zinc-400 border-zinc-500/30'
                      }`}
                    >
                      {config.isEnabled ? 'Enabled' : 'Disabled'}
                    </span>
                    {config.blocked && (
                      <span className="inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-full border bg-red-500/15 text-red-400 border-red-500/30">
                        Blocked
                      </span>
                    )}
                    {config.matchSubdomains && (
                      <span className="text-xs text-zinc-500">+subdomains</span>
                    )}
                  </div>

                  <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-zinc-500">
                    <span>
                      Chain:{' '}
                      <span className="text-zinc-300">
                        {(config.handlerChain || []).join(' → ')}
                      </span>
                    </span>
                    <span>
                      Timeout:{' '}
                      <span className="text-zinc-300">{config.requestTimeoutMs}ms</span>
                    </span>
                    <span>
                      Retries:{' '}
                      <span className="text-zinc-300">{config.maxRetries}</span>
                    </span>
                    {config.proxyUrl && (
                      <span>
                        Proxy:{' '}
                        <span className="text-zinc-300 truncate max-w-[200px] inline-block align-bottom">
                          {config.proxyUrl}
                        </span>
                      </span>
                    )}
                  </div>

                  {config.notes && (
                    <p className="text-xs text-zinc-600 mt-2 truncate">{config.notes}</p>
                  )}
                </div>

                <div className="flex items-center gap-2 shrink-0">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      openEdit(config);
                    }}
                    className="btn-secondary text-sm py-1.5"
                  >
                    Edit
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(config.domain);
                    }}
                    className="p-1.5 text-zinc-600 hover:text-red-400 transition-colors"
                    title="Delete"
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                      <path d="M3 4h10M5.5 4V3a1 1 0 011-1h3a1 1 0 011 1v1M6.5 7v4M9.5 7v4M4.5 4l.5 8a1 1 0 001 1h4a1 1 0 001-1l.5-8" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Form Modal */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-16 px-4">
          <div
            className="fixed inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setShowForm(false)}
          />
          <div className="relative bg-zinc-900 border border-zinc-800 rounded-xl w-full max-w-2xl max-h-[80vh] overflow-y-auto shadow-2xl">
            <div className="sticky top-0 bg-zinc-900 border-b border-zinc-800 px-6 py-4 flex items-center justify-between z-10">
              <h2 className="text-lg font-semibold text-zinc-100">
                {editingDomain ? `Edit: ${editingDomain}` : 'New Domain Config'}
              </h2>
              <button
                onClick={() => setShowForm(false)}
                className="text-zinc-500 hover:text-zinc-300"
              >
                <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <path d="M5 5l10 10M15 5l-10 10" />
                </svg>
              </button>
            </div>

            <div className="p-6 space-y-5">
              {formError && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
                  <p className="text-sm text-red-400">{formError}</p>
                </div>
              )}

              {/* Domain */}
              <div>
                <label className="label">Domain</label>
                <input
                  type="text"
                  value={form.domain}
                  onChange={(e) => setForm(prev => ({ ...prev, domain: e.target.value }))}
                  placeholder="example.com"
                  className="input font-mono"
                  disabled={!!editingDomain}
                />
              </div>

              {/* Toggles Row */}
              <div className="flex flex-wrap gap-6">
                <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.isEnabled}
                    onChange={(e) => setForm(prev => ({ ...prev, isEnabled: e.target.checked }))}
                    className="rounded bg-zinc-800 border-zinc-600"
                  />
                  Enabled
                </label>
                <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.matchSubdomains}
                    onChange={(e) => setForm(prev => ({ ...prev, matchSubdomains: e.target.checked }))}
                    className="rounded bg-zinc-800 border-zinc-600"
                  />
                  Match Subdomains
                </label>
                <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.blocked}
                    onChange={(e) => setForm(prev => ({ ...prev, blocked: e.target.checked }))}
                    className="rounded bg-zinc-800 border-zinc-600"
                  />
                  Blocked
                </label>
              </div>

              {/* Handler Chain */}
              <div>
                <label className="label">Handler Chain (order matters)</label>
                <div className="space-y-2">
                  {form.handlerChain.map((handler, i) => (
                    <div key={handler} className="flex items-center gap-2">
                      <span className="bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-1.5 text-sm font-mono text-zinc-200 flex-1">
                        {handler}
                      </span>
                      <button
                        type="button"
                        onClick={() => moveHandler(i, -1)}
                        disabled={i === 0}
                        className="p-1 text-zinc-500 hover:text-zinc-300 disabled:opacity-30"
                        title="Move up"
                      >
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M4 10l4-4 4 4"/></svg>
                      </button>
                      <button
                        type="button"
                        onClick={() => moveHandler(i, 1)}
                        disabled={i === form.handlerChain.length - 1}
                        className="p-1 text-zinc-500 hover:text-zinc-300 disabled:opacity-30"
                        title="Move down"
                      >
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M4 6l4 4 4-4"/></svg>
                      </button>
                      <button
                        type="button"
                        onClick={() => toggleHandler(handler)}
                        className="p-1 text-zinc-500 hover:text-red-400"
                        title="Remove"
                      >
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M4 4l8 8M12 4l-8 8"/></svg>
                      </button>
                    </div>
                  ))}
                  <div className="flex gap-2 mt-1">
                    {['http', 'browser'].filter(h => !form.handlerChain.includes(h)).map(h => (
                      <button
                        key={h}
                        type="button"
                        onClick={() => toggleHandler(h)}
                        className="text-xs text-emerald-400 hover:text-emerald-300 bg-emerald-500/10 px-2 py-1 rounded"
                      >
                        + {h}
                      </button>
                    ))}
                  </div>
                </div>
              </div>

              {/* Numeric Fields */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="label">Timeout (ms)</label>
                  <input
                    type="number"
                    value={form.requestTimeoutMs}
                    onChange={(e) => setForm(prev => ({ ...prev, requestTimeoutMs: parseInt(e.target.value) || 0 }))}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label">Max Retries</label>
                  <input
                    type="number"
                    value={form.maxRetries}
                    onChange={(e) => setForm(prev => ({ ...prev, maxRetries: parseInt(e.target.value) || 0 }))}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label">Priority</label>
                  <input
                    type="number"
                    value={form.priority}
                    onChange={(e) => setForm(prev => ({ ...prev, priority: parseInt(e.target.value) || 0 }))}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label">Min Content Length</label>
                  <input
                    type="number"
                    value={form.minContentLength}
                    onChange={(e) => setForm(prev => ({ ...prev, minContentLength: parseInt(e.target.value) || 0 }))}
                    className="input"
                  />
                </div>
              </div>

              {/* Text Fields */}
              <div>
                <label className="label">Custom User Agent</label>
                <input
                  type="text"
                  value={form.customUserAgent}
                  onChange={(e) => setForm(prev => ({ ...prev, customUserAgent: e.target.value }))}
                  placeholder="Mozilla/5.0 ..."
                  className="input font-mono text-sm"
                />
              </div>

              <div>
                <label className="label">Proxy URL</label>
                <input
                  type="text"
                  value={form.proxyUrl}
                  onChange={(e) => setForm(prev => ({ ...prev, proxyUrl: e.target.value }))}
                  placeholder="http://proxy:8080"
                  className="input font-mono text-sm"
                />
              </div>

              <div>
                <label className="label">Failure Patterns (one regex per line)</label>
                <textarea
                  value={form.failurePatterns}
                  onChange={(e) => setForm(prev => ({ ...prev, failurePatterns: e.target.value }))}
                  placeholder="captcha|blocked|access denied"
                  className="input min-h-[80px] resize-y font-mono text-sm"
                />
              </div>

              <div>
                <label className="label">Required Patterns (one regex per line)</label>
                <textarea
                  value={form.requiredPatterns}
                  onChange={(e) => setForm(prev => ({ ...prev, requiredPatterns: e.target.value }))}
                  placeholder="<article|<main|<div class=.content."
                  className="input min-h-[80px] resize-y font-mono text-sm"
                />
              </div>

              <div>
                <label className="label">Custom Headers (JSON)</label>
                <textarea
                  value={form.customHeaders}
                  onChange={(e) => setForm(prev => ({ ...prev, customHeaders: e.target.value }))}
                  className="input min-h-[80px] resize-y font-mono text-sm"
                />
              </div>

              {form.blocked && (
                <div>
                  <label className="label">Blocked Reason</label>
                  <input
                    type="text"
                    value={form.blockedReason}
                    onChange={(e) => setForm(prev => ({ ...prev, blockedReason: e.target.value }))}
                    className="input"
                  />
                </div>
              )}

              <div>
                <label className="label">Notes</label>
                <textarea
                  value={form.notes}
                  onChange={(e) => setForm(prev => ({ ...prev, notes: e.target.value }))}
                  placeholder="Any notes about this domain..."
                  className="input min-h-[60px] resize-y"
                />
              </div>

              {/* Actions */}
              <div className="flex justify-end gap-3 pt-2 border-t border-zinc-800">
                <button
                  type="button"
                  onClick={() => setShowForm(false)}
                  className="btn-secondary"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving || !form.domain.trim()}
                  className="btn-primary"
                >
                  {saving ? 'Saving...' : editingDomain ? 'Update' : 'Create'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
