package adapter

import (
	"context"
	"fmt"

	"github.com/aiNrve/proxy/internal/models"
)

// Adapter defines the provider integration contract used by the router.
type Adapter interface {
	Name() string
	Complete(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error)
	CompleteStream(ctx context.Context, req *models.ChatRequest) (<-chan string, error)
	EstimateCost(req *models.ChatRequest) float64
	IsHealthy(ctx context.Context) bool
}

// ProviderError captures provider HTTP error status for retry and cooldown logic.
type ProviderError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s returned %d: %s", e.Provider, e.StatusCode, e.Body)
}
