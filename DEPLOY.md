# Production Deployment Guide

## Prerequisites
- Ubuntu 22.04 / Debian 12 VPS
- Docker + Docker Compose v2 installed
- Two DNS **A records** already pointing to the server IP:
  - `visora.fybud.com` → `<your-server-ip>`
  - `api.visora.fybud.com` → `<your-server-ip>`

---

## Step 1 — Push code to the server

```bash
# On your local machine
rsync -avz --exclude '.git' --exclude 'node_modules' . user@your-server:/opt/visora/

# Or just git clone on the server:
git clone <your-repo-url> /opt/visora
cd /opt/visora
```

---

## Step 2 — Set production environment files

### Frontend (`frontend/.env`)
Copy the template and fill in the values:
```bash
cp frontend/.env.production frontend/.env
# Edit AUTH_SECRET — generate a strong one:
openssl rand -base64 32
```

**Your current Google credentials are already filled in the template.**

### Backend (`backend/.env`)
Make sure it contains your real API keys:
```bash
# Verify it has:
OPENAI_API_KEY=...
SERP_API_KEY=...
# etc.
```

### DB Password (optional — defaults to `changeme_strong_password`)
```bash
export DB_PASS="your_very_strong_db_password"
# Or add it to /etc/environment for persistence
```

---

## Step 3 — Install nginx

```bash
sudo apt update && sudo apt install -y nginx
```

### Copy nginx configs
```bash
sudo cp nginx/nginx.conf /etc/nginx/nginx.conf
sudo cp nginx/visora.fybud.com.conf /etc/nginx/sites-available/
sudo cp nginx/api.visora.fybud.com.conf /etc/nginx/sites-available/
```

---

## Step 4 — Get SSL certificates (first time only)

First, temporarily start nginx with HTTP only so Certbot can verify ownership.
Add this minimal config for both domains:

```bash
# Temporarily allow HTTP through (for certbot webroot challenge)
sudo tee /etc/nginx/sites-enabled/temp.conf > /dev/null <<'EOF'
server {
    listen 80;
    server_name visora.fybud.com api.visora.fybud.com;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 200 "ok"; }
}
EOF
sudo nginx -t && sudo systemctl reload nginx

# Now run the certbot script
sudo bash certbot-init.sh
```

---

## Step 5 — Start all containers

```bash
cd /opt/visora

docker compose -f docker-compose.prod.yml up -d --build
```

This starts:
- **postgres** — database (not exposed publicly)
- **api-server** — Go REST API on `127.0.0.1:7001`
- **worker** — background jobs (crawl, SERP, AI analysis)
- **frontend** — Next.js SSR on `127.0.0.1:7000`

Scale workers up anytime:
```bash
docker compose -f docker-compose.prod.yml up -d --scale worker=2
```

---

## Step 6 — Update Google OAuth for production

In [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials), add to your OAuth client:

**Authorized JavaScript origins:**
```
https://visora.fybud.com
```

**Authorized redirect URIs:**
```
https://visora.fybud.com/api/auth/callback/google
```

---

## Step 7 — Verify everything is live

```bash
# API health
curl https://api.visora.fybud.com/api/health

# Frontend
curl -I https://visora.fybud.com
```

---

## Deploying updates (after initial setup)

```bash
cd /opt/visora
./deploy.sh
```

This script pulls the latest code, rebuilds changed images, and restarts containers with zero-config.

---

## Useful commands

```bash
# View logs
docker compose logs -f frontend
docker compose logs -f api-server
docker compose logs -f worker

# Check container status
docker compose ps

# Restart a single service
docker compose -f docker-compose.prod.yml restart frontend

# Manual database backup
docker exec visora-postgres pg_dump -U visora visora > backup_$(date +%Y%m%d).sql
```

---

## Worker scaling notes

The Go worker uses **Postgres `SKIP LOCKED`** for job queuing — no Redis or RabbitMQ needed. Multiple worker replicas safely fan out across jobs without conflicts.

- **1 worker** = fine for < 50 users/day  
- **2 workers** = default prod setting  
- **4 workers** = recommended for > 200 concurrent scans  

```bash
# Scale live without downtime:
docker compose -f docker-compose.prod.yml up -d --scale worker=4 --no-recreate
```
