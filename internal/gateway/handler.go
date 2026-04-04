package gateway

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aiNrve/proxy/internal/adapter"
	"github.com/aiNrve/proxy/internal/models"
	"github.com/aiNrve/proxy/internal/router"
	"github.com/gin-gonic/gin"
)

type metricsStore struct {
	mu                sync.RWMutex
	requestCount      uint64
	routingTotalMS    float64
	routingSampleSize uint64
	providerTotals    map[string]uint64
	providerErrors    map[string]uint64
}

type metricsSnapshot struct {
	RequestCount     uint64
	AverageRoutingMS float64
	ProviderTotals   map[string]uint64
	ProviderErrors   map[string]uint64
}

func newMetricsStore() *metricsStore {
	return &metricsStore{
		providerTotals: map[string]uint64{},
		providerErrors: map[string]uint64{},
	}
}

func (m *metricsStore) record(provider string, routingLatency time.Duration, hasError bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount++
	m.routingTotalMS += float64(routingLatency.Milliseconds())
	m.routingSampleSize++
	if provider != "" {
		m.providerTotals[provider]++
		if hasError {
			m.providerErrors[provider]++
		}
	}
}

func (m *metricsStore) snapshot() metricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providerTotals := make(map[string]uint64, len(m.providerTotals))
	for k, v := range m.providerTotals {
		providerTotals[k] = v
	}
	providerErrors := make(map[string]uint64, len(m.providerErrors))
	for k, v := range m.providerErrors {
		providerErrors[k] = v
	}

	average := 0.0
	if m.routingSampleSize > 0 {
		average = m.routingTotalMS / float64(m.routingSampleSize)
	}

	return metricsSnapshot{
		RequestCount:     m.requestCount,
		AverageRoutingMS: average,
		ProviderTotals:   providerTotals,
		ProviderErrors:   providerErrors,
	}
}

// chatCompletions handles OpenAI-compatible completion requests.
func (s *Server) chatCompletions(c *gin.Context) {
	var req models.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	req.XAiNrveTask = strings.ToLower(strings.TrimSpace(c.GetHeader("X-AiNrve-Task")))
	baseCtx := c.Request.Context()
	if forcedProvider := strings.TrimSpace(c.GetHeader("X-AiNrve-Provider")); forcedProvider != "" {
		baseCtx = router.WithForcedProvider(baseCtx, forcedProvider)
	}

	excluded := make([]string, 0, 1)
	for attempt := 0; attempt < 2; attempt++ {
		routeCtx := baseCtx
		if len(excluded) > 0 {
			routeCtx = router.WithExcludedProviders(routeCtx, excluded)
		}

		routingStart := time.Now()
		decision, selectedAdapter, err := s.router.Route(routeCtx, &req)
		routingLatency := time.Since(routingStart)
		if err != nil {
			s.metrics.record("", routingLatency, true)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}

		providerName := decision.Provider
		callStart := time.Now()

		if req.Stream {
			stream, streamErr := selectedAdapter.CompleteStream(routeCtx, &req)
			statusCode := providerStatusCode(streamErr)
			callLatency := time.Since(callStart)
			s.router.RecordOutcome(providerName, callLatency, statusCode, streamErr)

			if streamErr != nil {
				if shouldRetry(statusCode, attempt) {
					excluded = append(excluded, providerName)
					continue
				}
				s.metrics.record(providerName, routingLatency, true)
				s.enqueueRequestLog(c.GetString(requestIDContextKey), providerName, &req, nil, selectedAdapter.EstimateCost(&req), callLatency, streamErr)
				c.JSON(statusCodeForClient(statusCode), gin.H{"error": streamErr.Error()})
				return
			}

			s.metrics.record(providerName, routingLatency, false)
			s.enqueueRequestLog(c.GetString(requestIDContextKey), providerName, &req, nil, selectedAdapter.EstimateCost(&req), callLatency, nil)
			c.Header("X-AiNrve-Provider", providerName)
			c.Header("X-AiNrve-Route-Reason", decision.Reason)
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.Status(http.StatusOK)

			flusher, ok := c.Writer.(http.Flusher)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported by response writer"})
				return
			}

			for line := range stream {
				_, _ = fmt.Fprintf(c.Writer, "%s\n\n", line)
				flusher.Flush()
			}
			return
		}

		resp, completeErr := selectedAdapter.Complete(routeCtx, &req)
		statusCode := providerStatusCode(completeErr)
		callLatency := time.Since(callStart)
		s.router.RecordOutcome(providerName, callLatency, statusCode, completeErr)

		if completeErr != nil {
			if shouldRetry(statusCode, attempt) {
				excluded = append(excluded, providerName)
				continue
			}

			s.metrics.record(providerName, routingLatency, true)
			s.enqueueRequestLog(c.GetString(requestIDContextKey), providerName, &req, nil, selectedAdapter.EstimateCost(&req), callLatency, completeErr)
			c.JSON(statusCodeForClient(statusCode), gin.H{"error": completeErr.Error()})
			return
		}

		s.metrics.record(providerName, routingLatency, false)
		s.enqueueRequestLog(c.GetString(requestIDContextKey), providerName, &req, resp, selectedAdapter.EstimateCost(&req), callLatency, nil)
		c.Header("X-AiNrve-Provider", providerName)
		c.Header("X-AiNrve-Route-Reason", decision.Reason)
		c.JSON(http.StatusOK, resp)
		return
	}

	c.JSON(http.StatusBadGateway, gin.H{"error": "all provider attempts failed"})
}

