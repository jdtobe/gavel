# Gavel — Authoring Workbench and Focus Modal

**Date:** 2026-11-03
**Status:** Draft for approval
**Extends:** `docs/superpowers/specs/2026-11-02-kev-playground-design.md` (browser console section)
**Target:** macOS on Apple Silicon, current Gavel PoC

## Intent

The current console is a good first look at Kev, but a poor editing loop. Every run evaluates every question, editing happens inside a one-third-width column, and after a run there is no signal about whether a change helped or hurt. The next step is not a new model or a new runtime. It is a tighter authoring loop:

> Open one question, change it, run just that question, and see how the answer moved — without losing the rest of the screen.

This design keeps the three-column **State → Questions → Answers** workbench and adds a dedicated **Focus** surface as a true modal. The workbench is for running and comparing. Focus is for editing one question with room to think. The two share the same draft and the same runs, so closing the modal never saves, syncs, or discards anything.

The testing in scope is **per-question execution and comparison**, not a case-file eval suite. That larger idea was explored as comp C and deliberately dropped for now.

## What exists today

- `web/index.html` serves the three columns. `web/app.js` renders the draft and runs all questions through `POST /api/systemone`. `web/model.js` holds pure validation, request building, answer formatting, and error parsing, tested by `node --test`.
- Questions are edited inline with `<details>`. There is no per-question run, no answer comparison, and no prompt lint.
- `internal/kevapi.Request.State` is a `string`. `internal/console` validates and forwards the body unchanged except for injecting `model: "kev-latest"`.
- There is no persistence: the preset loads on refresh, drafts and answers are in memory, and the footer says so.
- The existing console already has: add/delete questions, inline validation, a type picker, raw JSON drawer, latency/token usage, and the preset.

## Goals

1. Run a single question from its card, without opening an editor first.
2. Edit a question in a centered modal that has room for instructions, type-specific criteria, and lint.
3. Compare each answer to a previous run with a signed delta and a baseline tick on the bar.
4. Keep an in-session run history and let the user choose the comparison baseline.
5. Catch common prompt defects with deterministic, specific warnings that never block a run.
6. Keep the modal open across runs, remember its size, center it, and clamp it to the display.
7. Keep all pure logic in `web/model.js` and cover it with `node --test`. No new JS dependencies, no build step, no new Go endpoints.
8. Preserve the current keyboard-first posture: `⌘↵` runs all; `⌘⇧↵` runs the focused question; `Esc` closes Focus.

## Non-goals (this build)

- Case files, an eval matrix, expected answers, batch datasets, or a test bench. Comp C is out of scope.
- The Ollama drafting assistant. It is a later phase. The Focus footer shows a disabled **Tighten with local assistant** control as a placeholder, but there is no `/api/assist` endpoint, no model detection, and no prompt sent to Ollama in this build.
- Model comparison (0.8B vs 4B), replay/demo mode, template gallery, autosave, or draft persistence.
- Saved run history across reloads. Only the Focus size persists, in `localStorage`.
- Editing a question's id or type after creation. Ids are generated; types are chosen at add time. **Duplicate variant** creates a new id.
- Cancelling an in-flight request. Requests are short; the UI disables run controls while one is active.
- Authentication, remote access, or any new network traffic.

## Anti-goals

- A chat transcript UI, streaming text, or any implication that Kev generates prose.
- Hiding the state away behind the modal. The modal is for one question; the state stays in the workbench.
- A modal with a Save button, a dirty buffer, or a discard prompt. Edits are live and in-memory, like the rest of the console.
- Lint that blocks running or rewrites prompts automatically. It explains; the user decides.
- Presenting deltas as quality judgments or statistical significance. A delta is movement.
- Silently keeping drafts, runs, or answers after a refresh.
- A full-screen takeover that loses the other questions and the answers.

## Constraints

- Plain HTML, CSS, and ES modules embedded with `go:embed`. No Node build step.
- Pure logic in `web/model.js`; DOM glue stays thin. Browser logic is tested with `node --test`.
- `POST /api/systemone` keeps its shape and proxy behavior. The only Go API change is the state field type (below), needed for structured state.
- Both servers stay on `127.0.0.1`. No new network calls.
- Normal tests never download a model. The real-Kev e2e remains opt-in.
- TDD: failing test first, smallest implementation, green commit. `make vet && make test` before every commit.
- Human-only merges; no auto-merge.

## Approaches considered

### Workbench layout

1. **Workbench only (comp A).** Per-question run and deltas live inside the three columns; the existing inline `<details>` editor grows to hold the new fields. Keeps one screen and no modal machinery, but a long question, two descriptions, and a criteria list do not fit in a one-third column without pushing the answer out of view. The user liked the mechanics but not as the only editing surface.
2. **Focus only (comp B).** A dedicated editor replaces or covers the workbench for editing, and the workbench becomes a launcher. Lots of room, but the user loses the other questions, the answers column, and the one-click run while editing. The user liked the room but not the loss of context.
3. **Test bench (comp C).** Case files, expected answers, and an eval matrix. Powerful for regression work, but a much larger project with persistence, and the user did not see themselves using it.
4. **A + B, combined (chosen).** The workbench stays primary and gains per-question run, deltas, and run history. A true modal over a dimmed workbench provides the roomy editor. Clicking a question card opens Focus; ▶ runs it in place without opening anything.

### Focus surface

