package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLoaderMergesFileAndEnvironment(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("OPENAI_API_KEY", "env-openai")

	dir := t.TempDir()
	path := filepath.Join(dir, "ainrve.yaml")
	content := `
port: 8081
log_level: warn
providers:
  openai:
    enabled: true
    api_key: file-openai
    base_url: https://api.openai.com
    weight: 1.2
routing:
  default_strategy: balanced
  fallback_provider: openai
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewLoader(path)
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}

	cfg := loader.Get()
	if cfg.Port != 9090 {
		t.Fatalf("expected env port 9090, got %d", cfg.Port)
	}
	if got := cfg.Providers["openai"].APIKey; got != "env-openai" {
		t.Fatalf("expected env api key override, got %q", got)
	}
}

func TestReloadAppliesFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ainrve.yaml")

	first := `
routing:
  default_strategy: cost
  weight_cost: 0.9
  weight_latency: 0.1
`
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatalf("write first config: %v", err)
	}

	loader, err := NewLoader(path)
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	if got := loader.Get().Routing.DefaultStrategy; got != "cost" {
		t.Fatalf("expected strategy cost, got %q", got)
	}

	second := `
routing:
  default_strategy: latency
  weight_cost: 0.2
  weight_latency: 0.8
`
	if err := os.WriteFile(path, []byte(second), 0o600); err != nil {
		t.Fatalf("write second config: %v", err)
	}

	if err := loader.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	cfg := loader.Get()
	if cfg.Routing.DefaultStrategy != "latency" {
		t.Fatalf("expected strategy latency, got %q", cfg.Routing.DefaultStrategy)
	}
	if cfg.Routing.WeightLatency != 0.8 {
		t.Fatalf("expected weight_latency 0.8, got %f", cfg.Routing.WeightLatency)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ainrve.yaml")
	content := `
providers:
  groq:
    enabled: true
    api_key: groq-key
routing:
  task_overrides:
    code: groq
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewLoader(path)
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}

	cfg := loader.Get()
	provider := cfg.Providers["groq"]
	provider.APIKey = "mutated"
	cfg.Providers["groq"] = provider
	cfg.Routing.TaskOverrides["code"] = "openai"

	next := loader.Get()
	if got := next.Providers["groq"].APIKey; got != "groq-key" {
		t.Fatalf("expected internal provider key unchanged, got %q", got)
	}
	if got := next.Routing.TaskOverrides["code"]; got != "groq" {
		t.Fatalf("expected internal task override unchanged, got %q", got)
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Setenv("CONFIG_PATH", "")

	resolved := ResolveConfigPath("ainrve.yaml")
	if resolved == "" {
		t.Fatal("expected resolved path to be non-empty")
	}

	t.Setenv("CONFIG_PATH", "custom-config.yaml")
	resolvedFromEnv := ResolveConfigPath("ignored.yaml")
	if filepath.Base(resolvedFromEnv) != "custom-config.yaml" {
		t.Fatalf("expected CONFIG_PATH to win, got %q", resolvedFromEnv)
	}
}
