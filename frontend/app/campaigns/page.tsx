"use client";

import { useEffect, useState } from "react";
import { api, Campaign, ApiError } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";

export default function CampaignsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [dailyLimit, setDailyLimit] = useState(25);
  const [sendMode, setSendMode] = useState("manual");
  const [busyId, setBusyId] = useState<number | null>(null);

  function load() {
    api
      .listCampaigns()
      .then((res) => setCampaigns(res.campaigns))
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load campaigns"));
  }

  useEffect(load, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      await api.createCampaign({ name, daily_limit: dailyLimit, send_mode: sendMode });
      setName("");
      setDailyLimit(25);
      setSendMode("manual");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create campaign");
    }
  }

  async function withBusy(id: number, action: () => Promise<unknown>) {
    setBusyId(id);
    try {
      await action();
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Action failed");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">Campaigns</h1>

      <form onSubmit={handleCreate} className="card flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Name</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Q3 Outreach" />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Daily Limit</label>
          <input
            type="number"
            className="input w-24"
            value={dailyLimit}
            onChange={(e) => setDailyLimit(Number(e.target.value))}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Send Mode</label>
          <select className="input" value={sendMode} onChange={(e) => setSendMode(e.target.value)}>
            <option value="manual">Manual (approve before send)</option>
            <option value="automatic">Automatic</option>
          </select>
        </div>
        <button type="submit" className="btn-primary">
          Create Campaign
        </button>
      </form>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="card overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-xs uppercase text-slate-500">
              <th className="py-2">Name</th>
              <th className="py-2">Status</th>
              <th className="py-2">Daily Limit</th>
              <th className="py-2">Mode</th>
              <th className="py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {campaigns.map((c) => (
              <tr key={c.id} className="border-b border-slate-100 last:border-0">
                <td className="py-2 font-medium">{c.name}</td>
                <td className="py-2">
                  <StatusBadge status={c.status} />
                </td>
                <td className="py-2">{c.daily_limit}</td>
                <td className="py-2">
                  <StatusBadge status={c.send_mode} />
                </td>
                <td className="py-2">
                  <div className="flex gap-1.5">
                    {c.status === "active" ? (
                      <button
                        className="btn-secondary"
                        disabled={busyId === c.id}
                        onClick={() => withBusy(c.id, () => api.pauseCampaign(c.id))}
                      >
                        Pause
                      </button>
                    ) : c.status === "paused" ? (
                      <button
                        className="btn-secondary"
                        disabled={busyId === c.id}
                        onClick={() => withBusy(c.id, () => api.resumeCampaign(c.id))}
                      >
                        Resume
                      </button>
                    ) : null}
                    <button
                      className="btn-secondary"
                      disabled={busyId === c.id}
                      onClick={() => withBusy(c.id, () => api.cloneCampaign(c.id))}
                    >
                      Clone
                    </button>
                    {c.status !== "deleted" && (
                      <button
                        className="btn-danger"
                        disabled={busyId === c.id}
                        onClick={() => withBusy(c.id, () => api.deleteCampaign(c.id))}
                      >
                        Delete
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {campaigns.length === 0 && (
              <tr>
                <td colSpan={5} className="py-6 text-center text-slate-400">
                  No campaigns yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
