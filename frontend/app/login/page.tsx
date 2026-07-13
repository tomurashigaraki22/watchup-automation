"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import { setToken } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const { token } = await api.login(username, password);
      setToken(token);
      router.replace("/");
    } catch (err) {
      setError(err instanceof ApiError ? "Invalid username or password." : "Could not reach the API.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50">
      <form onSubmit={handleSubmit} className="card w-80">
        <h1 className="mb-1 text-lg font-semibold">WatchUp Outreach</h1>
        <p className="mb-4 text-sm text-slate-500">Sign in to the admin dashboard</p>

        <label className="mb-1 block text-xs font-medium text-slate-600">Username</label>
        <input
          className="input mb-3"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoFocus
        />

        <label className="mb-1 block text-xs font-medium text-slate-600">Password</label>
        <input
          className="input mb-4"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />

        {error && <p className="mb-3 text-sm text-red-600">{error}</p>}

        <button type="submit" className="btn-primary w-full" disabled={loading}>
          {loading ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
