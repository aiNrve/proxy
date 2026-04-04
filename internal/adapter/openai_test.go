package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aiNrve/proxy/internal/models"
)

func TestOpenAIAdapterName(t *testing.T) {
	a := NewOpenAIAdapter("key", "http://example.com")
	if got := a.Name(); got != "openai" {
		t.Fatalf("expected openai, got %q", got)
	}
}

func TestOpenAIAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatal("missing bearer auth header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.ChatResponse{
			ID:      "chatcmpl-1",
			Object:  "chat.completion",
			Created: 1712500000,
			Model:   "gpt-4o-mini",
			Choices: []models.Choice{{
				Index: 0,
				Message: models.Message{
					Role:    "assistant",
					Content: "hello",
				},
				FinishReason: "stop",
			}},
			Usage: models.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
		})
	}))
	defer server.Close()

	a := NewOpenAIAdapter("test-key", server.URL)
	resp, err := a.Complete(context.Background(), &models.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []models.Message{{
			Role:    "user",
			Content: "say hello",
		}},
	})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestOpenAIAdapterCompleteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"1\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	a := NewOpenAIAdapter("test-key", server.URL)
	ch, err := a.CompleteStream(context.Background(), &models.ChatRequest{Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("complete stream returned error: %v", err)
	}

	var lines []string
	for line := range ch {
		lines = append(lines, line)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 stream lines, got %d", len(lines))
	}
	if lines[1] != "data: [DONE]" {
		t.Fatalf("unexpected stream line: %q", lines[1])
	}
}

func TestOpenAIAdapterEstimateCost(t *testing.T) {
	a := NewOpenAIAdapter("test-key", "http://example.com")
	cost := a.EstimateCost(&models.ChatRequest{
		Messages:  []models.Message{{Role: "user", Content: "short prompt"}},
		MaxTokens: 100,
	})
	if cost <= 0 {
		t.Fatalf("expected cost > 0, got %f", cost)
	}
}

func TestOpenAIAdapterIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewOpenAIAdapter("test-key", server.URL)
	if !a.IsHealthy(context.Background()) {
		t.Fatal("expected healthy provider")
	}
}
