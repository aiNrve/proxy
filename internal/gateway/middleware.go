package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const requestIDContextKey = "request_id"

type rateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*tokenBucket
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(rate, burst float64) *rateLimiter {
	if rate <= 0 {
		rate = 10
	}
	if burst <= 0 {
		burst = 20
	}
	return &rateLimiter{
		rate:    rate,
		burst:   burst,
		buckets: map[string]*tokenBucket{},
	}
}

func defaultRateLimitConfig() (float64, float64) {
	rate := 10.0
	if env := strings.TrimSpace(os.Getenv("RATE_LIMIT_RPS")); env != "" {
		if parsed, err := strconv.ParseFloat(env, 64); err == nil && parsed > 0 {
			rate = parsed
		}
	}

	burst := 20.0
	if env := strings.TrimSpace(os.Getenv("RATE_LIMIT_BURST")); env != "" {
		if parsed, err := strconv.ParseFloat(env, 64); err == nil && parsed > 0 {
			burst = parsed
		}
	}

	return rate, burst
}

func (l *rateLimiter) allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		bucket = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[key] = bucket
	}

	elapsed := now.Sub(bucket.last).Seconds()
	bucket.tokens = minFloat(l.burst, bucket.tokens+elapsed*l.rate)
	bucket.last = now

	if bucket.tokens < 1 {
		return false
	}

	bucket.tokens -= 1
	return true
}

func (s *Server) requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := newRequestID()
		c.Set(requestIDContextKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		requestID := c.GetString(requestIDContextKey)
		s.appLogger.Info("http_request",
			zap.String("request_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(s.cfg.AiNrveAPIKey) == "" {
			c.Next()
			return
		}

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		if token != s.cfg.AiNrveAPIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}

		c.Next()
	}
}

func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("Authorization"))
		if key == "" {
			key = c.ClientIP()
		}
		if !s.limiter.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

func (s *Server) recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.appLogger.Error("panic recovered", zap.Any("error", recovered))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
