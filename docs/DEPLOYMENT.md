# Deploying to your VPS

Target: `outreach.watchup.site`, served externally on **port 7070**, TLS via
**certbot**. One DNS record, one nginx vhost, one certificate — nginx on the
host path-routes `/api/*` and `/health` to the Go API and everything else to
the Next.js dashboard, so the browser only ever talks to one origin.

Everything below except steps 1 (DNS) and parts of 3/5 (things that must run
*on* your VPS) is already prepared in this repo. I don't have access to your
VPS or domain registrar, so those specific steps are yours to run — commands
are given exactly.

## Architecture

```
Internet ──443/7070──▶ nginx (host, certbot TLS) ──▶ 127.0.0.1:8080  api (docker)
                                                  └──▶ 127.0.0.1:3000  frontend (docker)
                              (postgres :5432, redis :6379 — docker, localhost-only, never public)
```

## 1. DNS (do this first — it takes time to propagate)

At your domain registrar / DNS provider for `watchup.site`, add:

```
Type: A
Name: outreach
Value: <your VPS's public IPv4 address>
TTL:   auto / 300s
```

(If you use IPv6 too, add an `AAAA` record pointing at the VPS's IPv6.)

Verify propagation before continuing:
```bash
dig +short outreach.watchup.site
# should print your VPS's IP
```

## 2. Get the code onto the VPS

From your local machine:
```bash
# from C:\Users\HP\Desktop\watchup-automation
git init && git add -A && git commit -m "Initial deploy"   # if not already a git repo
# push to a private GitHub repo, or scp the whole directory:
scp -r . youruser@<vps-ip>:/opt/watchup-automation
```

Or, if you'd rather pull via git on the VPS:
```bash
# on the VPS
sudo mkdir -p /opt/watchup-automation && sudo chown $USER /opt/watchup-automation
git clone <your-repo-url> /opt/watchup-automation
```

Then copy the production env into place **on the VPS**:
```bash
cd /opt/watchup-automation
cp .env.production .env
chmod 600 .env   # readable only by you — it has real secrets in it
```

## 3. Install Docker + Docker Compose on the VPS (if not already present)

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER && newgrp docker
docker compose version   # should print v2.x
```

## 4. Bring up the app services (not yet publicly reachable — bound to localhost)

```bash
cd /opt/watchup-automation
docker compose up --build -d postgres redis api worker scheduler frontend
docker compose ps          # all should show "healthy" or "running"
curl -s http://127.0.0.1:8080/health   # {"status":"ok",...}
curl -sI http://127.0.0.1:3000         # HTTP/1.1 200
```

`docker-compose.yml` binds `postgres`/`redis`/`api`/`frontend` to
`127.0.0.1` only — nothing here is internet-reachable yet, which is
intentional (nginx is the only public entry point, set up next).

## 5. nginx + certbot (runs on the VPS host, not in Docker)

Install:
```bash
sudo apt update && sudo apt install -y nginx certbot python3-certbot-nginx
```

Copy the prepared vhost config:
```bash
sudo cp /opt/watchup-automation/deploy/nginx/outreach.watchup.site.conf /etc/nginx/sites-available/
sudo ln -s /etc/nginx/sites-available/outreach.watchup.site.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

Obtain the certificate. Because this vhost listens on **7070**, not the
standard 80/443, certbot's `--nginx` plugin needs port 80 free for the
HTTP-01 challenge — either temporarily, or permanently if you're fine
leaving a small redirect there:

```bash
# open port 80 in your firewall if it isn't already (needed only for the challenge)
sudo ufw allow 80/tcp   # skip if you don't use ufw / already open

sudo certbot --nginx -d outreach.watchup.site
```

Certbot will detect the `server_name outreach.watchup.site` block, ask which
one to secure, and rewrite it in place to add `ssl_certificate`/
`ssl_certificate_key` lines and an HTTPS `listen 7070 ssl` directive
alongside the existing plain block (or convert it, depending on your
certbot version — either way it edits
`/etc/nginx/sites-available/outreach.watchup.site.conf` directly). Confirm:

```bash
sudo nginx -t && sudo systemctl reload nginx
curl -skI https://outreach.watchup.site:7070/health
```

Certbot installs a renewal cron/systemd-timer automatically
(`sudo certbot renew --dry-run` to verify it'll actually renew later).

**If port 80 can't be freed** (something else is already using it on this
VPS): use the webroot or DNS-01 method instead —
```bash
sudo certbot certonly --webroot -w /var/www/html -d outreach.watchup.site
```
then manually add the `ssl_certificate /etc/letsencrypt/live/outreach.watchup.site/fullchain.pem;`
and `ssl_certificate_key .../privkey.pem;` lines to the `listen 7070 ssl;`
block yourself in `/etc/nginx/sites-available/outreach.watchup.site.conf`.

## 6. Open the port in your firewall

```bash
sudo ufw allow 7070/tcp
sudo ufw status
```

(Also keep 22/tcp (SSH) open, obviously — don't lock yourself out.)

## 7. Verify

```bash
curl -s https://outreach.watchup.site:7070/health
# {"status":"ok","service":"watchup-outreach-api","env":"production"}
```

Open `https://outreach.watchup.site:7070` in a browser → the dashboard login
page should load. Log in with `ADMIN_USERNAME`/`ADMIN_PASSWORD` from `.env`.

## 8. Before sending real email

Read [`docs/DELIVERABILITY.md`](DELIVERABILITY.md) — add SPF/DKIM/DMARC DNS
records for `watchup.space` (the sending domain, separate from
`watchup.site`/`outreach.watchup.site` which is just where the dashboard
lives). Sending will work without this, but will likely land in spam.

## 9. Redeploying after a code change

```bash
cd /opt/watchup-automation
git pull   # or re-scp
docker compose up --build -d
```

`worker` can be scaled horizontally if throughput matters:
```bash
docker compose up -d --scale worker=3
```
`scheduler` must stay at exactly one replica (see `docs/RUNBOOK.md`).

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `curl https://outreach.watchup.site:7070/health` times out | Firewall isn't allowing 7070, or DNS hasn't propagated yet (`dig +short outreach.watchup.site`) |
| nginx `502 Bad Gateway` | `docker compose ps` — is `api`/`frontend` actually running? `docker compose logs api` |
| Browser console shows CORS errors | `CORS_ALLOWED_ORIGIN` in `.env` doesn't match the exact origin the browser sent (scheme + host + port must match exactly: `https://outreach.watchup.site:7070`) |
| Login works but every other page 401s | `NEXT_PUBLIC_API_BASE_URL` was wrong *at build time* — it's baked into the JS bundle, so fix `.env` and `docker compose up --build -d frontend` (a plain restart won't pick up the new value) |
| certbot fails the HTTP-01 challenge | Port 80 isn't reachable from the internet (firewall, or something else bound to it) — use `--webroot`/DNS-01 instead, see step 5 |
