# Gavel — Kev Playground Design

**Date:** 2026-11-02
**Status:** Draft for approval
**Repo:** `github.com/jdtobe/gavel`
**Target:** macOS on Apple Silicon (arm64), Go 1.27+

## Intent

Build a local playground that makes Kev usable without the user touching Python, `uv`, MLX, or the Hugging Face Hub. The user launches one Go binary. A terminal UI handles first-run setup — dependency download options, model choice, progress, and errors — then stays open as the launcher. From there the user opens a browser console, pastes a state, adds typed questions, and reads Kev's calibrated probabilities.

Kev is a decision model, not a chat model. It answers three question types:

| Type | Question | Answer |
|---|---|---|
| `noul` | Is this true? | probability of yes |
| `choice` | Pick one of N | chosen option, full distribution, confidence |
| `score` | Where on this scale? | mean level, legend, distribution, confidence |

The PoC exists to make that loop fast to try on a laptop. It is not a chat UI and does not pretend Kev generates prose.

## Goals

- One `gavel` binary on macOS Apple Silicon; no Python knowledge required.
- TUI owns first-run setup: what to download, model choice, progress, cancel/retry, and diagnostics.
- TUI remains the launcher: open the browser console, switch models, restart the sidecar, view logs, quit.
- Browser console with the three-column layout: **State → Questions → Answers**.
- Support `noul`, `choice`, and `score` questions with add/edit/delete.
- Show calibrated probabilities, confidence, score legend, latency, and token usage.
- Local-only: bind to `127.0.0.1`, no account, no API key.
- Test-driven development for Go and browser logic.

## Non-goals (PoC)

- Fine-tuning or training.
- Kev's `permute` / `separate` demo endpoints, packed-vs-separate comparison, or chess.
- Text generation or streaming.
- Authentication, multi-user support, or remote access.
- Conversation history or saved runs. (A built-in preset is included; drafts are not persisted.)
- Windows, Linux, or Intel Macs.
- Container mode. Containers on macOS have no Metal, so Kev would fall back to CPU. This was considered and rejected for the PoC.
- Reimplementing Kev in Go. Kev is a LoRA adapter plus a pointer head with a custom forward pass over Qwen 3.5/3.8 hybrid layers; no llama.cpp/Ollama/GGUF path exists.

## Anti-goals

- Making the user install or debug Python by hand.
- Downloading multiple gigabytes without showing size and asking first.
- A generic chat box that implies Kev writes text.
- Requiring Docker or Apple `container` for the fast path.
- Silent network access or telemetry.

## Constraints and verified facts

- Kev requires Python `>=3.12,<3.14`; current checkpoints are Qwen 3.5/3.8 hybrid bases and run fastest on Apple Silicon via MLX.
- Kev is not installable under its own name from PyPI (`pip install kev` is an unrelated package). Install from a pinned GitHub source revision.
- Pinned Kev revision: `2074cc44256200bb37540103fa4572901faa10f3` (main, 2026-10-22).
- Pinned `uv`: `0.13.5`, `uv-aarch64-apple-darwin.tar.gz`, SHA-256 `743f85035a1996ecb350659e01cd9faea9709829be09b1b4b03c73eaf3c9cd73`.
- Model revisions: `jaredpalmer/kev-0.8b@v1.0` and `jaredpalmer/kev-4b@v1.0`.
- Download sizes (verified from the Hub):
  - Kev-0.8B base `Qwen/Qwen3.5-0.8B-Base`: 1.65 GB; adapter 0.06 GB.
  - Kev-4B base `Qwen/Qwen3.5-4B-Base`: 8.70 GB; adapter 0.15 GB.
- Kev's `kev.serve` API: `POST /v1/systemone`, `GET /v1/models`; accepts `model: "kev-latest"`.
- Kev writes state-prefix caches in memory; a repeated state is faster. The PoC benefits without extra work.
- No container GPU passthrough on macOS. MLX is host-only. This is why the runtime is native.

## Approaches considered

### Runtime

