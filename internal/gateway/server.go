package gateway

import (
	"fmt"
	"net/http"

	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/logger"
	"github.com/aiNrve/proxy/internal/router"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server represents the HTTP gateway.
type Server struct {
	cfg           config.Config
	router        *router.Router
	requestLogger *logger.Logger
	appLogger     *zap.Logger
	limiter       *rateLimiter
	metrics       *metricsStore
	engine        *gin.Engine
}

// NewServer creates a new gateway server with middleware and routes registered.
func NewServer(cfg config.Config, rt *router.Router, requestLogger *logger.Logger, appLogger *zap.Logger) (*Server, error) {
	if appLogger == nil {
		appLogger = zap.NewNop()
	}
	if rt == nil {
		return nil, fmt.Errorf("router is required")
	}

	rate, burst := defaultRateLimitConfig()
	s := &Server{
		cfg:           cfg,
		router:        rt,
		requestLogger: requestLogger,
		appLogger:     appLogger,
		limiter:       newRateLimiter(rate, burst),
		metrics:       newMetricsStore(),
		engine:        gin.New(),
	}

	s.engine.Use(
		s.requestIDMiddleware(),
		s.loggingMiddleware(),
		s.authMiddleware(),
		s.rateLimitMiddleware(),
		s.recoveryMiddleware(),
	)

	s.engine.POST("/v1/chat/completions", s.chatCompletions)
	s.engine.GET("/health", s.health)
	s.engine.GET("/metrics", s.metricsHandler)

	return s, nil
}

// Handler returns the HTTP handler for the gateway server.
func (s *Server) Handler() http.Handler {
	return s.engine
}
