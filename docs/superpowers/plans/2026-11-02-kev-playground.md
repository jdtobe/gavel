# Kev Playground (Gavel) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a macOS Apple Silicon Go binary that bootstraps a pinned, offline-after-setup Kev runtime, presents a TUI for setup/launch, and serves a three-column browser console for typed Kev decisions.

**Architecture:** One Go process owns a Bubble Tea TUI, an embedded HTTP console, and a `kev.serve` child process. A runtime manager downloads/verifies `uv`, installs Python 3.13 and Kev into an app data directory, prefetches model weights, then starts Kev bound to loopback. The browser talks only to Go; Go validates requests and proxies `POST /api/systemone` to Kev.

**Tech Stack:** Go 1.27.3, Bubble Tea + Bubbles + Lip Gloss, plain HTML/CSS/ES modules, `uv`, Python 3.13, Kev (pinned SHA), MLX on Apple Silicon, Hugging Face Hub cache.

**Spec:** `docs/superpowers/specs/2026-11-02-kev-playground-design.md`

## Global Constraints

- Target macOS on Apple Silicon (arm64) only. Unsupported platforms exit with a clear message.
- Go 1.27.3; Node 26+ for browser-logic tests.
- Kev source pin: `2074cc44256200bb37540103fa4572901faa10f3`.
- `uv` pin: `0.13.5`, `uv-aarch64-apple-darwin.tar.gz`, SHA-256 `743f85035a1996ecb350659e01cd9faea9709829be09b1b4b03c73eaf3c9cd73`.
- Model revisions: `jaredpalmer/kev-0.8b@v1.0` (default) and `jaredpalmer/kev-4b@v1.0`.
- Kev is a decision model. Never add text generation or a chat UI.
- Native `uv`-managed Python + MLX. No containers, Ollama, llama.cpp, or GGUF.
- Offline by default after setup: sidecar runs with `HF_HUB_OFFLINE=1` and `TRANSFORMERS_OFFLINE=1`.
- Bind Go and Kev servers to `127.0.0.1` only. No auth in the PoC.
- Browser UI has no Node build step. Pure logic lives in `web/model.js`; DOM glue stays thin.
- `GAVEL_HOME` overrides app data paths. Unit tests use temp dirs and never download real models.
- Required before every commit: `make vet && make test`.
- Use `uv sync --frozen --no-dev --extra serve`; no silent unpinned fallback.
- Every error screen offers a route back to Setup/model selection.
- Active model is written to config only after the sidecar reports ready.

## Review Focus

1. **Empty or whitespace-only state** must fail inline validation, not round-trip to Kev and return a confusing error.
2. **Choice/score boundary counts** (0, 1, 256 options/levels, empty names) must be rejected without panics or malformed JSON.
3. **Overlong states** (Kev returns `422` with a detail) must surface the upstream detail and leave Run usable.
4. **Concurrent tabs + restart** must either finish in-flight work within the 30 s grace or return `503`; no orphaned sidecar and no duplicate servers.
5. **Incomplete model cache with offline mode** must fail with "re-run setup" and never silently contact Hugging Face.

---

## File Structure

```
cmd/gavel/main.go                 entrypoint, flags, app/TUI wiring
internal/config/                  config.json load/save/defaults
internal/status/                  Phase + Status shared types (no import cycles)
internal/kevapi/                  request/response types + validation + fixtures
internal/runtime/                 paths, pins, command runner, bootstrap, prefetch
internal/runtime/prefetch.py      Python helper for model prefetch/progress
internal/sidecar/                 kev.serve lifecycle, health, lock, orphan reaping
internal/console/                 HTTP server, status/models/systemone proxy
internal/app/                     orchestration/state machine
internal/tui/                     Bubble Tea screens
web/                              embedded index.html, styles.css, app.js, model.js
web/embed.go                      go:embed FS
e2e/                              opt-in real-Kev smoke test
docs/superpowers/specs/           approved design
docs/superpowers/plans/           this plan
```

---

## Task 1: Config and app paths

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/runtime/paths.go`
- Create: `internal/runtime/paths_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `config.Config{Model string; DownloadedModels []string}`
  - `config.Default() Config`
  - `config.Load(path string) (Config, error)`
  - `config.Save(path string, c Config) error`
  - `runtime.Paths{Home, ConfigFile, RuntimeDir, CacheDir, LogsDir, HFHome, UVCacheDir, PythonDir, UVDir, LockFile, SidecarStateFile string}`
  - `runtime.ResolvePaths() (Paths, error)`
  - `runtime.ResolvePathsAt(home string) (Paths, error)`

- [ ] **Step 1: Write the failing config tests**

```go
func TestDefaultModel(t *testing.T) {
	if got := Default().Model; got != "jaredpalmer/kev-0.8b@v1.0" {
		t.Fatalf("Default().Model = %q", got)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != Default().Model {
		t.Fatalf("Model = %q", got.Model)
	}
}
```

Also cover: save/load round trip, unknown JSON fields ignored, save creates parent directories, file mode `0600`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config -v`
Expected: FAIL with `undefined: Default` / `undefined: Load`.

- [ ] **Step 3: Implement `internal/config/config.go`**

Use `encoding/json`, `os.MkdirAll`, and `os.WriteFile` with `0600`. Missing file returns `Default()`. Unknown fields are ignored by default with `json.Unmarshal`.

- [ ] **Step 4: Write the failing path tests**

```go
func TestResolvePathsAt(t *testing.T) {
	home := t.TempDir()
	p, err := ResolvePathsAt(home)
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigFile != filepath.Join(home, "config.json") {
		t.Fatalf("ConfigFile = %q", p.ConfigFile)
	}
	if p.HFHome != filepath.Join(home, "cache", "huggingface") {
		t.Fatalf("HFHome = %q", p.HFHome)
	}
}

func TestResolvePathsUsesGavelHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GAVEL_HOME", home)
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != home {
		t.Fatalf("Home = %q", p.Home)
	}
}
```

- [ ] **Step 5: Run path tests to verify they fail**

Run: `go test ./internal/runtime -run TestResolvePaths -v`
Expected: FAIL with `undefined: ResolvePathsAt`.

- [ ] **Step 6: Implement `internal/runtime/paths.go`**

`ResolvePaths()` returns `ResolvePathsAt(os.Getenv("GAVEL_HOME"))` when set; otherwise use `os.UserConfigDir()` and `os.UserCacheDir()`:

- config: `<config>/gavel/config.json`
- runtime: `<config>/gavel/runtime/`
- logs: `<config>/gavel/logs/`
- cache: `<cache>/gavel/`
- HF: `<cache>/gavel/huggingface/`
- uv cache: `<cache>/gavel/uv/`
- python: `<runtime>/python/`
- uv binary dir: `<runtime>/uv/`
- lock: `<runtime>/gavel.lock`
- sidecar state: `<runtime>/sidecar.json`

`ResolvePathsAt(home)` treats `home` as the app home and puts `config.json`, `runtime/`, `logs/`, `cache/` beneath it. This is the shape used by tests.

- [ ] **Step 7: Run tests and commit**

Run: `go test ./internal/config ./internal/runtime -v && make vet`
Expected: PASS.

```bash
git add internal/config internal/runtime/paths.go internal/runtime/paths_test.go
git commit -m "feat: add config and app path resolution"
```

---

## Task 2: Kev API types and validation

**Files:**
- Create: `internal/kevapi/types.go`
- Create: `internal/kevapi/validate.go`
- Create: `internal/kevapi/types_test.go`
- Create: `internal/kevapi/validate_test.go`
- Create: `internal/kevapi/testdata/response.json`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `kevapi.QuestionType` with `TypeNoul`, `TypeChoice`, `TypeScore`
  - `kevapi.Request{State string; Model string; Questions map[string]Question}`
  - `kevapi.Question{Type QuestionType; Instructions string; Criteria json.RawMessage}`
  - `kevapi.Answer`, `kevapi.Usage`, `kevapi.Response`
  - `kevapi.ValidationError{Field, Message string}` implementing `error`
  - `kevapi.ValidateRequest(Request) error`
  - `kevapi.ModelName = "kev-latest"`

- [ ] **Step 1: Write the failing type/round-trip test**

Use this response fixture (from Kev's README):

```json
{
  "model": "kev-latest",
  "answers": {
    "department": {"type": "choice", "choice": "returns", "confidence": 0.21, "probabilities": {"returns": 0.47, "shipping": 0.28, "billing": 0.25}},
    "escalate": {"type": "noul", "noul": 0.93},
    "frustration": {"type": "score", "score": 1.44, "confidence": 0.34, "legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"}, "probabilities": {"0": 0.00, "1": 0.56, "2": 0.44}}
  },
  "usage": {"input_tokens": 101, "output_tokens": 161},
  "latency_ms": 495
}
```

Assert all three answer shapes decode and re-encode without losing fields.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/kevapi -run TestResponseRoundTrip -v`
Expected: FAIL with `undefined: Response`.

- [ ] **Step 3: Implement `internal/kevapi/types.go`**

Use pointers for optional numeric fields (`Noul *float64`, `Score *float64`, `Confidence *float64`) so `0` is distinguishable from absent.

- [ ] **Step 4: Write the failing validation tests**

```go
func TestValidateRejectsEmptyState(t *testing.T) {
	err := ValidateRequest(Request{State: "   ", Questions: map[string]Question{
		"q1": {Type: TypeNoul, Instructions: "Is it true?"},
	}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "state" {
		t.Fatalf("err = %v", err)
	}
}
```

Table cases must cover:
- empty/whitespace state;
- zero questions;
- unknown type;
- empty instructions;
- choice with 0/1/256 options;
- choice option with empty name;
- choice option description non-string/non-null;
- score with 0/1/256 levels;
- score level empty/non-string;
- noul criteria not an object, or `true`/`false` not strings;
- malformed criteria JSON.

- [ ] **Step 5: Run to verify failure**

Run: `go test ./internal/kevapi -run TestValidate -v`
Expected: FAIL with `undefined: ValidateRequest`.

- [ ] **Step 6: Implement `internal/kevapi/validate.go`**

Validate by `Question.Type`, decoding `Criteria` per type:
- `noul`: criteria optional; object with optional string `true`/`false`.
- `choice`: object of 2–255 entries; each value is `string` or `null`; keys non-empty and unique by JSON semantics.
- `score`: array of 2–255 non-empty strings.
- instructions trimmed non-empty for all types.
- state trimmed non-empty.
Return `&ValidationError{Field: "questions.<id>.<field>", Message: ...}`.

- [ ] **Step 7: Run tests and commit**

Run: `go test ./internal/kevapi -v && make vet`
Expected: PASS.

```bash
git add internal/kevapi
git commit -m "feat: add Kev request types and validation"
```

---

## Task 3: Runtime bootstrap (uv, Python, Kev)

**Files:**
- Create: `internal/runtime/pins.go`
- Create: `internal/runtime/exec.go`
- Create: `internal/runtime/setup.go`
- Create: `internal/runtime/disk.go`
- Create: `internal/runtime/memory.go`
- Create: `internal/runtime/setup_test.go`
- Create: `internal/runtime/testdata/kev.tar.gz` (fixture with `pyproject.toml` + `uv.lock`)
- Create: `internal/runtime/testdata/uv.tar.gz` (fixture with `uv` executable)