1. **Native `uv`-managed Python + Kev + MLX (chosen).** The Go binary downloads a pinned `uv`, installs Python 3.13 into an app directory, syncs Kev's locked dependencies, prefetches the model, and runs `kev.serve` as a child process. Fast path, one binary UX, no Docker.
2. **Container (Docker or Apple `container`).** Reproducible and isolated, but no Metal inside a Linux VM. Kev would run CPU-only, with a multi-GB image and the same model download. Rejected for the PoC.
3. **Reimplement in Go.** Not feasible for a PoC; Kev's architecture and MLX kernels are non-trivial, and no generic engine supports it. Rejected.

### UI shell

1. **TUI + browser (chosen).** TUI owns setup, process lifecycle, and launcher duties; browser owns the rich three-column console. The TUI stays useful when the browser is closed and can show logs and errors.
2. **Browser-only.** Simpler, but first-run setup and sidecar management leak into terminal output, and there is no launcher when setup fails.
3. **TUI-only.** Would require building a full form editor in the terminal; slower to make good and cramped for a three-column console.

### First-run flow

1. **Guided sequence (chosen).** Setup options → download progress → ready menu. Each screen does one job.
2. **Single dashboard.** Checkboxes become progress bars in place. Denser, fewer screens; rejected in favor of the guided sequence.

### Console layout

1. **Three columns (chosen).** State | Questions | Answers. Makes the pipeline explicit and keeps answers visible while editing questions.
2. **Single column.** Closest to the Jev Console, but results require scrolling.
3. **Two-pane workbench.** Practical, but less explicit about the three-part flow.

## Architecture

```
┌──────────────────────── gavel (one Go binary) ────────────────────────┐
│                                                                       │
│  TUI (Bubble Tea)                                                     │
│   ├─ setup options / download progress / logs                         │
│   └─ ready menu: open browser, switch model, restart, quit            │
│                                                                       │
│  Runtime manager                         Console HTTP server          │
│   ├─ pinned uv                           ├─ GET  /                    │
│   ├─ Python 3.13                         ├─ GET  /static/*             │
│   ├─ Kev @ pinned SHA                    ├─ GET  /api/status           │
│   ├─ prefetch model(s)                   ├─ GET  /api/models           │
│   └─ marker files for idempotence        └─ POST /api/systemone        │
│                                                │                      │
│  Sidecar manager                               │ proxy                │
│   ├─ start/stop kev.serve                      ▼                      │
│   ├─ health poll /v1/models            ┌──────────────────┐           │
│   ├─ stdout/stderr ring buffer         │ kev.serve child  │           │
│   └─ SIGTERM → SIGKILL                 │ 127.0.0.1:<free> │           │
│                                        └──────────────────┘           │
│  Config: model selection, app paths                                    │
└───────────────────────────────────────────────────────────────────────┘
```

The Go process owns both servers. The browser only talks to Go; Go proxies to the Kev sidecar. This keeps one origin, hides the ephemeral sidecar port, and lets Go return a clean `503` while the model is loading.

## Paths and configuration

All paths are overridable with `GAVEL_HOME` for tests and development.

| Purpose | Default (macOS) |
|---|---|
| Config | `~/Library/Application Support/gavel/config.json` |
| Runtime | `~/Library/Application Support/gavel/runtime/` |
| uv + Python | `<runtime>/uv/`, `<runtime>/python/` |
| Kev source + venv | `<runtime>/kev/<sha>/` |
| Logs | `~/Library/Application Support/gavel/logs/kev.log` |
| Cache | `~/Library/Caches/gavel/` |
| Hugging Face cache | `<cache>/huggingface/` |
| uv cache | `<cache>/uv/` |

Config for the PoC:

```json
{
  "model": "jaredpalmer/kev-0.8b@v1.0",
  "downloaded_models": ["jaredpalmer/kev-0.8b@v1.0"]
}
```

No draft state or history is stored.

## Runtime bootstrap

Steps are idempotent and resumable. Each completed step writes a small marker JSON with the pinned version and timestamp.

