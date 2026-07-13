# WatchUp Partnership Outreach Automation

Fully automated partnership outreach platform for WatchUp. Discovers companies,
crawls their sites, extracts + validates contact emails, understands the
business with **Groq**, writes personalized partnership emails, sends via
Hostinger SMTP from `partnership@watchup.space`, detects replies, follows up,
and exposes an admin dashboard.

> **AI provider:** Groq only (behind an abstract `ai.Provider` interface; config
> validation rejects any other `AI_PROVIDER` at boot). Originally built against
> Gemini — that implementation (`internal/ai/gemini`) is kept as a working,
> tested alternate provider but is no longer the active one.

## Status

Built phase by phase — see [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md).

- [x] **Phase 1 — Foundation & skeleton** (config, DB + migrations, Redis, Zap, Fiber `/health`, Docker)
- [x] **Phase 2 — Data layer & domain models** (status enums, generic repositories, seed helper, unit tests)
- [x] **Phase 3 — Company discovery** (CompanySource interface, config-driven registry, CSV/RSS/GitHub sources + stubs, dedupe-by-domain, `/companies/import` endpoint)
- [x] **Phase 4 — Crawler & email extraction** (Colly/GoQuery crawler, allow-listed paths, email extraction from mailto/header/footer/script/JSON-LD, priority ranking, live-verified against stripe.com)
- [x] **Phase 5 — Email validation** (syntax, MX, disposable-domain, optional SMTP catch-all probe, 0-100 scoring, live-verified against real DNS)
- [x] **Phase 6 — AI provider** (abstract `ai.Provider` interface; active implementation is **Groq** via its OpenAI-compatible chat completions API — `internal/ai/groq`, switched from an initial Gemini build which is kept as an alternate implementation; shared prompt-loading in `internal/ai/prompts`; prompt templates in `/prompts`; retry/backoff; live-verified end-to-end)
- [x] **Phase 7 — SMTP sending + tracking** (retry/backoff, MIME+threading headers, daily-limit + suppression guardrails, unsubscribe footer, open/click tracking endpoints)
- [x] **Phase 8 — Queue, workers & scheduler** (Redis job queue, chained pipeline handlers discover→crawl→validate→analyze→generate→send, manual/automatic mode, randomized send pacing, hourly scheduler with resilience sweep)
- [x] **Phase 9 — Reply detection, followups, bounce** (IMAP scan matching replies/bounces by Message-ID threading, Day 5/12/20 followup sequence, hard-bounce + unsubscribe suppression, live-verified IMAP login against Hostinger)
- [x] **Phase 10 — REST API** (JWT auth, companies/campaigns/emails/metrics/search endpoints, rate limiting, audit logging, CORS)
- [x] **Phase 11 — Next.js dashboard** (login, metrics overview, company page, email preview/edit/approve, campaign management, search — live-verified end-to-end in browser against a real backend)
- [x] **Phase 12 — Deliverability, security, tests** (SPF/DKIM/DMARC guide, security checklist, concurrent load test, runbook, production Docker for all 4 services)

## Quick start

```bash
cp .env.example .env          # fill in GROQ_API_KEY, SMTP_PASSWORD, JWT_SECRET, ADMIN_PASSWORD
docker compose up --build     # postgres, redis, api (:8080), worker, scheduler, frontend (:3000)
curl http://localhost:8080/health
```

A real `.env` with working credentials already exists locally (git-ignored,
not committed) — see the "Credentials" note below. **Deploying to a VPS?**
See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) (nginx + certbot, prepared for
`outreach.watchup.site:7070`) and `.env.production`. See
[`docs/RUNBOOK.md`](docs/RUNBOOK.md) for local dev without Docker, scaling,
and troubleshooting; [`docs/DELIVERABILITY.md`](docs/DELIVERABILITY.md) for
SPF/DKIM/DMARC setup before real sending; [`docs/SECURITY.md`](docs/SECURITY.md)
for the security model.

## Layout