1. **Inline takeover.** Replaces the workbench with a full-screen editor. Maximum room, but it reads as navigation, not editing, and the answers disappear.
2. **Side sheet.** Slides in from the right and leaves the columns visible. Keeps context, but the remaining workbench is cramped, and the sheet fights the three-column layout for width.
3. **True modal (chosen).** Centered over a dark tint, resizable, with the workbench visibly inert behind it. It is unambiguously an editing surface, keeps the underlying context as a dimmed backdrop, and gives the editor and answer panel a predictable width. The user asked for exactly this and for a darker tint; the modal is the design.

## Decisions from the conversation

- A + B combined; C dropped.
- Focus is a true modal, centered over a dimmed workbench. The tint is dark: `rgba(8, 22, 18, 0.72)`.
- The modal is resizable from the bottom-right corner, remembers its size, stays centered, and is clamped to the display.
- The answer column stays a fixed width while the editor flexes, so the number does not move while typing.
- Clicking a question card opens Focus; the card's ▶ runs that question in place.
- Esc or ✕ closes Focus. A backdrop click does nothing.
- The Ollama drafting assistant is a later phase. The first build keeps a disabled placeholder button.
- Case files, eval matrices, and the test bench are out of scope.

## Design

### 1. Workbench

The three columns and the header stay. The Questions column becomes a list of compact cards; the Answers column gains deltas and run provenance; a sticky run bar spans the bottom.

```
┌ Gavel  Kev playground ─────────────── Kev-0.8B · MLX · ready ── [Raw JSON] [Preset] ┐
│ 01 STATE                    │ 02 QUESTIONS                 │ 03 ANSWERS              │
│ ┌─────────────────────────┐ │ ┌──────────────────────────┐ │ ┌─────────────────────┐ │
│ │ textarea                │ │ │ department  choice  ▶    │ │ │ department          │ │
│ │                         │ │ │ Which team should…       │ │ │ returns 0.62  +0.08 │ │
│ │                         │ │ ├──────────────────────────┤ │ │ ▓▓▓▓▓▓░░░┊          │ │
│ │                         │ │ │ escalate    noul    ▶    │ │ ├─────────────────────┤ │
│ │                         │ │ │ Does this need…    ⚠ 1   │ │ │ escalate            │ │
│ │                         │ │ ├──────────────────────────┤ │ │ 0.47  −0.11         │ │
│ │                         │ │ │ frustration score   ▶    │ │ │ ▓▓▓▓▓░░░┊           │ │
│ └─────────────────────────┘ │ └──────────────────────────┘ │ └─────────────────────┘ │
│ Text | JSON                 │ [+ Add question] [type ▾]    │                         │
├─────────────────────────────┴──────────────────────────────┴─────────────────────────┤
│ [Run all ⌘↵] [Run changed (2)]   runs [auto][1][2][3]   compare: auto    1.8s · 412/12 │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

- The header keeps the status line, Raw JSON, and Preset. No Templates button in this build.
- The Questions column keeps the `+ Add question` control and the type picker. Adding a question creates it immediately and opens Focus on it.
- The run bar replaces the old Run button in the Questions column. It is sticky at the bottom on wide screens, wraps on narrow screens, and shows the latest run's latency and token usage.
- The state editor stays in column 1 and is the only place the state is edited. Focus does not duplicate it.
- On narrow screens (`max-width: 1100px`) the columns stack in source order, as today. The run bar stays sticky at the bottom of the viewport.

#### Question card anatomy

```
┌──────────────────────────────────────────┐
│ [open button]                            │
│  escalate  noul            ⚠ 1     ▶     │
│  Does this need urgent human attention?  │
│  edited · 1.2 s                          │
└──────────────────────────────────────────┘
```

- The **open button** is a real `<button>` wrapping the id, type badge, instruction preview (two lines, clamped), and status markers. It has `aria-haspopup="dialog"` and opens Focus for that question.
- The **▶ button** is a sibling, not nested in the open button. It runs only this question. `event.stopPropagation()` is not needed because it is a sibling; the article's click handler ignores clicks whose target is a button.
- The **status markers**:
  - `edited` (a small muted dot) when the question is dirty since its latest successful run.
  - `⚠ n` when lint has warnings for the question; `title` lists the first message and `aria-label` says “n prompt warnings”.
  - `!` in the validation color when the question has blocking validation errors. The card is still clickable; ▶ opens Focus instead of running.
  - The latest run id (`run #3`) in muted text, when an answer exists.
- Clicking anywhere on the card except the ▶ button opens Focus. The article itself is not focusable; the open button is the keyboard path.
- The card never expands inline. Focus is the editor.

#### Run bar

- **Run all ⌘↵** — enabled when the state and every question are valid, Kev is ready, and no run is in flight.
- **Run changed (n)** — enabled when at least one question is dirty, the state is valid, every dirty question is valid, Kev is ready, and no run is in flight. The count is the number of dirty questions. State edits mark all questions dirty; editing one question marks only that one.
- **Run history chips** — a horizontal strip: `auto`, then the last 20 runs as `#1 … #n`, oldest to newest. The newest chip is marked `latest`. `auto` is the default baseline. Selecting a chip sets an explicit baseline; selecting it again returns to `auto`.
- **Usage** — the latest run's latency and input/output tokens, plus the request id when the response supplies one.

#### Preset