1. **Ensure directories.** Create config, runtime, cache, and log directories.
2. **Ensure uv.** If `<runtime>/uv/uv` is missing or its version differs, download the pinned tarball, verify SHA-256, extract, and check `uv --version`.
3. **Ensure Python.** `uv python install 3.13` with `UV_PYTHON_INSTALL_DIR=<runtime>/python`.
4. **Ensure Kev.** Download the GitHub tarball for the pinned SHA from `https://codeload.github.com/jaredpalmer/kev/tar.gz/<sha>`, extract to `<runtime>/kev/<sha>/`, then run:
   ```
   uv sync --frozen --no-dev --extra serve --python 3.13
   ```
   with `UV_PROJECT_ENVIRONMENT=<runtime>/kev/<sha>/.venv`, `UV_CACHE_DIR=<cache>/uv`, and `UV_PYTHON_INSTALL_DIR=<runtime>/python`.
   The extracted repo includes Kev's `uv.lock`, so dependency versions are reproducible.
5. **Prefetch model(s).** Run a small Python helper with the venv:
   ```
   <venv>/bin/python <runtime>/prefetch.py <model>@v1.0
   ```
   The helper imports `kev.checkpoint.Checkpoint` and `resolve_run`, resolves the adapter and its base at the recorded revisions, and downloads both with `huggingface_hub.snapshot_download`. It prints JSON progress lines:
   ```json
   {"event":"progress","repo":"Qwen/Qwen3.5-0.8B-Base","file":"model-00002-of-00004.safetensors","done":734003200,"total":1650000000}
   {"event":"done","model":"jaredpalmer/kev-0.8b@v1.0","path":"..."}
   ```
   The TUI aggregates bytes across files for a per-model progress bar. If structured progress is unavailable, it falls back to an indeterminate spinner plus the last log line.
6. **Start sidecar.** Launch `kev.serve` (see below) and poll `GET /v1/models` until ready.

`uv sync --frozen` is the primary path. If the lockfile cannot resolve on this platform, surface the exact `uv` error and stop; do not silently fall back to unpinned dependencies. A future version can add an explicit "retry unpinned" option.

### Disk space

Before downloading, compute required bytes = selected model sizes + runtime estimate (1.5 GB) + 1 GB headroom. Compare with `unix.Statfs` on the runtime filesystem. If insufficient, disable "Download & start" and show the shortfall. Already-downloaded items are not counted twice.

## TUI

Built with Bubble Tea, Bubbles (`spinner`, `progress`, `viewport`), and Lip Gloss.

### Screen 1 — Setup

Shown only when something required is missing.

```
┌ Gavel — Kev playground ─────────────────────────────┐
│ First run: choose what to install                   │
│                                                     │
│ Runtime                                             │
│   [x] Kev runtime (Python 3.13 + Kev + MLX)         │
│                                                     │
│ Models                                              │
│   [x] Kev-0.8B   fast           ~1.7 GB             │
│   [ ] Kev-4B     more accurate  ~8.7 GB             │
│                                                     │
│ Disk free: 412 GB                                   │
│ [ Download & start ]   [ Quit ]                     │
└─────────────────────────────────────────────────────┘
```

- Runtime is required and cannot be deselected.
- `space` toggles the focused model; `tab`/arrow keys move focus; `enter` activates.
- The 0.8B model is checked by default. At least one model must be selected.
- `q` or `ctrl+c` quits.
- If a model is already cached, it is marked `✓ downloaded` and cannot be unchecked in a way that deletes it.
- Disk shortfall disables the download action and prints the required/free numbers.

### Screen 2 — Downloading

```
┌ Downloading ────────────────────────────────────────┐
│ Runtime        ███████████░░░░  82%                 │
│ Kev-0.8B       ██████░░░░░░░░░  51%                 │
│   model-00002-of-00004.safetensors                   │
│                                                     │
│ [ cancel ]   [ l ] logs                             │
└─────────────────────────────────────────────────────┘
```

- One row per selected item: runtime, then each selected model.
- Current file name shown under the active model.
- `l` toggles a log viewport. `esc` cancels: kill running child commands, keep completed caches, return to Setup with a "cancelled" notice.
- Errors keep the completed items and show the failing command plus the last 20 log lines; `r` retries.
- When downloads finish, the same screen shows "Starting Kev…" and then transitions to Ready.

### Screen 3 — Ready

