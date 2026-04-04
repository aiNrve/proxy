package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aiNrve/proxy/internal/models"
)

func TestTogetherAdapterName(t *testing.T) {
	a := NewTogetherAdapter("key", "http://example.com")
	if got := a.Name(); got != "together" {
		t.Fatalf("expected together, got %q", got)
	}
}

func TestTogetherAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.ChatResponse{
			ID:      "chatcmpl-together",
			Object:  "chat.completion",
			Created: 1712500002,
			Model:   "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			Choices: []models.Choice{{
				Index: 0,
				Message: models.Message{
					Role:    "assistant",
					Content: "together-ok",
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer server.Close()

	a := NewTogetherAdapter("together-key", server.URL)
	resp, err := a.Complete(context.Background(), &models.ChatRequest{Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "together-ok" {
		t.Fatalf("expected together-ok, got %q", got)
	}
}

func TestTogetherAdapterEstimateCost(t *testing.T) {
	a := NewTogetherAdapter("together-key", "http://example.com")
	cost := a.EstimateCost(&models.ChatRequest{Messages: []models.Message{{Role: "user", Content: "hello"}}, MaxTokens: 64})
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
}

func TestTogetherAdapterIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	a := NewTogetherAdapter("together-key", server.URL)
	if !a.IsHealthy(context.Background()) {
		t.Fatal("expected healthy provider")
	}
}
