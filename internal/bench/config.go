// Package bench defines the benchmark configuration, the run loop, and the
// tokens-per-second measurement math.
package bench

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the parsed benchmarks/models.yaml. It is the source of truth for
// what gets benchmarked; see the file's header comment.
type Config struct {
	Defaults  Defaults   `yaml:"defaults"`
	Providers []Provider `yaml:"providers"`
}

// Defaults are applied to every model unless a model overrides them.
type Defaults struct {
	Prompt      string  `yaml:"prompt"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	// Reasoning is the default reasoning/thinking level for every model. We
	// benchmark generation speed, so the default is "none" (reasoning off).
	// Each adapter maps this to its provider's wire format; providers that
	// cannot disable reasoning fall back to their lowest level (see Reasoning).
	Reasoning      string `yaml:"reasoning"`
	WarmupRuns     int    `yaml:"warmup_runs"`
	MeasuredRuns   int    `yaml:"measured_runs"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// Provider is one API endpoint and the models to benchmark on it.
type Provider struct {
	ID        string  `yaml:"id"`
	BaseURL   string  `yaml:"base_url"`
	APIKeyEnv string  `yaml:"api_key_env"`
	Models    []Model `yaml:"models"`
}

// Model is a single benchmark target. Reasoning, when set, overrides the
// default reasoning level for just this model (e.g. a model that rejects
// "none" can be pinned to "low").
type Model struct {
	Name      string `yaml:"name"`
	Reasoning string `yaml:"reasoning,omitempty"`
	// Price is the per-model pricing used for cost estimation. Optional — a
	// model without it reports zero cost (and is flagged in the config check).
	// Human-maintained: prices change, so they live in config, not code.
	Price *Price `yaml:"price,omitempty"`
}

// Price is USD per 1,000,000 tokens, input and output.
type Price struct {
	InputPerM  float64 `yaml:"input_per_m"`
	OutputPerM float64 `yaml:"output_per_m"`
}

// Cost returns the USD cost of a completion with the given token counts.
func (p *Price) Cost(inputTokens, outputTokens int) float64 {
	if p == nil {
		return 0
	}
	return float64(inputTokens)/1e6*p.InputPerM + float64(outputTokens)/1e6*p.OutputPerM
}

// ReasoningFor returns the effective reasoning level for a model: the model's
// own override if set, else the config default, else "none".
func (c *Config) ReasoningFor(m Model) string {
	switch {
	case m.Reasoning != "":
		return m.Reasoning
	case c.Defaults.Reasoning != "":
		return c.Defaults.Reasoning
	default:
		return "none"
	}
}

// LoadConfig reads and validates a models.yaml file.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Defaults.MeasuredRuns < 1 {
		return fmt.Errorf("defaults.measured_runs must be >= 1")
	}
	if c.Defaults.MaxTokens < 1 {
		return fmt.Errorf("defaults.max_tokens must be >= 1")
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("no providers defined")
	}
	seen := map[string]bool{}
	for _, p := range c.Providers {
		if p.ID == "" {
			return fmt.Errorf("provider with empty id")
		}
		if p.BaseURL == "" {
			return fmt.Errorf("provider %q: empty base_url", p.ID)
		}
		if p.APIKeyEnv == "" {
			return fmt.Errorf("provider %q: empty api_key_env", p.ID)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("provider %q: no models", p.ID)
		}
		for _, m := range p.Models {
			key := p.ID + "/" + m.Name
			if seen[key] {
				return fmt.Errorf("duplicate model %q", key)
			}
			seen[key] = true
		}
	}
	return nil
}