Preset is a **full session reset**, not just a draft swap. It loads the support-ticket preset, clears the run history and the explicit baseline, resets the state mode to Text, resets `stateRev`, closes Focus, and clears the raw drawer. It is a no-op while a run is in flight. Without this, the preset's `department` / `escalate` / `frustration` ids would inherit the previous draft's answers and deltas, which would be visibly wrong.

### 2. Focus modal

Focus is a native `<dialog>` opened with `showModal()`. The browser supplies the top layer, focus trapping, background inertness, and Esc handling; the page supplies the layout and the tint.

```
┌────────────────────────────────── dialog::backdrop 72% dark ──────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ escalate  noul   latest run #3                                             [✕ esc]       │  │
│  ├───────────────────────────────────────────────────┬──────────────────────────────────────┤  │
│  │ Instructions                                      │ 03 ANSWER                            │  │
│  │ ┌───────────────────────────────────────────────┐ │ noul · yes probability               │  │
│  │ │ Does this need urgent human attention?        │ │ 0.47  yes                            │  │
│  │ └───────────────────────────────────────────────┘ │ ▓▓▓▓▓░░┊                            │  │
│  │ True means                                        │ no 0.53 · prev 0.58                 │  │
│  │ ┌───────────────────────────────────────────────┐ │ Confidence 6.0%                      │  │
│  │ │ Customer needs a person now: safety, legal…   │ │ 1.2 s · 96 in / 4 out                │  │
│  │ └───────────────────────────────────────────────┘ │                                      │  │
│  │ False means                                       │ [Run 3× stability]                   │  │
│  │ ┌───────────────────────────────────────────────┐ │                                      │  │
│  │ │ A normal queue reply is enough.               │ │                                      │  │
│  │ └───────────────────────────────────────────────┘ │                                      │  │
│  │ ⚠ Ambiguity: “urgent” mixes time pressure with…   │                                      │  │
│  ├───────────────────────────────────────────────────┴──────────────────────────────────────┤  │
│  │ [Run this question ⌘⇧↵] [Duplicate variant] [Tighten with local assistant]  [Delete]     │  │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                        ◢     │
└──────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Opening and closing

- Clicking a question card's open button calls `openFocus(qid)`. The card's current open button is remembered for focus return.
- `dialog.showModal()`; the instructions field receives focus (cursor at the end). The workbench behind is inert by the platform; no manual focus trap and no manual `inert` attributes.
- The ✕ button calls `dialog.close()`. Esc triggers the native `cancel`/`close`; there is no `preventDefault`.
- A click on the backdrop does nothing. There is deliberately no click handler that calls `close()`.
- On close: save the size, re-render the workbench cards and answers (the modal edited the live draft), and restore focus to the card that opened it. If that question was deleted, focus the next card or the Add button.
- The dialog is not a route: no URL change, no history entry.

#### Layout

- The dialog is 780 × 560 by default; min 560 × 420; max 92vw × 86vh. It is centered by the platform's dialog margins.
- The internal layout is three rows: header (44 px), body (`1fr`), footer (44 px). The body is a two-column grid: editor `minmax(0, 1fr)` and answer `270px`.
- The editor column scrolls independently. The answer column scrolls independently. The header and footer stay put.
- The answer column keeps its 270 px width at every size; the editor takes the remaining width. At the 560 px minimum the editor still has roughly 260 px, enough for the fields.
- The footer wraps its secondary actions on narrow widths; the primary run button stays first.

#### Resizing

- The visible grip is a 16 × 16 button in the bottom-right corner, with a diagonal hatch, `cursor: nwse-resize`, `touch-action: none`, and a visible focus ring. It is a `<button type="button">` with `aria-label="Resize dialog"` and a visually hidden hint: “Use the arrow keys to resize. Hold Shift for larger steps.”
- Pointer: `pointerdown` records the start size and pointer position, sets pointer capture, and prevents text selection. `pointermove` computes `dx, dy`, doubles them (because the centered dialog grows in both directions, so the bottom-right corner moves half the size change), clamps, and applies. `pointerup` releases capture and saves the preference.
- Keyboard: Left/Right change width, Up/Down change height, 16 px per press, 64 px with Shift; clamped. The value is saved on keyup.
- Window resize: re-clamp and re-apply the visual size, but do not overwrite the stored preference. If the window later grows, the size is re-applied from the preference on the next open.
- The preference is stored in `localStorage` under `gavel.focus.size.v1` as `{"w":780,"h":560}`. Invalid JSON, non-finite values, or missing storage fall back to the defaults. Only the size is stored — never the draft, the answers, or the runs.

#### Editor fields

- **Instructions** — a textarea, three rows, auto-growing to six. Always present.
- **noul** — optional **True means** and **False means** textareas (two rows each). These map to the existing `criteria: {true, false}` payload. They are always visible in Focus, not behind an “Advanced” disclosure; the modal has room and the descriptions are the main way to disambiguate a noul question.
- **choice** — an options list. Each row starts with a `⠿` drag handle, then a name input and an optional description input, and ends with a `×` remove button. Rows reorder by dragging the handle; when the handle has focus, ArrowUp/ArrowDown move the row one position and announce the new position. No up/down buttons. Add option. Minimum 2, maximum 255; the add and delete controls disable at the bounds.
- **score** — an ordered levels list. Each row starts with a `⠿` drag handle, then a level input, and ends with a `×` remove button. Reordering and keyboard behavior match choice. Add level. Minimum 2, maximum 255.
- The type badge in the header is not a picker. The type is immutable; to use another type, delete this question and add a new one of that type.
- The question id is not an input. It is shown in the header and in the card. **Duplicate variant** is the supported way to get a new id. (This is a deliberate change from the 2026-11-02 spec, which allowed id editing; see Builder calls.)
- Validation messages sit directly under the field they concern. Lint warnings sit in a callout below the fields and above the footer.
- All edits are live. There is no Save, Apply, or Cancel-with-changes. Closing preserves the draft; reloading discards it, as today.

#### Footer actions

- **Run this question ⌘⇧↵** — primary. Runs only this question. Disabled while a run is in flight, when Kev is not ready, or when the state or this question is invalid.
- **Duplicate variant** — deep-copies the question, assigns the next free id based on `<id>-copy`, inserts the clone directly after the source, marks it dirty, and reopens Focus on the clone. The clone's instructions field receives focus.
- **Run 3× stability** — in the answer column. Runs the same question three times sequentially and shows the spread. Details below.
- **Tighten with local assistant** — disabled placeholder with `title="Local assistant is a later phase."` and `aria-disabled="true"`. No network request is made.
- **Delete question** — a two-step control, not `window.confirm`. The first activation changes the label to **Confirm delete** for four seconds; a second activation deletes the question, closes Focus, and focuses the next card or Add. While the confirm is pending, Esc cancels the confirm **without closing the dialog** (the `cancel` handler calls `preventDefault` once); a second Esc closes. Any other action also cancels the pending delete. Deleting the last question is allowed; the workbench shows an empty state and the run controls disable.

### 3. Run model

#### Scopes

- `all` — every question, source order.
- `changed` — every dirty question, source order.
- `one` — one question, from a card's ▶ or Focus's run button.
- `stability` — one question, three sequential attempts, recorded as one run.

Each request is built from a snapshot of the state and the questions in scope. Edits made while a request is in flight do not change the request, and the dirty flags are only cleared if the captured revisions still match at success time.

#### Validation per scope

- State validity always blocks.
- `all` requires every question to be valid.
- `changed` requires every dirty question to be valid; an invalid question that is not being run does not block.
- `one` and `stability` require only that question to be valid.
- A ▶ on an invalid question does not run. It opens Focus on that question so the errors are visible, and Focus's run button stays disabled.

#### In-flight behavior

- One run at a time. All run buttons (cards, run bar, Focus) disable while a run is active.
- Answers for questions not in the current run stay visible. The requested questions show an `Evaluating…` card in place of their previous answer; other cards are untouched.
- On success: merge the returned answers, append a run record, clear the dirty flags whose revisions still match, re-render, and update the raw JSON drawer with the last request/response.
- On error: keep every previous answer, show the error in the banner (workbench) or in the answer panel (Focus), and do not append a run record. The raw drawer still shows the failed request and response.
- No cancel button in this build. Closing Focus does not abort an in-flight request; it resolves into the run history.

#### Run history

Each successful request appends one run:

```js
{
  id: 3,                       // monotonic, shown as #3
  scope: "all" | "changed" | "one" | "stability",
  qids: ["escalate"],
  answers: { escalate: { /* Kev answer */ } },
  attempts: [ /* stability only: 3 × {answer, latency_ms, usage} */ ],
  stateRev: 12,
  questionRevs: { escalate: 4 },
  latency_ms: 1180,
  usage: { input_tokens: 96, output_tokens: 4 },
  requestId: "…",              // when supplied
  at: 1793767590000
}
```

- The history is capped at 20 runs. The oldest is dropped; run ids remain monotonic. If the selected baseline is the run that was evicted, the selection resets to `auto` (the chip strip and the next render both reflect it).
- The Answers column shows, per question, the **latest** run containing that question. A per-question run therefore updates one answer without disturbing the others.
- The **baseline** for a question is:
  1. the run selected in the chip strip, if it contains that question; otherwise
  2. the most recent run *before* the question's latest run that contains it.
- `auto` means rule 2. Selecting a chip makes it explicit for every question. If the selected run has no answer for a question, that question shows no delta rather than a misleading zero.
- Question ids are allocated from a **session-monotonic used-id set**: a deleted id is never reissued. This prevents a new question from inheriting a deleted question's answers, provenance, or deltas. The same allocator serves Add and Duplicate variant.
- The chip strip is session-only. Refresh clears the runs, the answers, and the baseline; the preset reloads.

#### Delta rules

Deltas answer “how much did this move?”, not “is this better?” The chip's tooltip says `Movement, not quality.`

- **noul** — `current.noul − baseline.noul`, formatted to two decimals with a true minus sign (`−0.11`). The yes bar gets a tick at the baseline probability.
- **choice** — compare the probability of the *currently chosen* option with that option's probability in the baseline. If the chosen option changed, show `baseline.choice → current.choice` alongside the numeric delta. The chosen option's bar gets a tick at its baseline probability. If the baseline lacks that option, show the choice change without a number.
- **score** — `current.score − baseline.score`, formatted to two decimals. This is a move in mean level; there is no tick on the distribution bars.
- Direction is positive/teal (`--delta-up: #146858`), negative/rust (`--delta-down: #9a382a`), or flat/muted. The sign is always printed, so color is not the only signal.
- No delta is shown when there is no latest answer, no baseline, or the baseline has no comparable answer.
- An answer card also shows **stale** badges when the state revision or the question revision differs from the run that produced it: `state changed` and `question edited`. These are informational; re-running clears them.

