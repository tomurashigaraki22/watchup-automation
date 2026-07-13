# WatchUp Partnership Outreach Automation — Phase-by-Phase Implementation Plan

**Version:** 1.0
**Working dir:** `C:\Users\HP\Desktop\watchup-automation`
**AI provider:** **Gemini only** (behind an abstract `ai.Provider` interface so others can be added later, but only the Gemini implementation is wired up and used)
**Sender:** `partnership@watchup.space` via Hostinger SMTP

> **Update (post-Phase 9):** the active AI provider was switched from Gemini to
> **Groq** (`internal/ai/groq`, OpenAI-compatible chat completions API, model
> `llama-3.3-70b-versatile`) after the Gemini API key hit free-tier quota limits.
> The Gemini implementation (`internal/ai/gemini`) is kept as a working, tested
> alternate `ai.Provider` but is no longer wired into `cmd/worker`. Every
> "Gemini" reference below reflects the original plan as written; see
> `README.md` for current status.

---

## 0. Guiding principles

- **Modular & interface-driven.** Discovery sources, AI providers, and email transport are all interfaces. Only concrete impls we ship now: Gemini (AI), Hostinger SMTP/IMAP (email), a handful of discovery sources.
- **Runnable at every phase.** Each phase ends with something you can `docker compose up` (or `go run`) and observe. No phase leaves the tree un-compilable.
- **Idempotent pipeline.** Every stage is safe to re-run; work is tracked by row `status` so the hourly scheduler never double-sends.
- **Safety first.** Daily caps, randomized delays, suppression list, and hard-bounce guards are enforced in the send path, not bolted on later.
- **Observability.** Structured Zap logs + an `audit_logs` table for every side-effecting action (crawl, AI call, SMTP send, reply, followup).

### Decisions locked for this build
| Topic | Decision | Rationale |
|---|---|---|
| AI | Gemini `generateContent` REST (`gemini-2.5-flash` default, model from env) | Per instruction: Gemini only. REST avoids a heavy SDK. |
| Orchestration | Redis-backed job queue + cron-style scheduler (Temporal-ready, not required) | Temporal is "preferred" in PRD but heavy to run locally; we ship a Redis worker model and keep the worker interface Temporal-swappable. |
| ORM | GORM + AutoMigrate | Fast schema iteration for the 6 tables; PRD allows GORM. |
| API | Fiber v2 + JWT middleware | Per PRD. |
| Frontend | Next.js (App Router) + minimal fetch client | Per PRD. |
| Config | `.env` via `godotenv`, validated at boot | Secrets only from env. |

---

## Phase 1 — Foundation & skeleton (compiles, boots, connects)

**Goal:** A booting Fiber server + Postgres + Redis + config + logging, with the full folder structure in place.

**Build:**
- `go.mod` (module `watchup/automation`), `backend/cmd/api/main.go`, `backend/cmd/worker/main.go`, `backend/cmd/scheduler/main.go`.
- `internal/config` — typed config loaded from `.env`, fail-fast validation.
- `internal/db` — GORM connection + `AutoMigrate` for all 6 tables (`companies`, `contacts`, `outreach_campaigns`, `emails`, `followups`, `ai_generations`) plus `suppressions` and `audit_logs`.
- `internal/logging` — Zap production/dev logger.
- `internal/api` — Fiber app, `/health`, `/metrics` stub, route groups.
- `docker/` + `docker-compose.yml` (postgres, redis, api, worker, scheduler, frontend), `Dockerfile` (multi-stage Go), `.env.example`.

**Acceptance:** `docker compose up` boots; `GET /health` returns `200`; tables exist in Postgres; logs are structured JSON.

---

## Phase 2 — Data layer & domain models

**Goal:** Canonical Go structs + repositories for every entity.

