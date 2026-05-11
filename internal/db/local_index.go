package db

import (
	"buildorbail/internal/core"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var indexMu sync.Mutex

type localIndexDB struct {
	Threads map[string]indexedThread `json:"threads"`
}

type indexedThread struct {
	ThreadLink string `json:"threadLink"`
	FirstSeen  string `json:"firstSeen"`
	LastSeen   string `json:"lastSeen"`
	Query      string `json:"query"`
	Provider   string `json:"provider"`
	Saved      bool   `json:"saved"`
}

func localIndexPath() string {
	if v := os.Getenv("LOCAL_INDEX_DB_PATH"); v != "" {
		return v
	}
	return "data/idea_index_db.json"
}

func loadLocalIndex(path string) (localIndexDB, error) {
	var state localIndexDB
	state.Threads = map[string]indexedThread{}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(b) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	if state.Threads == nil {
		state.Threads = map[string]indexedThread{}
	}
	return state, nil
}

func saveLocalIndex(path string, state localIndexDB) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func normalizeThreadLink(p core.RedditPost) string {
	if p.Perma != "" {
		if strings.HasPrefix(p.Perma, "http://") || strings.HasPrefix(p.Perma, "https://") {
			return p.Perma
		}
		return "https://www.reddit.com" + p.Perma
	}
	if p.URL != "" {
		return p.URL
	}
	return "reddit:id:" + p.ID
}

func NormalizeThreadLinkFromValue(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	if strings.HasPrefix(link, "/r/") {
		return "https://www.reddit.com" + link
	}
	return link
}

func FilterUnindexedPosts(posts []core.RedditPost) ([]core.RedditPost, int, error) {
	indexMu.Lock()
	defer indexMu.Unlock()

	path := localIndexPath()
	state, err := loadLocalIndex(path)
	if err != nil {
		return nil, 0, fmt.Errorf("loading local index db: %w", err)
	}

	filtered := make([]core.RedditPost, 0, len(posts))
	skipped := 0
	for _, p := range posts {
		key := normalizeThreadLink(p)
		if _, exists := state.Threads[key]; exists {
			skipped++
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, skipped, nil
}

func IndexGeneratedPosts(posts []core.RedditPost, query, provider string) error {
	indexMu.Lock()
	defer indexMu.Unlock()

	path := localIndexPath()
	state, err := loadLocalIndex(path)
	if err != nil {
		return fmt.Errorf("loading local index db: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range posts {
		key := normalizeThreadLink(p)
		if existing, ok := state.Threads[key]; ok {
			existing.LastSeen = now
			existing.Query = query
			existing.Provider = provider
			state.Threads[key] = existing
			continue
		}
		state.Threads[key] = indexedThread{
			ThreadLink: key,
			FirstSeen:  now,
			LastSeen:   now,
			Query:      query,
			Provider:   provider,
			Saved:      false,
		}
	}

	if err := saveLocalIndex(path, state); err != nil {
		return fmt.Errorf("saving local index db: %w", err)
	}
	return nil
}

func MarkThreadSaved(threadLink string) error {
	threadLink = NormalizeThreadLinkFromValue(threadLink)
	if threadLink == "" {
		return nil
	}

	indexMu.Lock()
	defer indexMu.Unlock()

	path := localIndexPath()
	state, err := loadLocalIndex(path)
	if err != nil {
		return fmt.Errorf("loading local index db: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	record, ok := state.Threads[threadLink]
	if !ok {
		record = indexedThread{
			ThreadLink: threadLink,
			FirstSeen:  now,
		}
	}
	record.LastSeen = now
	record.Saved = true
	state.Threads[threadLink] = record

	if err := saveLocalIndex(path, state); err != nil {
		return fmt.Errorf("saving local index db: %w", err)
	}
	return nil
}
