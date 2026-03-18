const statusStyles: Record<string, string> = {
  pending: 'bg-yellow-500/15 text-yellow-400 border-yellow-500/30',
  processing: 'bg-blue-500/15 text-blue-400 border-blue-500/30',
  completed: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30',
  failed: 'bg-red-500/15 text-red-400 border-red-500/30',
};

export function StatusBadge({ status }: { status: string }) {
  const style = statusStyles[status] || 'bg-zinc-500/15 text-zinc-400 border-zinc-500/30';
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-medium rounded-full border ${style}`}
    >
      <span
        className={`w-1.5 h-1.5 rounded-full ${
          status === 'pending'
            ? 'bg-yellow-400'
            : status === 'processing'
            ? 'bg-blue-400 animate-pulse'
            : status === 'completed'
            ? 'bg-emerald-400'
            : 'bg-red-400'
        }`}
      />
      {status}
    </span>
  );
}
