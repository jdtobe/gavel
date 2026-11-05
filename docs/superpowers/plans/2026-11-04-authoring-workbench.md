# Authoring Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a three-column authoring workbench with focused question editing, scoped runs, comparisons, and stability checks.

**Architecture:** Keep Kev's local proxy and plain embedded browser application. Extend the pure browser model for state, validation, revisions, lint, history, comparison, and sizing; keep DOM and fetch orchestration in `web/app.js`. Accept structured state in Go without changing the proxy contract.

**Tech Stack:** Go, plain HTML/CSS/ES modules, `node --test`, native `<dialog>`.

**Spec:** `docs/superpowers/specs/2026-11-03-authoring-workbench-design.md` (approved; read fully alongside this plan). It extends `docs/superpowers/specs/2026-11-02-kev-playground-design.md`.

## Global Constraints

- macOS Apple Silicon PoC; servers bind `127.0.0.1`; no new network calls, endpoints, dependencies, or Node build step.
- Preserve `POST /api/systemone` shape and Kev's `noul`, `choice`, and `score` criteria; only state may additionally be an object or array.
- Model downloads remain opt-in; normal tests never download. Keep runtime, pinned dependencies, and offline behavior unchanged.
- Pure logic lives in `web/model.js`; DOM glue lives in `web/app.js`. Drafts and history are in-memory; only modal size persists.
- Run `make vet && make test` before each commit. Human alone merges; do not enable auto-merge. Push feature branch and open a PR after implementation.
- Follow the spec's exact copy, visual values, accessibility rules, and manual checklist. Lint is advisory. No Ollama integration, case files, persistence, or cancellation.

## Review Focus

- Edits and deletion during an in-flight run: submitted snapshots remain unchanged and newer revisions stay dirty (Task 4).
- Explicit baseline missing a question or evicted at the 20-run limit: no misleading delta; eviction selects auto (Task 3).
- Invalid JSON or storage unavailable/corrupt: block runs for invalid state; default modal size without crashing (Tasks 2, 6).
- First stability failure after partial success: preserve prior answer, record no run, retry all three (Task 7).
- Delete confirmation, keyboard focus, and Escape: first Escape cancels pending deletion, second closes; deleted card has a valid focus fallback (Task 5).

## File Structure

- `internal/kevapi/types.go`, `validate.go`, and their tests: state wire type and validation.
- `internal/console/*_test.go`: object forwarding and invalid-state regression.
- `web/model.js`, `web/model.test.js`: pure interfaces and tests for the entire workbench.
- `web/index.html`, `web/styles.css`: semantic workbench, dialog, run bar, responsive and accessible styling.
- `web/app.js`: session state, event handling, rendering, fetch and dialog orchestration.
- `web/embed_test.go`: update embedded-asset expectations if markup changes require it.

### Task 1: Structured state at the Go boundary

**Files:** Modify `internal/kevapi/types.go`, `internal/kevapi/validate.go`, their tests, and `internal/console` proxy tests; update other Go test fixtures that construct `kevapi.Request`.

**Interfaces:** `kevapi.Request.State json.RawMessage`; `kevapi.ValidateRequest(Request)` retains its existing result contract.

- [ ] Add table-driven tests asserting nonblank strings, objects, and arrays pass; absent, malformed, null, whitespace strings, booleans, and numbers fail with a state error. Assert raw object round-trip and fake upstream receives the object unchanged; invalid state yields HTTP 400.
- [ ] Run `go test ./internal/kevapi ./internal/console`; expect failure because state is currently a string.
- [ ] Change state to `json.RawMessage` and validate its JSON type and nonblank strings; retain existing question validation. Update all affected fixtures without changing proxy semantics.
- [ ] Run `make vet && make test`; expect success. Commit the task.

### Task 2: State, validation, revisions, and requests

**Files:** Modify `web/model.js`, `web/model.test.js`.

**Interfaces:** Export `parseStateJSON(text) -> {ok,value,error}`, `isStateValue(value) -> boolean`, `validateQuestion(draft, question) -> {ok,errors}`, `validateDraft(draft) -> {ok,errors}`, `buildRequest(draft, qids?) -> {state,questions}`, and `changedQuestionIds(draft) -> string[]`. Represent draft state with `state`, `stateMode`, `stateText`, `stateError`, `stateRev`; each question has `dirty` and `rev`.