```
backend/
  cmd/{api,worker,scheduler}   # three binaries, all fully wired
  internal/
    config/    logging/
    api/                          # REST API: auth, companies, campaigns, emails, metrics, search, tracking
    db/
      models/                  # GORM models + status enums
      repository/               # generic create/get/list/update/count/listWhere repos
      seed/                      # default-campaign seed helper
    sources/                     # CompanySource interface, registry, CSV/RSS/GitHub + stubs
    discovery/                   # discovery service: run sources, dedupe, upsert
    crawler/                     # Colly/GoQuery site crawler + persistence service
    email/extract/                # email discovery + priority ranking
    email/smtp/                   # Hostinger send: retry, MIME+threading, guardrails, tracking links
    email/imap/                   # reply/bounce/unsubscribe scanning
    validation/                  # deliverability scoring (MX/disposable/catch-all)
    ai/                          # abstract Provider interface + persistence service
    ai/groq/                      # active concrete Provider: Groq (OpenAI-compatible chat completions)
    ai/gemini/                    # alternate concrete Provider: Gemini REST (not currently wired up)
    ai/prompts/                    # shared prompt template loading, used by both providers
    queue/                        # Redis job queue (8 job types)
    workers/                      # job handlers: pipeline chaining + worker loop + send pacing
    scheduler/                    # hourly tick: discovery, resilience sweep, followups, reply scan
    testutil/                    # in-memory SQLite + miniredis test helpers
frontend/                      # Next.js dashboard (App Router, TypeScript, Tailwind)
  app/                            # login, dashboard, companies, emails, campaigns, search pages
  lib/                            # typed API client + auth token storage
  components/                     # Nav, StatCard, StatusBadge
prompts/                       # AI prompt templates (analysis, email, followup_1-3)
docker/                        # Dockerfile.backend, Dockerfile.frontend
docs/                          # implementation plan, deliverability, security, runbook
docker-compose.yml              # postgres, redis, api, worker, scheduler, frontend
```

## Credentials

`GROQ_API_KEY` and Hostinger `SMTP_USERNAME`/`SMTP_PASSWORD` are set in the
local `.env` (git-ignored). `SENDER_EMAIL` (the "From" address, `partnership@watchup.space`)
is separate from `SMTP_USERNAME` (the mailbox that authenticates, `devtomiwa@watchup.space`) —
Hostinger requires SMTP auth against a real mailbox login, and Phase 7 wires the
actual "From" header to `SENDER_EMAIL`. `GEMINI_API_KEY`/`GEMINI_MODEL` are also
still present in `.env` (unused by the active provider — kept in case Gemini is
ever switched back in).

Live-verified:
- **Groq**: real end-to-end call succeeds — `Analyze()` against a sample company
  returned a correctly-parsed 4-field JSON analysis with 398 tokens used, via
  `llama-3.3-70b-versatile`. No quota issues (unlike the earlier Gemini key).
- **Hostinger SMTP**: auth succeeds (`devtomiwa@watchup.space` over TLS on port 465).
  No real emails have been sent — that's a real, externally-visible action requiring
  explicit confirmation, so it's only exercised through hermetic tests (fake transport).
- **Hostinger IMAP**: login + search succeeds (`imap.hostinger.com:993`), confirmed
  against the real inbox (8 messages found in the last 24h at time of testing).
- *(Historical)* **Gemini**: API key authenticates, but `gemini-2.5-flash` was
  restricted for this key and `gemini-2.0-flash` hit `429 RESOURCE_EXHAUSTED, limit: 0`
  on the free tier — this is why the active provider was switched to Groq.

**Dashboard**: `ADMIN_USERNAME`/`ADMIN_PASSWORD` are set in `.env` for
dashboard login. Full login → dashboard → companies → company detail →
email preview/edit/save → campaigns (create/clone) → search flow was
live-verified in-browser (screenshots + DOM assertions) against a real
running instance of the actual `internal/api` server — using a temporary
SQLite-backed database seeded with realistic data (since Postgres/Redis
aren't available locally), not against Postgres. The server code path is
identical either way (`internal/api` doesn't know or care which SQL
dialect GORM is talking to); only the DB driver differed for this
verification pass.

**Not yet live-tested:** the full automated pipeline (discover → crawl →
validate → analyze → generate → send) running continuously against real
Postgres + Redis — covered instead by `internal/workers`' hermetic
integration tests (which exercise the exact same chain of handler calls,
just against in-memory SQLite + miniredis rather than live services).
