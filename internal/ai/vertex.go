package ai

import (
	"buildorbail/internal/core"
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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

// ServiceAccountKey matches the format of the Google Cloud JSON key file.
type ServiceAccountKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

func getGCloudToken() (string, error) {
	// 1. Try Service Account JSON (Production/Docker way)
	keyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if keyPath == "" {
		keyPath = "google-creds.json" // default name
	}

	if _, err := os.Stat(keyPath); err == nil {
		token, err := getServiceAccountToken(keyPath)
		if err == nil {
			return token, nil
		}
		log.Printf("    [auth] failed to get token from JSON: %v", err)
	}

	// 2. Fallback to gcloud CLI (Local development way)
	out, err := exec.Command("gcloud", "auth", "print-access-token").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gcloud token: %w. Make sure 'gcloud auth login' was run or google-creds.json exists.", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// getServiceAccountToken performs a manual JWT exchange for an access token (No SDKs).
func getServiceAccountToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var key ServiceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return "", err
	}

	now := time.Now().Unix()
	claims := map[string]interface{}{
		"iss":   key.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   key.TokenURI,
		"exp":   now + 3600,
		"iat":   now,
	}

	header := "{\"alg\":\"RS256\",\"typ\":\"JWT\"}"
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))

	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	payload := headerB64 + "." + claimsB64

	block, _ := pem.Decode([]byte(key.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block in private key")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("not an RSA private key")
	}

	h := sha256.New()
	h.Write([]byte(payload))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(nil, rsaKey, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	signedJWT := payload + "." + sigB64

	// Exchange JWT for Access Token
	resp, err := http.PostForm(key.TokenURI, map[string][]string{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signedJWT},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if res.Error != "" {
		return "", fmt.Errorf("oauth2 error: %s", res.Error)
	}

	return res.AccessToken, nil
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
