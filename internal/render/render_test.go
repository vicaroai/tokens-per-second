package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicaroai/tokens-per-second/internal/bench"
)

func sampleReport() *bench.Report {
	return &bench.Report{
		SchemaVersion: 1,
		GeneratedAt:   time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		ISOWeek:       "2026-W29",
		MaxTokens:     512,
		MeasuredRuns:  3,
		TotalCostUSD:  0.0210,
		Results: []bench.Result{
			{Provider: "fireworks", Model: "llama-70b", OK: true, TokensPerSecond: 210.5, TTFTMillis: 180, ReasoningApplied: "none", OutputTokens: 512, CostUSD: 0.0012, Costed: true, SuccessfulRuns: 3, TotalRuns: 3},
			{Provider: "anthropic", Model: "claude-sonnet-5", OK: true, TokensPerSecond: 95.2, TTFTMillis: 420, ReasoningApplied: "adaptive:low", OutputTokens: 500, CostUSD: 0.0198, Costed: true, SuccessfulRuns: 3, TotalRuns: 3},
			{Provider: "deepseek", Model: "down-model", OK: false, Error: "http 503", SuccessfulRuns: 0, TotalRuns: 3},
		},
	}
}

func TestLeaderboard(t *testing.T) {
	out := Leaderboard(sampleReport())
	// Ranked winner first, reasoning shown as "off".
	if !strings.Contains(out, "| 1 | fireworks | `llama-70b` | **210.5** | 180 | off |") {
		t.Errorf("expected ranked winner row with reasoning off, got:\n%s", out)
	}
	// Anthropic's un-disable-able reasoning is flagged, not shown as "off".
	if !strings.Contains(out, "adaptive:low ⚠️") {
		t.Errorf("expected Anthropic reasoning flagged, got:\n%s", out)
	}
	// Cost per model shown; total cost line rendered.
	if !strings.Contains(out, "$0.0012") {
		t.Errorf("expected per-model cost, got:\n%s", out)
	}
	if !strings.Contains(out, "Total cost of this run: **$0.0210**") {
		t.Errorf("expected total cost line, got:\n%s", out)
	}
	// Failed model rendered, not dropped.
	if !strings.Contains(out, "| — | deepseek | `down-model` | failed |") {
		t.Errorf("expected failed model row, got:\n%s", out)
	}
}

func TestUpdateReadmeReplacesOnlyMarkedRegion(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	original := "# Title\n\nIntro stays.\n\n" + beginMarker + "\nOLD TABLE\n" + endMarker + "\n\nFooter stays.\n"
	if err := os.WriteFile(readme, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReadme(readme, sampleReport()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(readme)
	s := string(got)
	if !strings.Contains(s, "Intro stays.") || !strings.Contains(s, "Footer stays.") {
		t.Error("human-owned README content was clobbered")
	}
	if strings.Contains(s, "OLD TABLE") {
		t.Error("old table not replaced")
	}
	if !strings.Contains(s, "fireworks") {
		t.Error("new leaderboard not written")
	}
}

func TestUpdateReadmeMissingMarkers(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	_ = os.WriteFile(readme, []byte("# no markers here\n"), 0o644)
	if err := UpdateReadme(readme, sampleReport()); err == nil {
		t.Error("expected error when markers are missing")
	}
}
