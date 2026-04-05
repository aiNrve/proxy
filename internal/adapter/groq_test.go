package adapter

import "testing"

func TestNewGroqAdapter(t *testing.T) {
	a := NewGroqAdapter("key", "http://example.com")
	if a == nil {
		t.Fatal("expected adapter instance")
	}
	if got := a.Name(); got != "groq" {
		t.Fatalf("expected groq, got %q", got)
	}
}
