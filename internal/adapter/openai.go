package adapter

import (
	extadapters "github.com/aiNrve/adapters"
	extopenai "github.com/aiNrve/adapters/providers/openai"
)

// NewOpenAIAdapter builds an OpenAI adapter using the external adapters module.
func NewOpenAIAdapter(apiKey, baseURL string) Adapter {
	return extopenai.New(extadapters.AdapterConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}
