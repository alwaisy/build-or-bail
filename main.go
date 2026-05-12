package main

import (
	"bufio"
	"buildorbail/internal/ai"
	"buildorbail/internal/core"
	"buildorbail/internal/db"
	"buildorbail/internal/discovery"
	"encoding/json"
	"fmt"
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

func requireUser(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	email := strings.TrimSpace(r.Header.Get("X-User-Email"))
	token := strings.TrimSpace(r.Header.Get("X-User-Token"))
	if email == "" || token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"type":    "auth_required",
			"error":   "missing auth headers",
			"message": "Please sign in to continue.",
		})
		return db.User{}, false
	}

	user, err := db.AuthenticateUser(email, token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"type":    "auth_invalid",
			"error":   err.Error(),
			"message": "Your access key is invalid. Sign in again.",
		})
		return db.User{}, false
	}
	return user, true
}

func handleIdeas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")

	// Provider: query param overrides env, default is openrouter
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = core.EnvOr("LLM_PROVIDER", "openrouter")
	}

	// Pagination: cursors and batch info
	cursorsStr := r.URL.Query().Get("cursors")
	batchNum := 0
	if bn := r.URL.Query().Get("batchNum"); bn != "" {
		fmt.Sscanf(bn, "%d", &batchNum)
	}
	totalBatches := 0
	if tb := r.URL.Query().Get("totalBatches"); tb != "" {
		fmt.Sscanf(tb, "%d", &totalBatches)
	}

	var cursors []string
	if cursorsStr != "" {
		// cursors is comma-separated after tokens
		parts := strings.Split(cursorsStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cursors = append(cursors, p)
			} else {
				cursors = append(cursors, "")
			}
		}
	}
	// Ensure 3 cursor slots for 3 intent queries
	for len(cursors) < 3 {
		cursors = append(cursors, "")
	}

	// seen: comma-separated post IDs already processed
	seenIDs := make(map[string]bool)
	if seenStr := r.URL.Query().Get("seen"); seenStr != "" {
		for _, id := range strings.Split(seenStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				seenIDs[id] = true
			}
		}
	}

	log.Printf("→ fetching reddit for: %q (provider=%s, batch=%d, cursors=%v)", query, provider, batchNum, cursors)

	result, err := discovery.FetchRedditPosts(query, 100, cursors, seenIDs)
	if err != nil {
		log.Printf("  reddit error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   err.Error(),
			"type":    "reddit_error",
			"message": "Failed to fetch posts from Reddit. Try again later.",
		})
		return
	}
	log.Printf("  got %d posts from reddit (%d new)", len(result.Posts), len(result.PostIDs))

	if len(result.Posts) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":   "no posts found",
			"type":    "empty_result",
			"message": "No posts found for this query. Try a different search term.",
		})
		return
	}

	ideas, err := ai.CallLLMDispatch(result.Posts, provider)
	if err != nil {
		log.Printf("  llm error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   err.Error(),
			"type":    "llm_error",
			"message": "AI processing failed. The service may be temporarily unavailable.",
		})
		return
	}

	ideas = core.EnrichIdeasWithRedditData(ideas, result.Posts)
	filteredIdeas, skipped, err := db.FilterUndecidedIdeas(user.ID, ideas)
	if err != nil {
		log.Printf("  [db warn] decision dedup unavailable, returning all ideas: %v", err)
	} else {
		ideas = filteredIdeas
		if skipped > 0 {
			log.Printf("  skipped %d already-decided ideas", skipped)
		}
	}
	if len(ideas) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":   "no fresh ideas",
			"type":    "empty_result",
			"message": "No fresh ideas left right now. Try again later.",
		})
		return
	}

	currentBatch := batchNum + 1
	if totalBatches > 0 && currentBatch > totalBatches {
		totalBatches = currentBatch
	} else if totalBatches == 0 {
		totalBatches = currentBatch
	}

	resp := core.IdeasResponse{
		Ideas:        ideas,
		Query:        query,
		FetchedAt:    core.CachedNow(),
		Source:       provider,
		BatchNum:     currentBatch,
		TotalBatches: totalBatches,
		HasMore:      result.HasMore,
		Cursors:      result.Cursors,
		PostIds:      result.PostIDs,
	}

	log.Printf("  ← %d ideas from %s (batch %d/%d, hasMore=%v)", len(ideas), provider, currentBatch, totalBatches, result.HasMore)
	writeJSON(w, http.StatusOK, resp)
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var idea core.Idea
	if err := json.NewDecoder(r.Body).Decode(&idea); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	if err := db.SaveIdea(user.ID, idea); err != nil {
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	ideas, err := db.GetSavedIdeas(user.ID)
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if err := db.DeleteSavedIdea(user.ID, req.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove saved idea"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsaved"})
}

func handleDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Action string    `json:"action"`
		Idea   core.Idea `json:"idea"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	if err := db.RecordDecision(user.ID, req.Idea, req.Action); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"type":    "auth_error",
			"error":   "invalid json body",
			"message": "Please enter your email to continue.",
		})
		return
	}

	user, err := db.RegisterUser(req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"type":    "auth_error",
			"error":   err.Error(),
			"message": "Could not create account with that email.",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"email":       user.Email,
		"accessToken": user.Token,
	})
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email       string `json:"email"`
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"type":    "auth_error",
			"error":   "invalid json body",
			"message": "Enter email and access key.",
		})
		return
	}

	user, err := db.AuthenticateUser(req.Email, req.AccessToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"type":    "auth_invalid",
			"error":   err.Error(),
			"message": "Email or access key did not match.",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"email":       user.Email,
		"accessToken": user.Token,
	})
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
	mux.HandleFunc("/api/auth/register", handleAuthRegister)
	mux.HandleFunc("/api/auth/login", handleAuthLogin)
	mux.HandleFunc("/api/ideas", handleIdeas)
	mux.HandleFunc("/api/decision", handleDecision)
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

		// Prevent stale frontend bundles in browsers after deploys.
		switch {
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Cache-Control", "no-store")
		case strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".css"):
			w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
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