- [ ] Add tests for JSON string/object/array and malformed/null/number/boolean/empty inputs; text whitespace; criteria validation; scoped validation where an unrelated invalid question does not block; request subsets and unchanged criteria; state and question revision behavior, including JSON edits and both mode conversions. Preserve existing tests and default `buildRequest(draft)` behavior.
- [ ] Run `node --test web/`; expect new tests to fail.
- [ ] Implement the interfaces and pure state/revision transition helpers used by tests. Text → JSON for a string does not increment `stateRev`; JSON → Text for structured state does; each JSON text edit increments it, including invalid or reverted edits. State edits dirty all questions; question edits dirty only that question.
- [ ] Run `make vet && make test`; expect success. Commit the task.

### Task 3: Lint, history, comparison, and editing helpers

**Files:** Modify `web/model.js`, `web/model.test.js`.

**Interfaces:** Export `lintDraft(draft)`, `appendRun(runs,run)`, `latestAnswers(runs)`, `baselineAnswer(runs,latest,qid,selectedRunId)`, `reconcileBaseline(runs,selectedRunId)`, `formatDelta(question,current,baseline)`, `stabilitySummary(question,attempts)`, `nextQuestionId(usedIds,base)`, `duplicateQuestion(question,usedIds)`, and `moveItem(items,from,to)`. Shapes and detailed rules are in the spec's data model and sections 3–4.

- [ ] Add positive and negative tests for all eleven exact lint rules, normalization, duplicate instructions, and clean preset. Test per-question latest and auto/explicit baselines, missing options, changed choices, signed two-decimal deltas and true minus, score/no tick, history cap/eviction, and stability summaries of all three types. Test unique never-reused IDs, deep copy, and moves up/down/same/out of bounds.
- [ ] Run `node --test web/`; expect missing-export failures.
- [ ] Implement the listed pure interfaces, using the spec's exact messages and answer formats. Explicit selected run lacking a question produces no baseline; do not fall back to auto. `appendRun` mutates and returns its capped array.
- [ ] Run `make vet && make test`; expect success. Commit the task.

### Task 4: Workbench and scoped execution

**Files:** Modify `web/index.html`, `web/styles.css`, `web/app.js`, and relevant `web/model.test.js` tests.

**Interfaces:** Use Tasks 2–3 exports. Run records have the exact fields in spec §3; a single session-monotonic used-id set backs Add and Duplicate. Run controller accepts scopes `all`, `changed`, and `one` plus a question id for `one`.

- [ ] First add pure tests for run snapshot/revision reconciliation, preset reset and its in-flight no-op, and deletion/editing during a request; implement pure helpers in `model.js` if needed. Run `node --test web/` and observe failures.
- [ ] Replace inline editors with compact cards and sibling accessible run buttons; retain add/type controls. Implement sticky run bar, scoped validation, dirty counts, request snapshots, history chips and explicit baseline toggling, latest-per-question answers, stale badges, delta bars/ticks, usage/request id, and raw request/response. Preserve previous answers on errors and show evaluating only for scoped questions.
- [ ] Implement reset of draft, runs, baseline, revisions, raw drawer and focus on Preset, no-op in flight. Ensure IDs cannot be reissued after deletion. Keep a functional editing path until Task 5 supplies Focus.
- [ ] Run `make vet && make test`; expect success. Manually inspect scoped runs using a ready local runtime or fake responses; do not download a model. Commit the task.

### Task 5: Focus dialog and accessible editing

**Files:** Modify `web/index.html`, `web/styles.css`, `web/app.js`; add pure tests in `web/model.test.js` where applicable.

**Interfaces:** `openFocus(qid)` and native dialog `showModal()`/`close()` operate on the live draft. Use `duplicateQuestion` and `moveItem` from Task 3; focus return targets the originating card, next card, or Add.

