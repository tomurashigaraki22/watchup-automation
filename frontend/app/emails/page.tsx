"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Email, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

const STATUSES = ["", "draft", "queued", "sent", "failed", "bounced"];

export default function EmailsPage() {
  const [emails, setEmails] = useState<Email[]>([]);
  const [status, setStatus] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listEmails({ status: status || undefined })
      .then((res) => setEmails(res.emails))
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load emails"));
  }, [status]);

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">Emails</h1>

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
              <th className="py-2">Subject</th>
              <th className="py-2">Status</th>
              <th className="py-2">Opened</th>
              <th className="py-2">Replied</th>
              <th className="py-2">Sent</th>
            </tr>
          </thead>
          <tbody>
            {emails.map((e) => (
              <tr key={e.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50">
                <td className="py-2">
                  <Link href={`/emails/${e.id}`} className="font-medium text-brand-700 hover:underline">
                    {e.subject || "(no subject)"}
                  </Link>
                </td>
                <td className="py-2">
                  <StatusBadge status={e.status} />
                </td>
                <td className="py-2">{e.opened ? "Yes" : "No"}</td>
                <td className="py-2">{e.replied ? "Yes" : "No"}</td>
                <td className="py-2 text-slate-500">{e.sent_at ? new Date(e.sent_at).toLocaleString() : "—"}</td>
              </tr>
            ))}
            {emails.length === 0 && (
              <tr>
                <td colSpan={5} className="py-6 text-center text-slate-400">
                  No emails yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
