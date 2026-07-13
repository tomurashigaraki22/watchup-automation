"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, EmailPreview, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

export default function EmailPreviewPage() {
  const params = useParams();
  const id = Number(params.id);
  const [email, setEmail] = useState<EmailPreview | null>(null);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [view, setView] = useState<"html" | "text">("html");
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  function load() {
    api
      .getEmail(id)
      .then((e) => {
        setEmail(e);
        setSubject(e.subject);
        setBody(e.body);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load email"));
  }

  useEffect(load, [id]);

  async function handleSave() {
    setSaving(true);
    setMessage(null);
    try {
      await api.updateEmail(id, { subject, body });
      setMessage("Saved.");
      load();
    } catch (err) {
      setMessage(err instanceof ApiError ? `Save failed: ${err.message}` : "Save failed.");
    } finally {
      setSaving(false);
    }
  }

  async function handleSend() {
    setSaving(true);
    setMessage(null);
    try {
      await api.sendEmail(id);
      setMessage("Queued to send.");
      load();
    } catch (err) {
      setMessage(err instanceof ApiError ? `Send failed: ${err.message}` : "Send failed.");
    } finally {
      setSaving(false);
    }
  }

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!email) return <p className="text-sm text-slate-400">Loading…</p>;

  const isDraft = email.status === "draft";

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <Link href="/emails" className="text-sm text-brand-700 hover:underline">
          ← Emails
        </Link>
        <div className="mt-1 flex items-center gap-3">
          <h1 className="text-xl font-semibold">Email Preview</h1>
          <StatusBadge status={email.status} />
        </div>
      </div>

      <div className="card space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Subject</label>
          <input
            className="input"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            disabled={!isDraft}
          />
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <label className="block text-xs font-medium text-slate-600">Body</label>
            <div className="flex gap-1 text-xs">
              <button
                className={`rounded px-2 py-0.5 ${view === "html" ? "bg-brand-100 text-brand-700" : "text-slate-500"}`}
                onClick={() => setView("html")}
              >
                HTML
              </button>
              <button
                className={`rounded px-2 py-0.5 ${view === "text" ? "bg-brand-100 text-brand-700" : "text-slate-500"}`}
                onClick={() => setView("text")}
              >
                Plaintext
              </button>
            </div>
          </div>

          {isDraft ? (
            <textarea
              className="input h-56 font-mono text-xs"
              value={body}
              onChange={(e) => setBody(e.target.value)}
            />
          ) : view === "html" ? (
            <div
              className="min-h-[8rem] rounded-md border border-slate-200 p-3 text-sm"
              dangerouslySetInnerHTML={{ __html: email.body_html }}
            />
          ) : (
            <pre className="min-h-[8rem] whitespace-pre-wrap rounded-md border border-slate-200 p-3 text-sm">
              {email.body_text || email.body}
            </pre>
          )}
        </div>

        {isDraft && view === "html" && (
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">HTML Preview</label>
            <div
              className="min-h-[6rem] rounded-md border border-slate-200 p-3 text-sm"
              dangerouslySetInnerHTML={{ __html: email.body_html }}
            />
          </div>
        )}

        {message && <p className="text-sm text-slate-500">{message}</p>}

        <div className="flex gap-2">
          {isDraft && (
            <>
              <button className="btn-secondary" onClick={handleSave} disabled={saving}>
                Save edits
              </button>
              <button className="btn-primary" onClick={handleSend} disabled={saving}>
                Approve &amp; Send
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
