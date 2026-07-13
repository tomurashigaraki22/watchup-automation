# Security

## Secrets

- **Env-only, never hardcoded.** `internal/config/config.go` reads every
  secret (Groq/Gemini API keys, SMTP/IMAP password, JWT secret, admin
  password) from environment variables loaded via `.env`. `.env` is
  git-ignored (see `.gitignore`) — only `.env.example` (no real values) is
  committed.
- **SMTP/IMAP credentials are never persisted to the database.** They're
  read once from env at process boot into memory (`config.Config`) and used
  directly by `internal/email/smtp` / `internal/email/imap`. The PRD's
  "encrypt SMTP credentials" requirement is satisfied by *not storing them
  anywhere persistent* rather than by encrypting a DB column — there is no
  SMTP-credentials table in this schema, so there's nothing at rest to
  encrypt. If a future phase adds per-campaign/per-tenant SMTP configs
  stored in Postgres, encrypt that column (e.g. `pgcrypto` or
  application-level AES-GCM with a key from env, distinct from `JWT_SECRET`)
  before persisting.
- **JWT secret** (`JWT_SECRET`) must be set — `config.Load()` fails closed
  (refuses to boot) if it's empty, rather than defaulting to something
  guessable.

## Authentication

- Single admin user (`ADMIN_USERNAME`/`ADMIN_PASSWORD` from env) — there is
  no user table in the PRD schema, so this build doesn't support multiple
  dashboard accounts. `POST /api/v1/auth/login` issues an HS256 JWT
  (24h expiry) on successful credential match.
- Every `/api/v1/*` route requires `Authorization: Bearer <token>`
  (`internal/api/middleware.go`, `jwtAuth`), **except**:
  - `POST /api/v1/auth/login` (obviously — that's how you get a token)
  - `GET /api/v1/t/o/:id` (open-tracking pixel) and `GET /api/v1/t/u/:id`
    (unsubscribe link) — these are clicked by external email recipients who
    have no token and never will; they're scoped to a single numeric email
    ID with no other side effects available through them.
  - `GET /health` — used by container orchestration/monitoring.
- If `ADMIN_PASSWORD` is unset, `/auth/login` returns `503` rather than
  silently accepting a blank password.

## Audit logging

`internal/db/models.AuditLog` records every mutating action:
- **Worker-driven**: every queued job execution (`internal/workers/worker.go`)
  logs `job.<type>` with outcome ("ok" or the error).
- **API-driven**: campaign create/update/delete/pause/resume/clone, email
  edit, manual send-trigger, and CSV import all call `Server.audit()`
  (`internal/api/helpers.go`) with actor `"api"`.

Query via `GET` is not yet exposed as its own endpoint — the table exists and
is populated; a `GET /api/v1/audit-logs` route is a natural, low-risk future
addition if operators need to browse it through the dashboard rather than
querying Postgres directly.

## Rate limiting

`internal/api/server.go` applies `github.com/gofiber/fiber/v2/middleware/limiter`
globally: `API_RATE_LIMIT_PER_MIN` requests per minute per client (default
120). Applies to every route including public ones (tracking links, login) —
this also incidentally rate-limits how fast an attacker could brute-force
the login endpoint.

## CORS

`internal/api/server.go` reads `AllowOrigins` from `CORS_ALLOWED_ORIGIN`
(default `"*"`, fine for local dev where the dashboard runs on a different
port). **Set `CORS_ALLOWED_ORIGIN` to the dashboard's exact origin before
hosting publicly** (e.g. `https://outreach.watchup.site:7070` — see
`docs/DEPLOYMENT.md`) — a wildcard is fine for a Bearer-token API (no cookies
involved, so no CSRF-via-CORS risk), but scoping it is still better practice
once the real origin is known.

## Input handling

- CSV import (`internal/sources/csv.go`) only parses the `website`,
  `name`, `industry`, `description`, `employees` columns via
  `encoding/csv` — no formula/macro execution risk (this isn't opened in
  Excel by the server).
- Search (`internal/api/search.go`) uses GORM parameterized queries
  (`Where("... LIKE ?", arg)`) throughout — no raw SQL string
  concatenation, so no SQL injection surface from user-supplied `q`/`status`.
- The email HTML preview (`emailsmtp.RenderPreviewHTML`) escapes body text
  via `html.EscapeString` before embedding it — an AI-generated email body
  containing `<script>` renders as literal text, not executable HTML, both
  in the actual sent email and in the dashboard preview
  (`dangerouslySetInnerHTML` in `app/emails/[id]/page.tsx` is safe here
  specifically because the source HTML was already escaped server-side).

## Crawler safety

`internal/crawler` caps pages (25) and depth (2) per company, sets a
descriptive User-Agent, and only follows same-domain links — it won't
recursively crawl the wider internet or hammer a single site.

## Known gaps / deliberate simplifications

- No per-endpoint authorization tiers (every authenticated user can do
  everything) — acceptable given the single-admin-user model; would need
  revisiting if multi-user support is added.
- No CSRF token — not needed for a Bearer-token API with no cookie-based
  session.
- No IP allowlisting — relies on the JWT + rate limiter. Consider adding a
  reverse-proxy-level IP allowlist for the dashboard once hosted, if the
  admin's network is static.