**Build:**
- `internal/db/models` — GORM models matching the PRD schema exactly (with the enum-ish `status` fields: company `status`, email `status`, campaign `status`).
- Repository functions: create/get/list/update for companies, contacts, campaigns, emails, followups, ai_generations, suppressions.
- Status enums as typed constants (`company.StatusDiscovered/Crawled/Analyzed/Queued/Contacted`, `email.StatusDraft/Queued/Sent/Failed/Bounced/Replied`, etc.).
- Seed helper + one default campaign.

**Acceptance:** Unit tests create and read each entity against a test Postgres; migrations round-trip cleanly.

---

## Phase 3 — Company Discovery (modular sources)

**Goal:** Pluggable `CompanySource` interface producing companies into the DB, de-duplicated by normalized website domain.

**Build:**
- `internal/sources` — `type CompanySource interface { Name() string; Discover(ctx) ([]Company, error) }`.
- A `Registry` that runs enabled sources (config-driven).
- Concrete sources:
  - **CSV import** (file/upload) — always available, the deterministic baseline.
  - **RSS feed** source (generic).
  - **GitHub Organizations** (public API, token optional).
  - **Product Hunt** and **YC / AI / SaaS directory** sources as HTML-scrape or API stubs behind the same interface (implemented where a stable public endpoint exists; others ship as clearly-marked stubs that return `[]` until credentials/endpoints are provided).
- Domain normalization + upsert (dedupe on host).

**Acceptance:** Running discovery inserts new companies with `status=discovered`; re-running does not duplicate; CSV import endpoint works end-to-end.

---

## Phase 4 — Crawler & email extraction

**Goal:** Given a discovered company, crawl the site and extract structured info + emails.

**Build:**
- `internal/crawler` using **Colly + GoQuery**: start at `/`, follow only the allow-listed paths (`/contact`, `/about`, `/team`, `/company`, `/privacy`, `/legal`, `/terms`, `/support`), **max 25 pages, max depth 2**, same-domain only, polite delay + user-agent.
- Extract: description, products, technologies, support links, social media.
- `internal/email/extract` — pull emails from HTML body, footer, header, `mailto:` links, inline JS, and JSON-LD structured data via regex; filter to the target local-parts (`support hello contact business partnership sales info founder admin`).
- **Prioritization** ranking: `partnership > business > hello > contact > info > support > (others)` → written to `contacts.priority`.
- Persist contacts (dedupe per company+email), set company `status=crawled`.

**Acceptance:** Unit tests over saved HTML fixtures extract the right emails and priorities; crawling a live site stores contacts and a description.

---

## Phase 5 — Email validation

**Goal:** Score deliverability before anything is sent.

**Build:**
- `internal/validation`: MX lookup, disposable-domain blocklist, syntax, optional SMTP RCPT probe (behind a flag; off by default to protect reputation), catch-all heuristic.
- Compute `verification_score` (0–100) and set `contacts.verified`.
- Store results; low scores are filtered out of the send queue.

**Acceptance:** Known-good domains score high; disposable/invalid score low; results persisted.

---

## Phase 6 — Gemini AI layer (analysis + generation)

**Goal:** The **only** AI provider — Gemini — wired behind the abstract interface.

**Build:**
- `internal/ai` — `type Provider interface { Analyze(ctx, CompanyContext) (Analysis, error); GenerateEmail(ctx, EmailContext) (Email, error); GenerateFollowup(ctx, FollowupContext) (Email, error) }`.
- `internal/ai/gemini` — the sole concrete implementation calling Gemini `:generateContent` REST with `GEMINI_API_KEY`, `GEMINI_MODEL` (default `gemini-2.5-flash`), retries + JSON-mode parsing. A `provider factory` returns Gemini and errors on any other configured provider (Gemini-only guarantee).
- **Prompts live in `/prompts`** (not hardcoded): `analysis.txt`, `email.txt`, `followup_1.txt`, `followup_2.txt`, `followup_3.txt`, loaded + templated at runtime.
- Analysis returns `{summary, industry, value_proposition, watchup_angle}` → stored on company + `ai_generations` (prompt, response, model, tokens).
- Email generation returns subject + HTML/plaintext body + CTA + PS, referencing company name/product/WatchUp fit; uniqueness enforced via temperature + varied templates; every send logged to `ai_generations`.

