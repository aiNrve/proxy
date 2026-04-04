package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aiNrve/proxy/internal/adapter"
	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/models"
	"github.com/aiNrve/proxy/internal/router"
	"go.uber.org/zap"
)

type fakeAdapter struct {
	name string
}

func (f *fakeAdapter) Name() string {
	return f.name
}

func (f *fakeAdapter) Complete(_ context.Context, _ *models.ChatRequest) (*models.ChatResponse, error) {
	return &models.ChatResponse{ID: "ok"}, nil
}

func (f *fakeAdapter) CompleteStream(_ context.Context, _ *models.ChatRequest) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (f *fakeAdapter) EstimateCost(_ *models.ChatRequest) float64 {
	return 0.1
}

func (f *fakeAdapter) IsHealthy(_ context.Context) bool {
	return true
}

func TestNewServerAndHandler(t *testing.T) {
	cfg := config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {Enabled: true},
		},
	}

	rt := router.NewRouter([]adapter.Adapter{&fakeAdapter{name: "openai"}}, cfg)
	s, err := NewServer(cfg, rt, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("new server returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	s.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health response: %s", resp.Body.String())
	}
}
