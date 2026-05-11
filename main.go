package main

import (
	"bufio"
	"buildorbail/internal/ai"
	"buildorbail/internal/core"
	"buildorbail/internal/db"
	"buildorbail/internal/discovery"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// ── ENV ──────────────────────────────────────────────────────────────────────

func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		// Only log if not in Docker/already has some env vars
		if os.Getenv("PORT") == "" && os.Getenv("LLM_PROVIDER") == "" {
			log.Printf("no %s file found, using system environment", path)
		}
		return
	}
	defer f.Close()

	for scanner := bufio.NewScanner(f); scanner.Scan(); {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
			log.Printf("  %s=%s", k, v)
		} else {
			log.Printf("  %s=%s (from env, .env skipped)", k, os.Getenv(k))
		}
	}
}

// ── API: /api/ideas ──────────────────────────────────────────────────────────

func handleIdeas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	// If query is empty, discovery.FetchRedditPosts will automatically run the intent queries.

	// Provider: query param overrides env, default is openrouter
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = core.EnvOr("LLM_PROVIDER", "openrouter")
	}

	log.Printf("→ fetching reddit for: %q (provider=%s)", query, provider)

	posts, err := discovery.FetchRedditPosts(query, 100)
	if err != nil {
		log.Printf("  reddit error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   err.Error(),
			"type":    "reddit_error",
			"message": "Failed to fetch posts from Reddit. Try again later.",
		})
		return
	}
	log.Printf("  got %d posts from reddit", len(posts))

	// Filter previously-seen threads before LLM (dedup by Reddit thread URL)
	freshPosts, skipped, err := db.FilterUnindexedPosts(posts)
	if err != nil {
		log.Printf("  [db warn] Thread indexing unavailable, proceeding with all posts: %v", err)
	} else {
		posts = freshPosts
		if skipped > 0 {
			log.Printf("  skipped %d already-indexed threads", skipped)
		}
	}

	if len(posts) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":   "no posts found",
			"type":    "empty_result",
			"message": "No posts found for this query. Try a different search term.",
		})
		return
	}

	ideas, err := ai.CallLLMDispatch(posts, provider)
	if err != nil {
		log.Printf("  llm error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   err.Error(),
			"type":    "llm_error",
			"message": "AI processing failed. The service may be temporarily unavailable.",
		})
		return
	}

	ideas = core.EnrichIdeasWithRedditData(ideas, posts)

	// Mark threads as seen so they won't be re-processed
	if err := db.IndexThreads(posts); err != nil {
		log.Printf("  [db warn] Failed to index threads: %v", err)
	}

	resp := core.IdeasResponse{
		Ideas:     ideas,
		Query:     query,
		FetchedAt: core.CachedNow(),
		Source:    provider,
	}

	log.Printf("  ← %d ideas from %s", len(ideas), provider)
	writeJSON(w, http.StatusOK, resp)
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var idea core.Idea
	if err := json.NewDecoder(r.Body).Decode(&idea); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	if err := db.SaveIdea(idea); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save idea"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func handleGetSaved(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ideas, err := db.GetSavedIdeas()
	if err != nil {
		log.Printf("  [db error] Failed to fetch saved ideas: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch saved ideas"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ideas": ideas,
	})
}

func handleUnsave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if err := db.DeleteSavedIdea(req.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove saved idea"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsaved"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ── MAIN ────────────────────────────────────────────────────────────────────

func main() {
	// Look for .env in current and parent dirs
	loadEnv(".env")
	if os.Getenv("PORT") == "" {
		loadEnv("../../.env")
	}

	// Initialize Turso DB
	db.InitDB()

	port := core.EnvOr("PORT", "5897")

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/ideas", handleIdeas)
	mux.HandleFunc("/api/save", handleSave)
	mux.HandleFunc("/api/saved", handleGetSaved)
	mux.HandleFunc("/api/unsave", handleUnsave)
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"showMock": core.EnvOr("SHOW_MOCK", "true") == "true",
			"provider": core.EnvOr("LLM_PROVIDER", "openrouter"),
		})
	})

	// Static files from web/ folder
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		path := r.URL.Path
		if path == "/" {
			path = "/app.html"
		}

		// Try root first, then web/
		fpath := "." + path
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			fpath = "web" + path
		}

		http.ServeFile(w, r, fpath)
	})

	log.Printf("Build or Bail running → http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