// health returns proxy and provider liveness state.
func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"providers": s.router.HealthSnapshot(),
	})
}

// metricsHandler exposes basic Prometheus-compatible metrics.
func (s *Server) metricsHandler(c *gin.Context) {
	snapshot := s.metrics.snapshot()
	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.Status(http.StatusOK)

	_, _ = fmt.Fprintln(c.Writer, "# TYPE request_count counter")
	_, _ = fmt.Fprintf(c.Writer, "request_count %d\n", snapshot.RequestCount)
	_, _ = fmt.Fprintln(c.Writer, "# TYPE routing_latency_ms gauge")
	_, _ = fmt.Fprintf(c.Writer, "routing_latency_ms %.2f\n", snapshot.AverageRoutingMS)
	_, _ = fmt.Fprintln(c.Writer, "# TYPE provider_error_rate gauge")
	for provider, total := range snapshot.ProviderTotals {
		errorsCount := snapshot.ProviderErrors[provider]
		rate := 0.0
		if total > 0 {
			rate = float64(errorsCount) / float64(total)
		}
		_, _ = fmt.Fprintf(c.Writer, "provider_error_rate{provider=\"%s\"} %.6f\n", provider, rate)
	}
}

func (s *Server) enqueueRequestLog(requestID, provider string, req *models.ChatRequest, resp *models.ChatResponse, costUSD float64, latency time.Duration, requestErr error) {
	if s.requestLogger == nil || req == nil {
		return
	}

	entry := &models.RequestLog{
		RequestID: requestID,
		Provider:  provider,
		Model:     req.Model,
		TaskType:  req.XAiNrveTask,
		CostUSD:   costUSD,
		LatencyMs: int(latency.Milliseconds()),
	}
	if resp != nil {
		entry.PromptTokens = resp.Usage.PromptTokens
		entry.CompletionTokens = resp.Usage.CompletionTokens
	}
	if requestErr != nil {
		entry.Error = requestErr.Error()
	}
	_ = s.requestLogger.Enqueue(entry)
}

func providerStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var providerErr *adapter.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.StatusCode
	}
	return http.StatusBadGateway
}

func shouldRetry(statusCode, attempt int) bool {
	return attempt == 0 && statusCode >= 500 && statusCode <= 599
}

func statusCodeForClient(providerStatus int) int {
	switch {
	case providerStatus == http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case providerStatus >= 400 && providerStatus <= 499:
		return providerStatus
	case providerStatus >= 500:
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}
