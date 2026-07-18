package bench

import "time"

// Report is the top-level results document written to
// benchmarks/results/latest.json and history/YYYY-WW.json. It is the
// machine-readable source of truth; the README table is rendered from it.
type Report struct {
	// SchemaVersion lets downstream consumers (and future us) handle format
	// changes without guessing. Bump on breaking changes to this struct.
	SchemaVersion int `json:"schema_version"`
	// GeneratedAt is when the run completed (UTC, RFC3339).
	GeneratedAt time.Time `json:"generated_at"`
	// ISOWeek identifies the weekly snapshot, e.g. "2026-W29".
	ISOWeek string `json:"iso_week"`
	// Prompt and settings are recorded so a result is self-describing and
	// reproducible without cross-referencing the config at that commit.
	Prompt       string   `json:"prompt"`
	MaxTokens    int      `json:"max_tokens"`
	Temperature  float64  `json:"temperature"`
	MeasuredRuns int      `json:"measured_runs"`
	WarmupRuns   int      `json:"warmup_runs"`
	Results      []Result `json:"results"`
	// TotalCostUSD is the estimated cost of the WHOLE run — every model, every
	// warmup + measured call — using the per-model prices in the config. Zero
	// for models without pricing (see Result.Costed).
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Result is one model's measured outcome. Timing fields are the MEDIAN across
// measured runs (warmup runs excluded). A model that failed every run is still
// recorded with OK=false and an Error, so the leaderboard is honest about gaps
// rather than silently dropping models.
type Result struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`

	// TokensPerSecond is output tokens / total duration (median run). This is
	// the headline number.
	TokensPerSecond float64 `json:"tokens_per_second"`
	// TTFTMillis is time-to-first-token in milliseconds (median run).
	TTFTMillis float64 `json:"ttft_millis"`
	// OutputTokens is the token count of the median run.
	OutputTokens int `json:"output_tokens"`
	// UsageReported is true when the token count came from the provider's own
	// usage field rather than a client-side estimate. Surfaced for honesty.
	UsageReported bool `json:"usage_reported"`
	// ReasoningApplied is the reasoning level actually sent to the provider,
	// e.g. "none" (reasoning off) or "adaptive:low" for Anthropic models which
	// cannot fully disable thinking. Lets readers see which rows include
	// reasoning tokens — the numbers are NOT comparable across reasoning levels.
	ReasoningApplied string `json:"reasoning_applied"`
	// SuccessfulRuns / TotalRuns show how many measured runs succeeded.
	SuccessfulRuns int `json:"successful_runs"`
	TotalRuns      int `json:"total_runs"`

	// InputTokens is the prompt-token count of the median run (for cost).
	InputTokens int `json:"input_tokens"`
	// CostUSD is this model's estimated contribution to the run cost: the sum
	// of every warmup + measured call's input/output tokens at the model's
	// price. Zero when the model has no configured price.
	CostUSD float64 `json:"cost_usd"`
	// Costed is false when the model has no price in the config, so a $0.00 can
	// be distinguished from "unpriced".
	Costed bool `json:"costed"`
}
