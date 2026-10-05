# Field-Data Part 3: Gate Cost — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `validate_plan` converge on an edited plan instead of re-reviewing it whole, take the `codebase_reference_checklist` out of the findings, and have the server write the CodeScene event records.

**Architecture:** A `validate_plan` call that names an earlier round's `plan_run_id` is a *round* on that run: the run keeps the reviewer's own output per task, keyed by a hash of the task's text, so a round sends the reviewer only the changed tasks (through the existing chunk path), re-runs the plan-level pass only when the plan text changed, and applies the current rulings to everything it carries. The checklist becomes an envelope field on both `validate_task_spec` and `validate_plan`, built before the verdict ladder runs. `validate_completion` appends a content-free record of a reported CodeScene run to `codescene-events.jsonl`, once per distinct result per session, and the operator hook that used to write that file becomes a no-op.

**Tech Stack:** Go 1.25 (`text/template` prompts with golden files, `testify` in `internal/` tests), bash for the retired hook script.

**Spec:** `docs/superpowers/specs/2026-10-04-field-data-improvements-design.md` (§6 is this plan; §9 holds the maintainer's rulings; §2.3, §2.4 and §2.6 are the evidence; §3.1 the success measures). Parts 1 and 2 of that spec are merged and are **not** in this plan.

## How this plan is executed

Every task's code is in two patch files beside this plan, in `docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/`:

- `NN-<slug>.tests.patch` — the task's new and changed tests (and prompt golden files).
- `NN-<slug>.impl.patch` — everything else the task changes, its `CHANGELOG.md` bullet included.

A task is: apply the tests patch, see the named tests fail, apply the implementation patch, see the suites pass, check the tree hash, commit. The patches were cut from a dry run in which the whole of Part 3 was built and tested on `origin/version/0.27.0` at `dbc5a0e`; `TREES` in the same directory holds, per task, a hash of the index the dry run had once the task's patches were applied, and `check-tree.sh NN` (run from the repository root, after `git apply --index`) compares the index with it, leaving this plan's own files out. `tree OK (NN)` is the acceptance gate for "the patch was applied exactly": it is stronger than reading the diff.

Apply a patch with `git apply --index <file>` from the repository root. **A patch that does not apply means the base has moved: stop and report it. Do not edit a patch, apply it with `--reject` or `-3`, or adapt the code by hand.** The per-task reviewers read the applied diff as ordinary code; what each task must achieve is in its Goal and Acceptance Criteria, and the design decisions behind it are in "Rulings this plan makes".

## Global Constraints

- **Branch and release.** Work is on `feature/field-data-part-3-gate-cost`, at `origin/version/0.27.0` (`dbc5a0e`) plus this plan. The pull request targets `version/0.27.0`, never `main`. Do **not** edit `VERSION`. CodeRabbit does not auto-review a pull request whose base is not `main`: ask it with a top-level `@coderabbitai review` comment each round, through `gh` only.
- **Do not gate this plan with anti-tangent.** It changes the tool. Skip `validate_plan`, `validate_task_spec`, `check_progress` and `validate_completion` for every task here; there is no `plan_run_id` and no `record_review_outcome` call. Subagent-driven spec and code-quality review per task, then a final whole-branch review, is the gate. A dispatch prompt for these tasks must **not** contain the heading `## Drift-protection protocol (anti-tangent-mcp)`: the installed guard would then refuse the implementer's edits.
- **The server stays advisory.** Nothing in this plan rejects a call. An unknown `plan_run_id` falls back to a full review; a missing one attaches nothing or attaches by title; a failed stats write is logged and changes no result.
- **Tasks run in order, one at a time.** Tasks 1, 2, 3, 6, 7 and 8 all edit `CHANGELOG.md`; tasks 1, 2, 6 and 7 edit `internal/mcpsrv/handlers.go`. Each patch was cut against the tree the task before it leaves.
- **Exact strings.** Envelope and plan-response field `codebase_reference_checklist` (array of strings). Event field `checklist_items`. Summary line `  checklist:     N unverified codebase reference(s), not findings`. `validate_plan` argument `plan_run_id`; response object `review_scope` with `revision`, `tasks_reviewed`, `tasks_carried`, `plan_level_reviewed`; summary line `  review_scope:  revision R: A task(s) reviewed, B carried; plan-level findings reviewed|carried`. Event field `tasks_carried`; `runs.jsonl` header fields `revision`, `tasks_carried`; `plan-runs.jsonl` header field `revision`. Advisory findings: `category: other`, `criterion: plan_run_id`, `severity: minor`. Prompt heading `## Earlier plan-level findings`. Stats file `codescene-events.jsonl`; session field `CodesceneEventKey`.
- **Additive wire changes only.** Every new JSON field is `omitempty` or sits inside a new optional object, so records written by 0.26.0 still decode. `scorecard/` imports only the standard library; the gnome-topbar daemon builds against it through a `replace` directive and must keep compiling. Nothing under `gnome-topbar/` is edited.
- **Content-free records.** `events.jsonl`, `runs.jsonl` and `codescene-events.jsonl` hold counts, verdicts, model ids, anti-tangent's own category names and, in `codescene-events.jsonl`, CodeScene's category names. Nothing here adds a title, a path, a ref name, finding text or a raw id to them. File paths a plan lists are kept in memory only (`json:"-"`).
- **Public repo.** No consumer ticket id, file name, plan title or task title from the field data appears in code, tests, docs or commit messages. Fixtures use invented names.
- **Protocol byte budgets (CI-enforced).** Each `docs/protocol/*.md` strictly under 16,000 bytes; `INTEGRATION.md` under 2,000; `plugin/anti-tangent-protocol/protocol/` identical to `docs/protocol/`. Task 1 takes `core.md` from 15,970 to 15,983; Task 8 takes `controller.md` from 15,802 to 15,904. Both patches include the resynced bundle. Do not renumber sections.
- **Changelog.** `CHANGELOG.md` has `## [0.27.0] - 2026-10-04`, shared by all three parts. The patches carry the bullets.
- **Comments.** Comments explain non-obvious behaviour or an invariant. No task, issue, PR or version references, no "previously" / "no longer" / "this replaced". The guard plugin enforces this on `Edit`/`Write`.
- **Prompt goldens.** Golden files are in the tests patches. After a task that changes a template, `go test ./internal/prompts/... -update` must leave `git status --short internal/prompts/testdata` empty.
- **Tests.** `go test -race ./...` from the repo root, and `cd gnome-topbar/daemon && go test -race ./...`. Guard plugin: `bash plugin/anti-tangent-guard/evals/run.sh` (198 cases) and `bash plugin/anti-tangent-guard/evals/fp-report.sh` (0 false positives). `bash examples/hooks/codescene-log_test.sh` prints `OK`. No test touches the network.

**User decisions (already made):**
- The checklist moves out of the findings (spec §9.7).
- The server writes the CodeScene records; the hook is retired (spec §9.10).
- `codescene_not_run` stays verdict-driving; an unevidenced CodeScene skip stays major.
- One release, 0.27.0, carries all three parts; this part merges into `version/0.27.0`.
- This plan is not gated with anti-tangent's own tools.

**Rulings this plan makes (reported to the maintainer with the PR):**
1. **The checklist leaves the findings on both tools.** Spec §6.2 and its evidence are about `validate_task_spec`; `validate_plan` already appended its checklist after the ladder, but as a finding. Both now return `codebase_reference_checklist` as a field: one entry per claim on `validate_task_spec`, one `Task N: …` entry per affected task on `validate_plan`. A caller that looked for the `codebase_reference_checklist` criterion in `findings` / `plan_findings` must read the field.
2. **`validate_task_spec` has no `Files:` argument**, so "paths listed in the task's own `Files:` section" come from two places: a `**Files:**` section inside the `context` argument, and the paths the plan run recorded for the task when `validate_plan` parsed the plan. The run keeps those paths in memory only.
3. **What "a path that is not a claim" means:** an `unverifiable_codebase_claim` whose evidence names at least one listed path, whole (not as a piece of a longer path or file name), and, once those paths and their line anchors are removed, nothing that reads as a code reference: a backticked span, a dotted name, a path, a call, a snake_case or camelCase identifier, or a capitalised word that does not open a sentence (in a claim about code that is usually a type name). The rule errs toward keeping: a claim that also names a symbol, another path or a convention stays on the checklist, at the cost of also keeping some claims that only mention a proper noun. The plan prompt is also told not to raise the claim.
4. **A task is "unchanged" when its heading and whole body are byte-identical** to the earlier round's. The heading carries the task's number, so a renumbered task is re-reviewed. Matching is by hash, not by position, so inserting a task does not re-review the tasks after it unless their numbers change.
5. **Text outside every task** (the plan's preamble) is plan-level: a change there re-runs the plan-level pass and carries every task. A carried task's result is the reviewer's judgement of that task against the plan as it was then: a finding that depended on the preamble, or on another task that has since changed (a type renamed in Task 2 that Task 3 uses), stays as it was until the task's own text changes. Cross-task problems an edit introduces are the plan-level pass's to find, and it runs on every such round. Omitting `plan_run_id` forces a full review.
6. **The run keeps the reviewer's output before rulings, verified references and the ladder**, and every round re-applies the current ones. Adding a `controller_rulings` entry or a `controller_verified_references` entry therefore costs no reviewer call, which is the most common reason for a second round. Neither is part of what decides "changed". One direction is not symmetric: the prompt also tells the reviewer not to raise what a ruling or a verified reference already covers, so a finding the reviewer never raised because of a ruling in force at review time does not come back on a carried task when a later round drops that ruling.
7. **Changed `project_knowledge`, `mode`, model or attached `context_paths` content** review every task again under the same id, and the earlier plan-level findings are not shown: they were raised against other inputs. `repo_root` and `max_tokens_override` change nothing about what is carried.
8. **"Pass is terminal" holds for every verdict:** an unchanged round makes no reviewer call whether the stored result was `pass`, `warn` or `fail`. A `warn` that the controller answers only with rulings converges without the reviewer.
9. **A round on a known run never reads or writes the three-minute pass cache.** Callers that pass no id keep it. A cache entry whose run a later round revised to a different plan text is not served.
10. **A truncated round** keeps the id and leaves the run's stored review and revision as they were. It returns the carried tasks and the tasks reviewed before the cut as a partial result whose verdict is `warn`, with a major `reviewer_response` finding saying how many tasks have no result: a response naming only carried tasks must not read as a pass on text the reviewer did not finish. Nothing is recovered from the truncating call's partial bytes (the full-review path does recover them): a recovered task numbers itself from its chunk, which collides with the carried tasks' plan positions. The retry reviews the changed tasks again, including any that had completed.
11. **Rows attached before a later round keep their task index.** A round that renumbers tasks after dispatch does not move them; rounds normally happen before dispatch.
12. **Concurrent rounds on one run:** each stores a complete review under the store's lock; the last one wins. No round can leave a half-merged record.
13. **Revision numbering starts at 1** on the round that mints the run. `runs.jsonl` and `plan-runs.jsonl` get one header line per round; the scorecard and the daemon already take the latest header per run hash. A round that made no reviewer call repeats the `plan_call` of the last round that did, so the header's model and latency stay those of a real review; `review_scope.revision` is the store's count, so two concurrent rounds cannot report the same one.
14. **Attach by title** needs exactly one live run on the server *and* exactly one plan heading matching the title. It ignores `task_index` both for the decision and for the row: a `task_index` sent without a run was never checked against this plan, so the matched heading names the row. A truncated review creates no session, attaches nothing, and gets the plain "this server holds a live run" advisory. A task that happens to share a title with a heading of an unrelated live plan would be attached to it; the advisory finding names the run so the caller can see it.
15. **CodeScene events are de-duplicated per session.** Spec §6.3 says "every `validate_completion` call whose `codescene` argument reports a run"; an implementer resends its result on every retry (3.24 calls per task in the field data), and the spec's own reason for retiring the hook is that a run must not be counted twice. A record is written when the reported result differs from the session's last recorded one. A lightweight call has no session and records every time. Two real runs with byte-identical counts in one session are recorded once.
16. **The event record holds** `ts`, `tool`, `quality_gate`, `files_analyzed`, `verdicts`, `trend`, `net_pp`, `category_counts` — what the retired hook wrote. The hook wrote the constant `analyze_change_set` as `tool`; the argument's `tool` is caller text, so the record says `analyze_change_set` when that (or nothing) was sent and `other` for anything else. `ran`, `skip_reason`, `skip_evidence` and `base_ref` are never written. `category_counts` keys are caller-sent CodeScene category names, capped at 20 entries of 100 characters by the existing normaliser.
17. **The retired hook script becomes a no-op** instead of being deleted: a `settings.json` that still registers it keeps working and can no longer double-count. Its fixture is deleted and its test asserts it writes nothing.
18. **Two new counters** make the §3.1 measures readable without joining files: `checklist_items` and `tasks_carried` on `events.jsonl`.
19. **Not done, and reported:** the gnome-topbar stats page still tells the operator to append records by hand (`gnome-topbar/daemon/internal/server/statspage.go`); it points at the setup document, which is now correct. Changing it is a daemon release.

---

### Task 1: `codebase_reference_checklist` leaves the findings

**Goal:** `validate_task_spec` and `validate_plan` return unverifiable codebase references in a `codebase_reference_checklist` field instead of as a finding, so the checklist counts toward no verdict.

**Files:**
- Modify: `internal/mcpsrv/handlers.go`
- Modify: `internal/mcpsrv/task_spec_normalize.go`
- Modify: `internal/mcpsrv/plan_normalize.go`
- Modify: `internal/mcpsrv/review_error.go`
- Modify: `internal/mcpsrv/summary.go`
- Modify: `internal/verdict/plan.go`
- Modify: `internal/stats/event.go`
- Modify: `README.md`, `CHANGELOG.md`, `docs/protocol/core.md`, `plugin/anti-tangent-protocol/protocol/core.md`
- Test (create): `internal/mcpsrv/checklist_field_test.go`
- Test: `internal/mcpsrv/handlers_test.go`, `internal/mcpsrv/handlers_plan_test.go`, `internal/mcpsrv/handlers_plan_rulings_test.go`, `internal/mcpsrv/plan_normalize_test.go`, `internal/mcpsrv/summary_forgery_test.go`

**Acceptance Criteria:**
- [ ] `Envelope` and `verdict.PlanResult` each have `CodebaseReferenceChecklist []string` with JSON key `codebase_reference_checklist`, `omitempty`.
- [ ] `splitTaskSpecChecklist` returns the non-claim findings and one trimmed, truncated entry per `unverifiable_codebase_claim`; `normalizeTaskSpecUnverifiableFindings` and `appendCodebaseReferenceChecklist` no longer exist.
- [ ] A `validate_task_spec` review returning two real minors and two unverifiable claims gives `pass`, two findings, no `noise_cluster`, a two-entry checklist, and session `PreFindings` equal to the two findings.
- [ ] `validate_plan` sets the field from the per-task lines (`Task N: …`), after the ladder; `plan_findings` holds no finding with criterion `codebase_reference_checklist`.
- [ ] The summary block of both tools prints `  checklist:     N unverified codebase reference(s), not findings` and one escaped bullet per entry; `next_action` names the field when it is non-empty.
- [ ] `events.jsonl` records `checklist_items` for both tools.
- [ ] `docs/protocol/core.md` is under 16,000 bytes and identical to its bundled copy.

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/verdict/...` → `ok` for each; `check-tree.sh 01` prints `tree OK (01)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/01-checklist-field.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/mcpsrv/...`
Expected: FAIL to build — `undefined: splitTaskSpecChecklist`, and `env.CodebaseReferenceChecklist undefined`.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/01-checklist-field.impl.patch
```

- [x] **Step 4: Run the suites**

Run: `go build ./... && go test -race ./...`
Expected: every package `ok`.

- [x] **Step 5: Check the tree and the budget**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 01 && wc -c docs/protocol/core.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol`
Expected: `tree OK (01)`; `15983 docs/protocol/core.md`; `diff` prints nothing.

- [x] **Step 6: Commit**

```bash
git commit -m "feat: codebase_reference_checklist is an envelope field, not a finding"
```

```json:metadata
{"files": ["internal/mcpsrv/handlers.go", "internal/mcpsrv/task_spec_normalize.go", "internal/mcpsrv/plan_normalize.go", "internal/mcpsrv/review_error.go", "internal/mcpsrv/summary.go", "internal/verdict/plan.go", "internal/stats/event.go", "README.md", "CHANGELOG.md", "docs/protocol/core.md", "plugin/anti-tangent-protocol/protocol/core.md", "internal/mcpsrv/checklist_field_test.go", "internal/mcpsrv/handlers_test.go", "internal/mcpsrv/handlers_plan_test.go", "internal/mcpsrv/handlers_plan_rulings_test.go", "internal/mcpsrv/plan_normalize_test.go", "internal/mcpsrv/summary_forgery_test.go"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/verdict/...", "acceptanceCriteria": ["Envelope and PlanResult carry codebase_reference_checklist", "splitTaskSpecChecklist replaces the rolled-up finding", "two minors plus two claims pass with no noise_cluster", "validate_plan sets the field after the ladder", "summary block prints the checklist and next_action names it", "events.jsonl records checklist_items", "core.md under 16000 bytes and bundle in sync", "check-tree.sh 01 prints tree OK"], "modelTier": "mechanical"}
```

### Task 2: Paths a task lists in its `Files:` section are not claims

**Goal:** An `unverifiable_codebase_claim` that only names paths the task's own `**Files:**` section lists is dropped before the checklist is built, on both tools.

**Files:**
- Create: `internal/mcpsrv/listed_file_claims.go`
- Modify: `internal/planparser/filerefs.go`
- Modify: `internal/planrun/planrun.go`
- Modify: `internal/mcpsrv/plan_normalize.go`
- Modify: `internal/mcpsrv/task_spec_normalize.go`
- Modify: `internal/mcpsrv/review_error.go`
- Modify: `internal/mcpsrv/handlers.go`
- Modify: `internal/prompts/templates/plan_rules.tmpl`
- Modify: `CHANGELOG.md`
- Test (create): `internal/mcpsrv/listed_file_claims_test.go`
- Test: `internal/planparser/filerefs_test.go`, `internal/planrun/planrun_test.go`, twelve `internal/prompts/testdata/plan_*.golden` files

**Acceptance Criteria:**
- [ ] `planparser.ListedPaths(body)` returns every path of the `**Files:**` section, whatever the bullet's label (`Create`, `Modify`, `Test`, …), without line anchors, each once, and nothing from a later section.
- [ ] `claimIsOnlyListedPaths` is true for a claim naming only listed paths (bare, backticked, with a line anchor, several in prose) and false when the claim also names a backticked symbol, a dotted name, a call, a snake_case or camelCase identifier, or an unlisted path or file name; false with no listed files.
- [ ] `validate_plan` drops such a claim for the task that lists the path and keeps the same claim on a task that does not; a plan whose only claims are listed paths has no checklist and passes.
- [ ] `planrun.PlanTask.Files` is `json:"-"`; `Store.TaskFiles(runID, ref)` resolves by index in range, else by the one matching heading, returns a copy, and returns nil for an unknown run or task. The ledger header carries no path.
- [ ] `validate_task_spec` drops claims for paths listed in a `**Files:**` section of `context` and for paths the named plan run recorded for the task; an unknown run drops nothing.
- [ ] `plan_rules.tmpl` tells the reviewer a path under a task's own `**Files:**` section is not a claim; only the twelve `plan_*` goldens change.

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/planparser/... ./internal/planrun/... ./internal/prompts/...` → `ok` for each; `check-tree.sh 02` prints `tree OK (02)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/02-listed-file-paths.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/planparser/... ./internal/planrun/... ./internal/mcpsrv/... ./internal/prompts/...`
Expected: FAIL — `undefined: ListedPaths`, `undefined: claimIsOnlyListedPaths`, `unknown field Files`, and the `plan_*` golden tests differ.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/02-listed-file-paths.impl.patch
```

- [x] **Step 4: Run the suites and check the goldens are current**

Run: `go build ./... && go test -race ./... && go test ./internal/prompts/... -update && git status --short internal/prompts/testdata`
Expected: every package `ok`; `git status` prints nothing.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 02`
Expected: `tree OK (02)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat: a path a task lists in its Files section is not a codebase claim"
```

```json:metadata
{"files": ["internal/mcpsrv/listed_file_claims.go", "internal/planparser/filerefs.go", "internal/planrun/planrun.go", "internal/mcpsrv/plan_normalize.go", "internal/mcpsrv/task_spec_normalize.go", "internal/mcpsrv/review_error.go", "internal/mcpsrv/handlers.go", "internal/prompts/templates/plan_rules.tmpl", "CHANGELOG.md", "internal/mcpsrv/listed_file_claims_test.go", "internal/planparser/filerefs_test.go", "internal/planrun/planrun_test.go", "internal/prompts/testdata"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/planparser/... ./internal/planrun/... ./internal/prompts/...", "acceptanceCriteria": ["ListedPaths returns every labelled bullet's paths of the Files section", "claimIsOnlyListedPaths keeps any claim naming another code reference", "validate_plan drops listed-path claims per task", "PlanTask.Files stays in memory and TaskFiles resolves by index then title", "validate_task_spec drops claims listed in context or by the plan run", "only the plan_* goldens change", "check-tree.sh 02 prints tree OK"], "modelTier": "mechanical"}
```

### Task 3: The server writes `codescene-events.jsonl`; the hook is retired

**Goal:** `validate_completion` appends one content-free record per reported CodeScene run, once per distinct result per session, and the operator hook that wrote the same file becomes a no-op that its script and setup document both declare retired.

**Files:**
- Create: `internal/mcpsrv/codescene_event.go`
- Modify: `internal/stats/codescene.go`
- Modify: `internal/session/session.go`, `internal/session/store.go`
- Modify: `internal/mcpsrv/handlers.go`
- Modify: `internal/codescene/codescene.go` (comments only)
- Modify: `examples/hooks/codescene-log.sh`
- Modify: `docs/team-setup/codescene-stats.md`, `README.md`, `CHANGELOG.md`
- Delete: `examples/hooks/testdata/analyze_change_set.stdin.json`
- Test (create): `internal/mcpsrv/codescene_event_test.go`
- Test: `internal/stats/codescene_test.go`, `internal/session/store_test.go`, `examples/hooks/codescene-log_test.sh`

**Acceptance Criteria:**
- [ ] `stats.RunRecord(d)` writes `Tool` as `analyze_change_set` when the caller sent that or nothing and as `other` otherwise, and keeps `QualityGate`, `FilesAnalyzed`, `Verdicts`, `Trend`, `NetPP`, `CategoryCounts` and nothing else; `(*Recorder).RecordCodescene` appends it with a timestamp to `codescene-events.jsonl`, is best-effort, and is safe on a nil recorder. The written line contains no `skip_reason`, `skip_evidence`, `base_ref` or `ran` key.
- [ ] Two `validate_completion` calls on one session sending the same `ran: true` result write one line; a different result writes a second.
- [ ] A skip, or a call with no `codescene` argument, writes nothing, and does not stop a later run being recorded.
- [ ] A lightweight call writes a line on every call that reports a run.
- [ ] `Session.CodesceneEventKey` is replaced only by a non-empty key through `ApplyReview`, and `ReviewState` returns it.
- [ ] `examples/hooks/codescene-log.sh` writes nothing, exits 0 on any input, and says it is retired and why; `codescene-log_test.sh` prints `OK`.
- [ ] `docs/team-setup/codescene-stats.md` describes the server-written channel, the de-duplication, the record shape, and tells an operator who registered the hook to remove it because a run would be counted twice.

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/session/... ./internal/codescene/... && bash examples/hooks/codescene-log_test.sh` → `ok` for each, then `OK`; `check-tree.sh 03` prints `tree OK (03)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/03-codescene-events.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/stats/... ./internal/session/... ./internal/mcpsrv/...; bash examples/hooks/codescene-log_test.sh`
Expected: FAIL — `undefined: codesceneRunKey`, `r.RecordCodescene undefined`, `unknown field CodesceneEventKey`; the hook test prints `FAIL: the retired hook wrote into the stats dir`.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/03-codescene-events.impl.patch
```

- [x] **Step 4: Run the suites**

Run: `go build ./... && go test -race ./... && bash examples/hooks/codescene-log_test.sh`
Expected: every package `ok`, then `OK`.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 03`
Expected: `tree OK (03)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat: the server writes codescene-events.jsonl; the PostToolUse hook is retired"
```

```json:metadata
{"files": ["internal/mcpsrv/codescene_event.go", "internal/stats/codescene.go", "internal/session/session.go", "internal/session/store.go", "internal/mcpsrv/handlers.go", "internal/codescene/codescene.go", "examples/hooks/codescene-log.sh", "docs/team-setup/codescene-stats.md", "README.md", "CHANGELOG.md", "examples/hooks/testdata/analyze_change_set.stdin.json", "internal/mcpsrv/codescene_event_test.go", "internal/stats/codescene_test.go", "internal/session/store_test.go", "examples/hooks/codescene-log_test.sh"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/session/... ./internal/codescene/... && bash examples/hooks/codescene-log_test.sh", "acceptanceCriteria": ["RunRecord keeps only the content-free fields and RecordCodescene appends it", "the same result on one session is recorded once, a different one again", "a skip or a missing argument writes nothing", "a lightweight call records every time", "CodesceneEventKey is replaced only by a non-empty key", "the hook script writes nothing, exits 0 and says it is retired", "the setup document describes the server-written channel and tells operators to remove the hook", "check-tree.sh 03 prints tree OK"], "modelTier": "mechanical"}
```

### Task 4: A plan run counts its rounds and keeps a review record

**Goal:** `planrun.Store` can revise a run in place — new verdict, quality and task list under the same id, with a revision count — and holds an opaque record of the plan's latest review; the ledger and `runs.jsonl` headers carry the revision.

**Files:**
- Modify: `internal/planrun/planrun.go`
- Modify: `internal/planrun/ledger.go`
- Modify: `scorecard/records.go`
- Test: `internal/planrun/planrun_test.go`, `internal/planrun/ledger_test.go`

**Acceptance Criteria:**
- [ ] `Run.Revision` is 1 on `CreateWithTasks`; `Store.Revise(runID, planVerdict, planQuality, tasks, review)` replaces verdict, quality, `TaskCount`, `Tasks` and the review, adds one to `Revision`, keeps rows and attached sessions, and returns a copy taken under the same lock that shares nothing with the run and does not expose the review; it returns false for an unknown run.
- [ ] `Store.Review(runID)` returns the stored review and the revision, `(nil, 1, true)` for a run with none, and `ok == false` for an unknown run; `Store.SetReview` returns false for an unknown run. `Snapshot` does not expose the review.
- [ ] `ledgerHeaderLine` and `ledgerLine` carry `revision`; `Ledger.Load` on a run with two header lines returns the higher revision's tasks, task count, verdict and quality, the earliest `CreatedAt`, and the run's rows.
- [ ] `scorecard.RunLine` has `Revision` and `TasksCarried`, both `omitempty`; `scorecard` still imports only the standard library.

**Verify:** `go test -race ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go test -race ./...)` → `ok` for each; `check-tree.sh 04` prints `tree OK (04)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/04-plan-run-revision.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/planrun/...`
Expected: FAIL to build — `s.Revise undefined`, `s.Review undefined`, `run.Revision undefined`, `revised.review undefined`.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/04-plan-run-revision.impl.patch
```

- [x] **Step 4: Run the suites**

Run: `go build ./... && go test -race ./... && (cd gnome-topbar/daemon && go test -race ./...)`
Expected: every package `ok` in both modules.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 04`
Expected: `tree OK (04)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat(planrun): a run counts its validate_plan rounds and keeps a review record"
```

```json:metadata
{"files": ["internal/planrun/planrun.go", "internal/planrun/ledger.go", "scorecard/records.go", "internal/planrun/planrun_test.go", "internal/planrun/ledger_test.go"], "verifyCommand": "go test -race ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go test -race ./...)", "acceptanceCriteria": ["Revision is 1 on create and Revise adds one under one lock, keeping rows and sessions", "Review and SetReview behave as specified and Snapshot hides the review", "the ledger header carries revision and Load takes the latest round's header", "RunLine has Revision and TasksCarried and scorecard stays stdlib-only", "check-tree.sh 04 prints tree OK"], "modelTier": "mechanical"}
```

### Task 5: The plan-level prompt can be shown the earlier round's findings

**Goal:** `prompts.RenderPlanFindingsOnly` renders an `## Earlier plan-level findings` section from `PlanInput.PriorPlanFindings`, in the per-call part of the prompt.

**Files:**
- Modify: `internal/prompts/prompts.go`
- Modify: `internal/prompts/templates/plan_findings_only.tmpl`
- Test: `internal/prompts/prompts_test.go`
- Test (create): `internal/prompts/testdata/plan_findings_only_with_prior_findings.golden`

**Acceptance Criteria:**
- [ ] `PlanInput` has `PriorPlanFindings []verdict.Finding`.
- [ ] With no prior findings the three existing `plan_findings_only*` goldens are byte-identical to before.
- [ ] With prior findings the section appears in `UserSuffix`, before `## Output`; `UserPrefix` is byte-identical to the render without them (the prefix is shared with the chunk prompts for provider-side caching).
- [ ] Each finding renders as `- [severity][category] criterion — evidence` on one line, a multi-line field folded by `oneLine`.
- [ ] The section tells the reviewer to drop a resolved finding without rewording it, to raise one that still holds with the same `category` and `criterion`, and to raise a finding outside the list only for a problem the edit introduced or a critical or major one.

**Verify:** `go test -race ./internal/prompts/...` → `ok`; `check-tree.sh 05` prints `tree OK (05)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/05-prior-plan-findings-prompt.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/prompts/...`
Expected: FAIL to build — `in.PriorPlanFindings undefined`.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/05-prior-plan-findings-prompt.impl.patch
```

- [x] **Step 4: Run the suite and check the goldens are current**

Run: `go test -race ./internal/prompts/... && go test ./internal/prompts/... -update && git status --short internal/prompts/testdata`
Expected: `ok`; `git status` prints nothing.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 05`
Expected: `tree OK (05)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat(prompts): the plan-level pass can be shown the earlier round's findings"
```

```json:metadata
{"files": ["internal/prompts/prompts.go", "internal/prompts/templates/plan_findings_only.tmpl", "internal/prompts/prompts_test.go", "internal/prompts/testdata/plan_findings_only_with_prior_findings.golden"], "verifyCommand": "go test -race ./internal/prompts/...", "acceptanceCriteria": ["PlanInput has PriorPlanFindings", "existing plan_findings_only goldens are unchanged", "the section is in UserSuffix before Output and UserPrefix is unchanged", "each finding renders on one line", "the section's three instructions are present", "check-tree.sh 05 prints tree OK"], "modelTier": "mechanical"}
```

### Task 6: `validate_plan` rounds — re-review only what changed

**Goal:** `validate_plan` takes `plan_run_id`; a round on a live run keeps the id, reviews only the tasks whose text changed, carries the rest, re-runs the plan-level pass only when the plan text changed, makes no reviewer call when nothing changed, and reports what it did.

**Files:**
- Create: `internal/mcpsrv/plan_round.go`
- Modify: `internal/mcpsrv/handlers.go`
- Modify: `internal/mcpsrv/review_error.go`
- Modify: `internal/mcpsrv/run_snapshots.go`
- Modify: `internal/mcpsrv/plan_cache.go`
- Modify: `internal/mcpsrv/summary.go`
- Modify: `internal/verdict/plan.go`
- Modify: `internal/stats/event.go`
- Modify: `CHANGELOG.md`
- Test (create): `internal/mcpsrv/handlers_plan_rounds_test.go`

**Acceptance Criteria:**
- [ ] `ValidatePlanArgs.PlanRunID` (`plan_run_id`) is described in the tool schema; the tool description says to pass the previous round's id after editing the plan.
- [ ] An unchanged round on a live run makes **no** reviewer call (asserted on the scripted reviewer's call count), for a `warn` result as well as a `pass`, with the pass cache expired; it returns the same id, verdict, findings, tasks and `next_action`, `review_scope` `{revision: 2, tasks_reviewed: 0, tasks_carried: N, plan_level_reviewed: false}`, and the run's revision is 2.
- [ ] With one task edited, the round makes exactly one plan-level call and one chunk call; the chunk prompt lists only the edited task; the plan-level prompt contains `## Earlier plan-level findings` with the earlier finding; unchanged tasks keep their results; every task's `task_index` is its plan position.
- [ ] A change outside every task re-runs only the plan-level pass; an inserted task is the only task reviewed and the run takes the new task list.
- [ ] A `controller_rulings` entry added on an unchanged round waives the carried finding with no reviewer call, and the next round without the ruling returns the finding again.
- [ ] Changed `project_knowledge` reviews every task under the same id and shows the plan-level pass no earlier findings.
- [ ] An unknown id reviews the whole plan under a new run and adds one minor `plan_run_id` finding, appended after the ladder, naming both ids.
- [ ] A truncated round returns a partial result under the same id with the carried tasks, `plan_verdict: warn` and a major `reviewer_response` finding that counts the tasks with no result; it sets no `review_scope`, leaves the revision and the stored review unchanged, and a retry reviews the changed task again. A truncated plan-level pass does not return the earlier round's verdict or `next_action`. A complete task result inside a truncated chunk is not filed under any task.
- [ ] A round with no reviewer call writes a header whose `plan_call` equals the previous round's; `review_scope.revision` equals the run's stored revision.
- [ ] A pass-cache hit reports `tasks_reviewed: 0`; a cache entry whose run a later round revised to another plan text is not served.
- [ ] Each round writes a `runs.jsonl` header under the same run hash with `revision` and `tasks_carried`, and an `events.jsonl` event with `tasks_carried`; the summary block has the `review_scope:` line.
- [ ] `reviewPlanChunked` behaves as before (its tests pass unmodified).

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/verdict/...` → `ok` for each; `check-tree.sh 06` prints `tree OK (06)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/06-validate-plan-rounds.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/mcpsrv/...`
Expected: FAIL to build — `unknown field PlanRunID in struct literal of type ValidatePlanArgs`, `undefined: verdict.PlanReviewScope`, `h.newPlanRound undefined`.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/06-validate-plan-rounds.impl.patch
```

- [x] **Step 4: Run the suites**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: every package `ok`.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 06`
Expected: `tree OK (06)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat: validate_plan rounds re-review only the tasks that changed"
```

```json:metadata
{"files": ["internal/mcpsrv/plan_round.go", "internal/mcpsrv/handlers.go", "internal/mcpsrv/review_error.go", "internal/mcpsrv/run_snapshots.go", "internal/mcpsrv/plan_cache.go", "internal/mcpsrv/summary.go", "internal/verdict/plan.go", "internal/stats/event.go", "CHANGELOG.md", "internal/mcpsrv/handlers_plan_rounds_test.go"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/stats/... ./internal/verdict/...", "acceptanceCriteria": ["plan_run_id is an argument described in the schema", "an unchanged round makes no reviewer call for any verdict", "one edited task costs one plan-level call and one chunk call", "a change outside tasks re-runs only the plan-level pass and an inserted task is the only one reviewed", "a ruling applies to a carried finding with no reviewer call", "changed inputs review everything under the same id", "an unknown id falls back to a full review with one advisory", "a truncated round keeps the id and the stored review", "the pass cache reports nothing reviewed and refuses a stale entry", "headers and events carry revision and tasks_carried", "check-tree.sh 06 prints tree OK"], "modelTier": "frontier", "tierReason": "The reviewers of this task, not its implementer, need the stronger model. The change is a cache keyed on task text in front of a gate: a wrong carry returns a stored pass for plan text the reviewer never saw, so a controller dispatches an unreviewed plan, and a wrong merge attributes one task's findings to another. Judging it means reasoning about which inputs invalidate a carried result, about shared state handed between concurrent calls on the run store (the stored review must never be mutated), and about the truncation path leaving the run consistent. Those are the classes — cache invalidation at a gate, and concurrency — where the standard tier is measurably weaker."}
```

### Task 7: `validate_task_spec` attaches by title to the single live run

**Goal:** A `validate_task_spec` call that names no plan run is attached to the server's single live run when its title matches exactly one of that run's plan headings.

**Files:**
- Modify: `internal/planrun/planrun.go`
- Modify: `internal/mcpsrv/plan_run_rows.go`
- Modify: `internal/mcpsrv/task_spec_normalize.go`
- Modify: `internal/mcpsrv/handlers.go`
- Test (create): `internal/mcpsrv/task_spec_attach_by_title_test.go`
- Test: `internal/planrun/planrun_test.go`

**Acceptance Criteria:**
- [ ] `Store.SoleLiveByTitle(title)` returns the run's id only when exactly one run has been used within the TTL and exactly one of its headings matches the title (compared as `titleKey` compares them); it does not refresh `LastAccessed` and is safe on a nil store.
- [ ] With one live run and a matching title, the call attaches a row at the heading's index (whatever `task_index` it sent), the session carries the run's id, the verdict is unchanged, and one minor `plan_run_id` finding says the task was attached and names the run; the finding is not among the session's `PreFindings`.
- [ ] A title matching no heading, or two live runs, attaches nothing and returns the existing "this server holds a live plan run" advisory.
- [ ] A call that names a run behaves as before and gets no advisory.
- [ ] After two `validate_plan` rounds on one run, a `validate_task_spec` call with no id finds that run by title.
- [ ] The paths the run recorded for the matched task are dropped from the checklist for a call attached by title too.
- [ ] A truncated review attaches nothing and its advisory does not say it was attached.

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/planrun/...` → `ok` for each; `check-tree.sh 07` prints `tree OK (07)`

**Steps:**

- [x] **Step 1: Apply the tests**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/07-attach-by-title.tests.patch
```

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/planrun/... ./internal/mcpsrv/...`
Expected: FAIL — `SoleLiveByTitle undefined` in `planrun`; in `mcpsrv`, `TestValidateTaskSpec_AttachesByTitleToTheSingleLiveRun` and `TestValidateTaskSpec_ARevisedPlanStillHasOneRunToAttachTo` fail.

- [x] **Step 3: Apply the implementation**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/07-attach-by-title.impl.patch
```

- [x] **Step 4: Run the suites**

Run: `go build ./... && go test -race ./...`
Expected: every package `ok`.

- [x] **Step 5: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 07`
Expected: `tree OK (07)`.

- [x] **Step 6: Commit**

```bash
git commit -m "feat: validate_task_spec attaches by title to the single live plan run"
```

```json:metadata
{"files": ["internal/planrun/planrun.go", "internal/mcpsrv/plan_run_rows.go", "internal/mcpsrv/task_spec_normalize.go", "internal/mcpsrv/handlers.go", "internal/mcpsrv/task_spec_attach_by_title_test.go", "internal/planrun/planrun_test.go"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/planrun/...", "acceptanceCriteria": ["SoleLiveByTitle needs exactly one live run and exactly one matching heading", "a matching call attaches a row and gets one advisory that moves no verdict", "no match or two live runs attaches nothing", "a named run behaves as before", "a revised plan still has one run to attach to", "check-tree.sh 07 prints tree OK"], "modelTier": "mechanical"}
```

### Task 8: Protocol, README and changelog for rounds and attach-by-title

**Goal:** The controller protocol tells the controller to pass the previous round's `plan_run_id`, and the README and changelog describe rounds, `review_scope` and attach-by-title.

**Files:**
- Modify: `docs/protocol/controller.md`, `plugin/anti-tangent-protocol/protocol/controller.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Acceptance Criteria:**
- [ ] `docs/protocol/controller.md` §5.1 step 4 says to call `validate_plan` again with the last round's `plan_run_id`; the file is 15,904 bytes, under 16,000, and identical to its bundled copy; no section is renumbered.
- [ ] `README.md` has a "Rounds (v0.27.0+)" paragraph under `validate_plan`, mentions `review_scope` and attach-by-title in the tool list, and says a path in a task's own `**Files:**` section is left off the checklist.
- [ ] `CHANGELOG.md` `[0.27.0]` has a bullet for attach-by-title and one for the controller protocol change.
- [ ] `INTEGRATION.md` is unchanged.

**Verify:** `wc -c docs/protocol/*.md INTEGRATION.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && go test -race ./...` → every protocol part under 16000, `INTEGRATION.md` 1828, `diff` empty, every package `ok`; `check-tree.sh 08` prints `tree OK (08)`

**Steps:**

- [x] **Step 1: Apply the patch**

```bash
git apply --index docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/08-docs.impl.patch
```

- [x] **Step 2: Check the budgets and the bundle**

Run: `wc -c docs/protocol/*.md INTEGRATION.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol`
Expected: `15904 docs/protocol/controller.md`, `15983 docs/protocol/core.md`, `15879 docs/protocol/implementer.md`, `1828 INTEGRATION.md`; `diff` prints nothing.

- [x] **Step 3: Run every suite**

Run: `go test -race ./... && (cd gnome-topbar/daemon && go test -race ./...) && bash plugin/anti-tangent-guard/evals/run.sh | tail -1 && bash plugin/anti-tangent-guard/evals/fp-report.sh | tail -1 && bash examples/hooks/codescene-log_test.sh`
Expected: every package `ok`; `198 passed, 0 failed`; `FALSE POSITIVES: 0`; `OK`.

- [x] **Step 4: Check the tree**

Run: `bash docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost/check-tree.sh 08`
Expected: `tree OK (08)`.

- [x] **Step 5: Commit**

```bash
git commit -m "docs: validate_plan rounds, review_scope and attach-by-title"
```

```json:metadata
{"files": ["docs/protocol/controller.md", "plugin/anti-tangent-protocol/protocol/controller.md", "README.md", "CHANGELOG.md"], "verifyCommand": "wc -c docs/protocol/*.md INTEGRATION.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && go test -race ./...", "acceptanceCriteria": ["controller.md step 4 names plan_run_id and the file is under 16000 bytes and in sync", "README documents rounds, review_scope, attach-by-title and the Files rule", "CHANGELOG has the two bullets", "INTEGRATION.md unchanged", "check-tree.sh 08 prints tree OK"], "modelTier": "mechanical"}
```
