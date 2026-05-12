package ai

import (
	"buildorbail/internal/core"
	"fmt"
	"log"
	"os"
)

func CallLLMDispatch(posts []core.RedditPost, provider string) ([]core.Idea, error) {
	switch provider {
	case "google":
		apiKey := os.Getenv("GOOGLE_AI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("GOOGLE_AI_API_KEY not set. Add it to .env")
		}
		model := core.EnvOr("GOOGLE_MODEL", "gemini-3.1-flash-lite")
		log.Printf("  calling Google AI (model=%s) with %d posts", model, len(posts))
		return callGoogle(apiKey, model, posts)
	case "vertex":
		projectID := os.Getenv("VERTEX_PROJECT_ID")
		region := core.EnvOr("VERTEX_REGION", "us-central1")
		model := core.EnvOr("GOOGLE_MODEL", "gemini-3.1-flash-lite")
		log.Printf("  calling Vertex AI (project=%s, region=%s) with %d posts", projectID, region, len(posts))
		return callVertex(projectID, region, model, posts)
	default:
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY not set. Add it to .env")
		}
		model := core.EnvOr("OPENROUTER_MODEL", "deepseek/deepseek-chat-v3-0324:free")
		log.Printf("  calling OpenRouter (model=%s) with %d posts", model, len(posts))
		return callLLM(apiKey, model, posts)
	}
}