**Interfaces:**
- Consumes: `runtime.Paths`, `runtime.ResolvePathsAt`.
- Produces:
  - `runtime.Pins{KevSHA, UVVersion, UVSHA256, Model08B, Model4B string}`
  - `runtime.DefaultPins() Pins`
  - `runtime.Runner interface { Run(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error }`
  - `runtime.ExecRunner{}` implementing `Runner`
  - `runtime.Fetcher interface { Get(ctx context.Context, url string) (io.ReadCloser, error) }`
  - `runtime.HTTPFetcher{Client *http.Client}` implementing `Fetcher`
  - `runtime.Setup{Paths Paths; Pins Pins; Runner Runner; Fetcher Fetcher; Log io.Writer; DiskFree func(string) (uint64, error); MemoryBytes func() (uint64, error)}`
  - `(*Setup).EnsureUV(ctx) error`, `EnsurePython(ctx) error`, `EnsureKev(ctx) error`
  - `(*Setup).CheckDisk(path string, required uint64) error`
  - `(*Setup).CheckMemory(model string) error`

- [ ] **Step 1: Write the failing checksum test**

```go
func TestEnsureUVRejectsBadChecksum(t *testing.T) {
	s := newTestSetup(t)
	s.Pins.UVSHA256 = strings.Repeat("0", 64)
	s.Fetcher = fakeFetcher{body: uvTarGz(t)}
	err := s.EnsureUV(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if s.Runner.(*fakeRunner).called {
		t.Fatal("runner must not run after a checksum mismatch")
	}
}
```

Also test: marker/version match skips download; correct checksum extracts and runs `uv --version`; `EnsurePython` runs exact args/env; `EnsureKev` downloads and runs exact `uv sync` args/env; markers make second calls no-ops.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime -run TestEnsureUV -v`
Expected: FAIL with `undefined: Setup`.

- [ ] **Step 3: Implement pins and runner**

`pins.go` contains the exact values from Global Constraints. `exec.go` wraps `exec.CommandContext` and wires `stdout`/`stderr`.

- [ ] **Step 4: Implement `EnsureUV`**

- If `<runtime>/uv/uv` exists and `uv --version` contains the pinned version, return nil.
- Download `https://github.com/astral-sh/uv/releases/download/<version>/uv-aarch64-apple-darwin.tar.gz`.
- Compute SHA-256 while streaming to a temp file; compare with `Pins.UVSHA256`.
- Extract the `uv` entry to `<runtime>/uv/uv`, mode `0755`.
- Run `<runtime>/uv/uv --version` and verify the version string.

- [ ] **Step 5: Implement `EnsurePython` and `EnsureKev`**

`EnsurePython` runs:

```
<uv> python install 3.13
```

with `UV_PYTHON_INSTALL_DIR=<PythonDir>`, `UV_CACHE_DIR=<UVCacheDir>`.

`EnsureKev`:
- Download `https://codeload.github.com/jaredpalmer/kev/tar.gz/<KevSHA>`.
- Extract to `<RuntimeDir>/kev/<KevSHA>/`.
- Run in that directory:

```
<uv> sync --frozen --no-dev --extra serve --python 3.13
```

with `UV_PROJECT_ENVIRONMENT=<dir>/.venv`, `UV_PYTHON_INSTALL_DIR=<PythonDir>`, `UV_CACHE_DIR=<UVCacheDir>`.
- Write marker `<dir>/.gavel-installed` containing the SHA and timestamp.

- [ ] **Step 6: Write the failing disk/memory tests**

```go
func TestCheckMemoryBlocks4BBelow32GB(t *testing.T) {
	s := newTestSetup(t)
	s.MemoryBytes = func() (uint64, error) { return 16 << 30, nil }
	if err := s.CheckMemory(DefaultPins().Model4B); err == nil {
		t.Fatal("expected an error for 4B under 32 GB")
	}
}
```

Cover: 0.8B allowed at 8 GB, blocked at 7 GB; 4B allowed at 32 GB; disk shortfall error includes required/free bytes.

- [ ] **Step 7: Add `golang.org/x/sys` and implement disk/memory checks**

Run `go get golang.org/x/sys@latest && go mod tidy` (pin the resolved version in `go.mod`).

`disk.go` uses `unix.Statfs` on the target directory. `memory.go` uses `unix.SysctlUint64("hw.memsize")`. Keep both behind injectable funcs for tests.

Run: `go test ./internal/runtime -v && make vet`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/runtime
git commit -m "feat: add uv, Python, and Kev runtime bootstrap"
```

---

## Task 4: Model prefetch and progress protocol

**Files:**
- Create: `internal/runtime/prefetch.go`
- Create: `internal/runtime/prefetch.py`
- Create: `internal/runtime/prefetch_test.go`
- Create: `internal/runtime/testdata/prefetch.jsonl`

**Interfaces:**
- Consumes: `runtime.Setup`, `runtime.Runner`.
- Produces:
  - `runtime.PrefetchProgress{Event, Item, File string; Done, Total int64}`
  - `runtime.ParsePrefetchLine(string) (PrefetchProgress, error)`
  - `(*Setup).Prefetch(ctx context.Context, model string, onProgress func(PrefetchProgress)) error`
  - `runtime.PrefetchScript` embedded with `go:embed prefetch.py`

- [ ] **Step 1: Write the failing parser test**

Fixture `prefetch.jsonl`:

```jsonl
{"event":"phase","item":"adapter"}
{"event":"progress","item":"base","file":"model-00001.safetensors","done":100,"total":1000}
{"event":"done","item":"base"}
{"event":"error","item":"base","message":"network down"}
```

Assert phase/done/progress parse, progress fields are preserved, and `error` returns a non-nil error when the helper exits.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime -run TestParsePrefetch -v`
Expected: FAIL with `undefined: ParsePrefetchLine`.

