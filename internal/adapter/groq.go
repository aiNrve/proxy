package adapter

import (
	extadapters "github.com/aiNrve/adapters"
	extgroq "github.com/aiNrve/adapters/providers/groq"
)

// NewGroqAdapter builds a Groq adapter using the external adapters module.
func NewGroqAdapter(apiKey, baseURL string) Adapter {
	return extgroq.New(extadapters.AdapterConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}
