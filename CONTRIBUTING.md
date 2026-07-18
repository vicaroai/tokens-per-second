# Contributing

Thanks for helping keep these benchmarks accurate and useful.

## Adding a model

Edit [`benchmarks/models.yaml`](benchmarks/models.yaml) and add a `name` under
the relevant provider's `models:` list. Open a PR. That's it — no code change.

## Adding a provider

1. If the provider exposes an **OpenAI-compatible** `/chat/completions`
   streaming API (most hosts do), you may only need a config entry — register
   its `id` in the OpenAI adapter's `init()` in
   [`internal/provider/openai.go`](internal/provider/openai.go) and add the
   provider block to `models.yaml`.
2. If it has a **different API** (like Anthropic's Messages API), add a new file
   in [`internal/provider/`](internal/provider/) implementing the `Client`
   interface from [`provider.go`](internal/provider/provider.go), and
   `register(...)` it. Keep timing logic in the adapter minimal — the
   tokens/sec math lives in `internal/bench`.

## API keys

Never commit keys. Locally they live in a gitignored `.env`; in CI they are
GitHub Actions **repository secrets** named exactly as the `api_key_env` value
in the config (e.g. `OPENAI_API_KEY`). When you add a provider, add the matching
secret in repo settings and wire it into
[`.github/workflows/benchmark.yml`](.github/workflows/benchmark.yml).

## Ground rules

- **Don't hand-edit generated output** — the README leaderboard region and
  everything under `benchmarks/results/` are written by the benchmark bot.
- **Methodology changes go through [docs/METHODOLOGY.md](docs/METHODOLOGY.md)**
  and should be called out explicitly (they can make old snapshots
  non-comparable).
- **CI must pass:** `gofmt`, `go vet`, `golangci-lint`, `go test -race`, and
  `go build`. Run `go test ./...` and `gofmt -l .` before pushing.
- Keep the harness simple and readable — this repo is public-facing.

## Secret scanning (do this first)

This repo talks to paid LLM APIs, so a leaked key is the worst-case mistake.
Install the pre-commit secret-scan hook once after cloning:

```bash
scripts/install-hooks.sh      # sets core.hooksPath -> .githooks
```

It blocks any commit that stages a provider-key-shaped string or a real `.env`.
CI also runs gitleaks server-side as an unbypassable backstop, so even without
the hook a secret can't merge — but the hook catches it before it ever enters
your local history. Never paste real keys into code, tests, or commit messages;
keys belong only in a gitignored `.env` (local) or GitHub Actions secrets (CI).

## Local dev

```bash
go test ./...
go run ./cmd/tps bench     # needs at least one provider key set
```