```
┌ Gavel — Kev playground ─────────────────────────────┐
│ Kev-0.8B · MLX · 127.0.0.1:52341 · ready             │
│                                                     │
│ [o] Open console in browser                         │
│ [m] Manage models                                   │
│ [r] Restart Kev server                              │
│ [l] Show logs                                       │
│ [q] Quit                                            │
└─────────────────────────────────────────────────────┘
```

- `o` runs `open <console-url>` (macOS). If `open` fails, print the URL and keep the TUI usable.
- `r` stops and restarts the sidecar with the active model.
- `l` opens a scrollable log viewport (`esc` returns).
- `q` stops the sidecar cleanly and exits.
- If the sidecar exits unexpectedly, show the exit code and last log lines; `r` restarts.
- The status line reports backend and precision from `/v1/models` (e.g., `MLX · bfloat16`).

### Screen 4 — Manage models

Reached with `m`.

- Lists available models with size, cached status, and an `active` marker.
- `space` selects additional models to download; `enter` on a cached model switches the active model and restarts the sidecar.
- `d` is **not** included in the PoC; deleting multi-GB caches is too easy to do by accident.
- `esc` returns to Ready.

### TUI behavior details

- Startup checks happen before the first screen; a failure becomes an error screen with `r` retry, `l` logs, `q` quit.
- The TUI never blocks the event loop on downloads or process waits; Bubble Tea commands run them and return messages.
- All child commands receive a context tied to the current screen; `esc` cancels it.
- Logs are ANSI-stripped for display and also appended to `logs/kev.log`.

## Browser console

Served from Go's embedded filesystem. No Node build step for the PoC: plain HTML, CSS, and ES modules. Pure logic lives in `web/model.js` so it can be unit-tested with `node --test`.

### Layout

```
┌ Kev Console ───────────── Kev-0.8B · ready · [Raw JSON] [Preset] ┐
│ 1 · STATE                 │ 2 · QUESTIONS          │ 3 · ANSWERS  │
│ ┌───────────────────────┐ │ ┌────────────────────┐ │ ┌──────────┐ │
│ │ textarea              │ │ │ department  choice │ │ │ returns  │ │
│ │                       │ │ │ escalate    noul   │ │ │ 0.47     │ │
│ │                       │ │ │ frustration score  │ │ │ ▓▓▓▓░░░  │ │
│ └───────────────────────┘ │ └────────────────────┘ │ └──────────┘ │
│                           │ [+ Add question]       │              │
│                           │ [ Run ⌘↵ ]             │              │
└───────────────────────────┴────────────────────────┴──────────────┘
```

- Header: app/model name, server status, raw JSON toggle, preset button.
- Footer: total latency, input tokens, output tokens, request id (when returned).
- Columns are fixed-width flex columns with independent scroll. Minimum usable width ~1100 px; below that, columns stack vertically in source order.

### State column

- Plain-text textarea for the PoC. Kev also accepts objects/arrays; that is a later feature.
- Live character count; no token count (requires the tokenizer, which lives in Python).
- Placeholder shows the support-ticket example.

### Questions column

- `+ Add question` opens a small type picker: `noul`, `choice`, `score`.
- Each question card shows an editable id, a type badge, the instructions, and a compact criteria summary.
- Clicking a card expands an inline editor:
  - **noul:** instructions; optional advanced fields for true/false descriptions.
  - **choice:** instructions; 2–255 options, each with a name and optional description; add/remove/reorder.
  - **score:** instructions; 2–255 ordered levels; add/remove/reorder.
- Delete question with a small `×` and confirmation only if the card is expanded.
- Ids are auto-generated (`q1`, `q2`, …) and editable. Duplicate ids are rejected.
- Validation is inline and mirrors Go validation. `Run` is disabled while invalid or in flight.

### Answers column

- Before a run: an empty state explaining that answers appear per question.
- During a run: skeleton cards keyed by question id.
- After a run, per question:
  - **noul:** "Yes" with probability, a yes/no bar, and the no probability.
  - **choice:** the chosen option and confidence, plus sorted probability bars for every option.
  - **score:** the mean score, confidence, legend, and probability bars per level; highlight the modal level.
- The request's question order is preserved; results are matched by id.
- Errors show Kev's `detail` string in a banner and leave the input intact.
- No text is generated, so there is no streaming.

### Preset

