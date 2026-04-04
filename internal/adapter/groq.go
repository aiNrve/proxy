package adapter

import "strings"

// GroqAdapter implements the Groq provider using OpenAI-compatible APIs.
type GroqAdapter struct {
	*openAICompatAdapter
}

// NewGroqAdapter builds a Groq adapter with sensible defaults.
func NewGroqAdapter(apiKey, baseURL string) *GroqAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.groq.com/openai"
	}
	core := newOpenAICompatAdapter("groq", apiKey, baseURL, 0.0010, 0.0020)
	return &GroqAdapter{openAICompatAdapter: core}
}
