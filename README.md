# Gavel

A local Kev playground for macOS on Apple Silicon. One Go binary manages a pinned
`uv`/Python/Kev sidecar and serves a three-column browser console for typed
decisions.

**Status:** design stage. The design is in
[`docs/superpowers/specs/2026-11-02-kev-playground-design.md`](docs/superpowers/specs/2026-11-02-kev-playground-design.md).
The current skeleton only reports its version.

## What it will do

- TUI first-run setup: choose models, download the runtime and weights with
  progress, cancel/retry, and read logs.
- TUI launcher: open the browser console, switch models, restart the sidecar,
  quit.
- Browser console: **State → Questions → Answers**. Supports Kev's `noul`
  (yes/no), `choice`, and `score` questions and shows calibrated probabilities.
- Local-only: binds to `127.0.0.1`; no account or API key.

Kev is a decision model, not a chat model. This app does not generate text.

## Requirements

- macOS on Apple Silicon (arm64).
- Go 1.27+.
- Node 26+ for browser-logic tests.
- ~1.7 GB for Kev-0.8B weights, or ~8.7 GB for Kev-4B, plus the managed
  Python runtime.

## Development

```sh
make test        # Go tests + browser-logic tests (skipped until web/ exists)
make test-go     # go test ./...
make test-web    # node --test web/
make vet         # go vet ./...
make build       # build ./cmd/gavel into bin/gavel
make run         # go run ./cmd/gavel
```

Run one Go test:

```sh
go test ./internal/version -run TestVersionDefault -v
```

## Design and scope

The design document is the source of truth for architecture, first-run flow,
download sizes, pinned versions, and non-goals. Read it before changing the
runtime or UI structure.

## License

TBD.
