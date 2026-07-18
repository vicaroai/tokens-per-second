package bench

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/vicaroai/tokens-per-second/internal/provider"
)

// Run executes every model in the config and returns a Report. It is
// fail-open per model: a model whose runs all error is recorded with OK=false
// rather than aborting the whole benchmark, so one flaky provider never blocks
// the weekly update. Missing API keys skip that provider's models (recorded as
// errors) instead of panicking.
func Run(ctx context.Context, cfg *Config, log *slog.Logger) *Report {
	now := time.Now().UTC()
	year, week := now.ISOWeek()

	rep := &Report{
		SchemaVersion: 1,
		GeneratedAt:   now,
		ISOWeek:       fmt.Sprintf("%d-W%02d", year, week),
		Prompt:        cfg.Defaults.Prompt,
		MaxTokens:     cfg.Defaults.MaxTokens,
		Temperature:   cfg.Defaults.Temperature,
		MeasuredRuns:  cfg.Defaults.MeasuredRuns,
		WarmupRuns:    cfg.Defaults.WarmupRuns,
	}

	for _, p := range cfg.Providers {
		apiKey := os.Getenv(p.APIKeyEnv)
		client, err := provider.New(p.ID, p.BaseURL, apiKey)
		for _, m := range p.Models {
			if err != nil {
				log.Warn("skipping model: provider unavailable", "provider", p.ID, "model", m.Name, "error", err)
				rep.Results = append(rep.Results, Result{
					Provider: p.ID, Model: m.Name, OK: false,
					Error: err.Error(), TotalRuns: cfg.Defaults.MeasuredRuns,
				})
				continue
			}
			rep.Results = append(rep.Results, benchmarkModel(ctx, cfg.Defaults, cfg.ReasoningFor(m), client, p, m, log))
		}
	}

	for _, r := range rep.Results {
		rep.TotalCostUSD += r.CostUSD
	}
	sortResults(rep.Results)
	return rep
}

// benchmarkModel runs warmup + measured runs for one model and returns the
// median-based Result.
func benchmarkModel(ctx context.Context, d Defaults, reasoning string, client provider.Client, p Provider, m Model, log *slog.Logger) Result {
	req := provider.Request{
		Model:       m.Name,
		Prompt:      d.Prompt,
		MaxTokens:   d.MaxTokens,
		Temperature: d.Temperature,
		Reasoning:   reasoning,
	}
	timeout := time.Duration(d.TimeoutSeconds) * time.Second

	// tokensIn/Out accumulate across EVERY call (warmup + measured) so the cost
	// reflects what the run actually spent, not just the median.
	var tokensIn, tokensOut int
	accrue := func(sr provider.StreamResult) {
		tokensIn += sr.InputTokens
		tokensOut += sr.OutputTokens
	}

	// Warmup runs are discarded from timing — they prime DNS, TLS, and
	// provider-side routing/autoscaling — but they still cost money, so their
	// tokens count toward the run cost.
	for i := 0; i < d.WarmupRuns; i++ {
		if sr, err := runOnce(ctx, client, req, timeout); err == nil {
			accrue(sr)
		}
	}

	res := Result{Provider: p.ID, Model: m.Name, TotalRuns: d.MeasuredRuns}
	var samples []provider.StreamResult
	var lastErr error
	for i := 0; i < d.MeasuredRuns; i++ {
		sr, err := runOnce(ctx, client, req, timeout)
		if err != nil {
			lastErr = err
			log.Warn("measured run failed", "provider", p.ID, "model", m.Name, "run", i+1, "error", err)
			continue
		}
		accrue(sr)
		samples = append(samples, sr)
	}

	res.SuccessfulRuns = len(samples)
	if len(samples) == 0 {
		res.OK = false
		if lastErr != nil {
			res.Error = lastErr.Error()
		} else {
			res.Error = "all runs failed"
		}
		return res
	}

	med := medianSample(samples)
	res.OK = true
	res.OutputTokens = med.OutputTokens
	res.InputTokens = med.InputTokens
	res.UsageReported = med.UsageReported
	res.ReasoningApplied = med.ReasoningApplied
	res.TTFTMillis = float64(med.TimeToFirstToken.Microseconds()) / 1000.0
	secs := med.TotalDuration.Seconds()
	if secs > 0 {
		res.TokensPerSecond = float64(med.OutputTokens) / secs
	}
	if m.Price != nil {
		res.Costed = true
		res.CostUSD = m.Price.Cost(tokensIn, tokensOut)
	}
	log.Info("benchmarked", "provider", p.ID, "model", m.Name,
		"tps", fmt.Sprintf("%.1f", res.TokensPerSecond), "ttft_ms", fmt.Sprintf("%.0f", res.TTFTMillis),
		"cost_usd", fmt.Sprintf("%.4f", res.CostUSD))
	return res
}

func runOnce(ctx context.Context, client provider.Client, req provider.Request, timeout time.Duration) (provider.StreamResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Stream(runCtx, req)
}

// medianSample picks the sample with the median tokens-per-second. Using a
// whole sample (rather than per-field medians) keeps each reported result an
// internally consistent, real observation.
func medianSample(samples []provider.StreamResult) provider.StreamResult {
	sorted := make([]provider.StreamResult, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool {
		return tps(sorted[i]) < tps(sorted[j])
	})
	return sorted[len(sorted)/2]
}

func tps(s provider.StreamResult) float64 {
	if s.TotalDuration.Seconds() <= 0 {
		return 0
	}
	return float64(s.OutputTokens) / s.TotalDuration.Seconds()
}

// sortResults orders the leaderboard: successful models by descending
// tokens/sec, then failed models last (alphabetical) so gaps are visible but
// don't clutter the top.
func sortResults(rs []Result) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].OK != rs[j].OK {
			return rs[i].OK
		}
		if rs[i].OK {
			return rs[i].TokensPerSecond > rs[j].TokensPerSecond
		}
		if rs[i].Provider != rs[j].Provider {
			return rs[i].Provider < rs[j].Provider
		}
		return rs[i].Model < rs[j].Model
	})
}
