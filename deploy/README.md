# Build or Bail - VPS Deployment Guide

## Prerequisites

- VPS with Docker and Docker Compose installed
- Domain pointed to your VPS IP (optional)
- `.env` file with your API keys

## Quick Start

### 1. Upload project to VPS

```bash
# From your local machine, copy project to VPS
scp -r /path/to/build-or-bail-project user@your-vps:/home/user/buildorbail/

# Or use git clone on VPS
ssh user@your-vps
git clone <your-repo-url> /home/user/buildorbail
```

### 2. Configure environment

```bash
cd /home/user/buildorbail

# Copy and edit environment file
cp .env.example .env
nano .env
# Add your API keys:
#   OPENROUTER_API_KEY=sk-...
#   GOOGLE_AI_API_KEY=...  (Google AI Studio - simpler)
#   TURSO_DB_URL=https://...
#   TURSO_AUTH_TOKEN=...
```

**For Vertex AI (service account):**
```bash
# 1. In Google Cloud Console → IAM → Service Accounts → Create
#    Role: "Vertex AI User"
# 2. Create JSON key → download → copy to project root
scp google-creds.json user@vps:/home/user/buildorbail/google-creds.json
# 3. Set in .env:
#    GOOGLE_APPLICATION_CREDENTIALS=google-creds.json
#    VERTEX_PROJECT_ID=your-project-id
```

### 3. Configure and Deploy

```bash
cd /home/user/buildorbail/deploy

# Set your port (defaults to 5897)
export HOST_PORT=5897

# Build and start
docker compose up -d --build

# Check status
docker compose ps

# View logs
docker compose logs -f
```

### 4. Verify

```bash
curl http://localhost:${HOST_PORT:-5897}/api/config
```

## Production Deployment (with reverse proxy)

### Nginx config example:

```nginx
server {
    listen 80;
    server_name yourdomain.com;

    location / {
        proxy_pass http://localhost:${HOST_PORT:-5897};

# View logs
docker compose logs -f
```

### 4. Verify

```bash
curl http://localhost:5897/api/config
```

## Production Deployment (with Caddy)

Caddy is the recommended reverse proxy. It handles automatic SSL (HTTPS) and has a simple configuration.

### Caddyfile example:

```caddy
buildorbail.yourdomain.com {
    reverse_proxy localhost:5897
    encode zstd gzip
}
```

### Apply configuration:

```bash
# If running Caddy as a service:
sudo systemctl reload caddy

# Or using Docker:
# docker run -d -p 80:80 -p 443:443 -v $PWD/Caddyfile:/etc/caddy/Caddyfile caddy
```

## Updating

```bash
cd /home/user/buildorbail
git pull
cd deploy
docker compose up -d --build
```

## Useful Commands

```bash
# Restart
docker compose restart

# Stop
docker compose down

# View logs
docker compose logs -f app

# Shell into container
docker compose exec app sh
```

## Troubleshooting

### Container won't start
```bash
docker compose logs app
# Check .env has required variables
```

### Can't connect to API
```bash
# Check if port is open on firewall
sudo ufw allow 5897
```

### Memory issues
```bash
# Check Docker memory limits
docker stats
```
