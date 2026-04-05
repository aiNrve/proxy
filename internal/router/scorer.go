package router

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/aiNrve/proxy/internal/adapter"
	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/models"
)

type scoredCandidate struct {
	name         string
	adapter      adapter.Adapter
	total        float64
	costScore    float64
	latencyScore float64
	cost         float64
	latencyMS    float64
	weight       float64
}

type scoreWeights struct {
	cost    float64
	latency float64
}

type latencyTracker struct {
	mu     sync.RWMutex
	size   int
	values map[string][]float64
}

func newLatencyTracker(size int) *latencyTracker {
	if size <= 0 {
		size = 100
	}
	return &latencyTracker{
		size:   size,
		values: map[string][]float64{},
	}
}

func (t *latencyTracker) Add(provider string, latencyMS float64) {
	provider = normalizeName(provider)
	if provider == "" || latencyMS <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	bucket := append(t.values[provider], latencyMS)
	if len(bucket) > t.size {
		bucket = bucket[len(bucket)-t.size:]
	}
	t.values[provider] = bucket
}

func (t *latencyTracker) P50(provider string) float64 {
	provider = normalizeName(provider)

	t.mu.RLock()
	bucket := t.values[provider]
	t.mu.RUnlock()

	if len(bucket) == 0 {
		return 0
	}

	copied := make([]float64, len(bucket))
	copy(copied, bucket)
	sort.Float64s(copied)
	return copied[len(copied)/2]
}

func (r *Router) scoredCandidates(ctx context.Context, req *models.ChatRequest) []scoredCandidate {
	now := time.Now()
	excluded := excludedProviderSetFromContext(ctx)
	externalReq := adapter.ToExternalRequest(req)

	r.mu.RLock()
	weights := r.weightsLocked()
	raw := make([]scoredCandidate, 0, len(r.order))
	for _, name := range r.order {
		if _, skip := excluded[name]; skip {
			continue
		}
		if !r.isAvailableLocked(name, now) {
			continue
		}
		item := r.adapters[name]
		candidate := scoredCandidate{
			name:      name,
			adapter:   item,
			cost:      item.EstimateCost(externalReq),
			latencyMS: r.latency.P50(name),
			weight:    providerWeight(r.cfg, name),
		}
		if candidate.latencyMS <= 0 {
			candidate.latencyMS = 1000
		}
		if candidate.weight <= 0 {
			candidate.weight = 1
		}
		raw = append(raw, candidate)
	}
	r.mu.RUnlock()

	if len(raw) == 0 {
		return nil
	}

	maxCost := 0.0
	maxLatency := 0.0
	for _, item := range raw {
		if item.cost > maxCost {
			maxCost = item.cost
		}
		if item.latencyMS > maxLatency {
			maxLatency = item.latencyMS
		}
	}

	for i := range raw {
		raw[i].costScore = normalizedScore(raw[i].cost, maxCost)
		raw[i].latencyScore = normalizedScore(raw[i].latencyMS, maxLatency)
		raw[i].total = ((weights.cost * raw[i].costScore) + (weights.latency * raw[i].latencyScore)) * raw[i].weight
	}

	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].total == raw[j].total {
			return raw[i].name < raw[j].name
		}
		return raw[i].total > raw[j].total
	})

	return raw
}

func normalizedScore(value, max float64) float64 {
	if max <= 0 {
		return 1
	}
	score := 1 - (value / max)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func (r *Router) weightsLocked() scoreWeights {
	strategy := normalizeName(r.cfg.Routing.DefaultStrategy)
	switch strategy {
	case "cost":
		return scoreWeights{cost: 1, latency: 0}
	case "latency":
		return scoreWeights{cost: 0, latency: 1}
	default:
		cost := r.cfg.Routing.WeightCost
		latency := r.cfg.Routing.WeightLatency
		total := cost + latency
		if total <= 0 {
			return scoreWeights{cost: 0.5, latency: 0.5}
		}
		return scoreWeights{cost: cost / total, latency: latency / total}
	}
}

func providerWeight(cfg config.Config, provider string) float64 {
	providerCfg, ok := cfg.Providers[provider]
	if !ok || providerCfg.Weight <= 0 {
		return 1
	}
	return providerCfg.Weight
}