**Acceptance:** With a real `GEMINI_API_KEY`, analyzing a crawled company yields the 4-field JSON; generating an email yields a personalized subject/body; token usage recorded. Without a key, the module returns a clear typed error (no crash).

---

## Phase 7 — SMTP sending + tracking + safety controls

**Goal:** Send real, throttled, tracked email via Hostinger.

**Build:**
- `internal/email/smtp` — Hostinger SMTP (`SMTP_HOST/PORT/USERNAME/PASSWORD`), from `partnership@watchup.space`, **3 retries w/ exponential backoff**, store SMTP response + `Message-ID`, set proper threading headers.
- **Send guardrails (all enforced here):** per-campaign **daily limit**, global suppression-list check, hard-bounce check, **randomized 45–240s delay** between sends (no identical timing).
- Unsubscribe footer on every email ("Reply 'unsubscribe' or click here") + open/click tracking pixel/link endpoints (`opened`, `clicked`).
- Email lifecycle: `draft → queued → sent/failed/bounced`.

**Acceptance:** A queued email sends via SMTP, stores Message-ID + response, respects the daily cap, waits a randomized interval, and never sends to a suppressed/hard-bounced address.

---

## Phase 8 — Queue, workers & scheduler (the pipeline)

**Goal:** Tie stages together into the hourly automated pipeline.

**Build:**
- `internal/queue` — Redis list/stream queue with typed jobs (`discover, crawl, validate, analyze, generate, send, followup, reply_scan`).
- `internal/workers` — worker pool consuming jobs; each job is one idempotent stage; failures retried with backoff and audit-logged.
- `internal/scheduler` — runs **every hour**: enqueues discovery, then walks companies through crawl → extract → validate → analyze → generate → queue → send, plus due followups and reply scanning. Respects daily caps and manual/automatic mode.
- **Manual approval mode** vs **fully automatic mode** (campaign/global flag): in manual mode, generation stops at `draft` awaiting approval; automatic mode proceeds to `queued`.

**Acceptance:** With the scheduler running and a seeded company, the pipeline advances the company through all states within the hour tick (or on-demand trigger), ending in a sent (or draft-for-approval) email — horizontally scalable by running N workers.

---

## Phase 9 — Reply detection, followups & bounce handling

**Goal:** Close the loop: detect replies, stop followups, handle bounces.

**Build:**
- `internal/email/imap` — connect to Hostinger IMAP inbox, match incoming mail to sent emails via `In-Reply-To` / `References` / thread ID, set `emails.replied=true`, **cancel remaining followups** for that thread.
- **Followup sequence** (auto-generated, never resends the original): **Day 5** friendly reminder, **Day 12** checking in, **Day 20** last email — scheduled in `followups`, generated via Gemini followup prompts, threaded onto the original.
- **Bounce handling**: parse bounce/NDR messages → mark hard vs soft; hard bounce → suppress + never retry; soft bounce → limited retry.
- **Unsubscribe**: "unsubscribe" replies → add to suppression list; suppressed contacts are never emailed.

**Acceptance:** A reply to a sent thread flips `replied=true` and cancels scheduled followups; an unsubscribe reply suppresses the contact; a hard bounce suppresses and is never retried; due followups send on schedule only when no reply.

---

## Phase 10 — REST API surface

**Goal:** Full API for the dashboard and automation control.

**Build (Fiber + JWT):**
- Companies: `GET /companies`, `GET /companies/:id`, `POST /companies/import`.
- Campaigns: `GET /campaigns`, `POST /campaigns`, `PATCH /campaigns/:id`, `DELETE /campaigns/:id` (+ pause/resume/clone).
- Emails: `GET /emails`, `GET /emails/:id`, `POST /emails/:id/send` (+ approve, edit).
- Metrics: `GET /metrics`.
- Search across company/email/campaign/status.
- Middleware: JWT auth, API rate limiting, request logging, audit-log writes.

