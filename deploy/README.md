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
#   GOOGLE_AI_API_KEY=...
#   TURSO_DB_URL=https://...
#   TURSO_AUTH_TOKEN=...
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

## Production Deployment (with reverse proxy)

### Nginx config example:

```nginx
server {
    listen 80;
    server_name yourdomain.com;

    location / {
        proxy_pass http://localhost:${HOST_PORT:-5897};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_cache_bypass $http_upgrade;
    }
}
```

### SSL with Certbot:

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d yourdomain.com
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
