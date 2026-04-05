package adapter

import "testing"

func TestNewTogetherAdapter(t *testing.T) {
	a := NewTogetherAdapter("key", "http://example.com")
	if a == nil {
		t.Fatal("expected adapter instance")
	}
	if got := a.Name(); got != "together" {
		t.Fatalf("expected together, got %q", got)
	}
}