- [ ] Add tests for duplicate insertion/independence and reorder bounds; run `node --test web/` and verify the new assertions fail before adding any missing pure behavior.
- [ ] Implement dialog with fixed 270px answer panel, live instructions/type-specific fields, field validation, lint, run-one, duplicate, disabled assistant placeholder, and two-step four-second delete. Option and level handles support pointer drag and ArrowUp/ArrowDown with polite position announcements; enforce 2–255 controls.
- [ ] Implement native focus trapping and Escape, including first Escape cancelling pending delete, other actions cancelling confirmation, no backdrop dismissal, and correct focus return. Add card click/keyboard semantics and `⌘↵` versus `⌘⇧↵` routing. Ensure closing Focus never cancels a run.
- [ ] Run `make vet && make test`; expect success. Manually verify focus, deletion, drag/keyboard reordering and live editing against spec §§2, 7–8. Commit the task.

### Task 6: Dialog sizing and visual polish

**Files:** Modify `web/model.js`, `web/model.test.js`, `web/app.js`, `web/styles.css`.

**Interfaces:** Export `clampModalSize(size,viewport) -> {w,h}`, `resizeFromCorner(start,delta,viewport) -> {w,h}`, and `readModalSize(raw,viewport) -> {w,h}`. Store preference only as `gavel.focus.size.v1` JSON `{w,h}`.

- [ ] Test default 780×560, min 560×420 subject to tiny viewport, max 92vw×86vh, rounding, non-finite/missing/string values, invalid JSON, and doubled centered pointer movement. Run `node --test web/`; expect failure.
- [ ] Implement sizing helpers and accessible 16px grip: pointer capture, 16px arrow steps/64px with Shift, save on pointerup/keyup/close, safe storage fallback. Window resize visually clamps without overwriting preference; next open restores it.
- [ ] Apply the spec's exact palette, dark backdrop, centered dialog, independent scrolling, sticky run bar, responsive 1100px stacking, focus indicators, and reduced-motion rules.
- [ ] Run `make vet && make test`; expect success. Manually check pointer/keyboard resizing, tiny window, stored preference, and unavailable storage. Commit the task.

### Task 7: JSON editor and three-attempt stability

**Files:** Modify `web/index.html`, `web/app.js`, `web/styles.css`, `web/model.test.js` if new pure orchestration is introduced.

**Interfaces:** Use Tasks 2–3 state and stability interfaces. Stability uses three sequential `one` requests from one immutable snapshot; successful aggregate run uses the last attempt's answer, usage, latency, and request ID and stores all attempts.

- [ ] Add failing tests for state-mode conversion and invalid JSON blocking, plus pure stability aggregation and failure/retry behavior if extracted. Verify with `node --test web/`.
- [ ] Implement Text/JSON segmented editor, parse errors, character line and explicit conversion hint. Respect exact revision rules and scoped validation.
- [ ] Implement Focus stability progress, three sequential attempts, one history record on full success, summary and variance note. Stop on first failure; show partial attempts and error, preserve previous answer, record nothing, and Retry starts all three again. Clear partial state when Focus closes or changes question.
- [ ] Run `make vet && make test`; expect success. Manually exercise failure on attempt 2 and retry, and inspect structured state in Raw JSON. Commit the task.

### Task 8: Acceptance and handoff

**Files:** Modify documentation only if behavior, commands, layout, or constraints changed; include the approved spec and this plan in the feature branch without overwriting unrelated work.

**Interfaces:** Full application behavior from Tasks 1–7 and the spec's manual smoke checklist.

- [ ] Execute all eleven manual smoke checks in spec §Testing strategy, including accessibility, long lists, no-question and loading/error states, invalid JSON, baseline eviction, and storage failure. Use an already-installed model or fake backend; do not initiate downloads without consent. Record any checks that could not be run.
- [ ] Run `make vet && make test` and `make build`; expect all to pass. Fix regressions with a failing test first and rerun verification.
- [ ] Review changes against the entire approved spec and original runtime constraints. Update `AGENTS.md` only if commands, layout, or constraints changed; commit any remaining changes after `make vet && make test`.
- [ ] Push the feature branch and create a PR describing verification and unrun checks. Do not merge or enable auto-merge.
