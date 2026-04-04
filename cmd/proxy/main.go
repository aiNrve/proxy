package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aiNrve/proxy/internal/adapter"
	"github.com/aiNrve/proxy/internal/config"
	"github.com/aiNrve/proxy/internal/gateway"
	"github.com/aiNrve/proxy/internal/logger"
	"github.com/aiNrve/proxy/internal/router"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgLoader, err := config.NewLoader("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgLoader.Get()

	appLogger, err := buildLogger(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("init zap logger: %w", err)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	adapters := buildAdapters(cfg, appLogger)
	if len(adapters) == 0 {
		return errors.New("no adapters initialized: enable at least one provider and set its API key")
	}

	rt := router.NewRouter(adapters, cfg)
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	rt.StartHealthChecker(healthCtx, time.Duration(cfg.HealthCheckInterval)*time.Second)

	requestLogger, err := logger.New(cfg.DatabaseURL, appLogger)
	if err != nil {
		return fmt.Errorf("init request logger: %w", err)
	}

	gw, err := gateway.NewServer(cfg, rt, requestLogger, appLogger)
	if err != nil {
		_ = requestLogger.Close(context.Background())
		return fmt.Errorf("init gateway: %w", err)
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		appLogger.Info("starting gateway", zap.String("addr", httpServer.Addr))
		if listenErr := httpServer.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			errCh <- listenErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		appLogger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case listenErr := <-errCh:
		healthCancel()
		_ = requestLogger.Close(context.Background())
		return fmt.Errorf("http serve error: %w", listenErr)
	}

	healthCancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("http shutdown error", zap.Error(err))
	}

	if err := requestLogger.Close(shutdownCtx); err != nil {
		appLogger.Error("request logger shutdown error", zap.Error(err))
	}

	appLogger.Info("shutdown complete")
	return nil
}

func buildLogger(level string) (*zap.Logger, error) {
	zapLevel := zapcore.InfoLevel
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info", "":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		return nil, fmt.Errorf("invalid log level %q", level)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	return cfg.Build()
}

func buildAdapters(cfg config.Config, appLogger *zap.Logger) []adapter.Adapter {
	adapters := make([]adapter.Adapter, 0, 5)

	openAI := cfg.Providers["openai"]
	if openAI.Enabled && strings.TrimSpace(openAI.APIKey) != "" {
		adapters = append(adapters, adapter.NewOpenAIAdapter(openAI.APIKey, openAI.BaseURL))
	}

	anthropic := cfg.Providers["anthropic"]
	if anthropic.Enabled && strings.TrimSpace(anthropic.APIKey) != "" {
		adapters = append(adapters, adapter.NewAnthropicAdapter(anthropic.APIKey, anthropic.BaseURL))
	}

	groq := cfg.Providers["groq"]
	if groq.Enabled && strings.TrimSpace(groq.APIKey) != "" {
		adapters = append(adapters, adapter.NewGroqAdapter(groq.APIKey, groq.BaseURL))
	}

	together := cfg.Providers["together"]
	if together.Enabled && strings.TrimSpace(together.APIKey) != "" {
		adapters = append(adapters, adapter.NewTogetherAdapter(together.APIKey, together.BaseURL))
	}

	gemini := cfg.Providers["gemini"]
	if gemini.Enabled && strings.TrimSpace(gemini.APIKey) != "" {
		adapters = append(adapters, adapter.NewGeminiAdapter(gemini.APIKey, gemini.BaseURL))
	}

	if len(adapters) == 0 {
		appLogger.Warn("no adapters enabled with API keys")
	}

	return adapters
}