### 4. Prompt lint

Lint is deterministic, synchronous, computed in `model.js`, and advisory. It never blocks a run and never edits a prompt. The card shows a count; Focus lists the messages for that question. Messages name the offending values.

| Rule | Trigger | Message |
|---|---|---|
| `noul-double-barreled` | noul instructions contain ` and ` or ` or ` (word boundaries) and a `?` | “Ask one thing per noul question. Kev returns one probability; split this into two.” |
| `noul-wh` | noul instructions begin with What / Which / How / Why / Who / Where | “This reads as an open question. Rewrite it so the answer is yes or no.” |
| `noul-comparative` | noul instructions contain `more/less/better/worse/faster/slower/higher/lower than` | “Name the reference point for ‘…than’. Kev cannot infer what it is compared with.” |
| `noul-descriptions-same` | both true and false descriptions exist and normalize equal | “The true and false descriptions are the same. Make them distinguishable.” |
| `choice-no-descriptions` | choice has ≥3 options and none has a description | “None of the options has a description. Kev only sees the names; add descriptions if the names are ambiguous.” |
| `choice-same-name` | two option names normalize equal (lowercase, trim, collapse whitespace, strip punctuation) | “‘Returns’ and ‘returns.’ read the same. Make them distinct.” |
| `choice-long` | more than 7 options | “A choice with 9 options is hard to calibrate. Consider 3–7.” |
| `score-same-level` | two levels normalize equal | “‘Calm’ and ‘calm’ read the same. Make them distinct.” |
| `score-long` | more than 7 levels | “A score with 9 levels is hard to calibrate. Consider 3–7.” |
| `duplicate-instructions` | two questions' instructions normalize equal | “‘escalate’ and ‘urgency’ ask the same thing. Combine them or make them distinct.” |
| `whitespace` | an instruction, option name, level, or description has leading/trailing whitespace | “‘ returns’ has leading or trailing spaces. Trim it before running.” |

