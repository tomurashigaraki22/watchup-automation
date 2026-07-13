# Runbook

## Local development (no Docker)

Backend:
```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

There's no local Postgres/Redis on this dev machine — tests use in-memory
SQLite (`internal/testutil.NewDB`) and miniredis (`internal/testutil.NewRedis`)
instead of live services. This is why every phase in this project was
verified via `go build`/`go vet`/`go test` rather than live `docker compose up`
boots, with targeted live checks (real Groq/SMTP/IMAP calls) done separately
via throwaway scripts under `cmd/devtest/` (removed after each check).

Frontend:
```bash
cd frontend
npm install
npm run build   # verifies TypeScript + catches build errors
npm run dev     # http://localhost:3000
```

## Hosted deployment

```bash
cp .env.example .env   # fill in real credentials
docker compose up --build
```

Brings up: `postgres`, `redis`, `api` (:8080), `worker`, `scheduler`,
`frontend` (:3000). The frontend's `NEXT_PUBLIC_API_BASE_URL` is baked in at
**build time** (Next.js inlines `NEXT_PUBLIC_*` vars into the client bundle) —
set it via the `NEXT_PUBLIC_API_BASE_URL` environment variable before
`docker compose build` if the API isn't reachable at `http://localhost:8080`
from the browser (e.g. a real domain once hosted).

Before real sending: read `docs/DELIVERABILITY.md` (SPF/DKIM/DMARC) and set
`APP_BASE_URL` to the API's real public URL (embedded in tracking/unsubscribe
links sent to real recipients — `http://localhost:8080` in an email is
useless to anyone but you).

## Horizontal scaling

- **`worker`** is the only service safe to run multiple replicas of
  (`docker compose up --scale worker=3`) — each worker pulls from the same
  Redis queue (`BLPOP`, atomic), so jobs are never double-processed. This is
  literally the PRD's "horizontally scalable" requirement.
- **`scheduler`** must run as exactly one instance — running two would
  double-enqueue every hourly tick (duplicate discovery runs, duplicate
  followup enqueues, though duplicate `JobSend`s would still be caught by the
  handler's own idempotent status checks, so it's wasteful rather than
  dangerous, but still don't do it).
- **`api`** can run multiple replicas behind a load balancer — it's
  stateless (JWT auth, no server-side sessions).

## Monitoring / troubleshooting

- `GET /health` — checks DB connectivity; used for container health checks
  and uptime monitoring.
- Structured JSON logs (Zap) from every service — `docker compose logs -f worker`
  etc. Every job execution and API mutation writes an `audit_logs` row too
  (queryable directly in Postgres: `SELECT * FROM audit_logs ORDER BY created_at DESC LIMIT 50;`).
- **Emails stuck in `draft`**: campaign is in `manual` send mode (expected —
  needs approval via the dashboard) or there's no `active` campaign
  (`internal/workers/handlers.go`'s `firstActiveCampaign` returns none, logged
  as a warning).
- **Emails stuck in `queued`/not sending**: check `DAILY_LIMIT` isn't already
  hit for the day (`ErrDailyLimitReached` is logged at `info`, not `error` —
  it's expected behavior, not a bug); check the worker is actually running
  (`docker compose ps worker`).
- **Gemini/Groq errors**: check the relevant API key's quota/billing status
  first — this project hit exactly this issue with Gemini's free tier during
  development (see `README.md` Credentials section) before switching to Groq.
- **IMAP scan finds nothing**: `internal/email/imap.Scanner` only looks back
  30 days (`lookback` const) — a reply older than that won't be picked up on
  a fresh deploy; this is intentional (keeps scans fast) not a bug.

## Rotating credentials

Update `.env`, then restart the affected service(s)
(`docker compose restart api worker scheduler` — no rebuild needed, env vars
are read at process start). `JWT_SECRET` rotation invalidates all existing
dashboard sessions (users must log in again) — there's no session revocation
list, tokens are just JWTs verified by signature+expiry.
