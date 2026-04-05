package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aiNrve/adapters"
	"github.com/aiNrve/proxy/internal/adapter"
	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/models"
)

type forcedProviderKey struct{}
type excludedProvidersKey struct{}

// Router routes requests to providers based on config, health, and scoring.
type Router struct {
	mu           sync.RWMutex
	adapters     map[string]adapter.Adapter
	order        []string
	cfg          config.Config
	health       map[string]bool
	coolingUntil map[string]time.Time
	latency      *latencyTracker
}

// NewRouter creates a router from the external adapter registry and runtime config.
func NewRouter(registry *adapters.Registry, cfg config.Config) *Router {
	registered := make([]adapter.Adapter, 0)
	if registry != nil {
		registered = registry.All()
	}

	r := &Router{
		adapters:     make(map[string]adapter.Adapter, len(registered)),
		order:        make([]string, 0, len(registered)),
		cfg:          cfg,
		health:       make(map[string]bool, len(registered)),
		coolingUntil: make(map[string]time.Time, len(registered)),
		latency:      newLatencyTracker(100),
	}

	for _, item := range registered {
		if item == nil {
			continue
		}

		name := normalizeName(item.Name())
		if name == "" {
			continue
		}
		r.adapters[name] = item
		r.order = append(r.order, name)
		r.health[name] = true
	}

	return r
}

// Route returns the selected provider decision and adapter for a request.
func (r *Router) Route(ctx context.Context, req *models.ChatRequest) (*models.RouteDecision, adapter.Adapter, error) {
	if req == nil {
		return nil, nil, errors.New("request is nil")
	}

	forcedProvider := normalizeName(forcedProviderFromContext(ctx))
	if forcedProvider != "" {
		if forcedAdapter, ok := r.getAdapter(forcedProvider); ok {
			if r.isAvailable(forcedProvider) && !isExcluded(ctx, forcedProvider) {
				return &models.RouteDecision{
					Provider:     forcedProvider,
					Reason:       "forced provider override",
					ScoreCost:    1,
					ScoreLatency: 1,
				}, forcedAdapter, nil
			}
			return nil, nil, fmt.Errorf("forced provider %q is unavailable", forcedProvider)
		}
		return nil, nil, fmt.Errorf("forced provider %q not found", forcedProvider)
	}

	if taskProvider := r.providerForTask(req.XAiNrveTask); taskProvider != "" {
		if taskAdapter, ok := r.getAdapter(taskProvider); ok {
			if r.isAvailable(taskProvider) && !isExcluded(ctx, taskProvider) {
				return &models.RouteDecision{
					Provider:     taskProvider,
					Reason:       fmt.Sprintf("task override for %s", strings.ToLower(strings.TrimSpace(req.XAiNrveTask))),
					ScoreCost:    1,
					ScoreLatency: 1,
				}, taskAdapter, nil
			}
		}
	}

	candidates := r.scoredCandidates(ctx, req)
	if len(candidates) == 0 {
		if fallback := r.fallbackProvider(); fallback != "" {
			if fallbackAdapter, ok := r.getAdapter(fallback); ok && !isExcluded(ctx, fallback) {
				return &models.RouteDecision{
					Provider:     fallback,
					Reason:       "fallback provider",
					ScoreCost:    0,
					ScoreLatency: 0,
				}, fallbackAdapter, nil
			}
		}
		return nil, nil, errors.New("no available providers")
	}

	top := candidates[0]
	reason := "highest routing score"
	if len(excludedProviderSetFromContext(ctx)) > 0 {
		reason = "retry after provider failure"
	}

	decision := &models.RouteDecision{
		Provider:     top.name,
		Reason:       reason,
		ScoreCost:    top.costScore,
		ScoreLatency: top.latencyScore,
	}
	return decision, top.adapter, nil
}

// RecordOutcome updates health and latency state based on provider call results.
func (r *Router) RecordOutcome(provider string, latency time.Duration, statusCode int, callErr error) {
	name := normalizeName(provider)
	if name == "" {
		return
	}

	if latency > 0 {
		r.latency.Add(name, float64(latency.Milliseconds()))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.adapters[name]; !ok {
		return
	}

	now := time.Now()
	switch {
	case callErr != nil:
		r.health[name] = false
	case statusCode == 429:
		r.health[name] = false
		r.coolingUntil[name] = now.Add(60 * time.Second)
	case statusCode >= 500:
		r.health[name] = false
		r.coolingUntil[name] = now.Add(15 * time.Second)
	case statusCode > 0:
		r.health[name] = true
		delete(r.coolingUntil, name)
	}
}

// WithForcedProvider stores a force-provider override in context.
func WithForcedProvider(ctx context.Context, provider string) context.Context {
	provider = normalizeName(provider)
	if provider == "" {
		return ctx
	}
	return context.WithValue(ctx, forcedProviderKey{}, provider)
}

// WithExcludedProviders stores excluded providers in context for retry routing.
func WithExcludedProviders(ctx context.Context, providers []string) context.Context {
	set := map[string]struct{}{}
	for _, provider := range providers {
		name := normalizeName(provider)
		if name != "" {
			set[name] = struct{}{}
		}
	}
	if len(set) == 0 {
		return ctx
	}
	return context.WithValue(ctx, excludedProvidersKey{}, set)
}

func (r *Router) providerForTask(task string) string {
	task = strings.ToLower(strings.TrimSpace(task))
	if task == "" {
		return ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return normalizeName(r.cfg.Routing.TaskOverrides[task])
}

func (r *Router) fallbackProvider() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return normalizeName(r.cfg.Routing.FallbackProvider)
}

func (r *Router) getAdapter(name string) (adapter.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.adapters[normalizeName(name)]
	return item, ok
}

func (r *Router) isAvailable(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isAvailableLocked(normalizeName(provider), time.Now())
}

func (r *Router) isAvailableLocked(provider string, now time.Time) bool {
	if !r.providerEnabledLocked(provider) {
		return false
	}
	if !r.health[provider] {
		if until, ok := r.coolingUntil[provider]; ok && until.After(now) {
			return false
		}
	}
	if until, ok := r.coolingUntil[provider]; ok && until.After(now) {
		return false
	}
	return true
}

func (r *Router) providerEnabledLocked(provider string) bool {
	providerCfg, ok := r.cfg.Providers[provider]
	if !ok {
		return true
	}
	return providerCfg.Enabled
}

func forcedProviderFromContext(ctx context.Context) string {
	value, _ := ctx.Value(forcedProviderKey{}).(string)
	return value
}

func isExcluded(ctx context.Context, provider string) bool {
	_, ok := excludedProviderSetFromContext(ctx)[normalizeName(provider)]
	return ok
}

func excludedProviderSetFromContext(ctx context.Context) map[string]struct{} {
	if set, ok := ctx.Value(excludedProvidersKey{}).(map[string]struct{}); ok {
		return set
	}
	return map[string]struct{}{}
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
