package ai

import (
	"buildorbail/internal/core"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ── OPENROUTER / LLM ─────────────────────────────────────────────────────────

type orRequest struct {
	Model    string      `json:"model"`
	Messages []orMessage `json:"messages"`
}

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func callLLM(apiKey, model string, posts []core.RedditPost) ([]core.Idea, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Here are %d Reddit posts about frustrations:\n\n", len(posts)))
	for i, p := range posts {
		sb.WriteString(fmt.Sprintf("--- Post %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Subreddit: r/%s\n", p.Sub))
		sb.WriteString(fmt.Sprintf("Title: %s\n", p.Title))
		sb.WriteString(fmt.Sprintf("Body: %s\n", p.Body))
		sb.WriteString(fmt.Sprintf("Upvotes: %d | Comments: %d\n\n", p.Upvotes, p.Comments))
	}

	reqBody := orRequest{
		Model: model,
		Messages: []orMessage{
			{Role: "system", Content: GetSystemPrompt()},
			{Role: "user", Content: GetPrimaryPrompt() + "\n\n" + sb.String()},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "http://localhost:"+core.EnvOr("PORT", "5897"))
	req.Header.Set("X-Title", "Build or Bail")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling openrouter: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openrouter returned %d: %s", resp.StatusCode, string(respBody))
	}

	var orResp orResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if orResp.Error.Message != "" {
		return nil, fmt.Errorf("openrouter error: %s", orResp.Error.Message)
	}

	if len(orResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(orResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var ideas []core.Idea
	if err := json.Unmarshal([]byte(content), &ideas); err != nil {
		return nil, fmt.Errorf("parsing ideas json: %w\nraw: %s", err, content)
	}

	return ideas, nil
}