- Lint result shape: `{ rule, questionId, message }`.
- Normalization: lowercase, trim, collapse runs of whitespace; option/level names also strip punctuation. It is applied consistently in tests.
- The lint callout says “Warnings don't block a run.” It is not an `aria-live` region; re-announcing on every keystroke would be noise.
- No LLM judge and no rubric: the checks are text heuristics, intentionally few, and each one explains its reason.

### 5. State editor (Text / JSON)

The state stays in column 1. A segmented control switches between **Text** and **JSON**.

- **Text** — the existing textarea. The value is a string; empty/whitespace is invalid.
- **JSON** — the textarea holds JSON. On every input, `parseStateJSON` validates; a valid parse replaces the draft's state value; an invalid parse sets an inline error and blocks every run. The accepted values are a JSON string, object, or array. `null`, numbers, and booleans are rejected with “State must be a JSON string, object, or array.”
- The character line in JSON mode reads `valid JSON · 147 characters` or the error.
- Switching Text → JSON sets the editor text to `JSON.stringify(value, null, 2)`. A text state becomes a quoted JSON string literal; the user can then edit it into an object or array.
- Switching JSON → Text pretty-prints a structured value and makes that string the state. The state column says so in a hint: “Text sends a string. Switching from JSON converts structured state to a pretty-printed string.” This is a deliberate conversion, not a silent one.
- `stateRev` bumps only when the value that `buildRequest` would send actually changes. Text → JSON on a string value is representation-only and does not mark questions dirty. JSON → Text on an object/array converts the payload to a string, so it bumps `stateRev` and marks all questions dirty. Editing the JSON text always bumps `stateRev`, even if the parsed value is later reverted.
- The draft records `stateMode`; the active mode's value is what `buildRequest` sends. The mode is not persisted.
- The Go side must accept the same three shapes. `kevapi.Request.State` changes from `string` to `json.RawMessage`, and `ValidateRequest` accepts a non-empty JSON string, object, or array and rejects everything else. The proxy still forwards the raw state; no other endpoint changes.

### 6. Stability check

- Available only in Focus, as **Run 3× stability** in the answer column.
- Issues three sequential single-question requests with the same snapshot. The UI shows `Attempt 1/3…` and disables the run controls.
- If all three succeed, one run record is appended with `scope: "stability"`, the last answer in `answers`, and all three in `attempts`. The chip is labeled `#4 ×3`. The history counts it as one run. The record's top-level `latency_ms`, `usage`, and `requestId` come from the **last** attempt (the one reflected in `answers`); each attempt carries its own values in `attempts`.
- The answer panel's stability block shows the three values and a summary:
  - **noul:** `Yes 0.45–0.51 · spread 0.06`, plus the three attempts.
  - **choice:** `returns 2/3 · billing 1/3`, plus the three top choices.
  - **score:** `Mean 2.1–2.3 · spread 0.2`, plus the three means.
- Attempts stop at the **first failure**. No run is recorded; the panel shows the successful attempts, the failing attempt's error, and a **Retry** button. Retry reruns the whole three-attempt batch. The previous answer remains the latest. The partial attempt state is ephemeral Focus state: closing the dialog or opening another question clears it.
- The panel carries the note “Same input, same model. Variation is model variance, not a different question.”

