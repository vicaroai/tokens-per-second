// Command tps runs the tokens-per-second benchmarks and renders the results.
//
// Usage:
//
//	tps bench   [-config PATH] [-results DIR] [-readme PATH]   run benchmarks, write results + update README
//	tps render  [-results DIR] [-readme PATH]                  re-render README from existing latest.json (no API calls)
//
// API keys are read from the environment (the api_key_env names in the config).
// tps never reads or writes keys to disk.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/vicaroai/tokens-per-second/internal/bench"
	"github.com/vicaroai/tokens-per-second/internal/render"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch os.Args[1] {
	case "bench":
		os.Exit(cmdBench(os.Args[2:], log))
	case "render":
		os.Exit(cmdRender(os.Args[2:], log))
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `tps — tokens-per-second benchmarks

Commands:
  bench    Run benchmarks against every model in the config, write results, update README.
  render   Re-render the README leaderboard from the existing latest.json (no API calls).

Run "tps <command> -h" for flags.
`)
}

func cmdBench(args []string, log *slog.Logger) int {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	configPath := fs.String("config", "benchmarks/models.yaml", "path to the benchmark config")
	resultsDir := fs.String("results", "benchmarks/results", "directory for result JSON files")
	readmePath := fs.String("readme", "README.md", "README to update with the leaderboard")
	_ = fs.Parse(args)

	cfg, err := bench.LoadConfig(*configPath)
	if err != nil {
		log.Error("load config", "error", err)
		return 1
	}

	// Cancel cleanly on Ctrl-C / SIGTERM so a partial run doesn't wedge CI.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("running benchmarks", "config", *configPath, "measured_runs", cfg.Defaults.MeasuredRuns)
	rep := bench.Run(ctx, cfg, log)

	if err := render.WriteResults(*resultsDir, rep); err != nil {
		log.Error("write results", "error", err)
		return 1
	}
	if err := render.UpdateReadme(*readmePath, rep); err != nil {
		log.Error("update readme", "error", err)
		return 1
	}

	ok, failed := tally(rep)
	log.Info("done", "results", filepath.Join(*resultsDir, "latest.json"), "ok", ok, "failed", failed)
	printCostSummary(rep)
	// A run where EVERY model failed is a real failure (bad keys, network) and
	// should fail CI. A partial failure still publishes what worked.
	if ok == 0 {
		log.Error("all models failed — not a usable benchmark")
		return 1
	}
	return 0
}

func cmdRender(args []string, log *slog.Logger) int {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	resultsDir := fs.String("results", "benchmarks/results", "directory holding latest.json")
	readmePath := fs.String("readme", "README.md", "README to update")
	_ = fs.Parse(args)

	raw, err := os.ReadFile(filepath.Join(*resultsDir, "latest.json"))
	if err != nil {
		log.Error("read latest.json", "error", err)
		return 1
	}
	var rep bench.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		log.Error("parse latest.json", "error", err)
		return 1
	}
	if err := render.UpdateReadme(*readmePath, &rep); err != nil {
		log.Error("update readme", "error", err)
		return 1
	}
	log.Info("README updated from latest.json")
	return 0
}

// printCostSummary writes a per-model + total cost breakdown to stdout at the
// end of a run, so the spend of each benchmark is visible immediately. Models
// without configured pricing are listed as "unpriced" so a missing price is
// never silently counted as $0.
func printCostSummary(rep *bench.Report) {
	fmt.Println()
	fmt.Println("Cost of this benchmark run (all warmup + measured calls):")
	fmt.Printf("  %-42s %12s\n", "MODEL", "COST (USD)")
	fmt.Printf("  %s\n", "----------------------------------------------------------")
	var unpriced int
	for _, r := range rep.Results {
		if !r.Costed {
			unpriced++
			fmt.Printf("  %-42s %12s\n", trunc(r.Provider+"/"+r.Model, 42), "unpriced")
			continue
		}
		fmt.Printf("  %-42s %12s\n", trunc(r.Provider+"/"+r.Model, 42), fmt.Sprintf("$%.4f", r.CostUSD))
	}
	fmt.Printf("  %s\n", "----------------------------------------------------------")
	fmt.Printf("  %-42s %12s\n", "TOTAL", fmt.Sprintf("$%.4f", rep.TotalCostUSD))
	if unpriced > 0 {
		fmt.Printf("  (%d model(s) unpriced — add a `price:` block in models.yaml to include them)\n", unpriced)
	}
	fmt.Println()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func tally(rep *bench.Report) (ok, failed int) {
	for _, r := range rep.Results {
		if r.OK {
			ok++
		} else {
			failed++
		}
	}
	return ok, failed
}
