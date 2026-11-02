# Gavel

A local Kev playground for macOS on Apple Silicon. One Go binary manages a pinned
`uv`/Python/Kev sidecar and serves a three-column browser console for typed
decisions.

**Status:** implemented PoC; automated tests, a stub-backed browser smoke test,
and real Kev-0.8B acceptance on Apple Silicon pass. Real acceptance includes
MLX/bfloat16 inference, offline restart, missing-cache recovery, and shutdown.
The multi-GB test remains opt-in. [Acceptance evidence](docs/acceptance/2026-11-02-kev-08b.md).
The design is in
[`docs/superpowers/specs/2026-11-02-kev-playground-design.md`](docs/superpowers/specs/2026-11-02-kev-playground-design.md).

## What it does

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
- At least 8 GiB unified memory for Kev-0.8B; 32 GiB for Kev-4B.
- ~1.71 GB for Kev-0.8B weights, or ~8.85 GB for Kev-4B, plus the managed
  Python runtime.

## Run

```sh
make build
bin/gavel             # setup/launcher TUI
bin/gavel --version
```

On first run, select models and press Enter to consent to downloading the pinned
runtime and weights. Kev-0.8B is selected by default; Kev-4B is opt-in. First load
may take several minutes. Press `o` at Ready to open the console and try the preset.
Use `m` for model management, `r` to restart, `l` for logs, and `q` to stop and quit.
`Esc` cancels a download without deleting completed caches.

Both HTTP servers bind to `127.0.0.1`. Ordinary launches use only local model
caches (`HF_HUB_OFFLINE=1`, `TRANSFORMERS_OFFLINE=1`); missing cache files require
explicit setup again. There is no unpinned dependency fallback.

Config/runtime/logs live in `~/Library/Application Support/gavel/`; model and uv
caches live in `~/Library/Caches/gavel/`. Set `GAVEL_HOME` to put everything beneath
one directory, e.g. `GAVEL_HOME="$HOME/gavel-dev" bin/gavel`. The app never deletes
model caches automatically.

## Development

```sh
make test        # Go tests + browser-logic tests; no model downloads
make test-go     # go test ./...
make test-web    # node --test web/
make vet         # go vet ./...
make build       # build ./cmd/gavel into bin/gavel
make run         # go run ./cmd/gavel
make e2e         # opt-in: real runtime/model downloads into a temp directory
```

`make e2e` explicitly opts into a multi-GB download; it is skipped in normal tests
and CI. It builds a fresh runtime, starts Kev, submits all three question types,
checks offline restart/missing-cache failure, and verifies clean shutdown. Each
run uses a fresh temporary home and removes the downloaded runtime/model cache
when finished; repeated runs download again. Unit tests use temporary paths and
fake commands or small helper processes instead of real models. If `python3` is
available, Go tests also run dependency-free Python helper regression tests.

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
