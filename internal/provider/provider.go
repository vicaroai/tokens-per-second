// Package provider defines the interface every LLM backend implements, plus
// the concrete adapters. Adding a new provider means adding one file here that
// satisfies Client — the runner and config layers stay untouched.
package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Request is a single streaming completion to measure.
type Request struct {
	Model       string
	Prompt      string
	MaxTokens   int
	Temperature float64
	// Reasoning is the desired reasoning/thinking level ("none", "low", ...).
	// Each adapter maps it to its provider's wire format; a provider that
	// cannot honor the exact level (e.g. Anthropic cannot do "none") applies
	// the closest it supports and reports what it actually used via
	// StreamResult.ReasoningApplied.
	Reasoning string
}

// StreamResult is the raw timing captured from ONE streaming completion. The
// tokens-per-second math lives in package bench, not here — providers only
// report what they observed on the wire.
type StreamResult struct {
	// TimeToFirstToken is wall-clock from request send to the first streamed
	// content token.
	TimeToFirstToken time.Duration
	// TotalDuration is wall-clock from request send to stream close.
	TotalDuration time.Duration
	// OutputTokens is the number of completion tokens produced. Prefer the
	// provider's own usage count; fall back to a client-side count when the
	// provider does not report usage on streamed responses.
	OutputTokens int
	// InputTokens is the prompt (input) token count from the provider's usage,
	// used for cost estimation. Zero if the provider didn't report it.
	InputTokens int
	// UsageReported is true when OutputTokens came from the provider's usage
	// field rather than a client-side estimate. Recorded for transparency.
	UsageReported bool
	// ReasoningApplied is the reasoning level the adapter actually sent to the
	// provider (e.g. "none", or "adaptive:low" for Anthropic which cannot fully
	// disable thinking). Surfaced so the leaderboard can flag models whose
	// numbers include reasoning.
	ReasoningApplied string
}

// Client streams one completion and reports its timing. Implementations must
// be safe to call sequentially; the runner never calls Stream concurrently for
// the same Client.
type Client interface {
	// ID is the provider id from models.yaml (e.g. "openai").
	ID() string
	// Stream runs one completion and returns its raw timing.
	Stream(ctx context.Context, req Request) (StreamResult, error)
}

// Factory builds a Client for a provider config. baseURL and apiKey come from
// models.yaml (key resolved from its env var by the caller).
type Factory func(id, baseURL, apiKey string) (Client, error)

// registry maps a provider id to the adapter that speaks its API. Most hosts
// are OpenAI-compatible; Anthropic has its own Messages API.
var registry = map[string]Factory{}

// register wires a provider id to its factory. Called from adapter init().
func register(id string, f Factory) { registry[id] = f }

// secretLike matches token/key-shaped substrings (sk-…, fw_…, long bearer
// blobs) so a verbose provider error can't carry a credential fragment into a
// public commit or CI log.
var secretLike = regexp.MustCompile(`(?i)\b(sk|fw|pk|key|token|bearer)[-_ ]?[A-Za-z0-9._-]{12,}`)

// scrubError trims and redacts a provider error body before it is surfaced.
// Belt-and-suspenders for a public repo: results and PR bodies are world-
// readable, so no upstream error text should ever echo a key back out.
func scrubError(s string) string {
	return strings.TrimSpace(secretLike.ReplaceAllString(s, "[redacted]"))
}

// New builds the Client for the given provider id.
func New(id, baseURL, apiKey string) (Client, error) {
	f, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (no adapter registered)", id)
	}
	return f(id, baseURL, apiKey)
}
