# AGENTS.md

## Start here

- Read `docs/superpowers/specs/2026-11-02-kev-playground-design.md` before changing architecture. It is the source of truth for scope, pinned versions, and non-goals.
- This is a macOS Apple Silicon PoC under implementation. Config, paths, Kev API validation, runtime bootstrap/prefetch, and sidecar management exist; the console and TUI are still planned.

## Commands

- `make test` — Go tests plus browser-logic tests (browser is skipped until `web/` exists).
- `make test-go` — `go test ./...`
- `make test-web` — `node --test web/`
- `make vet` — `go vet ./...`
- `make build` — build `bin/gavel`
- Single Go test: `go test ./internal/version -run TestVersionDefault -v`
- Required before a commit: `make vet && make test`.

## What this is

- `gavel` is one Go binary. A Bubble Tea TUI handles first-run setup and launches a browser console.
- The browser console is three columns — State, Questions, Answers — and speaks Kev's typed-decision API (`noul`, `choice`, `score`). It is not a chat UI.
- Kev is a Python decision model. The Go app manages a pinned `uv` → Python 3.13 → Kev sidecar, then proxies `POST /api/systemone` to `kev.serve`.

## Hard constraints

- Kev is not a GGUF/Ollama/llama.cpp model. Do not propose generic inference engines.
- Do not use containers for the fast path: macOS Linux containers have no Metal, and Kev's MLX backend is host-only. The chosen path is native `uv`-managed Python + MLX.
- Kev is not on PyPI under this name (`pip install kev` is unrelated). Install from the pinned GitHub revision in the design doc.
- Model downloads are multi-GB. The TUI must show sizes and require consent. Default to Kev-0.8B (~1.7 GB); Kev-4B (~8.7 GB) is opt-in.
- Bind both servers to `127.0.0.1`. No auth in the PoC.
- First model load can take minutes; keep the sidecar health timeout long and show progress.
- The browser UI has no Node build step: plain HTML/CSS/ES modules embedded with `go:embed`. Keep pure logic in `web/model.js` for `node --test`.
- Use `uv sync --frozen`; do not silently fall back to unpinned dependencies.
- `GAVEL_HOME` overrides app data paths. Unit tests must use temp dirs and must never download models.
- Offline after setup: the sidecar forces `HF_HUB_OFFLINE=1` and `TRANSFORMERS_OFFLINE=1`. Only explicit setup/prefetch enables network downloads.

## Layout

- `cmd/gavel` — entrypoint (current skeleton).
- `internal/version` — build version (current skeleton).
- `internal/config`, `internal/runtime`, `internal/kevapi`, `internal/sidecar` — implemented runtime foundation; tests use fakes and helper subprocesses.
- Planned per the design doc: `internal/status`, `internal/app`, `internal/console`, `internal/tui`, `web/`.
- `.superpowers/` is local brainstorm state; it is gitignored.

## Workflow

- TDD: write a failing test first, then the smallest implementation, then commit at a green breakpoint.
- Every commit must pass `make vet && make test`.
- Keep browser logic testable without a browser; only thin DOM glue belongs in the page.
- Update this file if commands, layout, or constraints change.
