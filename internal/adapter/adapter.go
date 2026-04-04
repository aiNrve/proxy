package adapter

import (
	"context"

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
