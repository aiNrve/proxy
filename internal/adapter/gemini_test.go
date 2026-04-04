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

func TestGeminiAdapterName(t *testing.T) {
	a := NewGeminiAdapter("key", "http://example.com")
	if got := a.Name(); got != "gemini" {
		t.Fatalf("expected gemini, got %q", got)
	}
}

func TestGeminiAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") == "" {
			t.Fatal("expected api key query parameter")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{{
						"text": "gemini-ok",
					}},
				},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":     11,
				"candidatesTokenCount": 5,
				"totalTokenCount":      16,
			},
		})
	}))
	defer server.Close()

	a := NewGeminiAdapter("gemini-key", server.URL)
	resp, err := a.Complete(context.Background(), &models.ChatRequest{Model: "gemini-1.5-flash"})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "gemini-ok" {
		t.Fatalf("expected gemini-ok, got %q", got)
	}
	if got := resp.Choices[0].FinishReason; got != "stop" {
		t.Fatalf("expected stop finish reason, got %q", got)
	}
}

func TestGeminiAdapterCompleteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	a := NewGeminiAdapter("gemini-key", server.URL)
	ch, err := a.CompleteStream(context.Background(), &models.ChatRequest{Model: "gemini-1.5-flash"})
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
}

func TestGeminiAdapterEstimateCost(t *testing.T) {
	a := NewGeminiAdapter("gemini-key", "http://example.com")
	cost := a.EstimateCost(&models.ChatRequest{Messages: []models.Message{{Role: "user", Content: "hello gemini"}}, MaxTokens: 100})
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
}

func TestGeminiAdapterIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1beta/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	a := NewGeminiAdapter("gemini-key", server.URL)
	if !a.IsHealthy(context.Background()) {
		t.Fatal("expected healthy provider")
	}
}
