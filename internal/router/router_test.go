package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aiNrve/adapters"
	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/models"
)

type fakeAdapter struct {
	name    string
	cost    float64
	healthy bool
}

func (f *fakeAdapter) Name() string {
	return f.name
}

func (f *fakeAdapter) Complete(_ context.Context, _ *adapters.Request) (*adapters.Response, error) {
	return &adapters.Response{ID: "ok"}, nil
}

func (f *fakeAdapter) CompleteStream(_ context.Context, _ *adapters.Request) (<-chan adapters.StreamChunk, error) {
	ch := make(chan adapters.StreamChunk)
	close(ch)
	return ch, nil
}

func (f *fakeAdapter) EstimateCost(_ *adapters.Request) float64 {
	return f.cost
}

func (f *fakeAdapter) IsHealthy(_ context.Context) bool {
	return f.healthy
}

func (f *fakeAdapter) Models() []string {
	return []string{"test-model"}
}

func TestRouteScoringTable(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.Config
		req          *models.ChatRequest
		setupContext func(context.Context) context.Context
		seed         func(*Router)
		wantProvider string
		wantErr      bool
	}{
		{
			name: "cost strategy picks cheapest",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"openai": {Enabled: true},
					"groq":   {Enabled: true},
				},
				Routing: config.RoutingConfig{DefaultStrategy: "cost"},
			},
			req:          &models.ChatRequest{Model: "x"},
			wantProvider: "groq",
		},
		{
			name: "latency strategy picks fastest",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"openai": {Enabled: true},
					"groq":   {Enabled: true},
				},
				Routing: config.RoutingConfig{DefaultStrategy: "latency"},
			},
			req: &models.ChatRequest{Model: "x"},
			seed: func(r *Router) {
				r.RecordOutcome("openai", 35*time.Millisecond, 200, nil)
				r.RecordOutcome("groq", 220*time.Millisecond, 200, nil)
			},
			wantProvider: "openai",
		},
		{
			name: "forced provider bypasses scoring",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"openai": {Enabled: true},
					"groq":   {Enabled: true},
				},
				Routing: config.RoutingConfig{DefaultStrategy: "cost"},
			},
			req: &models.ChatRequest{Model: "x"},
			setupContext: func(ctx context.Context) context.Context {
				return WithForcedProvider(ctx, "openai")
			},
			wantProvider: "openai",
		},
		{
			name: "task override takes precedence",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"openai": {Enabled: true},
					"groq":   {Enabled: true},
				},
				Routing: config.RoutingConfig{
					DefaultStrategy: "cost",
					TaskOverrides: map[string]string{
						"code": "openai",
					},
				},
			},
			req:          &models.ChatRequest{Model: "x", XAiNrveTask: "code"},
			wantProvider: "openai",
		},
		{
			name: "excluded provider enables retry routing",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"openai": {Enabled: true},
					"groq":   {Enabled: true},
				},
				Routing: config.RoutingConfig{DefaultStrategy: "cost"},
			},
			req: &models.ChatRequest{Model: "x"},
			setupContext: func(ctx context.Context) context.Context {
				return WithExcludedProviders(ctx, []string{"groq"})
			},
			wantProvider: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openai := &fakeAdapter{name: "openai", cost: 1.0, healthy: true}
			groq := &fakeAdapter{name: "groq", cost: 0.1, healthy: true}

			registry := adapters.NewRegistry()
			registry.Register("openai", openai)
			registry.Register("groq", groq)

			r := NewRouter(registry, tt.cfg)
			if tt.seed != nil {
				tt.seed(r)
			}

			ctx := context.Background()
			if tt.setupContext != nil {
				ctx = tt.setupContext(ctx)
			}

			decision, selected, err := r.Route(ctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected routing error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected routing error: %v", err)
			}
			if selected == nil {
				t.Fatal("expected selected adapter")
			}
			if decision == nil {
				t.Fatal("expected route decision")
			}
			if selected.Name() != tt.wantProvider {
				t.Fatalf("expected provider %q, got %q", tt.wantProvider, selected.Name())
			}
		})
	}
}

func TestRouterRecordOutcomeAndHealthChecker(t *testing.T) {
	cfg := config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {Enabled: true},
		},
	}
	openai := &fakeAdapter{name: "openai", cost: 1, healthy: true}

	registry := adapters.NewRegistry()
	registry.Register("openai", openai)

	r := NewRouter(registry, cfg)

	r.RecordOutcome("openai", 50*time.Millisecond, 0, errors.New("upstream error"))
	if r.HealthSnapshot()["openai"] {
		t.Fatal("expected provider to be marked unhealthy after upstream error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	openai.healthy = true
	r.StartHealthChecker(ctx, 10*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HealthSnapshot()["openai"] {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected health checker to mark provider healthy")
}
