package discovery

import (
	"buildorbail/internal/core"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ── QUERY SETS ────────────────────────────────────────────────────────────────

var intentQueries = []string{
	`"is there a tool that" OR "is there an app that" OR "I'd pay for" OR "does anyone know a way to"`,
	`"I hate having to" OR "every time I have to" OR "wish there was a way to" OR "manually doing" OR "I just use a spreadsheet"`,
	`"wish it could" OR "doesn't support" OR "missing feature" OR "switched from" OR "looking for alternative"`,
}

// ── REDDIT ────────────────────────────────────────────────────────────────────

type redditSearchResponse struct {
	Data struct {
		After    string `json:"after"`
		Children []struct {
			Data core.RedditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// FetchResult holds posts, updated cursors, and post IDs for dedup.
type FetchResult struct {
	Posts   []core.RedditPost
	Cursors []string
	PostIDs []string
	HasMore bool
}

// FetchRedditPosts runs one or more queries and returns deduplicated, filtered posts.
// If query is empty, it runs all three intent queries.
// If query is provided, it runs only that query (existing behavior preserved).
// cursors: after tokens for each query (use empty string for first page).
// seenIDs: post IDs already processed (to avoid re-fetching).
func FetchRedditPosts(query string, limit int, cursors []string, seenIDs map[string]bool) (*FetchResult, error) {
	queries := []string{query}
	if query == "" {
		queries = intentQueries
	}

	// Ensure cursors slice matches query count
	for len(cursors) < len(queries) {
		cursors = append(cursors, "")
	}

	querySeen := make(map[string]bool)
	var allPosts []core.RedditPost
	var allIDs []string
	newCursors := make([]string, len(queries))
	hasMore := false

	for i, q := range queries {
		if i > 0 {
			time.Sleep(1500 * time.Millisecond)
		}

		after := ""
		if i < len(cursors) {
			after = cursors[i]
		}

		log.Printf("  running query %d/%d: %s (after=%s)", i+1, len(queries), q, after)
		posts, nextAfter, err := fetchSingle(q, limit, after)
		if err != nil {
			log.Printf("  query failed, skipping: %v", err)
			continue
		}
		newCursors[i] = nextAfter
		if nextAfter != "" {
			hasMore = true
		}
		log.Printf("    found %d raw posts", len(posts))

		for _, p := range posts {
			if seenIDs[p.ID] || querySeen[p.ID] {
				continue
			}
			querySeen[p.ID] = true
			allPosts = append(allPosts, p)
			allIDs = append(allIDs, p.ID)
		}
	}

	if len(allPosts) == 0 {
		return nil, fmt.Errorf("no new posts found across all queries")
	}

	return &FetchResult{
		Posts:   allPosts,
		Cursors: newCursors,
		PostIDs: allIDs,
		HasMore: hasMore,
	}, nil
}

// fetchSingle hits the Reddit search API for one query.
func fetchSingle(query string, limit int, after string) ([]core.RedditPost, string, error) {
	u := fmt.Sprintf(
		"https://www.reddit.com/search.json?q=%s&sort=top&t=week&limit=%d",
		url.QueryEscape(query),
		limit,
	)
	if after != "" {
		u += "&after=" + url.QueryEscape(after)
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building request: %w", err)
	}

	// Use a realistic browser User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	// Setup client with proxy if configured
	client := &http.Client{Timeout: 30 * time.Second}
	transportMode := "direct"
	if proxyURLStr := os.Getenv("PROXY_URL"); proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err != nil {
			log.Printf("    [proxy error] invalid PROXY_URL: %v", err)
		} else {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			transportMode = fmt.Sprintf("proxy(%s)", proxyURL.Host)
		}
	}
	log.Printf("    [reddit] request url=%s transport=%s", req.URL.String(), transportMode)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching reddit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("    [reddit error] status=%d body=%s", resp.StatusCode, string(body))
		return nil, "", fmt.Errorf("reddit returned %d: %s", resp.StatusCode, string(body))
	}

	var result redditSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decoding reddit json: %w", err)
	}

	var posts []core.RedditPost
	for _, child := range result.Data.Children {
		p := child.Data
		if p.Upvotes < 10 || p.Comments < 50 {
			continue
		}
		sub := strings.ToLower(p.Sub)
		if strings.Contains(sub, "memes") || strings.Contains(sub, "gaming") ||
			strings.Contains(sub, "funny") || strings.Contains(sub, "jokes") {
			continue
		}
		posts = append(posts, p)
	}

	return posts, result.Data.After, nil
}
