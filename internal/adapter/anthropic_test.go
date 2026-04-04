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

func TestAnthropicAdapterName(t *testing.T) {
	a := NewAnthropicAdapter("key", "http://example.com")
	if got := a.Name(); got != "anthropic" {
		t.Fatalf("expected anthropic, got %q", got)
	}
}

func TestAnthropicAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Fatal("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Fatal("expected anthropic-version header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_1",
			"type":        "message",
			"model":       "claude-3-5-sonnet",
			"stop_reason": "end_turn",
			"content": []map[string]any{{
				"type": "text",
				"text": "anthropic-ok",
			}},
			"usage": map[string]any{
				"input_tokens":  12,
				"output_tokens": 6,
			},
		})
	}))
	defer server.Close()

	a := NewAnthropicAdapter("anthropic-key", server.URL)
	resp, err := a.Complete(context.Background(), &models.ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []models.Message{{
			Role:    "user",
			Content: "hello",
		}},
	})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "anthropic-ok" {
		t.Fatalf("expected anthropic-ok, got %q", got)
	}
	if got := resp.Choices[0].FinishReason; got != "stop" {
		t.Fatalf("expected stop finish reason, got %q", got)
	}
}

func TestAnthropicAdapterCompleteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body := make([]byte, 2048)
		n, _ := r.Body.Read(body)
		if !strings.Contains(string(body[:n]), "\"stream\":true") {
			t.Fatal("expected stream=true in anthropic request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"text\":\"hi\"}}\n\n"))
	}))
	defer server.Close()

	a := NewAnthropicAdapter("anthropic-key", server.URL)
	ch, err := a.CompleteStream(context.Background(), &models.ChatRequest{Model: "claude-3-5-sonnet"})
	if err != nil {
		t.Fatalf("complete stream returned error: %v", err)
	}

	var lines []string
	for line := range ch {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		t.Fatal("expected stream lines")
	}
}

func TestAnthropicAdapterEstimateCost(t *testing.T) {
	a := NewAnthropicAdapter("anthropic-key", "http://example.com")
	cost := a.EstimateCost(&models.ChatRequest{Messages: []models.Message{{Role: "user", Content: "anthropic"}}, MaxTokens: 128})
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
}

func TestAnthropicAdapterIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	a := NewAnthropicAdapter("anthropic-key", server.URL)
	if !a.IsHealthy(context.Background()) {
		t.Fatal("expected healthy provider")
	}
}