**Acceptance:** Every endpoint returns correct data with auth enforced; unauthorized requests rejected; rate limiting active.

---

## Phase 11 — Next.js dashboard

**Goal:** Admin UI over the API.

**Build:**
- Auth (JWT login) + fetch client.
- **Dashboard metrics:** companies discovered/crawled, emails extracted/verified/sent, replies, open rate, bounce rate, followups, campaign performance.
- **Search** page.
- **Company page:** website, description, emails, AI summary, generated email, history, reply, notes.
- **Email preview:** rendered HTML + plaintext, subject, editable, approve/send (manual mode).
- **Campaign management:** create/pause/resume/delete/clone.

**Acceptance:** Log in, see live metrics, drill into a company, preview/edit/approve an email, manage a campaign — all against the running backend.

---

## Phase 12 — Deliverability, security, tests, hardening

**Goal:** Production-readiness.

**Build:**
- **Deliverability docs + checks:** SPF/DKIM/DMARC setup guide for `watchup.space`, enforced random intervals, daily caps, threaded followups, no-duplicate-subject guard.
- **Security:** secrets from env only, encrypt SMTP credentials at rest, JWT auth, audit logs, API rate limiting.
- **Tests:** unit (crawler, extractor, SMTP, AI, validation), integration (pipeline), a lightweight load test.
- Production `Dockerfile` + `docker-compose.yml` finalized; `docs/` runbook.

**Acceptance:** Test suite green; deliverability checklist documented; `docker compose up` yields the full working system meeting all 11 success criteria.

---

## Dependency order (critical path)

```
P1 Foundation
      │
P2 Data layer
      │
P3 Discovery ──► P4 Crawl+Extract ──► P5 Validation ──► P6 Gemini AI ──► P7 SMTP send
                                                                              │
                                            P8 Queue/Workers/Scheduler ◄──────┘
                                                                              │
                                            P9 Reply/Followup/Bounce ◄────────┘
                                                                              │
                                            P10 REST API ──► P11 Dashboard ──► P12 Hardening
```

Phases 3–7 build the linear pipeline stages; Phase 8 wires them into automation; 9 closes the feedback loop; 10–11 expose it; 12 hardens.

---

## Success-criteria traceability

| # | Success criterion | Delivered in |
|---|---|---|
| 1 | Discover companies | P3, P8 |
| 2 | Crawl websites | P4 |
| 3 | Extract contact emails | P4 |
| 4 | Validate emails | P5 |
| 5 | Understand business (AI) | P6 (Gemini) |
| 6 | Generate personalized email | P6 (Gemini) |
| 7 | Send via Hostinger SMTP | P7 |
| 8 | Detect replies | P9 |
| 9 | Stop followups after replies | P9 |
| 10 | Discover on a schedule | P8 |
| 11 | Complete dashboard | P10, P11 |

---

## Environment variables (`.env`)

```
# Server
APP_ENV=development
API_PORT=8080
JWT_SECRET=change-me

# Postgres
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_USER=watchup
POSTGRES_PASSWORD=watchup
POSTGRES_DB=watchup

# Redis
REDIS_ADDR=redis:6379

# AI — GEMINI ONLY
AI_PROVIDER=gemini
GEMINI_API_KEY=
GEMINI_MODEL=gemini-2.5-flash

# Email — Hostinger
SMTP_HOST=smtp.hostinger.com
SMTP_PORT=465
SMTP_USERNAME=partnership@watchup.space
SMTP_PASSWORD=
IMAP_HOST=imap.hostinger.com
IMAP_PORT=993

# Sending policy
SENDER_EMAIL=partnership@watchup.space
DAILY_LIMIT=25
SEND_DELAY_MIN_SECONDS=45
SEND_DELAY_MAX_SECONDS=240
SEND_MODE=manual   # manual | automatic
```

---

**Next step:** On approval of this plan, begin **Phase 1 — Foundation & skeleton** in `C:\Users\HP\Desktop\watchup-automation`.