### 7. Keyboard and accessibility

- `⌘↵` runs all when Focus is closed. While Focus is open it does nothing; use `⌘⇧↵`.
- `⌘⇧↵` runs the focused question while Focus is open.
- `Esc` closes Focus (native dialog).
- `Tab` / `Shift+Tab` stay inside the dialog (native).
- Arrow keys resize when the resize grip has focus; Shift multiplies the step.
- When an option or level row's `⠿` drag handle has focus, ArrowUp/ArrowDown move the row one position; the handle announces the new position through a polite live region.
- The resize grip is a button with an accessible label and a visually hidden hint. It is not the only way to size the dialog; the dialog is usable at its default size.
- The dialog has `aria-labelledby` pointing at the question id heading; the lint callout is referenced by `aria-describedby`.
- Buttons have text or `aria-label`. The ▶ buttons are labeled “Run {id}”. The warning/error markers have text alternatives, not color alone.
- Deltas always print `+`, `−`, or `0.00`; color is supplementary.
- The answer column and the modal's answer panel use `aria-live="polite"` for run completion. The stability progress is also polite.
- `prefers-reduced-motion: reduce` disables the dialog's open scale/fade and any bar transitions.
- Focus returns to the card that opened the dialog, or to the next card / Add after a delete.

### 8. Empty, loading, and error states

- **No questions:** the Questions column shows “No questions yet. Add one to run Kev.” The Answers column shows the same in its empty style. Run buttons are disabled.
- **No answers:** “Run a question to see its answer.”
- **Running:** the requested cards show `Evaluating…`; the run bar shows the scope; all run controls are disabled. Focus's run button shows `Running…`.
- **Model not ready:** the run controls are disabled with `title="Kev is still starting."`; the status line polls as today.
- **Validation error:** the field is marked; the card shows `!`; the run bar's disabled button explains the first blocking error in its title. Focus shows field messages.
- **Request error:** the banner (workbench) or the answer panel (Focus) shows Kev's `detail`; the previous answers remain; the raw drawer shows the failed exchange.
- **JSON state invalid:** the inline error names the parse problem; every run control is disabled until it is valid.

## Data model and pure functions

Add to `web/model.js` (names are indicative; the plan may refine signatures):

```js
// state
parseStateJSON(text) -> {ok, value, error}
isStateValue(value) -> boolean

// validation and request
validateDraft(draft) -> {ok, errors}
validateQuestion(draft, question) -> {ok, errors}
buildRequest(draft, qids) -> {state, questions}

// lint
lintDraft(draft) -> [{rule, questionId, message}]

// answers and comparison
formatAnswer(question, answer)
formatDelta(question, current, baseline) -> {text, direction, baselineP?, from?} | null
latestAnswers(runs) -> { [qid]: {runId, answer} }
baselineAnswer(runs, latest, qid, selectedRunId) -> answer | null

// run history
appendRun(runs, run) -> runs            // cap 20
changedQuestionIds(draft) -> [id]
stabilitySummary(question, attempts) -> {headline, rows, spread, agreement}

// modal
clampModalSize(size, viewport) -> {w, h}
resizeFromCorner(start, delta, viewport) -> {w, h}
readModalSize(raw, viewport) -> {w, h}
nextQuestionId(usedIds, base) -> string
duplicateQuestion(question, usedIds) -> question
reconcileBaseline(runs, selectedRunId) -> selectedRunId | null
```

Draft additions:

```js
{
  state: "…" | {…} | […],
  stateMode: "text" | "json",
  stateText: "…",
  stateError: "",
  stateRev: 0,
  questions: [{
    id, type, instructions,
    trueDescription?, falseDescription?,
    options?: [{name, description}],
    levels?: string[],
    dirty: false,
    rev: 0
  }]
}
```

- `latestAnswers` is derived from `runs`; there is no separate answer store to drift.
- `formatDelta` returns `baselineP` for noul and choice so the renderer can draw the tick; score returns no tick.
- `clampModalSize` uses default 780 × 560, min 560 × 420 (or the viewport if smaller), and max 92vw × 86vh. It ignores non-finite input and rounds.
- `appendRun` shifts the oldest beyond 20 and returns the same array.
- `duplicateQuestion` deep-copies, generates `<id>-copy`, `-copy-2`, … and sets `dirty: true`, `rev: 0`.

DOM glue stays in `web/app.js`: rendering the cards, the run bar, the answer cards, the dialog, the options/levels rows, the drag and keyboard reorder handling, the resize pointer handling, and the fetch calls. No DOM logic in `model.js`.

`moveItem(items, from, to)` is a pure helper in `model.js`; both pointer drag and the handle's arrow keys call it, so the reorder rules and bounds live in one tested place.

## Go changes

`internal/kevapi/types.go`:

```go
type Request struct {
    State     json.RawMessage     `json:"state"`
    Model     string              `json:"model,omitempty"`
    Questions map[string]Question `json:"questions"`
}
```

`internal/kevapi/validate.go`:

- Replace the string trim check with `validateState`.
- Accept a JSON string that is not whitespace-only, or a JSON object/array.
- Reject empty, `null`, numbers, booleans, and malformed input with a `state` validation error.
- Keep the existing question validation unchanged.

`internal/console` does not change. Its decoder already handles `json.RawMessage`; the proxy re-marshals the request and preserves the state value. Tests update to construct raw states and to assert an object state reaches the fake upstream unchanged.

