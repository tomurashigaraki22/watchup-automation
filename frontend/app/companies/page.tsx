"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { api, Company, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

const STATUSES = ["", "discovered", "crawled", "validated", "analyzed", "failed"];

export default function CompaniesPage() {
  const [companies, setCompanies] = useState<Company[]>([]);
  const [status, setStatus] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [importMsg, setImportMsg] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  function load() {
    api
      .listCompanies(status || undefined)
      .then((res) => setCompanies(res.companies))
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load companies"));
  }

  useEffect(load, [status]);

  async function handleImport() {
    const file = fileRef.current?.files?.[0];
    if (!file) return;
    setImportMsg("Importing…");
    try {
      const result = await api.importCompaniesCSV(file);
      setImportMsg(`Imported: ${result.inserted} inserted, ${result.skipped} skipped, ${result.errors} errors.`);
      if (fileRef.current) fileRef.current.value = "";
      load();
    } catch (err) {
      setImportMsg(err instanceof ApiError ? `Import failed: ${err.message}` : "Import failed.");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Companies</h1>
        <div className="flex items-center gap-2">
          <input ref={fileRef} type="file" accept=".csv" className="text-xs" />
          <button className="btn-secondary" onClick={handleImport}>
            Import CSV
          </button>
        </div>
      </div>
      {importMsg && <p className="text-sm text-slate-500">{importMsg}</p>}

      <div className="flex gap-2">
        {STATUSES.map((s) => (
          <button
            key={s || "all"}
            onClick={() => setStatus(s)}
            className={`btn-secondary ${status === s ? "ring-1 ring-brand-500" : ""}`}
          >
            {s || "All"}
          </button>
        ))}
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="card overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-xs uppercase text-slate-500">
              <th className="py-2">Name</th>
              <th className="py-2">Website</th>
              <th className="py-2">Industry</th>
              <th className="py-2">Status</th>
            </tr>
          </thead>
          <tbody>
            {companies.map((c) => (
              <tr key={c.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50">
                <td className="py-2">
                  <Link href={`/companies/${c.id}`} className="font-medium text-brand-700 hover:underline">
                    {c.name}
                  </Link>
                </td>
                <td className="py-2 text-slate-500">{c.website}</td>
                <td className="py-2 text-slate-500">{c.industry || "—"}</td>
                <td className="py-2">
                  <StatusBadge status={c.status} />
                </td>
              </tr>
            ))}
            {companies.length === 0 && (
              <tr>
                <td colSpan={4} className="py-6 text-center text-slate-400">
                  No companies yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
