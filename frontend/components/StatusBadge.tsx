const COLORS: Record<string, string> = {
  discovered: "bg-slate-100 text-slate-700",
  crawled: "bg-sky-100 text-sky-700",
  validated: "bg-cyan-100 text-cyan-700",
  analyzed: "bg-indigo-100 text-indigo-700",
  queued: "bg-amber-100 text-amber-700",
  contacted: "bg-emerald-100 text-emerald-700",
  failed: "bg-red-100 text-red-700",
  draft: "bg-slate-100 text-slate-700",
  sent: "bg-emerald-100 text-emerald-700",
  bounced: "bg-red-100 text-red-700",
  active: "bg-emerald-100 text-emerald-700",
  paused: "bg-amber-100 text-amber-700",
  deleted: "bg-red-100 text-red-700",
  manual: "bg-slate-100 text-slate-700",
  automatic: "bg-indigo-100 text-indigo-700",
};

export default function StatusBadge({ status }: { status: string }) {
  const cls = COLORS[status] || "bg-slate-100 text-slate-700";
  return <span className={`badge ${cls}`}>{status}</span>;
}