## Visual specification

- Reuse the existing palette and typography. No new fonts, icons, or dependencies. Controls use text/Unicode glyphs already in the app (`▶`, `✕`, `⚠`, `⠿`, `×`).
- Answer probability bars use a green gradient from `--accent` to `#63b39b`, not a flat fill; the same green family is used in the TUI's setup progress bar.
- Overlay: `rgba(8, 22, 18, 0.72)`, no blur. It is dark enough that the workbench reads as context.
- Dialog surface: `#fffef9`, 1px `#cbd6ca`, 12px radius, shadow `0 30px 80px rgba(0,0,0,.5)`. Answer column `#eef2e9` with a 1px `#d8dfd6` left border.
- Run bar: `#fffef9`, top border `#d8dfd6`, sticky, primary button `#146858`.
- Delta chips: up `#146858` on `#e3efe9`; down `#9a382a` on `#ffede5`; flat `#65776d` on `#eef2e9`.
- Lint callout: `#ffede5` background, 3px `#9a382a` left border, `#7a2c20` text. Validation uses the existing `#9a382a`.
- Dirty dot: 6px, muted `#65776d`, with the label `edited`; it is not a warning.
- Resize grip: 16 × 16, diagonal hatch in `#b6c6ba`, bottom-right, with a focus ring.
- Responsive: the dialog's inline width/height is always clamped by `max-width: 92vw; max-height: 86vh`. At 560px the editor is narrow but usable; the footer wraps.

## Testing strategy (TDD)

Every milestone starts with a failing test.

### Browser (`node --test web/`)

- `validateDraft` / `validateQuestion`: text and JSON state, empty state, object/array, null/number, invalid JSON, per-scope validity.
- `buildRequest`: subset of questions; string and structured state; noul/choice/score criteria unchanged.
- `lintDraft`: one positive and one negative per rule; clean preset has no warnings; normalization; duplicate instructions.
- `formatDelta`: noul up/down/flat; choice same/changed/missing; score; no baseline; two-decimal formatting and the true minus.
- `latestAnswers` / `baselineAnswer`: per-question runs, subset runs, explicit baseline, missing baseline, stability runs.
- `appendRun` / `reconcileBaseline`: cap at 20, monotonic ids retained, explicit baseline resets to `auto` when evicted.
- `changedQuestionIds` and revision behavior, including Text/JSON mode switches and JSON → Text conversion.
- `nextQuestionId` / `duplicateQuestion`: session-monotonic allocation; a deleted id is never reissued; Add and Duplicate share the allocator.
- `moveItem`: moving up, down, to the same position, and beyond either bound; both pointer drag and keyboard reorder call it.
- Preset: clears runs, baseline, state mode, state revision, and the raw drawer; no-op while running; the preset's ids do not inherit old answers.
- `clampModalSize` / `resizeFromCorner`: defaults, max, min, tiny viewport, garbage input, doubling.
- `readModalSize`: valid, invalid JSON, missing, string values, out-of-range.
- `parseStateJSON`: string, object, array, null, number, bool, malformed, empty.
- `stabilitySummary`: noul, choice, score; spread and agreement; stop-at-first-failure and retry-all behavior is app-level and covered by the manual checklist.
- `duplicateQuestion`: deep copy, dirty flag, unique id, no reuse after delete.

### Go

- `kevapi.ValidateRequest`: string/object/array state; empty, whitespace string, null, number, bool, malformed.
- `kevapi.Request` JSON round-trip with a raw object state.
- `console`: object state reaches the fake upstream unchanged; invalid state returns 400; existing proxy/status behavior unchanged.

### Manual smoke test

1. Load the preset; run all; confirm three answers and the run bar usage.
2. Edit `escalate` in Focus; run it with `⌘⇧↵`; confirm only that answer and its delta change.
3. Click a run chip; confirm the delta baseline changes; click `auto`; confirm it returns.
4. Run changed; confirm only the dirty question is sent (visible in Raw JSON).
5. Resize the modal; close and reopen; confirm the size persists and the modal is recentered.
6. Shrink the browser window; confirm the modal clamps and stays usable.
7. Tab through Focus; confirm focus stays inside; Esc closes; backdrop click does nothing; focus returns to the card.
8. Trigger each lint rule once; confirm the message is specific and Run stays enabled.
9. Run 3× stability; confirm three attempts and a spread summary.
10. Switch the state to JSON with an object; run; confirm Raw JSON shows the object. Switch back to Text; confirm the conversion hint.
11. `make vet && make test`; `make build`; launch the binary and repeat the main loop.

### Commands

```
make vet
make test
make build
```

No real model download is part of these tests. The opt-in e2e is unchanged.

## Milestones

1. **State type and validation.** Go `json.RawMessage` change, validation, proxy tests. The browser still sends a string, so the app works unchanged.
2. **Pure model.** `model.js` additions and their `node --test` suite: state parsing, per-scope validation, request subsetting, lint, deltas, history, stability, modal size. Old app behavior remains green.
3. **Workbench.** Cards with run buttons, sticky run bar, per-question/all/changed runs, answer cards with deltas and stale badges, run chips and baseline selection, raw JSON updates.
4. **Focus.** Native dialog, editor fields, answer panel, run-in-place, duplicate, two-step delete, focus return, live validation/lint.
5. **Resize and persistence.** Pointer and keyboard resizing, clamp, `localStorage` size, window-resize behavior.
6. **Stability and JSON state.** The 3× run and the Text/JSON editor, including the conversion hint and Go integration.
7. **Polish and acceptance.** Empty/loading/error states, responsive review, manual checklist, docs update if needed, `make vet && make test`.

