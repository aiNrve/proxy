package adapter

import "testing"

func TestNewGeminiAdapter(t *testing.T) {
	a := NewGeminiAdapter("key", "http://example.com")
	if a == nil {
		t.Fatal("expected adapter instance")
	}
	if got := a.Name(); got != "gemini" {
		t.Fatalf("expected gemini, got %q", got)
	}
}