A "Preset" button loads Kev's README support-ticket example:

- State: late shoes, wrong size, two card charges.
- `department` (choice): returns / shipping / billing.
- `escalate` (noul): urgent human attention?
- `frustration` (score): Calm / Frustrated / Very angry.

This gives the first run something meaningful to execute.

### Raw JSON

A drawer showing the exact request sent to Kev and the exact response received, pretty-printed. Read-only in the PoC. Useful while validating question semantics.

## Go console API

### `GET /api/status`

```json
{
  "phase": "ready",
  "model": "jaredpalmer/kev-0.8b@v1.0",
  "backend": "mlx",
  "dtype": "bfloat16",
  "sidecar_url": "http://127.0.0.1:52341",
  "error": ""
}
```

`phase` is one of `setup`, `downloading`, `starting`, `ready`, `error`, `stopping`. The browser uses this for the header and to disable Run.

### `GET /api/models`

Proxies Kev's `GET /v1/models` when ready; otherwise returns the configured model with a `ready: false` note. The console only needs backend/dtype/model name.

### `POST /api/systemone`

Request body from the console:

```json
{
  "state": "…",
  "questions": {
    "department": {
      "type": "choice",
      "instructions": "Which team should handle this?",
      "criteria": { "returns": "Exchanges, refunds", "shipping": "Delivery", "billing": "Charges" }
    },
    "escalate": { "type": "noul", "instructions": "Does this need urgent human attention?" },
    "frustration": { "type": "score", "instructions": "How frustrated is the customer?", "criteria": ["Calm", "Frustrated", "Very angry"] }
  }
}
```

- Go validates the body, injects `"model": "kev-latest"`, and forwards it to `POST /v1/systemone`.
- Upstream status and body are returned as-is. Network failures become `502`; not-ready becomes `503`; validation becomes `400`.
- Timeout: 5 minutes (large states can be slow on first use). Browser cancellation propagates.
- Request body limit: 10 MB.

### Static files

`GET /` serves the console; `/static/*` serves CSS/JS. Assets are embedded with `go:embed`. The console server binds `127.0.0.1:0` and reports its actual port to the TUI.

## Sidecar management

- Command: `<venv>/bin/python -m kev.serve --run <model> --host 127.0.0.1 --port <free-port>`.
- Environment:
  - `PYTHONUNBUFFERED=1`
  - `HF_HOME=<cache>/huggingface`
  - `HF_HUB_DISABLE_TELEMETRY=1`
  - `TOKENIZERS_PARALLELISM=false`
  - `KEV_BACKEND=auto` (Kev chooses MLX on Apple Silicon)
- Health: poll `GET /v1/models` every 500 ms. Allow up to 10 minutes on first load; the TUI shows "Starting Kev…" and the last log line.
- Shutdown: send `SIGTERM` to the process group, wait 10 s, then `SIGKILL`. Reap the child and close pipes.
- Logs: stdout/stderr are scanned line by line into a 1,000-line ring buffer (ANSI stripped) and appended to `logs/kev.log`.
- Duplicate instances: acquire an exclusive `flock` on `<runtime>/gavel.lock`. If held, exit with "Gavel is already running" and the console URL if it can be discovered. The lock is released on exit. Stale locks are detected by PID liveness.

## Error handling

| Failure | Behavior |
|---|---|
| No network during setup | Error screen with the failing URL, last logs, `r` retry |
| `uv` download checksum mismatch | Refuse to execute; show expected/actual |
| `uv sync` failure | Show the exact command, exit code, and last 20 log lines |
| Disk full | Pre-download check; runtime errors also surface the OS message |
| Model download interrupted | Keep the HF cache; retry resumes |
| `kev.serve` exits during startup | Show exit code and log tail; `r` retries |
| `kev.serve` exits while ready | Header shows error; `r` restarts |
| Console request while starting | `503` with `{"detail":"Kev is still starting"}` |
| Kev `422` (e.g. state too long) | Pass through to the browser; show the detail |
| Browser cannot be opened | TUI prints the URL and keeps running |

## Testing strategy (TDD)

Every milestone starts with a failing test. No real downloads or MLX execution in unit tests.

### Go