The work can merge at any green milestone, but the run bar and Focus are the point of the change; the plan should keep them together for review.

## Risks

- **Modal complexity.** Native `<dialog>` removes the focus-trap and inertness code that usually makes modals fragile. The remaining risk is layout at small sizes; test at 560 × 420 and with long option lists.
- **Resize math while centered.** A centered dialog grows in both directions; the corner must double the pointer delta to track the cursor. The pure function is tested; the pointer glue is manually checked.
- **Run history semantics.** Per-question runs make “previous” ambiguous. The design resolves it with per-question latest + auto baseline; the UI must label the baseline, or the deltas will feel arbitrary.
- **Lint false positives.** Warnings are advisory and explained; the rule set is deliberately small. Do not add a model judge or a scoring system.
- **State type change.** `json.RawMessage` touches only `kevapi` and its tests; the proxy forwards it unchanged. Run the existing console tests.
- **Stability cost.** Three sequential calls can take several seconds. The UI shows progress and disables reruns; no cancel in v1.
- **No persistence.** A refresh loses runs and drafts by design. The only persistence is the modal size; do not let that grow into draft autosave without a new decision.
- **Small-screen editing.** At 560 px the editor is narrow; the fields are vertical and scroll. If it proves unusable, a later change can stack the answer column below the editor below 640 px.

## Out of scope / next phase

### Ollama drafting assistant (later)

The user asked whether a local Ollama model could help write better prompts. The direction is sound, but it is deliberately not in this build. When it lands:

- A Go `POST /api/assist` proxy to `127.0.0.1:11434`, never exposed beyond loopback, with a short timeout and a clear “Ollama is not running” state.
- Strict JSON-schema prompts for a small set of tasks: state → candidate questions, tighten instructions, suggest criteria, and critique ambiguity.
- Suggestions are shown as diffs with **Apply** and **Discard**; nothing is auto-applied. The assistant never answers a `noul`/`choice`/`score` question and never runs Kev.
- A feature flag and a model picker that lists local models; the Focus button stays disabled until a model is reachable.
- No state or answers leave the machine.

### Other deferred ideas

- Case files and an eval matrix (comp C), with expected answers and weighted checks.
- Model comparison (0.8B vs 4B) with blind reveal, not Elo.
- Replay/demo mode, template gallery, draft autosave, batch datasets, and a request timeline.

## Builder calls (check these while reading)

1. **Native `<dialog>`** instead of a hand-built overlay/focus trap. It provides the top layer, inertness, focus trapping, and Esc; the tint is `::backdrop`.
2. **Ids and types are immutable after creation.** Renaming would orphan answer keys and run history; **Duplicate variant** is the supported path to a new id. This changes the 2026-11-02 spec's “editable id”.
3. **Run history is session-only and capped at 20.** Only the modal size persists; drafts, answers, and runs do not.
4. **Baseline defaults to the previous run containing that question**, not the previous run overall. Per-question runs make a single global “previous” misleading.
5. **Deltas are movement, not quality.** Teal/rust with explicit signs; no “improved/regressed” labels.
6. **Lint is eleven deterministic warnings**, no LLM judge, no auto-fix, no blocking.
7. **The modal's size is the only persisted thing**, under `gavel.focus.size.v1`, and the stored value is the user's preference; window resizes clamp visually without overwriting it.
8. **Delete is a two-step inline confirm**, not `window.confirm` inside a `<dialog>`.
9. **No cancel for in-flight requests**; the stability batch is the only long action and shows progress.
10. **`⌘↵` is run-all only when Focus is closed; `⌘⇧↵` is run-one in Focus.** This avoids a hidden global action behind the modal.
11. **`changed` scope validates only the dirty questions**; an untouched invalid question does not block running the changed ones.
12. **Text/JSON state conversion is explicit**: Text always sends a string; JSON sends the parsed value; switching to Text converts structured state to a pretty-printed string, with a hint. The revision bumps only when the payload changes; Text → JSON on a string is representation-only.
13. **Preset is a full session reset**: draft, runs, baseline, state mode, revisions, Focus, and the raw drawer. It is a no-op while running.
14. **Ids are session-monotonic and never reused**, including after delete. Add and Duplicate share one used-id set.
15. **Stability stops at the first failure**; Retry reruns all three; a successful record's top-level latency/usage/request id come from the last attempt.
16. **A pending delete consumes the first Esc** (the one `preventDefault` exception); the second Esc closes Focus.
17. **An evicted baseline resets to auto** rather than dangling.
18. **State-mode switches bump `stateRev` only when the payload changes**; JSON → Text on an object does, Text → JSON on a string does not.
19. **Option and level rows use a left `⠿` drag handle and a right `×` remove button**, not ↑/↓ buttons. Pointer drag and the handle's ArrowUp/ArrowDown both reorder, through the same pure `moveItem` helper.

## Open questions

None blocking. The builder check surfaced six follow-ups — preset reset, id reuse, stability failure/aggregation, pending-delete Esc, baseline eviction, and state-mode revision — all folded into the design and listed in Builder calls.
