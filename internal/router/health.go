package router

import (
	"context"
	"time"
)

// StartHealthChecker starts a background health-check loop for all providers.
func (r *Router) StartHealthChecker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		r.mu.RLock()
		if r.cfg.HealthCheckInterval > 0 {
			interval = time.Duration(r.cfg.HealthCheckInterval) * time.Second
		}
		r.mu.RUnlock()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		r.runHealthCheck(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runHealthCheck(ctx)
			}
		}
	}()
}

// HealthSnapshot returns a copy of current provider health status.
func (r *Router) HealthSnapshot() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]bool, len(r.health))
	for provider, healthy := range r.health {
		out[provider] = healthy
	}
	return out
}

func (r *Router) runHealthCheck(parent context.Context) {
	r.mu.RLock()
	targets := make(map[string]struct {
		adapter interface{ IsHealthy(context.Context) bool }
		until   time.Time
	}, len(r.order))
	for _, name := range r.order {
		targets[name] = struct {
			adapter interface{ IsHealthy(context.Context) bool }
			until   time.Time
		}{
			adapter: r.adapters[name],
			until:   r.coolingUntil[name],
		}
	}
	r.mu.RUnlock()

	now := time.Now()
	for name, target := range targets {
		if target.until.After(now) {
			continue
		}

		healthCtx, cancel := context.WithTimeout(parent, 5*time.Second)
		healthy := target.adapter.IsHealthy(healthCtx)
		cancel()

		r.mu.Lock()
		r.health[name] = healthy
		if healthy {
			delete(r.coolingUntil, name)
		}
		r.mu.Unlock()
	}
}
