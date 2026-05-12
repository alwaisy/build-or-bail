# Build or Bail

Scans Reddit for real product frustrations, sends them to an LLM for analysis, and returns scored startup ideas with build-or-bail verdicts. Go backend, vanilla HTML frontend, zero dependencies.

## What It Does

1. Searches Reddit for posts where people complain about tools, products, and workflows
2. Filters for high-engagement threads (10+ upvotes, 50+ comments)
3. Sends filtered posts to an LLM with a scoring prompt
4. Returns scored startup ideas: market size, pain intensity, solution gap, monetization potential
5. Each idea gets a verdict: **Build** (75+), **Maybe** (50-74), or **Bail** (<50)
6. Users sign in with **email + generated access key** (no password reset flow)
7. Dedup is per-user and based on prior **Build/Bail decisions**

## Quick Start

```bash
# Clone
git clone <repo-url> && cd build-or-bail-project/vanilla-version

# Set up environment
cp .env.example .env
# Add your API key(s) to .env:
#   OPENROUTER_API_KEY=sk-...
#   TURSO_DB_URL=https://...
#   TURSO_AUTH_TOKEN=...

# Run
go run .

# Open http://localhost:5897
```

## LLM Providers

| Provider | Env Var | Default Model |
|----------|---------|---------------|
| OpenRouter (default) | `OPENROUTER_API_KEY` | `deepseek/deepseek-chat-v3-0324:free` |
| Google AI Studio | `GOOGLE_AI_API_KEY` | `gemini-3.1-flash-lite` |
| Vertex AI | `VERTEX_PROJECT_ID` + `gcloud auth login` | `gemini-3.1-flash-lite` |

Override per-request: `GET /api/ideas?q=frustrated&provider=google`

## API

```
POST /api/auth/register                            Create email + access key
POST /api/auth/login                               Sign in with email + access key
GET  /api/ideas?q=<query>&provider=<provider>   Generate ideas
GET  /api/ideas                                  Mega-query (runs 3 intent queries)
GET  /api/config                                 App config (showMock, provider)
POST /api/decision                               Record build/bail decision
POST /api/save                                   Save an idea
GET  /api/saved                                  Get saved ideas
POST /api/unsave                                 Remove a saved idea
```

Protected endpoints require:

```
X-User-Email: <email>
X-User-Token: <access key>
```

## Deploy to VPS

```bash
# Copy project to VPS
scp -r . user@vps:/opt/buildorbail/

# On VPS
cd /opt/buildorbail/deploy
docker compose up -d --build
```

Caddy reverse proxy:
```caddy
buildorbail.yourdomain.com {
    reverse_proxy localhost:5897
    encode zstd gzip
}
```

See `deploy/README.md` for the full deployment guide with Caddy and SSL.

## Project Structure

```
main.go                     HTTP server and routes
internal/
  core/types.go             Shared types (Idea, RedditPost)
  ai/provider.go            LLM provider dispatch
  ai/openrouter.go          OpenRouter client
  ai/vertex.go              Google AI + Vertex AI clients
  ai/prompt.go              Prompt embedding via //go:embed
  ai/prompts/               Prompt templates (base.md, rules.md, voice.md)
  discovery/reddit.go       Reddit search with intent queries
  db/turso.go               Turso SQLite for saved ideas
web/
  app.html                  Single-page frontend
  api.js                    API client + auth session helpers
  mock.js                   Mock data for development
deploy/
  Dockerfile                Multi-stage Docker build
  docker-compose.yml        Container orchestration
```

## Build

```bash
go build -o buildorbail .
go vet ./...
gofmt -w .
```
