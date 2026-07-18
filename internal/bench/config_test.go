package bench

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "models.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const validConfig = `
defaults:
  prompt: "hi"
  max_tokens: 128
  temperature: 0.0
  measured_runs: 3
  timeout_seconds: 120
providers:
  - id: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY
    models:
      - name: gpt-4o-mini
`

func TestLoadConfigValid(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Models[0].Name != "gpt-4o-mini" {
		t.Fatalf("config not parsed as expected: %+v", cfg)
	}
}

func TestReasoningFor(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Reasoning: "none"}}
	if got := cfg.ReasoningFor(Model{Name: "m"}); got != "none" {
		t.Errorf("default reasoning: got %q, want none", got)
	}
	if got := cfg.ReasoningFor(Model{Name: "m", Reasoning: "low"}); got != "low" {
		t.Errorf("model override: got %q, want low", got)
	}
	empty := &Config{}
	if got := empty.ReasoningFor(Model{Name: "m"}); got != "none" {
		t.Errorf("fallback reasoning: got %q, want none", got)
	}
}

func TestPriceCost(t *testing.T) {
	p := &Price{InputPerM: 1.00, OutputPerM: 10.00}
	// 1000 input @ $1/M = $0.001; 500 output @ $10/M = $0.005; total $0.006.
	if got := p.Cost(1000, 500); got < 0.00599 || got > 0.00601 {
		t.Errorf("Cost = %f, want ~0.006", got)
	}
	var nilPrice *Price
	if got := nilPrice.Cost(1000, 500); got != 0 {
		t.Errorf("nil price Cost = %f, want 0", got)
	}
}

func TestLoadConfigRejectsDuplicateModel(t *testing.T) {
	body := `
defaults:
  max_tokens: 128
  measured_runs: 1
providers:
  - id: openai
    base_url: "https://x"
    api_key_env: K
    models:
      - name: dup
      - name: dup
`
	if _, err := LoadConfig(writeConfig(t, body)); err == nil {
		t.Fatal("expected duplicate-model error")
	}
}

func TestLoadConfigRejectsZeroRuns(t *testing.T) {
	body := `
defaults:
  max_tokens: 128
  measured_runs: 0
providers:
  - id: openai
    base_url: "https://x"
    api_key_env: K
    models:
      - name: m
`
	if _, err := LoadConfig(writeConfig(t, body)); err == nil {
		t.Fatal("expected measured_runs validation error")
	}
}
