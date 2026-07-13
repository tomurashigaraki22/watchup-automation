"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, CompanyDetail, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

interface Analysis {
  summary?: string;
  industry?: string;
  value_proposition?: string;
  watchup_angle?: string;
}

function AnalysisCard({ raw, model }: { raw: string; model: string }) {
  let parsed: Analysis | null = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    parsed = null;
  }

  return (
    <div className="card">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-700">AI Summary</h2>
        <span className="text-xs text-slate-400">{model}</span>
      </div>
      {parsed ? (
        <dl className="space-y-2 text-sm">
          <div>
            <dt className="text-xs font-medium uppercase text-slate-400">Summary</dt>
            <dd className="text-slate-600">{parsed.summary}</dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase text-slate-400">Value Proposition</dt>
            <dd className="text-slate-600">{parsed.value_proposition}</dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase text-slate-400">WatchUp Angle</dt>
            <dd className="text-slate-600">{parsed.watchup_angle}</dd>
          </div>
        </dl>
      ) : (
        <pre className="whitespace-pre-wrap text-xs text-slate-600">{raw}</pre>
      )}
    </div>
  );
}

export default function CompanyDetailPage() {
  const params = useParams();
  const id = Number(params.id);
  const [company, setCompany] = useState<CompanyDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .getCompany(id)
      .then(setCompany)
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load company"));
  }, [id]);

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!company) return <p className="text-sm text-slate-400">Loading…</p>;

  const analysis = company.ai_generations.filter((g) => g.kind === "analysis");

  return (
    <div className="space-y-6">
      <div>
        <Link href="/companies" className="text-sm text-brand-700 hover:underline">
          ← Companies
        </Link>
        <div className="mt-1 flex items-center gap-3">
          <h1 className="text-xl font-semibold">{company.name}</h1>
          <StatusBadge status={company.status} />
        </div>
        <a href={company.website} target="_blank" rel="noreferrer" className="text-sm text-slate-500 hover:underline">
          {company.website}
        </a>
      </div>

      <div className="card">
        <h2 className="mb-2 text-sm font-semibold text-slate-700">Description</h2>
        <p className="text-sm text-slate-600">{company.description || "No description yet."}</p>
        {company.industry && <p className="mt-2 text-xs text-slate-400">Industry: {company.industry}</p>}
      </div>

      <div className="card">
        <h2 className="mb-2 text-sm font-semibold text-slate-700">Contacts / Extracted Emails</h2>
        {company.contacts.length === 0 ? (
          <p className="text-sm text-slate-400">No contacts extracted yet.</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-xs uppercase text-slate-500">
                <th className="py-1.5">Email</th>
                <th className="py-1.5">Source</th>
                <th className="py-1.5">Priority</th>
                <th className="py-1.5">Verified</th>
              </tr>
            </thead>
            <tbody>
              {company.contacts.map((c) => (
                <tr key={c.id} className="border-b border-slate-100 last:border-0">
                  <td className="py-1.5">{c.email}</td>
                  <td className="py-1.5 text-slate-500">{c.source}</td>
                  <td className="py-1.5 text-slate-500">{c.priority}</td>
                  <td className="py-1.5">
                    {c.verified ? (
                      <span className="badge bg-emerald-100 text-emerald-700">verified ({c.verification_score})</span>
                    ) : (
                      <span className="badge bg-slate-100 text-slate-500">unverified</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {analysis.length > 0 && <AnalysisCard raw={analysis[0].response} model={analysis[0].model} />}

      <div className="card">
        <h2 className="mb-2 text-sm font-semibold text-slate-700">Email History</h2>
        {company.emails.length === 0 ? (
          <p className="text-sm text-slate-400">No emails generated yet.</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-xs uppercase text-slate-500">
                <th className="py-1.5">Subject</th>
                <th className="py-1.5">Status</th>
                <th className="py-1.5">Replied</th>
                <th className="py-1.5">Sent</th>
              </tr>
            </thead>
            <tbody>
              {company.emails.map((e) => (
                <tr key={e.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50">
                  <td className="py-1.5">
                    <Link href={`/emails/${e.id}`} className="text-brand-700 hover:underline">
                      {e.subject || "(no subject)"}
                    </Link>
                  </td>
                  <td className="py-1.5">
                    <StatusBadge status={e.status} />
                  </td>
                  <td className="py-1.5">{e.replied ? "Yes" : "No"}</td>
                  <td className="py-1.5 text-slate-500">{e.sent_at ? new Date(e.sent_at).toLocaleString() : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
