# Methodology

These benchmarks aim to be **honest, reproducible, and comparable over time**.
This document describes exactly what is measured so the numbers can be trusted
and independently checked. If any of this is unclear or seems unfair, open an
issue — the methodology is meant to be scrutinized.

## What we measure

For each model we stream a single completion and record:

- **Tokens per second (headline)** — completion (output) tokens ÷ total
  wall-clock streaming duration, from request send to stream close.
- **Time to first token (TTFT)** — wall-clock from request send to the first
  streamed content token, in milliseconds.

Both are reported as the **median of several measured runs** (default 3), after
discarding **warmup runs** (default 1). Median, not mean, so a single slow run
(a GC pause on the provider, a network blip) doesn't skew the number.

## What "tokens" means

We prefer each **provider's own reported completion-token count** (from the
`usage` block on the streamed response). When a provider does not return usage
on streamed responses, we fall back to a **client-side estimate** (counting
streamed content deltas). Every result records `usage_reported: true|false` so
you can see which number is authoritative and which is an estimate. Because
tokenizers differ between providers, token counts are **not** identical across
models for the same text — this is an inherent limitation of cross-provider
token comparison, not a bug.

## Reasoning is turned OFF (where possible)

We benchmark **raw generation speed**, so reasoning/thinking is disabled by
default (`reasoning: none` in the config). Each provider expresses this
differently, and the adapter maps it to the right wire format:

| Provider | How reasoning-off is requested | Can it be fully off? |
|----------|--------------------------------|----------------------|
| OpenAI (gpt-5.x) | `reasoning_effort: "none"` | ✅ Yes (except gpt-5-nano — see below) |
| Fireworks (GLM, DeepSeek, MiniMax) | `reasoning_effort: "none"` | ✅ Yes |
| DeepSeek (native API) | `thinking: {type: "disabled"}` | ✅ Yes |
| Anthropic (Opus 4.8, Sonnet 5, Haiku 4.5) | `thinking: {type: "disabled"}` | ✅ Yes |

These wire formats were **verified empirically against each live API**, not just
from docs — the published docs were inaccurate in two places we hit:

- **`gpt-5-nano`** rejects `reasoning_effort: "none"`; its lowest level is
  `"minimal"`, so it is pinned to that in the config. Its row therefore includes
  a little reasoning and is flagged (`minimal`) rather than shown as off.
- **Anthropic** *can* disable thinking after all: `thinking: {type: "disabled"}`
  is accepted and genuinely suppresses the thinking block (verified — the
  response contains no `thinking` content and output-token counts drop to the
  bare answer). The docs claiming it returns 400 were wrong.

Every result records the reasoning level actually applied (`reasoning_applied`),
and the leaderboard shows it (`off`, or a flag for anything non-off) so any row
that still includes reasoning is visible at a glance.

## Cost

Each run prints — and records in `latest.json` — its **estimated USD cost**:
the sum over every model of every call it made (warmup **and** measured, since
warmups cost real money) priced at the per-model `price` in
[`benchmarks/models.yaml`](../benchmarks/models.yaml) (`input_per_m` /
`output_per_m`, USD per 1M tokens). Prices are **human-maintained list prices** —
they are an estimate for budgeting, not a billing figure, and should be kept
current via PR. A model with no configured price is reported as `unpriced` (never
silently counted as $0). The leaderboard shows per-model run cost and a run total.

## Controls for fairness

- **Identical prompt** for every model (see `prompt` in
  [`benchmarks/models.yaml`](../benchmarks/models.yaml)).
- **Identical output cap** (`max_tokens`) so no model is rewarded for stopping
  early or penalized for rambling.
- **Temperature 0.0** — we measure speed, not output quality.
- **Warmup runs discarded** to prime DNS/TLS and provider-side routing and
  autoscaling, so we report steady-state, not cold-start.
- Each reported result is a **single real run** (the median sample), not an
  average of fields stitched from different runs — so the numbers are
  internally consistent.

## Known caveats (read before quoting a number)

- **Network location matters.** Runs execute from GitHub-hosted runners
  (currently `ubuntu-latest`, typically US regions). A provider whose endpoint
  is far from the runner will show higher TTFT. This measures throughput *as
  seen from the runner*, not the provider's best-case regional latency.
- **Time-of-day / load.** Providers autoscale and get congested. A weekly
  snapshot is a sample, not a guarantee. The `history/` snapshots let you see
  variance over time.
- **Model routing.** Some hosts route a model name to different hardware or
  quantizations over time. We record the model *name* as configured; we can't
  see the backend it lands on.
- **max_tokens truncation.** Fast models often hit the output cap; the number
  reflects throughput up to the cap, which is what we intend to compare.

## Reproducing a result

Everything needed to reproduce a run is committed:

- The exact `prompt`, `max_tokens`, `temperature`, and `measured_runs` are
  recorded **inside** each `latest.json` / history snapshot, not just in the
  config — so a historical result is self-describing even if the config later
  changes.
- Run it yourself: `go run ./cmd/tps bench` with the relevant API keys set (see
  the [README](../README.md)).

## Changing the methodology

Changes to how we measure (prompt, run counts, token accounting) are made via
pull request to this file and the code, and should bump `schema_version` in
[`internal/bench/result.go`](../internal/bench/result.go) if the results format
changes. Methodology changes make old and new snapshots not strictly
comparable — call that out in the PR.
