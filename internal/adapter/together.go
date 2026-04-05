package adapter

import (
	extadapters "github.com/aiNrve/adapters"
	exttogether "github.com/aiNrve/adapters/providers/together"
)

// NewTogetherAdapter builds a Together adapter using the external adapters module.
func NewTogetherAdapter(apiKey, baseURL string) Adapter {
	return exttogether.New(extadapters.AdapterConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}