- `internal/kevapi`: request/response types, JSON round-trips, validation table tests, golden fixtures from Kev's README example.
- `internal/runtime`: a `Runner` interface wraps command execution. Tests use a fake runner to assert:
  - step order and idempotent skips via markers;
  - checksum mismatch stops before extraction;
  - cancel propagates and leaves completed items;
  - disk-space calculation.
- `internal/sidecar`: tests start a stub Python/HTTP server (`testdata/fake_kev.py`) on a random port, assert health detection, log capture, clean shutdown, and crash detection. No model, no MLX.
- `internal/console`: `httptest` tests for validation, model injection, proxy behavior, `400/502/503`, body limit, and static asset serving.
- `internal/tui`: Bubble Tea `Update` tests for key handling, screen transitions, focus, selection, and error states. `View` assertions are substring-based, not full golden files.
- `internal/config`: read/write defaults and unknown-field tolerance.

### Browser

- `web/model.js` pure functions tested with `node --test`:
  - build a Kev request from editor state;
  - validate ids, types, criteria, and duplicate ids;
  - sort probabilities deterministically;
  - format noul/choice/score answers;
  - parse error bodies.
- DOM glue is kept thin; no browser automation in the PoC. Manual smoke test is part of acceptance.

### End-to-end (opt-in)

- `GAVEL_E2E=1 go test ./e2e -run TestRealKev`:
  - uses `GAVEL_HOME` in a temp directory;
  - performs the real bootstrap and downloads Kev-0.8B;
  - starts the sidecar, posts the preset request, and asserts the three answer types are present;
  - skipped by default and never in CI.
- Manual first-run test: delete the app data dir and run the binary; confirm setup, download, ready, open browser, run preset, quit, and confirm the child process is gone.

### Commands

```
go test ./...
node --test web/
go vet ./...
```

## Milestones

1. **Scaffold.** Go module, `cmd/gavel`, embedded `web/`, a TUI that prints the app title and quits. Tests for main wiring.
2. **Kev API layer.** Types, validation, fixtures, request builder. TDD.
3. **Runtime bootstrap.** Pins, paths, `Runner`, uv/Python/Kev steps, markers, disk check. Fake-runner tests.
4. **Model prefetch.** Python helper, progress protocol, aggregation, cancel. Fake-helper tests.
5. **Sidecar + console server.** Process lifecycle, health, logs, lock, proxy, static serving. Stub server tests.
6. **TUI flows.** Setup options, progress, ready menu, logs, manage models, errors. Bubble Tea tests.
7. **Browser console.** Three-column UI, editors, validation, results, raw JSON, preset. `node --test` for pure logic.
8. **End-to-end and polish.** Real M1 Max run, first-run UX, error copy, README, cleanup.

## Risks

- **Kev dependency install is the fragile step.** PyTorch/MLX wheels and Kev's lockfile must resolve on macOS arm64. Pin everything, show exact errors, and test on the target machine.
- **First model load is slow.** Allow a long health timeout and make progress visible; do not mistake a loading model for a hang.
- **Kev moves quickly.** Pin the source SHA and model revisions; document the upgrade procedure.
- **Browser console complexity.** Vanilla JS keeps the build simple but requires discipline. Pure logic in `model.js` and thin DOM glue mitigate this.
- **Apple Silicon only.** The PoC exits with a clear message on unsupported platforms.
- **Local security.** Both servers bind to loopback. No auth in the PoC; do not expose them to a network.
- **Disk usage.** Show sizes, require consent, and never delete caches automatically.

## Decisions made in this design (builder calls)

- App name: **Gavel**, with the subtitle "Kev playground".
- Browser console is plain HTML/CSS/ES modules, embedded in the Go binary; no Node build step.
- Default model: Kev-0.8B. Kev-4B is opt-in.
- Both models can be downloaded, but only one serves at a time.
- TUI framework: Bubble Tea + Bubbles + Lip Gloss.
- Logs: 1,000-line ring buffer for the TUI, appended to `logs/kev.log`.
- No draft persistence; the preset covers the empty first run.
- No model deletion in the PoC.
- `uv sync --frozen` is strict; no silent unpinned fallback.

## Open questions for the human partner

None blocking.
