import { getToken, clearToken } from "./auth";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((init.headers as Record<string, string>) || {}),
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { ...init, headers });

  if (res.status === 401) {
    clearToken();
    if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new ApiError(401, "unauthorized");
  }
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new ApiError(res.status, text || `request failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export interface Company {
  id: number;
  name: string;
  website: string;
  industry: string;
  description: string;
  employees: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface Contact {
  id: number;
  company_id: number;
  email: string;
  name: string;
  source: string;
  priority: number;
  verified: boolean;
  verification_score: number;
  created_at: string;
}

export interface Email {
  id: number;
  campaign_id: number;
  company_id: number;
  contact_id: number;
  subject: string;
  body: string;
  body_text: string;
  message_id: string;
  status: string;
  opened: boolean;
  clicked: boolean;
  replied: boolean;
  bounced: boolean;
  smtp_response: string;
  sent_at: string | null;
  created_at: string;
}

export interface EmailPreview extends Email {
  body_html: string;
}

export interface AIGeneration {
  id: number;
  company_id: number;
  kind: string;
  prompt: string;
  response: string;
  model: string;
  tokens: number;
  created_at: string;
}

export interface Campaign {
  id: number;
  name: string;
  status: string;
  daily_limit: number;
  send_mode: string;
  created_at: string;
}

export interface CompanyDetail extends Company {
  contacts: Contact[];
  emails: Email[];
  ai_generations: AIGeneration[];
}

export interface CampaignPerformance {
  campaign_id: number;
  name: string;
  status: string;
  sent: number;
  replies: number;
  open_rate: number;
}

export interface Metrics {
  companies_discovered: number;
  companies_crawled: number;
  companies_analyzed: number;
  emails_extracted: number;
  emails_verified: number;
  emails_sent: number;
  replies: number;
  open_rate: number;
  bounce_rate: number;
  followups_sent: number;
  followups_pending: number;
  campaigns: CampaignPerformance[];
}

export interface SearchResults {
  companies: Company[];
  emails: Email[];
  campaigns: Campaign[];
}

export const api = {
  login: (username: string, password: string) =>
    request<{ token: string; expires_at: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),

  metrics: () => request<Metrics>("/api/v1/metrics"),

  search: (q: string, status: string) => {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    if (status) params.set("status", status);
    return request<SearchResults>(`/api/v1/search?${params.toString()}`);
  },

  listCompanies: (status?: string) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    return request<{ companies: Company[] }>(`/api/v1/companies?${params.toString()}`);
  },
  getCompany: (id: number) => request<CompanyDetail>(`/api/v1/companies/${id}`),
  importCompaniesCSV: async (file: File) => {
    const token = getToken();
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`${API_BASE}/api/v1/companies/import`, {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    });
    if (!res.ok) throw new ApiError(res.status, await res.text());
    return res.json() as Promise<{ inserted: number; skipped: number; errors: number }>;
  },

  listEmails: (params: { status?: string; campaign_id?: number; company_id?: number } = {}) => {
    const sp = new URLSearchParams();
    if (params.status) sp.set("status", params.status);
    if (params.campaign_id) sp.set("campaign_id", String(params.campaign_id));
    if (params.company_id) sp.set("company_id", String(params.company_id));
    return request<{ emails: Email[] }>(`/api/v1/emails?${sp.toString()}`);
  },
  getEmail: (id: number) => request<EmailPreview>(`/api/v1/emails/${id}`),
  updateEmail: (id: number, data: { subject?: string; body?: string }) =>
    request<Email>(`/api/v1/emails/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  sendEmail: (id: number) => request<{ status: string }>(`/api/v1/emails/${id}/send`, { method: "POST" }),

  listCampaigns: () => request<{ campaigns: Campaign[] }>("/api/v1/campaigns"),
  createCampaign: (data: { name: string; daily_limit: number; send_mode: string }) =>
    request<Campaign>("/api/v1/campaigns", { method: "POST", body: JSON.stringify(data) }),
  updateCampaign: (
    id: number,
    data: Partial<{ name: string; status: string; daily_limit: number; send_mode: string }>
  ) => request<Campaign>(`/api/v1/campaigns/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  deleteCampaign: (id: number) => request<Campaign>(`/api/v1/campaigns/${id}`, { method: "DELETE" }),
  pauseCampaign: (id: number) => request<Campaign>(`/api/v1/campaigns/${id}/pause`, { method: "POST" }),
  resumeCampaign: (id: number) => request<Campaign>(`/api/v1/campaigns/${id}/resume`, { method: "POST" }),
  cloneCampaign: (id: number) => request<Campaign>(`/api/v1/campaigns/${id}/clone`, { method: "POST" }),
};
