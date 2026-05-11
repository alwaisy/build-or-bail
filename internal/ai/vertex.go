package ai

import (
	"buildorbail/internal/core"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

// ── GOOGLE GENERATIVE AI ────────────────────────────────────────────────────

type googleRequest struct {
	SystemInstruction *googleSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []googleContent          `json:"contents"`
}

type googleSystemInstruction struct {
	Parts []googlePart `json:"parts"`
}

type googleContent struct {
	Role  string       `json:"role"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func getGCloudToken() (string, error) {
	out, err := exec.Command("gcloud", "auth", "print-access-token").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gcloud token: %w. Make sure 'gcloud auth login' was run.", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func callGoogle(apiKey, model string, posts []core.RedditPost) ([]core.Idea, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Here are %d Reddit posts about frustrations:\n\n", len(posts)))
	for i, p := range posts {
		sb.WriteString(fmt.Sprintf("--- Post %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Subreddit: r/%s\n", p.Sub))
		sb.WriteString(fmt.Sprintf("Title: %s\n", p.Title))
		sb.WriteString(fmt.Sprintf("Body: %s\n", p.Body))
		sb.WriteString(fmt.Sprintf("Upvotes: %d | Comments: %d\n\n", p.Upvotes, p.Comments))
	}

	reqBody := googleRequest{
		SystemInstruction: &googleSystemInstruction{
			Parts: []googlePart{{Text: GetSystemPrompt()}},
		},
		Contents: []googleContent{
			{Role: "user", Parts: []googlePart{
				{Text: GetPrimaryPrompt() + "\n\n" + sb.String()},
			}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, apiKey,
	)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling google ai: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google ai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var gResp googleResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if gResp.Error.Message != "" {
		return nil, fmt.Errorf("google ai error: %s", gResp.Error.Message)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from google ai")
	}

	content := strings.TrimSpace(gResp.Candidates[0].Content.Parts[0].Text)
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

func callVertex(projectID, region, model string, posts []core.RedditPost) ([]core.Idea, error) {
	token, err := getGCloudToken()
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Here are %d Reddit posts about frustrations:\n\n", len(posts)))
	for i, p := range posts {
		sb.WriteString(fmt.Sprintf("--- Post %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Subreddit: r/%s\n", p.Sub))
		sb.WriteString(fmt.Sprintf("Title: %s\n", p.Title))
		sb.WriteString(fmt.Sprintf("Body: %s\n", p.Body))
		sb.WriteString(fmt.Sprintf("Upvotes: %d | Comments: %d\n\n", p.Upvotes, p.Comments))
	}

	reqBody := googleRequest{
		SystemInstruction: &googleSystemInstruction{
			Parts: []googlePart{{Text: GetSystemPrompt()}},
		},
		Contents: []googleContent{
			{Role: "user", Parts: []googlePart{
				{Text: GetPrimaryPrompt() + "\n\n" + sb.String()},
			}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Vertex AI URL format (stream is more reliable for large responses)
	// NOTE: For Gemini 3 series, the location segment in the path must be "global"
	// and we use the base aiplatform.googleapis.com endpoint.
	url := fmt.Sprintf(
		"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:streamGenerateContent",
		projectID, model,
	)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling vertex ai: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vertex ai returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Vertex AI streamGenerateContent returns an array of candidates
	var vResp []googleResponse
	if err := json.Unmarshal(respBody, &vResp); err != nil {
		// Try single object if not a stream array
		var singleResp googleResponse
		if err2 := json.Unmarshal(respBody, &singleResp); err2 == nil {
			vResp = []googleResponse{singleResp}
		} else {
			return nil, fmt.Errorf("decoding vertex response: %w\nraw: %s", err, string(respBody))
		}
	}

	var fullContent strings.Builder
	for _, r := range vResp {
		if len(r.Candidates) > 0 && len(r.Candidates[0].Content.Parts) > 0 {
			fullContent.WriteString(r.Candidates[0].Content.Parts[0].Text)
		}
	}

	content := strings.TrimSpace(fullContent.String())
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
