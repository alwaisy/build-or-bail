package ai

import _ "embed"

//go:embed prompts/base.md
var basePrompt string

//go:embed prompts/voice.md
var voicePrompt string

//go:embed prompts/rules.md
var rulesPrompt string

// GetSystemPrompt returns the "Voice" and "Rules" instructions.
// This tells the AI HOW to speak.
func GetSystemPrompt() string {
	// Temporarily disabled voicePrompt for testing
	return rulesPrompt
}

// GetPrimaryPrompt returns the core "Role" and "Task" instructions.
// This tells the AI WHAT to do.
func GetPrimaryPrompt() string {
	return basePrompt
}
