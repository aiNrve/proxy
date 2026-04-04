package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aiNrve/proxy/internal/models"
)

func TestGroqAdapterName(t *testing.T) {
	a := NewGroqAdapter("key", "http://example.com/openai")
	if got := a.Name(); got != "groq" {
		t.Fatalf("expected groq, got %q", got)
	}
}

func TestGroqAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.ChatResponse{
			ID:      "chatcmpl-groq",
			Object:  "chat.completion",
			Created: 1712500001,
			Model:   "llama-3.1-8b-instant",
			Choices: []models.Choice{{
				Index: 0,
				Message: models.Message{
					Role:    "assistant",
					Content: "groq-ok",
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer server.Close()

	a := NewGroqAdapter("groq-key", server.URL+"/openai")
	resp, err := a.Complete(context.Background(), &models.ChatRequest{Model: "llama-3.1-8b-instant"})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "groq-ok" {
		t.Fatalf("expected groq-ok, got %q", got)
	}
}

func TestGroqAdapterEstimateCost(t *testing.T) {
	a := NewGroqAdapter("groq-key", "http://example.com/openai")
	cost := a.EstimateCost(&models.ChatRequest{Messages: []models.Message{{Role: "user", Content: "cheap"}}, MaxTokens: 32})
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
}

func TestGroqAdapterIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openai/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	a := NewGroqAdapter("groq-key", server.URL+"/openai")
	if !a.IsHealthy(context.Background()) {
		t.Fatal("expected healthy provider")
	}
}
