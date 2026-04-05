package adapter

import (
	"fmt"

	extadapters "github.com/aiNrve/adapters"
)

// Adapter aliases the external provider integration contract.
type Adapter = extadapters.Adapter

// AdapterConfig aliases the external provider configuration struct.
type AdapterConfig = extadapters.AdapterConfig

// ProviderError captures provider HTTP error status for retry and cooldown logic.
type ProviderError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s returned %d: %s", e.Provider, e.StatusCode, e.Body)
}
