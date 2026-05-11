package discovery

import (
	"buildorbail/internal/core"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
		Children []struct {
			Data core.RedditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// FetchRedditPosts runs one or more queries and returns deduplicated, filtered posts.
// If query is empty, it runs all three intent queries.
// If query is provided, it runs only that query (existing behavior preserved).
func FetchRedditPosts(query string, limit int) ([]core.RedditPost, error) {
	queries := []string{query}
	if query == "" {
		queries = intentQueries
	}

	seen := make(map[string]bool)
	var all []core.RedditPost

	for i, q := range queries {
		if i > 0 {
			// Small delay between queries to avoid rate limits
			time.Sleep(1500 * time.Millisecond)
		}

		log.Printf("  running query %d/%d: %s", i+1, len(queries), q)
		posts, err := fetchSingle(q, limit)
		if err != nil {
			// log and continue rather than aborting the whole batch
			log.Printf("  query failed, skipping: %v", err)
			continue
		}
		log.Printf("    found %d raw posts", len(posts))

		for _, p := range posts {
			if !seen[p.ID] {
				seen[p.ID] = true
				all = append(all, p)
			}
		}
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no posts found across all queries")
	}

	return all, nil
}

// fetchSingle hits the Reddit search API for one query.
func fetchSingle(query string, limit int) ([]core.RedditPost, error) {
	u := fmt.Sprintf(
		"https://www.reddit.com/search.json?q=%s&sort=top&t=week&limit=%d",
		url.QueryEscape(query),
		limit,
	)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	
	// Use a realistic browser User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching reddit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("    [reddit error] status=%d body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("reddit returned %d: %s", resp.StatusCode, string(body))
	}

	var result redditSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding reddit json: %w", err)
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

	return posts, nil
}
