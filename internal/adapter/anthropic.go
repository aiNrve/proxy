package adapter

import (
	extadapters "github.com/aiNrve/adapters"
	extanthropic "github.com/aiNrve/adapters/providers/anthropic"
)

// NewAnthropicAdapter builds an Anthropic adapter using the external adapters module.
func NewAnthropicAdapter(apiKey, baseURL string) Adapter {
	return extanthropic.New(extadapters.AdapterConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}
