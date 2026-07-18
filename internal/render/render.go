// Package render turns a bench.Report into the files that live in the repo:
// the JSON source-of-truth artifacts and the human-facing README leaderboard.
// Rendering is deterministic so re-running on the same Report yields identical
// bytes (clean diffs, no spurious churn).
package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vicaroai/tokens-per-second/internal/bench"
)

// WriteResults writes latest.json and the per-week history snapshot under
// resultsDir (typically benchmarks/results).
func WriteResults(resultsDir string, rep *bench.Report) error {
	if err := os.MkdirAll(filepath.Join(resultsDir, "history"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(resultsDir, "latest.json"), data, 0o644); err != nil {
		return fmt.Errorf("write latest.json: %w", err)
	}
	histPath := filepath.Join(resultsDir, "history", rep.ISOWeek+".json")
	if err := os.WriteFile(histPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", histPath, err)
	}
	return nil
}

// Leaderboard renders the results as a Markdown table.
func Leaderboard(rep *bench.Report) string {
	var b strings.Builder
	b.WriteString("| Rank | Provider | Model | Tokens/sec | TTFT (ms) | Reasoning | Output tokens | Run cost | Runs |\n")
	b.WriteString("|-----:|----------|-------|-----------:|----------:|-----------|--------------:|---------:|:----:|\n")
	rank := 0
	for _, r := range rep.Results {
		if !r.OK {
			fmt.Fprintf(&b, "| — | %s | `%s` | failed | — | — | — | %s | %d/%d |\n",
				r.Provider, r.Model, costLabel(r), r.SuccessfulRuns, r.TotalRuns)
			continue
		}
		rank++
		fmt.Fprintf(&b, "| %d | %s | `%s` | **%.1f** | %.0f | %s | %d | %s | %d/%d |\n",
			rank, r.Provider, r.Model, r.TokensPerSecond, r.TTFTMillis,
			reasoningLabel(r.ReasoningApplied), r.OutputTokens, costLabel(r), r.SuccessfulRuns, r.TotalRuns)
	}
	fmt.Fprintf(&b, "\n_Total cost of this run: **$%.4f**._\n", rep.TotalCostUSD)
	return b.String()
}

// reasoningLabel renders the applied reasoning level for the leaderboard.
// "none" means reasoning was off; "adaptive:low" (Anthropic) means reasoning
// could not be disabled and its lowest level was used — flagged with a marker
// so readers know that row's numbers include reasoning tokens.
func reasoningLabel(applied string) string {
	switch applied {
	case "", "default":
		return "default"
	case "none":
		return "off"
	default:
		return applied + " ⚠️"
	}
}

// costLabel renders a model's run cost, distinguishing an unpriced model
// (no price in config) from a real $0.00.
func costLabel(r bench.Result) string {
	if !r.Costed {
		return "—"
	}
	return fmt.Sprintf("$%.4f", r.CostUSD)
}

const (
	beginMarker = "<!-- BENCHMARK:BEGIN -->"
	endMarker   = "<!-- BENCHMARK:END -->"
)

// UpdateReadme replaces the region between the BENCHMARK markers in the README
// with a freshly rendered leaderboard + provenance line. The rest of the
// README (methodology links, how-it-works) is human-owned and never touched.
func UpdateReadme(readmePath string, rep *bench.Report) error {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	content := string(raw)
	start := strings.Index(content, beginMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 || end < start {
		return fmt.Errorf("README missing %s / %s markers", beginMarker, endMarker)
	}

	var section strings.Builder
	section.WriteString(beginMarker + "\n")
	fmt.Fprintf(&section, "_Last updated: **%s** (%s) · prompt capped at %d output tokens · median of %d runs._\n\n",
		rep.GeneratedAt.Format("2006-01-02"), rep.ISOWeek, rep.MaxTokens, rep.MeasuredRuns)
	section.WriteString(Leaderboard(rep))
	section.WriteString("\n")

	updated := content[:start] + section.String() + content[end:]
	return os.WriteFile(readmePath, []byte(updated), 0o644)
}
