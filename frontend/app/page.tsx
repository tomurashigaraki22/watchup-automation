"use client";

import { useEffect, useState } from "react";
import { api, Metrics, ApiError } from "@/lib/api";
import StatCard from "@/components/StatCard";
import StatusBadge from "@/components/StatusBadge";

function pct(n: number): string {
  return `${(n * 100).toFixed(1)}%`;
}

export default function DashboardPage() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .metrics()
      .then(setMetrics)
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load metrics"));
  }, []);

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!metrics) return <p className="text-sm text-slate-400">Loading…</p>;

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">Dashboard</h1>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        <StatCard label="Companies Discovered" value={metrics.companies_discovered} />
        <StatCard label="Companies Crawled" value={metrics.companies_crawled} />
        <StatCard label="Companies Analyzed" value={metrics.companies_analyzed} />
        <StatCard label="Emails Extracted" value={metrics.emails_extracted} />
        <StatCard label="Emails Verified" value={metrics.emails_verified} />
        <StatCard label="Emails Sent" value={metrics.emails_sent} />
        <StatCard label="Replies" value={metrics.replies} />
        <StatCard label="Open Rate" value={pct(metrics.open_rate)} />
        <StatCard label="Bounce Rate" value={pct(metrics.bounce_rate)} />
        <StatCard label="Followups Sent" value={metrics.followups_sent} />
        <StatCard label="Followups Pending" value={metrics.followups_pending} />
      </div>

      <div className="card">
        <h2 className="mb-3 text-sm font-semibold text-slate-700">Campaign Performance</h2>
        {metrics.campaigns.length === 0 ? (
          <p className="text-sm text-slate-400">No campaigns yet.</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-xs uppercase text-slate-500">
                <th className="py-2">Campaign</th>
                <th className="py-2">Status</th>
                <th className="py-2">Sent</th>
                <th className="py-2">Replies</th>
                <th className="py-2">Open Rate</th>
              </tr>
            </thead>
            <tbody>
              {metrics.campaigns.map((c) => (
                <tr key={c.campaign_id} className="border-b border-slate-100 last:border-0">
                  <td className="py-2 font-medium">{c.name}</td>
                  <td className="py-2">
                    <StatusBadge status={c.status} />
                  </td>
                  <td className="py-2">{c.sent}</td>
                  <td className="py-2">{c.replies}</td>
                  <td className="py-2">{pct(c.open_rate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
