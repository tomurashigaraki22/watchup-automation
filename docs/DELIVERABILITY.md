# Deliverability

Sending from `partnership@watchup.space` via Hostinger SMTP. Two layers keep this
inbox-safe: DNS-level authentication (your responsibility, one-time DNS setup)
and sending-behavior controls (already enforced in code).

## 1. DNS authentication (SPF / DKIM / DMARC)

These live in the DNS zone for `watchup.space`, not in this codebase. Required
before sending any real volume — without them, most providers (Gmail, Outlook)
will spam-box or reject mail from a new sending domain.

### SPF

Authorizes Hostinger's mail servers to send as `watchup.space`. Add a TXT
record at the domain apex:

```
v=spf1 include:_spf.hostinger.com ~all
```

If `watchup.space` already has an SPF record (e.g. from another provider),
merge the `include:` into the existing record — a domain can only have **one**
SPF TXT record.

### DKIM

Hostinger's control panel (Emails → your domain → DNS/DKIM settings) generates
the DKIM key pair and gives you the exact TXT record to add — typically:

```
Host: hostingermail1._domainkey.watchup.space  (or similar, per Hostinger's UI)
Value: v=DKIM1; k=rsa; p=<public key from Hostinger>
```

Enable DKIM signing in Hostinger's panel after the DNS record propagates
(check with `dig TXT hostingermail1._domainkey.watchup.space`).

### DMARC

Add after SPF + DKIM are live and passing (check via
[mail-tester.com](https://www.mail-tester.com) or a test send to a Gmail
account, "Show original" → look for `spf=pass dkim=pass`):

```
Host: _dmarc.watchup.space
Value: v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@watchup.space; pct=100
```

Start with `p=quarantine` (not `p=reject`) for the first few weeks so a
misconfiguration doesn't silently drop legitimate mail — tighten to
`p=reject` once DMARC reports (sent to `rua=`) show consistent alignment.

## 2. Sending-behavior controls (already enforced in code)

| Control | Where | Behavior |
|---|---|---|
| Daily send cap | `internal/email/smtp/service.go` (`ErrDailyLimitReached`) | Counts `emails` with `status=sent AND sent_at >= today` per campaign; blocks further sends past `campaign.daily_limit`. |
| Randomized send interval | `internal/workers/worker.go` (`paceSend`) | 45–240s (configurable via `SEND_DELAY_MIN_SECONDS`/`SEND_DELAY_MAX_SECONDS`) random delay applied after every `JobSend`/`JobFollowup` dispatch — never identical timing between sends. |
| Suppression list | `internal/email/smtp/service.go` (`ErrSuppressed`) | Checked before every send; populated by unsubscribe replies, unsubscribe link clicks, and hard bounces. Never overridden. |
| Threaded followups | `internal/email/smtp/message.go` | Followups set `In-Reply-To`/`References` to the original email's `Message-ID`, so mail clients group them as one thread rather than looking like repeat cold sends. |
| No resending the original | `internal/workers/handlers.go` (`handleFollowup`) | Followups always generate new content via Gemini/Groq (`GenerateFollowup`), never resend the original email body. |
| Hard-bounce suppression | `internal/email/imap/scanner.go` | SMTP status code `5.x.x` → suppress immediately, cancel pending followups, never retry. `4.x.x` (soft) → marked bounced but not suppressed. |
| Reply detection stops followups | `internal/email/imap/scanner.go` | Any matched reply cancels all pending `followups` rows for that thread. |
| Unsubscribe footer + link | `internal/email/smtp/service.go` (`appendUnsubscribeFooter`) | Every email includes "Reply 'unsubscribe' or click here" — both paths (IMAP reply scan and the `/api/v1/t/u/:id` click endpoint) suppress the contact. |
| Open/click tracking | `internal/api/tracking.go` | Invisible pixel + click-through unsubscribe link, both public (unauthenticated) routes since recipients trigger them directly. |

## 3. Subject-line variation

The AI prompts (`/prompts/followup_1.txt`, `followup_2.txt`, `followup_3.txt`)
explicitly instruct the model to generate a "reply-style subject... not
identical to" the prior one. This is prompt-enforced, not code-enforced —
there's no hard uniqueness check in `internal/ai`. Acceptable for this build's
scale; if abuse/duplication becomes visible in practice, add a check in
`ai.Service.GenerateFollowup` comparing against the original/prior subjects
and re-prompting on an exact match.

## 4. Verification checklist before going live

1. `dig TXT watchup.space` → SPF record present, includes Hostinger.
2. `dig TXT <selector>._domainkey.watchup.space` → DKIM key present.
3. `dig TXT _dmarc.watchup.space` → DMARC record present.
4. Send a real test email to a Gmail/Outlook test account → "Show original" →
   confirm `spf=pass`, `dkim=pass`, `dmarc=pass`.
5. Set `DAILY_LIMIT` conservatively at first (e.g. `10-25`) and ramp up over
   1-2 weeks — this is standard practice for a new sending domain/IP
   reputation ("warming up"), independent of anything this codebase enforces.