- [ ] **Step 3: Implement `ParsePrefetchLine` and `Prefetch`**

`Prefetch` writes `prefetch.py` from the embedded FS to `<RuntimeDir>/prefetch.py` if the content hash differs, then runs:

```
<venv>/bin/python <RuntimeDir>/prefetch.py <model>
```

with `HF_HOME=<HFHome>`, `HF_HUB_DISABLE_TELEMETRY=1`, `PYTHONUNBUFFERED=1`, and `TRANSFORMERS_OFFLINE=0`. Scan stdout line by line; call `onProgress` for each parsed line. A non-zero exit returns an error containing the last `error` line and the command's stderr tail.

- [ ] **Step 4: Write `prefetch.py`**

The helper must:
1. Accept the model id/revision as `sys.argv[1]`.
2. `from kev.checkpoint import Checkpoint, resolve_run`.
3. `ck = Checkpoint(model)` (downloads the adapter).
4. `resolve_run(f"{ck.meta.base}@{ck.meta.base_revision or ''}")` (downloads the base).
5. `AutoTokenizer.from_pretrained(ck.meta.base, revision=ck.meta.base_revision)` (tokenizer files).
6. Use `huggingface_hub.snapshot_download(..., tqdm_class=JsonTqdm)` for both adapter and base, printing the JSON lines above.
7. Print `{"event":"done","item":...}` for each repo and a final `{"event":"done","item":"model"}`.

`JsonTqdm` subclasses `huggingface_hub.utils.tqdm` and prints `{"event":"progress","item":ITEM,"file":self.desc,"done":int(self.n),"total":int(self.total or 0)}` on `update`.

- [ ] **Step 5: Write the failing orchestration test**

Use a `fakeRunner` that writes the fixture lines to `stdout`. Assert the callback saw adapter phase, base progress, and done, and that the command used the venv Python and the model argument.

Add a cancel test: cancel the context while the fake runner blocks; assert `Prefetch` returns `context.Canceled`.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/runtime -v && make vet`
Expected: PASS.

```bash
git add internal/runtime/prefetch.go internal/runtime/prefetch.py internal/runtime/prefetch_test.go internal/runtime/testdata/prefetch.jsonl
git commit -m "feat: prefetch Kev models with structured progress"
```

---

## Task 5: Sidecar lifecycle, health, lock, orphan reaping

**Files:**
- Create: `internal/sidecar/sidecar.go`
- Create: `internal/sidecar/health.go`
- Create: `internal/sidecar/lock.go`
- Create: `internal/sidecar/sidecar_test.go`
- Create: `internal/sidecar/helper_test.go`

**Interfaces:**
- Consumes: `runtime.Paths`.
- Produces:
  - `sidecar.ModelInfo{Name, Backend, Dtype, Device string}`
  - `sidecar.Manager{Paths runtime.Paths; Python string; CommandFactory func(ctx context.Context, name string, args ...string) *exec.Cmd; HTTPClient *http.Client; Log io.Writer}`
  - `(*Manager).Start(ctx context.Context, model string, port int) error`
  - `(*Manager).WaitReady(ctx context.Context, timeout time.Duration) (ModelInfo, error)`
  - `(*Manager).Stop(ctx context.Context) error`
  - `(*Manager).URL() string`
  - `sidecar.AcquireLock(path string) (release func() error, err error)`
  - `sidecar.ReapOrphan(stateFile string) error`
  - `sidecar.WriteState(stateFile string, pid, pgid int) error`

- [ ] **Step 1: Write the failing health test**

`httptest.NewServer` returns:

```json
{"models":[{"name":"kev-latest","backend":"mlx","dtype":"bfloat16","device":"mps"}]}
```

Assert `WaitReady` returns `ModelInfo{Backend:"mlx", Dtype:"bfloat16", Device:"mps"}` and that a non-200 or malformed body returns an error.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/sidecar -run TestWaitReady -v`
Expected: FAIL with `undefined: Manager`.

- [ ] **Step 3: Implement health polling**

Poll `GET <URL>/v1/models` every 500 ms until timeout. Return the first model entry. Respect `ctx.Done()`.

- [ ] **Step 4: Write the failing process lifecycle test**

Use the standard helper-process pattern in `helper_test.go`:

```go
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SIDECAR_HELPER") != "1" {
		return
	}
	// start httptest server, print its URL, wait for SIGTERM
}
```

Tests:
- `Start` launches the helper with `Setpgid: true`, writes `sidecar.json`, and `Stop` kills the process group (helper exits).
- `WaitReady` works against the helper.
- An unexpected helper exit is detected.
- `ReapOrphan` kills a live recorded PID and removes the state file.
- `AcquireLock` rejects a second acquisition in the same process and releases on `close`.

- [ ] **Step 5: Implement the manager**

`Start`:
- If `sidecar.json` exists, call `ReapOrphan`.
- Build `<venv>/bin/python -m kev.serve --run <model> --host 127.0.0.1 --port <port>`.
- Set env from the design: `PYTHONUNBUFFERED=1`, `HF_HOME`, `HF_HUB_OFFLINE=1`, `TRANSFORMERS_OFFLINE=1`, `HF_HUB_DISABLE_TELEMETRY=1`, `TOKENIZERS_PARALLELISM=false`, `KEV_BACKEND=auto`.
- `SysProcAttr: &syscall.SysProcAttr{Setpgid: true}`.
- Start, write PID/PGID state, copy stdout/stderr to the log writer line by line.

