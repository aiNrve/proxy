package logger

import (
	"context"
	"testing"
	"time"

	"github.com/aiNrve/proxy/internal/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewEnqueueAndCloseStdoutMode(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	base := zap.New(core)

	l, err := New("", base)
	if err != nil {
		t.Fatalf("new logger returned error: %v", err)
	}

	ok := l.Enqueue(&models.RequestLog{
		RequestID:        "req-1",
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		TaskType:         "code",
		PromptTokens:     10,
		CompletionTokens: 5,
		CostUSD:          0.001,
		LatencyMs:        50,
	})
	if !ok {
		t.Fatal("expected enqueue success")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.Close(ctx); err != nil {
		t.Fatalf("close returned error: %v", err)
	}

	if logs.Len() == 0 {
		t.Fatal("expected emitted stdout log entries")
	}
}

func TestEnqueueAfterCloseReturnsFalse(t *testing.T) {
	l, err := New("", zap.NewNop())
	if err != nil {
		t.Fatalf("new logger returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.Close(ctx); err != nil {
		t.Fatalf("close returned error: %v", err)
	}

	ok := l.Enqueue(&models.RequestLog{RequestID: "late"})
	if ok {
		t.Fatal("expected enqueue to fail after close")
	}
}
