"use client";

import { useState } from "react";
import Link from "next/link";
import { api, SearchResults, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

export default function SearchPage() {
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [results, setResults] = useState<SearchResults | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    if (!q && !status) return;
    setError(null);
    try {
      const res = await api.search(q, status);
      setResults(res);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Search failed");
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">Search</h1>

      <form onSubmit={handleSearch} className="card flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Query</label>
          <input
            className="input w-64"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Company, email subject, or campaign name"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Status</label>
          <input className="input w-40" value={status} onChange={(e) => setStatus(e.target.value)} placeholder="e.g. sent" />
        </div>
        <button type="submit" className="btn-primary">
          Search
        </button>
      </form>

      {error && <p className="text-sm text-red-600">{error}</p>}

      {results && (
        <div className="space-y-6">
          <div className="card">
            <h2 className="mb-2 text-sm font-semibold text-slate-700">Companies ({results.companies.length})</h2>
            {results.companies.map((c) => (
              <div key={c.id} className="flex items-center justify-between border-b border-slate-100 py-1.5 last:border-0 text-sm">
                <Link href={`/companies/${c.id}`} className="text-brand-700 hover:underline">
                  {c.name}
                </Link>
                <StatusBadge status={c.status} />
              </div>
            ))}
            {results.companies.length === 0 && <p className="text-sm text-slate-400">No matches.</p>}
          </div>

          <div className="card">
            <h2 className="mb-2 text-sm font-semibold text-slate-700">Emails ({results.emails.length})</h2>
            {results.emails.map((e) => (
              <div key={e.id} className="flex items-center justify-between border-b border-slate-100 py-1.5 last:border-0 text-sm">
                <Link href={`/emails/${e.id}`} className="text-brand-700 hover:underline">
                  {e.subject || "(no subject)"}
                </Link>
                <StatusBadge status={e.status} />
              </div>
            ))}
            {results.emails.length === 0 && <p className="text-sm text-slate-400">No matches.</p>}
          </div>

          <div className="card">
            <h2 className="mb-2 text-sm font-semibold text-slate-700">Campaigns ({results.campaigns.length})</h2>
            {results.campaigns.map((c) => (
              <div key={c.id} className="flex items-center justify-between border-b border-slate-100 py-1.5 last:border-0 text-sm">
                <span>{c.name}</span>
                <StatusBadge status={c.status} />
              </div>
            ))}
            {results.campaigns.length === 0 && <p className="text-sm text-slate-400">No matches.</p>}
          </div>
        </div>
      )}
    </div>
  );
}