`Stop`: `SIGTERM` to `-pgid`, wait 10 s, then `SIGKILL`; remove the state file.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/sidecar -v && make vet`
Expected: PASS.

```bash
git add internal/sidecar
git commit -m "feat: manage Kev sidecar lifecycle and health"
```

---

## Task 6: Status types, console HTTP server, and proxy

**Files:**
- Create: `internal/status/status.go`
- Create: `internal/status/status_test.go`
- Create: `internal/console/server.go`
- Create: `internal/console/proxy.go`
- Create: `internal/console/server_test.go`
- Create: `web/embed.go`
- Create: `web/index.html` (minimal placeholder until Task 11)
- Create: `web/styles.css` (minimal placeholder)
- Create: `web/app.js` (minimal placeholder)

**Interfaces:**
- Consumes: `kevapi`.
- Produces:
  - `status.Phase` constants: `PhaseSetup`, `PhaseDownloading`, `PhaseStarting`, `PhaseReady`, `PhaseError`, `PhaseStopping`.
  - `status.Status{Phase Phase; Model, Backend, Dtype, SidecarURL, Error string}`
  - `status.Provider interface { Status() status.Status }`
  - `status.ModelState{Name string; Cached, Active bool}` in `internal/status`.
  - `console.New(provider status.Provider, upstream func() string, client *http.Client) *Server`
  - `(*Server).Handler() http.Handler`
  - `(*Server).Start(ctx context.Context) (url string, err error)`
  - `(*Server).WaitIdle(ctx context.Context) error` — blocks until no proxied request is in flight
  - `console.MaxBodyBytes = 10 << 20`
  - Routes: `GET /`, `GET /static/*`, `GET /api/status`, `GET /api/models`, `POST /api/systemone`.

- [ ] **Step 1: Write the failing status type and endpoint tests**

`internal/status/status_test.go` pins the wire values:

```go
func TestPhaseJSONValues(t *testing.T) {
	cases := map[Phase]string{
		PhaseSetup: "setup", PhaseDownloading: "downloading", PhaseStarting: "starting",
		PhaseReady: "ready", PhaseError: "error", PhaseStopping: "stopping",
	}
	for p, want := range cases {
		if string(p) != want {
			t.Fatalf("%q != %q", p, want)
		}
	}
}
```

`internal/console/server_test.go` covers the endpoint:

```go
func TestStatusServesPhase(t *testing.T) {
	s := New(fakeProvider{status.Status{Phase: status.PhaseReady, Model: "m"}}, func() string { return "" }, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/status", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"phase":"ready"`) {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Write the failing static test**

```go
func TestRootServesIndex(t *testing.T) {
	s := New(fakeProvider{status.Status{Phase: status.PhaseReady}}, func() string { return "" }, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "<!doctype html>") {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/status ./internal/console -run 'TestPhase|TestStatus|TestRoot' -v`
Expected: FAIL with `undefined: Phase` and then `undefined: New`.

- [ ] **Step 4: Implement status types and static routes**

Define `Phase`, `Status`, `Provider`, and `ModelState` in `internal/status/status.go`. Use `go:embed` from the `web` package. Serve `/static/*` with `http.FileServer(http.FS(web.FS))` after stripping `/static/`. The root serves `index.html`.

- [ ] **Step 5: Write the failing proxy tests**

Use `httptest.NewServer` upstream that records:
- method/path;
- `Authorization` absence;
- body has `"model":"kev-latest"` even when the client omitted it.

Cases:
- valid request -> upstream 200 passed through;
- invalid request -> `400` and upstream not called;
- overlong state -> upstream `422` detail is preserved (`TestProxyPassesThrough422ForOverlongState`);
- upstream connection failure -> `502`;
- provider phase not ready -> `503`;
- body over 10 MB -> `413`/`400`;
- upstream returns `x-typesafe-request-id` and it is preserved;
- two concurrent requests -> both complete; `WaitIdle` returns only after both;
- `WaitIdle` with a canceled context returns `context.Canceled`.

- [ ] **Step 6: Implement the proxy**

`POST /api/systemone`:
1. Reject when phase is not `ready` with `503 {"detail":"Kev is still starting"}`.
2. Limit body to `MaxBodyBytes`.
3. Decode into `kevapi.Request`, call `ValidateRequest`.
4. Set `Model = kevapi.ModelName`; marshal.
5. Forward to `<upstream>/v1/systemone` with the request context and a client timeout of 5 minutes.
6. Copy upstream status, content type, and `x-typesafe-request-id`; copy the body.

Track in-flight proxy calls with a counter/condition; `WaitIdle(ctx)` waits for it to reach zero or `ctx.Done()`.

`GET /api/models` proxies to `<upstream>/v1/models` when ready; otherwise returns the configured status with `"ready": false`.

- [ ] **Step 7: Run tests and commit**

Run: `go test ./internal/console ./internal/status ./web -v && make vet`
Expected: PASS.

```bash
git add internal/status internal/console web
git commit -m "feat: serve console and proxy Kev requests"
```

---

## Task 7: App orchestration

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`

**Interfaces:**
- Consumes: `config`, `runtime`, `sidecar`, `console`, `status`.
- Produces:
  - `app.Runtime` interface: `EnsureUV`, `EnsurePython`, `EnsureKev`, `Prefetch`, `CheckDisk`, `CheckMemory` (signatures match `*runtime.Setup`).
  - `app.Sidecar` interface: `Start(ctx, model string, port int) error`, `WaitReady(ctx, timeout) (sidecar.ModelInfo, error)`, `Stop(ctx) error`, `URL() string`.
  - `app.Console` interface: `Start(ctx) (string, error)`, `WaitIdle(ctx) error`.
  - `app.Deps{Paths runtime.Paths; Config config.Config; Runtime Runtime; Sidecar Sidecar; Console Console; Listen func() (net.Listener, error)}` (`Listen` defaults to `net.Listen("tcp", "127.0.0.1:0")` when nil).
  - `app.App{...}` with:
    - `New(Deps) *App`
    - `Status() status.Status`
    - `Setup(ctx context.Context, models []string, onProgress func(runtime.PrefetchProgress)) error`
    - `Start(ctx context.Context) error`
    - `Restart(ctx context.Context) error`
    - `SwitchModel(ctx context.Context, model string) error`
    - `Models() []status.ModelState` (`status.ModelState{Name string; Cached, Active bool}`)
    - `StartConsole(ctx context.Context) (string, error)` — starts the embedded console HTTP server before setup; safe to call once
    - `Stop(ctx context.Context) error`
    - `ConsoleURL() string`
    - `OpenConsole() error`

`Setup` runs the whole first-run sequence: ensure `uv`/Python/Kev, prefetch each selected model, call `Start`, then persist the active model. `Start` alone is used for ordinary launch when the runtime and models already exist. Both leave the app at `PhaseReady` only after `WaitReady` succeeds.

- [ ] **Step 1: Write the failing transition tests**

```go
func TestSetupTransitionsToReady(t *testing.T) {
	a := newTestApp(t)
	if a.Status().Phase != status.PhaseSetup {
		t.Fatalf("initial phase = %q", a.Status().Phase)
	}
	if err := a.Setup(context.Background(), []string{"jaredpalmer/kev-0.8b@v1.0"}, nil); err != nil {
		t.Fatal(err)
	}
	if a.Status().Phase != status.PhaseReady {
		t.Fatalf("phase = %q", a.Status().Phase)
	}
}

func TestStartTransitionsToReady(t *testing.T) {
	a := newTestApp(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Status().Phase != status.PhaseReady {
		t.Fatalf("phase = %q", a.Status().Phase)
	}
}
```

Use fakes for `Runtime`, `Sidecar`, and `Console`. Cover:
- `Setup` reports `PhaseDownloading`, then `PhaseStarting`, then `PhaseReady`;
- setup failure -> `PhaseError` with message and downloads are not retried automatically;
- config is not written on failed start;
- config is written with the active model only after ready;
- offline env is passed to the sidecar (assert via the fake);
- `Restart` returns `503` to new requests while it waits, waits up to 30 s for `Console.WaitIdle`, then stops/starts;
- `Restart` with a hung in-flight request cancels after the 30 s budget and still reaches `PhaseReady`;
- `SwitchModel` writes config only after ready and keeps the previous model on failure;
- `Models` marks cached and active models correctly;
- `StartConsole` calls `Console.Start` once and `ConsoleURL` returns its URL;
- `Stop` transitions to `PhaseStopping` and calls `Sidecar.Stop`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/app -run 'TestSetupTransitions|TestStartTransitions' -v`
Expected: FAIL with `undefined: App`.

- [ ] **Step 3: Implement the app**

Keep the app free of Bubble Tea and HTTP details. Expose plain methods and let the TUI call them through commands. Use a mutex around status/config writes. Use a `phase` guard so `console` returns `503` while restarting. Use `context.WithTimeout` for restart grace. Accept the fixed 30 s grace as a package constant (`restartGrace = 30 * time.Second`) so tests can assert it without sleeping. Pick the sidecar port by listening on `127.0.0.1:0`, recording the port, and closing the listener before `Sidecar.Start`; keep it injectable (`Listen func() (net.Listener, error)`) so tests can supply a fake port.

- [ ] **Step 4: Implement `OpenConsole`**

Use `exec.Command("open", url)` on darwin. Return the error; the TUI prints the URL as a fallback.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/app -v && make vet`
Expected: PASS.

```bash
git add internal/app
git commit -m "feat: orchestrate setup, sidecar, and console state"
```

---

## Task 8: TUI setup and progress screens

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/setup.go`
- Create: `internal/tui/progress.go`
- Create: `internal/tui/model_test.go`
- Create: `internal/tui/setup_test.go`
- Modify: `go.mod` (add Bubble Tea/Bubbles/Lip Gloss)

**Interfaces:**
- Consumes: `status`, `runtime`.
- Produces:
  - `tui.App` interface with `Status() status.Status`, `Setup(ctx, models, onProgress) error`, `Start(ctx) error`, `Restart(ctx) error`, `SwitchModel(ctx, model) error`, `Models() []status.ModelState`, `StartConsole(ctx) (string, error)`, `Stop(ctx) error`, `ConsoleURL() string`, `OpenConsole() error` (matches `*app.App`).
  - `tui.Deps{App App; OpenBrowser func(string) error; MemoryBytes func() (uint64, error); DiskFree func(string) (uint64, error)}`
  - `tui.New(Deps) Model`
  - `tui.Model` implementing `tea.Model`
  - messages: `setupProgressMsg`, `setupDoneMsg`, `serverReadyMsg`, `serverErrMsg`, `logLineMsg`, `tickMsg`

Tests define a `fakeApp` implementing `tui.App`; the TUI never imports `*app.App` directly.

- [ ] **Step 1: Add dependencies**

Run:

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go mod tidy
```

Expected: `go.mod` records the three modules.

- [ ] **Step 2: Write the failing setup-selection test**

```go
func TestSpaceTogglesModel(t *testing.T) {
	m := New(Deps{App: fakeApp(), MemoryBytes: func() (uint64, error) { return 64 << 30, nil }})
	pins := runtime.DefaultPins()
	if !m.selected[pins.Model08B] || m.selected[pins.Model4B] {
		t.Fatalf("default selection = %#v", m.selected)
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m2.(Model).selected[pins.Model08B] {
		t.Fatal("space should deselect 0.8B")
	}

	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m4, _ := m3.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m4.(Model).selected[pins.Model4B] {
		t.Fatal("space should select 4B when memory allows it")
	}
}
```

Also test:
- 0.8B selected by default;
- 4B disabled below 32 GB;
- disk shortfall disables download;
- `enter` emits a setup command with the selected models;
- `q` quits;
- `esc` during download cancels;
- progress messages update the matching row;
- error screen shows `r`, `m`, `l`, `q`.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/tui -run TestSpaceToggles -v`
Expected: FAIL with `undefined: New`.

- [ ] **Step 4: Implement the setup screen**

Render the exact labels from the design: "First run: choose what to install", "Runtime", "Models", "Disk free", "Download & start", "Quit". Use Lip Gloss for focus styling. Keep selection in `map[string]bool`.

- [ ] **Step 5: Implement progress/error screens**

Progress rows show item name, bar, percent, and current file. Use `bubbles/progress`. Error screen shows the failing command, last 20 log lines, and the `r/m/l/q` actions.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/tui -v && make vet`
Expected: PASS.

```bash
git add go.mod go.sum internal/tui
git commit -m "feat: add TUI setup and download progress"
```

---

## Task 9: TUI ready, manage, and logs screens

**Files:**
- Create: `internal/tui/ready.go`
- Create: `internal/tui/manage.go`
- Create: `internal/tui/logs.go`
- Create: `internal/tui/ready_test.go`

**Interfaces:**
- Consumes: `tui.Model`, `tui.App`.
- Produces:
  - `ready` screen state with actions `o`, `m`, `r`, `l`, `q`.
  - `manage` screen state with cached/active markers and `space`/`enter`/`esc`.
  - `logs` screen state with viewport scrolling and `esc`.

- [ ] **Step 1: Write the failing ready-action tests**

```go
func TestReadyOOpensBrowser(t *testing.T) {
	var opened string
	m := readyModel(Deps{OpenBrowser: func(u string) error { opened = u; return nil }})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if opened == "" {
		t.Fatal("browser was not opened")
	}
	_ = m2
}
```

Cover:
- `m` opens Manage; `esc` returns to Ready;
- `r` emits restart and shows "restarting";
- `l` opens Logs; `esc` returns;
- `q` returns `tea.Quit`;
- sidecar error message shows the error screen;
- switching model emits a switch command and waits for `serverReadyMsg` before marking active.

Define `readyModel(deps Deps) Model` in the test file as a helper that calls `New(deps)` and drives it to the ready screen with the fake app reporting `PhaseReady`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui -run TestReady -v`
Expected: FAIL with missing `readyModel`/actions.

- [ ] **Step 3: Implement ready/manage/logs**

Status line format: `<model> · <backend> · <host:port> · <phase>`. Use `App.Status()`. Logs use `bubbles/viewport` with the ring buffer from the sidecar. Manage lists models with size and cached/active markers from `App.Models()`; `enter` on a cached model calls `App.SwitchModel` and waits for `serverReadyMsg` before marking it active.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/tui -v && make vet`
Expected: PASS.

```bash
git add internal/tui
git commit -m "feat: add TUI launcher, model manager, and logs"
```

---

## Task 10: Browser pure logic

**Files:**
- Create: `web/model.js`
- Create: `web/model.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces (ES module exports):
  - `emptyQuestion(type, id) -> Question`
  - `validateDraft(draft) -> {ok: boolean, errors: Record<string,string>}`
  - `buildRequest(draft) -> {state: string, questions: Record<string,Question>}`
  - `sortProbabilities(probabilities) -> Array<{name: string, p: number}>`
  - `formatAnswer(question, answer) -> {kind: "noul"|"choice"|"score", headline: string, bars: Array<{name, p}>, legend?: Record<string,string>}`
  - `parseErrorBody(text) -> string`

Draft shape:

```js
{
  state: "…",
  questions: [
    { id: "department", type: "choice", instructions: "…", options: [{name: "returns", description: "…"}] },
    { id: "escalate", type: "noul", instructions: "…" },
    { id: "frustration", type: "score", instructions: "…", levels: ["Calm", "Frustrated", "Very angry"] }
  ]
}
```

- [ ] **Step 1: Write the failing build/validate tests**

```js
import test from "node:test";
import assert from "node:assert/strict";
import { buildRequest, validateDraft } from "./model.js";

test("buildRequest maps choice options to criteria", () => {
  const draft = { state: "ticket", questions: [
    { id: "dept", type: "choice", instructions: "pick", options: [
      { name: "returns", description: "refunds" }, { name: "billing", description: null }
    ] }
  ]};
  assert.deepEqual(buildRequest(draft), { state: "ticket", questions: {
    dept: { type: "choice", instructions: "pick", criteria: { returns: "refunds", billing: null } }
  }});
});
```

Add cases for:
- empty/whitespace state -> `{ok:false, errors:{state:...}}`;
- choice 0/1/256 options -> invalid;
- empty/duplicate option names -> invalid;
- score 0/1/256 levels -> invalid;
- noul with no criteria -> valid;
- duplicate question ids -> invalid;
- `sortProbabilities` descending and stable for ties;
- `formatAnswer` for all three types;
- `parseErrorBody` for JSON `{"detail":"…"}` and plain text.

- [ ] **Step 2: Run to verify failure**

Run: `node --test web/`
Expected: FAIL with `Cannot find module './model.js'` or missing exports.

- [ ] **Step 3: Implement `web/model.js`**

Keep functions pure. `buildRequest` omits `criteria` for `noul`, maps choice options to `{name: description|null}`, and uses the ordered levels array for score. `formatAnswer` returns display data only; no DOM.

- [ ] **Step 4: Run tests and commit**

Run: `node --test web/ && make vet && make test`
Expected: PASS.

```bash
git add web/model.js web/model.test.js
git commit -m "feat: add browser console request and answer logic"
```

---

## Task 11: Browser console UI and embedded assets

**Files:**
- Modify: `web/index.html`
- Modify: `web/styles.css`
- Modify: `web/app.js` (replace the Task 6 placeholder)
- Modify: `web/embed.go` only if the embed pattern is explicit rather than a glob
- Create: `web/embed_test.go`

**Interfaces:**
- Consumes: `web/model.js`.
- Produces: the three-column console page served by `internal/console`.

- [ ] **Step 1: Write the failing embed test**

```go
func TestAssetsEmbedded(t *testing.T) {
	for _, name := range []string{"index.html", "styles.css", "app.js", "model.js"} {
		if _, err := FS.Open(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
```

Also assert `index.html` references `/static/app.js` and `/static/model.js`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./web -v`
Expected: FAIL until `app.js` contains the real console code and `embed.go` covers every asset.

- [ ] **Step 3: Implement the three-column HTML/CSS**

Exact structure:
- header with title, model/status, Raw JSON toggle, Preset;
- `<section>` for State, Questions, Answers;
- `textarea#state`;
- `#questions` list with `+ Add question`;
- `#answers` empty/loading/results;
- `#run` button;
- footer with latency/tokens;
- raw JSON drawer `<pre>`;
- minimum width ~1100 px; stack below that.

Use semantic labels and visible focus styles. No external fonts/CDNs.

- [ ] **Step 4: Implement `web/app.js`**

- Load `/api/status`; poll every 2 s while not ready.
- Render the preset on first load.
- Add/edit/delete questions using the draft shape from Task 10.
- Validate on every edit; disable Run while invalid or in flight.
- `Cmd+Enter` runs.
- POST `/api/systemone`; render results with `formatAnswer`.
- Show upstream `detail` on error and re-enable Run.
- Raw JSON drawer shows the exact request and response.
- Preset loads the support-ticket example.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./web ./internal/console -v && node --test web/ && make vet`
Expected: PASS.

Manual smoke: `make run` with a fake/ready backend; open the console and confirm the three columns render and Run is disabled for invalid input.

```bash
git add web
git commit -m "feat: build three-column browser console"
```

---

## Task 12: Wire main, end-to-end smoke, and docs

**Files:**
- Modify: `cmd/gavel/main.go`
- Create: `e2e/real_test.go`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `Makefile` (add `e2e` target)

**Interfaces:**
- Consumes: all previous packages.
- Produces: the runnable binary and the opt-in real-Kev test.

- [ ] **Step 1: Write the failing main wiring test**

Refactor `main` into `run(args []string) error` in `cmd/gavel/main.go` and test:
- `run([]string{"--version"})` prints `gavel <version>` and exits 0;
- unsupported platform returns an error containing "Apple Silicon";
- `GAVEL_HOME` temp dir is honored.

Use a `GOOS`/`GOARCH` check in `run` for the platform message; keep it injectable for tests if needed.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/gavel -v`
Expected: FAIL with `undefined: run`.

- [ ] **Step 3: Wire the app and TUI**

`run`:
1. Resolve paths.
2. Load config.
3. Create the app (runtime setup, sidecar, console).
4. Call `app.StartConsole(ctx)` at launch, before setup, so the browser can show progress.
5. Start the Bubble Tea program.
6. On exit, stop the sidecar, cancel the console server context, and release the lock.

The TUI's `o` action opens the console URL. The console can be opened during setup and will show progress.

- [ ] **Step 4: Add the opt-in e2e test**

`e2e/real_test.go`:

```go
func TestRealKev(t *testing.T) {
	if os.Getenv("GAVEL_E2E") != "1" {
		t.Skip("set GAVEL_E2E=1 to run the real Kev smoke test")
	}
	// GAVEL_HOME=t.TempDir(); real bootstrap; download 0.8B; start sidecar;
	// POST the preset; assert noul/choice/score answers; stop; assert no child.
}
```

Add `make e2e` -> `GAVEL_E2E=1 go test ./e2e -run TestRealKev -v`.

- [ ] **Step 5: Update README and AGENTS.md**

Replace "pre-implementation" language with the real build/run/test commands. Keep the hard constraints and add the offline policy and `GAVEL_HOME` testing note.

- [ ] **Step 6: Run the full local suite and commit**

Run:

```bash
make vet
make test
go build -o bin/gavel ./cmd/gavel
```

Expected: PASS; `bin/gavel --version` prints `gavel dev`.

Manual first-run acceptance:
1. Move/rename the app data dir.
2. Run `bin/gavel`.
3. Select 0.8B, download, wait for Ready.
4. Press `o`; run the preset; confirm three answer shapes.
5. Quit; confirm `kev.serve` is gone (`pgrep -fl kev.serve`).

```bash
git add cmd/gavel e2e README.md AGENTS.md Makefile
git commit -m "feat: wire gavel binary and add end-to-end smoke test"
```

---

## Self-Review

- **Spec coverage:** config/paths (Task 1), Kev API (2), runtime bootstrap (3), prefetch (4), sidecar (5), status types + console/proxy (6), orchestration (7), TUI setup/progress (8), TUI ready/manage/logs (9), browser logic (10), browser UI (11), wiring/e2e/docs (12). All design sections map to a task.
- **Boundary tests:** empty state, choice/score counts, malformed criteria, overlong-state pass-through, concurrent requests + restart grace, and offline cache failure are each assigned to a task.
- **Type consistency:** `status` is created once in Task 6 and consumed by console, app, and TUI; `runtime.PrefetchProgress` flows setup → app → TUI; `app.Runtime`/`app.Sidecar`/`app.Console` and `tui.App` are interfaces so the app and TUI tests use fakes; `kevapi` types are shared by the console proxy and the browser request builder.
- **Proportion:** tasks specify interfaces, exact commands, and representative tests; bodies are left to the implementer except for protocol/algorithm details that the tests do not determine.
