package adapter

import (
	extadapters "github.com/aiNrve/adapters"
	extgemini "github.com/aiNrve/adapters/providers/gemini"
)

// NewGeminiAdapter builds a Gemini adapter using the external adapters module.
func NewGeminiAdapter(apiKey, baseURL string) Adapter {
	return extgemini.New(extadapters.AdapterConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}
