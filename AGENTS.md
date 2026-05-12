# Build or Bail

Go backend that fetches Reddit posts about product frustrations, sends them to an LLM (OpenRouter, Google AI, or Vertex AI) for analysis, and returns scored "build or bail" startup ideas. Single HTML frontend served from the same binary. Persists saved ideas via Turso SQLite; deduplicates processed threads in browser IndexedDB.

## Build & Test

- Build binary: `go build -o buildorbail .`
- Run development server: `go run .`
- Test single endpoint: `curl "http://localhost:5897/api/ideas?q=frustrated"`
- Test empty query (mega-query): `curl "http://localhost:5897/api/ideas"`
- Test config endpoint: `curl "http://localhost:5897/api/config"`
- Test saved ideas: `curl "http://localhost:5897/api/saved"`
- Lint: `go vet ./...`
- Format: `gofmt -w .`

## Project Layout

```
├─ main.go                          → HTTP server, routes, .env loading
├─ go.mod                           → Go 1.26.2, zero external dependencies
├─ internal/
│  ├─ core/types.go                 → Shared types (Idea, RedditPost, IdeasResponse), EnvOr helper
│  ├─ ai/
│  │  ├─ provider.go                → LLM provider dispatch (openrouter/google/vertex)
│  │  ├─ openrouter.go             → OpenRouter chat completions API client
│  │  ├─ vertex.go                  → Google AI + Vertex AI clients (Vertex uses gcloud token)
│  │  ├─ prompt.go                  → Embeds prompts via //go:embed
│  │  └─ prompts/                   → Markdown prompt files (base.md, rules.md, voice.md)
│  ├─ discovery/reddit.go          → Reddit search with intent queries and engagement filtering
│  └─ db/
│     └─ turso.go                   → Turso SQLite REST API client (persists saved ideas)
├─ web/
│  ├─ app.html                      → Single-page frontend (~71KB vanilla HTML/JS/CSS)
│  ├─ api.js                        → Frontend API client + IndexedDB thread index
│  └─ mock.js                       → Mock data fallback for development
└─ .gitignore                       → Ignores binaries, .env, my-office/, .gemini/, .factory/
```

## Architecture Overview

**Request flow:** `GET /api/ideas?q=<query>&provider=<provider>&cursors=<after>&seen=<postIds>&batchNum=<n>` → if empty query, runs 3 intent queries in batch → fetches Reddit posts with `after` cursor pagination (top, this week, limit 100/query) → filters 10+ upvotes AND 50+ comments, excludes meme/gaming/funny/jokes subreddits, skips already-seen post IDs → dispatches to configured LLM provider with two-part prompt (system=rules.md, user=base.md) → LLM returns JSON array of scored ideas → ideas enriched with Reddit metadata → response JSON with `batchNum`, `totalBatches`, `hasMore`, `cursors`, `postIds`. Frontend persists each batch in IndexedDB and shows `← Batch 1/3 →` navigation.

**Persistence layer:** Turso SQLite via REST API (`TURSO_DB_URL`, `TURSO_AUTH_TOKEN`) stores saved ideas. Browser IndexedDB has 3 stores: `idea_threads` (dedup by thread key), `idea_batches` (persist generated batches with cursors/postIds), `seen_post_ids` (track all post IDs sent to LLM to avoid re-processing). Batches survive page refresh.

**LLM providers:** `openrouter` (default), `google` (Google AI Studio), `vertex` (Google Cloud Vertex AI). All share prompt templates embedded via `//go:embed`. Provider can be overridden per-request via `?provider=` query param.

**Error types:** `reddit_error` (fetch failure), `empty_result` (no posts), `llm_error` (AI processing). All return JSON with `type`, `error`, `message`.

**Binary:** ~10MB, zero external deps beyond Go standard library. No server-side database or file-based caching.

## Development Patterns & Constraints

### Go Conventions
- Go 1.26.2 module: `buildorbail` (zero external deps in go.mod)
- Package layout: `internal/` for private packages (`core`, `ai`, `discovery`, `db`), root for `main`
- Error handling: `fmt.Errorf` with `%w` wrapping, returned up the stack, logged at handler level
- JSON: struct tags on all serializable types, `json.NewEncoder`/`json.NewDecoder` for I/O
- Naming: PascalCase exported, camelCase unexported, snake_case JSON fields
- Imports: standard library first, then internal packages grouped by function
- HTTP: `net/http` standard mux (`http.NewServeMux`), no framework, handlers use early returns
- Async: synchronous blocking calls. No goroutines or channels
- Embed: `//go:embed` for prompt .md files (requires rebuild after changes)

### Frontend Conventions
- Vanilla HTML/JS/CSS in single `app.html` (~71KB, 1800+ lines)
- No build tooling, no bundler, no framework
- Chart.js via CDN (`cdn.jsdelivr.net/npm/chart.js`) for radar charts
- API calls via `fetch()` with inline error handling in `api.js`
- Mock data in `mock.js` loaded when `SHOW_MOCK=true` in `.env`
- State via global `APP_CONFIG` object and DOM manipulation
- iOS-style mobile-first design (max-width 430px shell, -apple-system font)
- IndexedDB v2 with 3 stores: `idea_threads` (thread dedup), `idea_batches` (batch persistence), `seen_post_ids` (post-level dedup)
- Batch navigation: `← Batch 1/3 →` UI, prev loads from IndexedDB instantly, next triggers new fetch+LLM if not cached

