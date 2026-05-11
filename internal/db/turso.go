package db

import (
	"buildorbail/internal/core"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// ── TURSO REST API ──────────────────────────────────────────────────────────

type tursoRequest struct {
	Requests []tursoStatement `json:"requests"`
}

type tursoStatement struct {
	Type string    `json:"type"`
	Stmt tursoStmt `json:"stmt"`
}

type tursoStmt struct {
	Sql  string     `json:"sql"`
	Args []tursoArg `json:"args,omitempty"`
}

type tursoArg struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type tursoResponse struct {
	Results []struct {
		Type     string `json:"type"`
		Response struct {
			Result struct {
				Cols []struct {
					Name string `json:"name"`
				} `json:"cols"`
				Rows [][]interface{} `json:"rows"`
			} `json:"result"`
		} `json:"response"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	} `json:"results"`
}

func getTursoConfig() (string, string, error) {
	url := os.Getenv("TURSO_DB_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")

	if url == "" || token == "" {
		return "", "", fmt.Errorf("turso credentials missing in .env")
	}
	return url, token, nil
}

func execTurso(sql string, args []tursoArg) error {
	dbUrl, token, err := getTursoConfig()
	if err != nil {
		return err
	}

	reqBody := tursoRequest{
		Requests: []tursoStatement{
			{
				Type: "execute",
				Stmt: tursoStmt{
					Sql:  sql,
					Args: args,
				},
			},
			{Type: "close"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling turso request: %w", err)
	}

	// Turso HTTP API endpoint
	apiURL := dbUrl + "/v2/pipeline"

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("building turso request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling turso: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("turso returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var tResp tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return fmt.Errorf("decoding turso response: %w", err)
	}

	// Check for errors in the individual statement results
	for _, result := range tResp.Results {
		if result.Error != nil {
			return fmt.Errorf("turso sql error: %s", result.Error.Message)
		}
	}

	return nil
}

// InitDB creates the ideas table if it doesn't exist
func InitDB() {
	_, _, err := getTursoConfig()
	if err != nil {
		log.Println("  [db] Turso not configured, skipping DB init")
		return
	}

	sql := `
	CREATE TABLE IF NOT EXISTS buildorbail_ideas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		summary TEXT,
		problem TEXT,
		target_user TEXT,
		solution TEXT,
		monetization TEXT,
		total_score INTEGER,
		verdict TEXT,
		sample_post TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	err = execTurso(sql, nil)
	if err != nil {
		log.Printf("  [db error] Failed to initialize table: %v", err)
	} else {
		log.Println("  [db] Turso database initialized")
	}

	// Lightweight schema migrations for previously-created tables.
	addColumnIfMissing("buildorbail_ideas", "competitors TEXT")
	addColumnIfMissing("buildorbail_ideas", "verdict_label TEXT")
	addColumnIfMissing("buildorbail_ideas", "subs_json TEXT")
	addColumnIfMissing("buildorbail_ideas", "posts_found INTEGER")
	addColumnIfMissing("buildorbail_ideas", "sample_upvotes INTEGER")
	addColumnIfMissing("buildorbail_ideas", "sample_comments INTEGER")
	addColumnIfMissing("buildorbail_ideas", "score_market_size INTEGER")
	addColumnIfMissing("buildorbail_ideas", "score_pain_intensity INTEGER")
	addColumnIfMissing("buildorbail_ideas", "score_solution_gap INTEGER")
	addColumnIfMissing("buildorbail_ideas", "score_monetization INTEGER")
}

// SaveIdea saves a single idea to Turso when the user explicitly clicks 'Build'
func SaveIdea(idea core.Idea) error {
	subsJSON, _ := json.Marshal(idea.Subs)
	sql := `
	INSERT INTO buildorbail_ideas (
		title, summary, problem, target_user, solution, monetization, competitors,
		total_score, verdict, verdict_label, sample_post, subs_json, posts_found,
		sample_upvotes, sample_comments, score_market_size, score_pain_intensity,
		score_solution_gap, score_monetization
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	args := []tursoArg{
		{Type: "text", Value: idea.Title},
		{Type: "text", Value: idea.Summary},
		{Type: "text", Value: idea.Problem},
		{Type: "text", Value: idea.TargetUser},
		{Type: "text", Value: idea.Solution},
		{Type: "text", Value: idea.Monetization},
		{Type: "text", Value: idea.Competitors},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.Total)},
		{Type: "text", Value: idea.VerdictType},
		{Type: "text", Value: idea.VerdictLabel},
		{Type: "text", Value: idea.SamplePost},
		{Type: "text", Value: string(subsJSON)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.PostsFound)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.SampleUpvotes)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.SampleComments)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.Scores.MarketSize)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.Scores.PainIntensity)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.Scores.SolutionGap)},
		{Type: "integer", Value: fmt.Sprintf("%d", idea.Scores.Monetization)},
	}

	err := execTurso(sql, args)
	if err != nil {
		log.Printf("  [db error] Failed to save idea '%s': %v", idea.Title, err)
		return err
	}

	log.Printf("  [db] Saved manually selected idea to Turso: %s", idea.Title)
	return nil
}

// DeleteSavedIdea removes a saved idea by database row id.
func DeleteSavedIdea(id int) error {
	sql := `DELETE FROM buildorbail_ideas WHERE id = ?`
	args := []tursoArg{
		{Type: "integer", Value: fmt.Sprintf("%d", id)},
	}
	if err := execTurso(sql, args); err != nil {
		log.Printf("  [db error] Failed to delete saved idea id=%d: %v", id, err)
		return err
	}
	log.Printf("  [db] Removed saved idea id=%d", id)
	return nil
}

func parseTursoValue(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	val, ok := m["value"].(string)
	if !ok {
		return ""
	}
	return val
}

func GetSavedIdeas() ([]core.Idea, error) {
	dbUrl, token, err := getTursoConfig()
	if err != nil {
		return nil, err
	}

	sql := `
		SELECT
			id, title, summary, problem, target_user, solution, monetization, competitors,
			total_score, verdict, verdict_label, sample_post, subs_json, posts_found,
			sample_upvotes, sample_comments, score_market_size, score_pain_intensity,
			score_solution_gap, score_monetization
		FROM buildorbail_ideas
		ORDER BY created_at DESC`

	reqBody := tursoRequest{
		Requests: []tursoStatement{
			{
				Type: "execute",
				Stmt: tursoStmt{Sql: sql},
			},
			{Type: "close"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling turso request: %w", err)
	}

	req, err := http.NewRequest("POST", dbUrl+"/v2/pipeline", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("building turso request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling turso: %w", err)
	}
	defer resp.Body.Close()

	var tResp tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, fmt.Errorf("decoding turso response: %w", err)
	}

	if len(tResp.Results) == 0 || tResp.Results[0].Error != nil {
		errMsg := "unknown error"
		if len(tResp.Results) > 0 && tResp.Results[0].Error != nil {
			errMsg = tResp.Results[0].Error.Message
		}
		return nil, fmt.Errorf("turso fetch error: %s", errMsg)
	}

	var ideas []core.Idea
	rows := tResp.Results[0].Response.Result.Rows
	for _, row := range rows {
		if len(row) < 20 {
			continue
		}

		idStr := parseTursoValue(row[0])
		id, _ := strconv.Atoi(idStr)

		scoreStr := parseTursoValue(row[8])
		totalScore, _ := strconv.Atoi(scoreStr)
		postsFound, _ := strconv.Atoi(parseTursoValue(row[13]))
		sampleUpvotes, _ := strconv.Atoi(parseTursoValue(row[14]))
		sampleComments, _ := strconv.Atoi(parseTursoValue(row[15]))
		scoreMarketSize, _ := strconv.Atoi(parseTursoValue(row[16]))
		scorePainIntensity, _ := strconv.Atoi(parseTursoValue(row[17]))
		scoreSolutionGap, _ := strconv.Atoi(parseTursoValue(row[18]))
		scoreMonetization, _ := strconv.Atoi(parseTursoValue(row[19]))

		var subs []string
		subsRaw := parseTursoValue(row[12])
		if subsRaw != "" {
			_ = json.Unmarshal([]byte(subsRaw), &subs)
		}

		idea := core.Idea{
			ID:             id,
			Title:          parseTursoValue(row[1]),
			Summary:        parseTursoValue(row[2]),
			Problem:        parseTursoValue(row[3]),
			TargetUser:     parseTursoValue(row[4]),
			Solution:       parseTursoValue(row[5]),
			Monetization:   parseTursoValue(row[6]),
			Competitors:    parseTursoValue(row[7]),
			Total:          totalScore,
			VerdictType:    parseTursoValue(row[9]),
			VerdictLabel:   parseTursoValue(row[10]),
			SamplePost:     parseTursoValue(row[11]),
			Subs:           subs,
			PostsFound:     postsFound,
			SampleUpvotes:  sampleUpvotes,
			SampleComments: sampleComments,
			Scores: core.IdeaScores{
				MarketSize:    scoreMarketSize,
				PainIntensity: scorePainIntensity,
				SolutionGap:   scoreSolutionGap,
				Monetization:  scoreMonetization,
			},
		}
		ideas = append(ideas, idea)
	}

	return ideas, nil
}

func addColumnIfMissing(table, definition string) {
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, definition)
	if err := execTurso(sql, nil); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return
		}
		log.Printf("  [db warn] Migration skipped for %s (%s): %v", table, definition, err)
	}
}
