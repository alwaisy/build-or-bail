package db

import (
	"buildorbail/internal/core"
	"bytes"
	"crypto/rand"
	"encoding/hex"
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

type User struct {
	ID    int
	Email string
	Token string
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

func queryTursoRows(sql string, args []tursoArg) ([][]interface{}, error) {
	dbURL, token, err := getTursoConfig()
	if err != nil {
		return nil, err
	}

	reqBody := tursoRequest{
		Requests: []tursoStatement{
			{
				Type: "execute",
				Stmt: tursoStmt{Sql: sql, Args: args},
			},
			{Type: "close"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling turso request: %w", err)
	}

	req, err := http.NewRequest("POST", dbURL+"/v2/pipeline", bytes.NewReader(bodyBytes))
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

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("turso returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var tResp tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, fmt.Errorf("decoding turso response: %w", err)
	}

	if len(tResp.Results) == 0 {
		return nil, fmt.Errorf("turso returned empty results")
	}
	if tResp.Results[0].Error != nil {
		return nil, fmt.Errorf("turso sql error: %s", tResp.Results[0].Error.Message)
	}

	return tResp.Results[0].Response.Result.Rows, nil
}

// InitDB creates the ideas table if it doesn't exist
func InitDB() {
	_, _, err := getTursoConfig()
	if err != nil {
		log.Println("  [db] Turso not configured, skipping DB init")
		return
	}

	// Ideas table
	execTurso(`CREATE TABLE IF NOT EXISTS buildorbail_ideas (
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
	);`, nil)

	// Lightweight users table for multi-user isolation.
	execTurso(`CREATE TABLE IF NOT EXISTS buildorbail_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		access_token TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`, nil)

	// User decisions: both build + bail are tracked for per-user dedup.
	execTurso(`CREATE TABLE IF NOT EXISTS buildorbail_user_decisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		decision_key TEXT NOT NULL,
		decision TEXT NOT NULL,
		idea_json TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, decision_key)
	);`, nil)
	execTurso(`CREATE INDEX IF NOT EXISTS idx_bob_user_decisions_lookup
		ON buildorbail_user_decisions (user_id, decision);`, nil)

	log.Println("  [db] Turso database initialized")

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

	// Legacy table cleanup (thread-level dedup is no longer used).
	execTurso(`DROP TABLE IF EXISTS buildorbail_threads;`, nil)
}

func normalizedEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func buildDecisionKey(idea core.Idea) string {
	title := strings.ToLower(strings.TrimSpace(idea.Title))
	link := strings.ToLower(strings.TrimSpace(idea.SampleLink))
	sample := strings.ToLower(strings.TrimSpace(idea.SamplePost))
	summary := strings.ToLower(strings.TrimSpace(idea.Summary))
	return fmt.Sprintf("%s::%s::%s::%s", title, link, sample, summary)
}

func newAccessToken() (string, error) {
	buf := make([]byte, 16) // 32 hex chars
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed generating token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func RegisterUser(email string) (User, error) {
	email = normalizedEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return User{}, fmt.Errorf("please enter a valid email")
	}

	token, err := newAccessToken()
	if err != nil {
		return User{}, err
	}

	sql := `INSERT INTO buildorbail_users (email, access_token) VALUES (?, ?)`
	args := []tursoArg{
		{Type: "text", Value: email},
		{Type: "text", Value: token},
	}
	if err := execTurso(sql, args); err != nil {
		if strings.Contains(err.Error(), "buildorbail_users.email") {
			return User{}, fmt.Errorf("email already registered. use your access key to sign in")
		}
		return User{}, err
	}

	user, err := AuthenticateUser(email, token)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func AuthenticateUser(email, token string) (User, error) {
	email = normalizedEmail(email)
	token = strings.TrimSpace(token)
	if email == "" || token == "" {
		return User{}, fmt.Errorf("email and access key are required")
	}

	rows, err := queryTursoRows(
		`SELECT id, email, access_token FROM buildorbail_users WHERE email = ? AND access_token = ? LIMIT 1`,
		[]tursoArg{
			{Type: "text", Value: email},
			{Type: "text", Value: token},
		},
	)
	if err != nil {
		return User{}, err
	}
	if len(rows) == 0 || len(rows[0]) < 3 {
		return User{}, fmt.Errorf("invalid credentials")
	}

	id, _ := strconv.Atoi(parseTursoValue(rows[0][0]))
	return User{
		ID:    id,
		Email: parseTursoValue(rows[0][1]),
		Token: parseTursoValue(rows[0][2]),
	}, nil
}

func RecordDecision(userID int, idea core.Idea, decision string) error {
	if userID <= 0 {
		return fmt.Errorf("invalid user")
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision == "pass" {
		decision = "bail"
	}
	if decision != "build" && decision != "bail" {
		return fmt.Errorf("invalid decision")
	}

	ideaJSON, err := json.Marshal(idea)
	if err != nil {
		return fmt.Errorf("marshaling idea: %w", err)
	}

	sql := `
	INSERT INTO buildorbail_user_decisions (user_id, decision_key, decision, idea_json)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(user_id, decision_key)
	DO UPDATE SET
		decision = excluded.decision,
		idea_json = excluded.idea_json,
		updated_at = CURRENT_TIMESTAMP`

	args := []tursoArg{
		{Type: "integer", Value: fmt.Sprintf("%d", userID)},
		{Type: "text", Value: buildDecisionKey(idea)},
		{Type: "text", Value: decision},
		{Type: "text", Value: string(ideaJSON)},
	}

	if err := execTurso(sql, args); err != nil {
		log.Printf("  [db error] Failed to store %s decision for '%s': %v", decision, idea.Title, err)
		return err
	}
	return nil
}

func SaveIdea(userID int, idea core.Idea) error {
	return RecordDecision(userID, idea, "build")
}

// DeleteSavedIdea removes a saved (build) decision by row id for one user.
func DeleteSavedIdea(userID, id int) error {
	sql := `DELETE FROM buildorbail_user_decisions WHERE id = ? AND user_id = ? AND decision = 'build'`
	args := []tursoArg{
		{Type: "integer", Value: fmt.Sprintf("%d", id)},
		{Type: "integer", Value: fmt.Sprintf("%d", userID)},
	}
	if err := execTurso(sql, args); err != nil {
		log.Printf("  [db error] Failed to delete saved idea id=%d user=%d: %v", id, userID, err)
		return err
	}
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

func GetSavedIdeas(userID int) ([]core.Idea, error) {
	rows, err := queryTursoRows(
		`SELECT id, idea_json
		 FROM buildorbail_user_decisions
		 WHERE user_id = ? AND decision = 'build'
		 ORDER BY updated_at DESC`,
		[]tursoArg{{Type: "integer", Value: fmt.Sprintf("%d", userID)}},
	)
	if err != nil {
		return nil, err
	}

	ideas := make([]core.Idea, 0, len(rows))
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		rowID, _ := strconv.Atoi(parseTursoValue(row[0]))
		raw := parseTursoValue(row[1])
		if raw == "" {
			continue
		}
		var idea core.Idea
		if err := json.Unmarshal([]byte(raw), &idea); err != nil {
			log.Printf("  [db warn] skipping malformed saved idea row id=%d: %v", rowID, err)
			continue
		}
		idea.ID = rowID
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

func FilterUndecidedIdeas(userID int, ideas []core.Idea) ([]core.Idea, int, error) {
	if userID <= 0 || len(ideas) == 0 {
		return ideas, 0, nil
	}

	keySeen := make(map[string]bool, len(ideas))
	keys := make([]string, 0, len(ideas))
	for _, idea := range ideas {
		key := buildDecisionKey(idea)
		if key == "" || keySeen[key] {
			continue
		}
		keySeen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ideas, 0, nil
	}

	placeholders := make([]string, len(keys))
	args := make([]tursoArg, 0, len(keys)+1)
	args = append(args, tursoArg{Type: "integer", Value: fmt.Sprintf("%d", userID)})
	for i, key := range keys {
		placeholders[i] = "?"
		args = append(args, tursoArg{Type: "text", Value: key})
	}

	sql := fmt.Sprintf(`SELECT decision_key
		FROM buildorbail_user_decisions
		WHERE user_id = ?
		  AND decision IN ('build', 'bail')
		  AND decision_key IN (%s)`, strings.Join(placeholders, ","))

	rows, err := queryTursoRows(sql, args)
	if err != nil {
		return ideas, 0, err
	}

	decided := make(map[string]bool, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		key := parseTursoValue(row[0])
		if key != "" {
			decided[key] = true
		}
	}

	filtered := make([]core.Idea, 0, len(ideas))
	skipped := 0
	for _, idea := range ideas {
		if decided[buildDecisionKey(idea)] {
			skipped++
			continue
		}
		filtered = append(filtered, idea)
	}
	return filtered, skipped, nil
}
