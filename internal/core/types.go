package core

import (
	"os"
	"time"
)

// ── TYPES ───────────────────────────────────────────────────────────────────

type Idea struct {
	ID             int        `json:"id"`
	Subs           []string   `json:"subs"`
	PostsFound     int        `json:"postsFound"`
	SampleUpvotes  int        `json:"sampleUpvotes"`
	SampleComments int        `json:"sampleComments"`
	SamplePost     string     `json:"samplePost"`
	SampleLink     string     `json:"sampleLink,omitempty"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Problem        string     `json:"problem"`
	TargetUser     string     `json:"targetUser"`
	Solution       string     `json:"solution"`
	Monetization   string     `json:"monetization"`
	Competitors    string     `json:"competitors"`
	Scores         IdeaScores `json:"scores"`
	Total          int        `json:"total"`
	VerdictType    string     `json:"verdictType"`
	VerdictLabel   string     `json:"verdictLabel"`
}

type IdeaScores struct {
	MarketSize    int `json:"marketSize"`
	PainIntensity int `json:"painIntensity"`
	SolutionGap   int `json:"solutionGap"`
	Monetization  int `json:"monetization"`
}

type IdeasResponse struct {
	Ideas     []Idea `json:"ideas"`
	Query     string `json:"query"`
	FetchedAt string `json:"fetchedAt"`
	Source    string `json:"source"`
}

type RedditPost struct {
	ID       string  `json:"id"`
	Sub      string  `json:"subreddit"`
	Title    string  `json:"title"`
	Body     string  `json:"selftext"`
	Score    int     `json:"score"`
	Upvotes  int     `json:"ups"`
	Comments int     `json:"num_comments"`
	URL      string  `json:"url"`
	Perma    string  `json:"permalink"`
	Created  float64 `json:"created_utc"`
}

func EnrichIdeasWithRedditData(ideas []Idea, posts []RedditPost) []Idea {
	for i := range ideas {
		ideas[i].ID = i + 1
		if ideas[i].Subs == nil {
			ideas[i].Subs = []string{}
		}
		if ideas[i].PostsFound == 0 && len(posts) > 0 {
			ideas[i].PostsFound = len(posts)
		}
		if ideas[i].SamplePost == "" && len(posts) > 0 {
			p := posts[0]
			if len(p.Body) > 200 {
				ideas[i].SamplePost = p.Body[:197] + "..."
			} else {
				ideas[i].SamplePost = p.Body
			}
			if p.Perma != "" {
				ideas[i].SampleLink = p.Perma
			} else {
				ideas[i].SampleLink = p.URL
			}
			ideas[i].SampleUpvotes = p.Upvotes
			ideas[i].SampleComments = p.Comments
		}
	}
	return ideas
}

func CachedNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
