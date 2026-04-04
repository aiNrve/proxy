package adapter

import "strings"

// TogetherAdapter implements the Together AI provider using OpenAI-compatible APIs.
type TogetherAdapter struct {
	*openAICompatAdapter
}

// NewTogetherAdapter builds a Together adapter with sensible defaults.
func NewTogetherAdapter(apiKey, baseURL string) *TogetherAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.together.ai"
	}
	core := newOpenAICompatAdapter("together", apiKey, baseURL, 0.0008, 0.0008)
	return &TogetherAdapter{openAICompatAdapter: core}
}
