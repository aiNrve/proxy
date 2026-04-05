package adapter

import "testing"

func TestNewAnthropicAdapter(t *testing.T) {
	a := NewAnthropicAdapter("key", "http://example.com")
	if a == nil {
		t.Fatal("expected adapter instance")
	}
	if got := a.Name(); got != "anthropic" {
		t.Fatalf("expected anthropic, got %q", got)
	}
}
