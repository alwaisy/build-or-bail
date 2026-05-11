# Deploy Build or Bail

### 1. Update Code & Clear Stale Files
```bash
git pull
# Remove stray files that override the /web folder
rm -f ./api.js ./mock.js ./app.html
```

### 2. Build & Restart (Force Fresh)
```bash
cd deploy
docker compose down
docker compose build --no-cache
docker compose up -d
```

### 3. Caddy Config
Update `/etc/caddy/Caddyfile` with the config below (ensures JS updates take effect):

```caddy
buildorbail.alwaisy.dev {
    reverse_proxy localhost:5897
    encode zstd gzip

    # 1. Scripts and styles (Must revalidate to avoid stale code)
    @scripts path *.js *.css
    header @scripts Cache-Control "public, max-age=3600, must-revalidate"

    # 2. Truly static assets
    @static path *.woff* *.woff2 *.ttf *.otf *.ico *.png *.jpg *.jpeg *.gif *.svg *.webp
    header @static Cache-Control "public, max-age=31536000, immutable"

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "strict-origin-when-cross-origin"
        -Server ""
    }
}
```

Then run:
```bash
sudo systemctl reload caddy
```

### 4. Troubleshooting ReferenceError
If the browser still shows an error, it is because of the old `immutable` header. 
**Open in Incognito** or manually clear site data in your browser to break the old cache.