### Prompt Architecture
- `GetSystemPrompt()` returns rules.md content (HOW to speak: no AI buzzwords, no formulaic patterns, em-dashes replaced)
- `GetPrimaryPrompt()` returns base.md content (WHAT to do: strict JSON array output, scoring rules 0-25 per category, verdict thresholds at 50/75)
- Response parsing: strip markdown code fences, then `json.Unmarshal`

### Reddit Data Filtering
- Minimum engagement: 10+ upvotes AND 50+ comments
- Excludes subreddits containing: memes, gaming, funny, jokes
- User-Agent: `BuildOrBail/1.0`, no authentication
- Searches "top" posts from past week, limit 100 per query

### Intent Queries (Mega-Query)
When `q` is empty, runs 3 queries in batch:
1. `"is there a tool that" OR "is there an app that" OR "I'd pay for" OR "does anyone know a way to"`
2. `"I hate having to" OR "every time I have to" OR "wish there was a way to" OR "manually doing" OR "I just use a spreadsheet"`
3. `"wish it could" OR "doesn't support" OR "missing feature" OR "switched from" OR "looking for alternative"`

## Security

- API keys in `.env` (gitignored): `GOOGLE_AI_API_KEY`, `OPENROUTER_API_KEY`, `VERTEX_PROJECT_ID`, `TURSO_DB_URL`, `TURSO_AUTH_TOKEN`
- Vertex AI requires `gcloud auth login` manually before first use
- No CORS headers (same-origin assumed)
- No rate limiting on `/api/ideas`
- Reddit API accessed without authentication
- LLM responses parsed as JSON without schema validation
- No input sanitization on `q` beyond URL encoding

## Git Workflows

- `.gitignore` ignores: binaries, `.env`, `data/`, `my-office/`, `.gemini/`, `.factory/`, OS files
- Repository not initialized (no `.git` directory)
- No branch protection, commit conventions, or CI/CD established

## Evidence Required for Every PR

- Build succeeds: `go build -o buildorbail .`
- `go vet ./...` passes with no issues
- Code formatted: `gofmt -w .` produces no changes
- No new dependencies added to `go.mod` without justification
- Frontend changes tested at `http://localhost:5897/`
- Prompt changes tested with at least one provider

## API Endpoints

`GET /api/ideas` - Fetch startup ideas
- `q` (optional): search query. Empty = 3 intent queries
- `provider` (optional): `openrouter` | `google` | `vertex`
- `cursors` (optional): comma-separated Reddit `after` tokens, one per query
- `seen` (optional): comma-separated post IDs already processed (dedup)
- `batchNum` (optional): current batch number for tracking
- `totalBatches` (optional): total batches generated so far
- Returns: `{ ideas, query, fetchedAt, source, batchNum, totalBatches, hasMore, cursors, postIds }`

## External Services

- Reddit Search API - no key - `reddit.com/search.json` (top/week)
- OpenRouter - `OPENROUTER_API_KEY` - chat completions, default model: `deepseek/deepseek-chat-v3-0324:free`
- Google AI Studio - `GOOGLE_AI_API_KEY` - generateContent, default model: `gemini-3.1-flash-lite`
- Google Vertex AI - `VERTEX_PROJECT_ID`, `VERTEX_REGION` (default: `us-central1`) - requires `gcloud auth login`
- Turso SQLite - `TURSO_DB_URL`, `TURSO_AUTH_TOKEN` - persists saved ideas via REST API

## Gotchas

- Build with `go build -o buildorbail .` from project root, NOT from `./cmd/buildorbail/`
- Vertex AI endpoint requires `locations/global` in URL path, not region value
- Prompts embedded at compile time via `//go:embed`; rebuild after editing `.md` files
- IndexedDB thread deduplication runs client-side in browser; no server-side index
- `SHOW_MOCK=true` in `.env` enables mock data fallback (useful for frontend development)
- Default port is `5897` (synced with `.env`)
- Provider query param (`?provider=google`) overrides `LLM_PROVIDER` env var per-request
- Turso DB initialized lazily on first save operation; `InitDB()` runs `CREATE TABLE IF NOT EXISTS` + migrations
- **Batch pagination**: Reddit `after` cursors are per-query (3 cursors for 3 intent queries). Pass as comma-separated `?cursors=t3_abc,,t3_xyz` (empty slot = first page)
- **Batch pagination**: `?seen` param filters post IDs already sent to LLM (comma-separated). Backend skips these before dispatching to LLM
- **Batch pagination**: Response includes `hasMore` (bool) and `cursors` ([]string). Frontend persists these in IndexedDB for "Fetch More" continuation
- **Batch pagination**: Each batch is a full LLM call (~15-35s). Already-generated batches load instantly from IndexedDB
- **Batch pagination**: `resetFeed()` clears IndexedDB batch data via `clearBatchData()`. Browser refresh restores last batch from IndexedDB

## Performance Considerations

- Reddit fetch: ~2-5s, LLM call: ~10-30s, total response: 15-35s
- Multi-query mode (empty `q`) runs queries sequentially, one at a time
- No connection pooling beyond `http.DefaultClient`
- No response caching, no CDN
- Single HTML file is ~71KB unminified; Chart.js CDN adds ~240KB

## Deployment

- Build binary: `GOOS=linux GOARCH=amd64 go build -o buildorbail .`
- **Docker** (recommended for VPS):
  - `cd deploy && docker compose up -d --build`
  - See `deploy/README.md` for full VPS deployment guide with Nginx/SSL
- Deploy: binary + `web/` folder + `.env` to any Linux server
- Run: `./buildorbail` (reads `.env` from `.` and `../../.` automatically)
- Docker image based on Alpine Linux 3.19, ~15MB compressed
