package adapter

import "testing"

func TestNewOpenAIAdapter(t *testing.T) {
	a := NewOpenAIAdapter("key", "http://example.com")
	if a == nil {
		t.Fatal("expected adapter instance")
	}
	if got := a.Name(); got != "openai" {
		t.Fatalf("expected openai, got %q", got)
	}
}
