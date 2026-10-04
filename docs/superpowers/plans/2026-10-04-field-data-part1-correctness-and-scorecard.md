# Field-Data Part 1: Correctness Review and Scorecard Inputs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `validate_completion` look for correctness and test-adequacy defects, and make the scorecard able to show whether that lowered the escape rate.

**Architecture:** Two reviewer categories (`correctness`, `test_adequacy`) join the per-task schema, and `post.tmpl` gains two sections that ask for them ahead of the comment and over-building checks. Task rows and run snapshots record per-category finding counts and diff size; the `scorecard` package normalises model ids when it builds cohorts, prefers a same-model baseline, and reports two correctness rates. `record_review_outcome` names the tasks it was given no implementer model for.

**Tech Stack:** Go 1.25, `text/template` prompts with golden files, stdlib-only `scorecard` package, `github.com/stretchr/testify` in `internal/` tests (plain `testing` in `scorecard/`).

**Spec:** `docs/superpowers/specs/2026-10-04-field-data-improvements-design.md` (§4 is this plan; §2 is the evidence). Parts 2 and 3 of that spec are **not** in this plan.

## Global Constraints

- **Branch.** This plan was written on `feature/field-data-improvements`. Implement it on a branch named `version/0.27.0`, cut from that branch's tip: CI requires the branch's `X.Y.Z` to match a `## [X.Y.Z] - YYYY-MM-DD` heading in `CHANGELOG.md`. No `version/*` branch above 0.25.0 was in flight on 2026-10-04 and 0.26.0 is released; re-check `git branch -r | grep version/` before cutting, and take the next free minor if 0.27.0 is taken. Do **not** edit `VERSION`; the release workflow bumps it. The merge commit into `main` carries `[minor]`.
- **Do not gate this plan with anti-tangent.** It changes the tool. Skip `validate_plan`, `validate_task_spec`, `check_progress` and `validate_completion` for every task here. Subagent-driven spec and code-quality review is the gate.
- **Changelog.** Task 1 creates `## [0.27.0] - 2026-10-04` in `CHANGELOG.md`. Every later task that changes user-visible behaviour adds its bullet under that heading's `### Added` or `### Changed`, in the same commit as the code.
- **Content-free records.** `runs.jsonl`, `outcomes.jsonl`, `scorecard.json` and `events.jsonl` hold counts, verdicts, model ids and anti-tangent's own category names. Nothing in this plan may add a task title, plan heading, file path, finding text or raw id to them.
- **Public repo.** No consumer ticket id, file name, plan title or task title from the field data appears in code, tests, docs or commit messages. Test fixtures use invented names.
- **The server stays advisory.** No change here rejects a call or blocks a caller. `record_review_outcome` reports a gap in its result, never as an error.
- **Additive wire changes only.** Every new JSON field is `omitempty` or zero-safe, so records written by 0.26.0 still decode and score. `scorecard/` imports only the standard library; the gnome-topbar daemon builds against it through a `replace` directive and must keep compiling.
- **Exact strings.** Categories `correctness`, `test_adequacy`. Snapshot keys `categories`, `lines_added`, `lines_removed`. Metric keys `correctness_escape_rate`, `correctness_flag_recall`, `lines_added_p50`. Result key `missing_implementer_models`. Counted criterion `plan_run_id`.
- **Protocol byte budgets (CI-enforced).** Each `docs/protocol/*.md` under 16,000 bytes; `INTEGRATION.md` under 2,000; `plugin/anti-tangent-protocol/protocol/` identical to `docs/protocol/`. `core.md` (15,973) and `implementer.md` (15,954) have no room: this plan edits neither. Only `outcome.md` (2,976) changes, and the bundle is resynced in the same commit.
- **Comments.** Comments explain non-obvious behaviour or an invariant. No task, issue, PR or version references, no "previously" / "no longer" / "this replaced". Applies to code comments in every file this plan touches.
- **Prompt goldens.** After editing a template run `go test ./internal/prompts/... -update`, then read `git diff internal/prompts/testdata/` before committing: only `post_*.golden` files may change.
- **Tests.** `go test -race ./...` from the repo root, and `cd gnome-topbar/daemon && go test -race ./...`. No test touches the network.

**User decisions (already made):**
- `check_progress` is triggered automatically from the `anti-tangent-guard` plugin (Part 2, not this plan).
- `codescene_not_run` stays verdict-driving.
- Agents circling back to fix findings is wanted; the target is findings that repeat without resolving.
- Lean guidance gets a measurement (code size per task; recorded in this plan) and teeth (Part 2).
- The improvements ship in parts; Part 1 is the correctness review plus the scorecard inputs.

---

### Task 1: `correctness` and `test_adequacy` reviewer categories

**Goal:** The per-task reviewer schema and parser accept two new finding categories, with the reviewer's severity preserved.

**Files:**
- Modify: `internal/verdict/verdict.go`
- Modify: `internal/verdict/schema.json`
- Modify: `internal/verdict/parser.go`
- Modify: `CHANGELOG.md`
- Test: `internal/verdict/parser_test.go`

**Acceptance Criteria:**
- [ ] `verdict.Parse` accepts a finding with `"category":"correctness"` and one with `"category":"test_adequacy"`, and returns each with the severity the reviewer gave (`major` stays `major`).
- [ ] The category enum in `internal/verdict/schema.json` contains `correctness` and `test_adequacy`; `plan_schema.json`, `tasks_only_schema.json`, `plan_findings_only_schema.json`, `prime_schema.json` and `extract_schema.json` are unchanged.
- [ ] `verdict.Parse` still rejects an unknown category with an error containing `invalid category`.
- [ ] `CHANGELOG.md` has a `## [0.27.0] - 2026-10-04` heading above `## [0.26.0]` with an `### Added` bullet for the two categories.

**Verify:** `go test -race ./internal/verdict/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing tests**

Append to `internal/verdict/parser_test.go`:

```go
func TestParse_CorrectnessAndTestAdequacy_AcceptedAndNotFloored(t *testing.T) {
	for _, cat := range []Category{CategoryCorrectness, CategoryTestAdequacy} {
		raw := []byte(`{
			"verdict":"warn",
			"findings":[{
				"severity":"major",
				"category":"` + string(cat) + `",
				"criterion":"returns 404 for an unknown id",
				"evidence":"handler.go:12 ignores the lookup error",
				"suggestion":"return the error to the caller",
				"same_as":null
			}],
			"next_action":"fix the defect"
		}`)
		r, err := Parse(raw)
		require.NoError(t, err, "%s must be a valid category", cat)
		require.Len(t, r.Findings, 1)
		require.Equal(t, cat, r.Findings[0].Category)
		require.Equal(t, SeverityMajor, r.Findings[0].Severity, "%s must not be floored to minor", cat)
	}
}

func TestSchema_ListsCorrectnessCategories(t *testing.T) {
	s := string(Schema())
	require.Contains(t, s, `"correctness"`)
	require.Contains(t, s, `"test_adequacy"`)
	for name, other := range map[string][]byte{
		"plan": PlanSchema(), "tasks_only": TasksOnlySchema(), "plan_findings_only": PlanFindingsOnlySchema(),
		"prime": PrimeSchema(), "extract": ExtractSchema(),
	} {
		require.NotContains(t, string(other), `"test_adequacy"`, "%s schema must not gain the category", name)
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/verdict/... -run 'CorrectnessAndTestAdequacy|ListsCorrectnessCategories'`
Expected: compile error, `undefined: CategoryCorrectness`.

- [ ] **Step 3: Add the constants**

In `internal/verdict/verdict.go`, directly after the `CategoryContradictedCodebaseClaim` line and before the `CategoryMalformedEvidence` comment, insert:

```go
	// CategoryCorrectness is emitted by the completion review for a defect the
	// submitted evidence shows: a wrong result, a dropped failure, an
	// unprotected shared write. CategoryTestAdequacy is its counterpart for a
	// test that would not fail if the behaviour it covers broke. Neither is in
	// applySeverityFloor's list: both report what the evidence shows, so the
	// reviewer's chosen severity is preserved.
	CategoryCorrectness  Category = "correctness"
	CategoryTestAdequacy Category = "test_adequacy"
```

- [ ] **Step 4: Accept them in the parser**

In `internal/verdict/parser.go`, in `validCategory`, change

```go
		CategoryContradictedCodebaseClaim,
		CategoryKBGap, CategoryAmbiguousPick, CategoryMissingIndexEntry,
```

to

```go
		CategoryContradictedCodebaseClaim,
		CategoryCorrectness, CategoryTestAdequacy,
		CategoryKBGap, CategoryAmbiguousPick, CategoryMissingIndexEntry,
```

- [ ] **Step 5: Add them to the per-task schema**

In `internal/verdict/schema.json`, change

```json
              "attestation_contradiction",
              "kb_gap",
```

to

```json
              "attestation_contradiction",
              "correctness",
              "test_adequacy",
              "kb_gap",
```

Do not edit any other `*_schema.json`.

- [ ] **Step 6: Run the package**

Run: `go test -race ./internal/verdict/...`
Expected: `ok`. If a schema-invariant test fails, read its message: it checks every schema's `required` list against `properties`, which this change does not alter.

- [ ] **Step 7: Create the changelog entry**

In `CHANGELOG.md`, insert above `## [0.26.0] - 2026-09-24`:

```markdown
## [0.27.0] - 2026-10-04

### Added

- `correctness` and `test_adequacy` finding categories for the per-task review tools. Neither is severity-floored, and a major one is an open code finding: `next_action` tells the implementer to fix and re-validate.

```

- [ ] **Step 8: Commit**

```bash
git add internal/verdict/verdict.go internal/verdict/parser.go internal/verdict/schema.json internal/verdict/parser_test.go CHANGELOG.md
git commit -m "feat(verdict): correctness and test_adequacy finding categories"
```

```json:metadata
{"files": ["internal/verdict/verdict.go", "internal/verdict/schema.json", "internal/verdict/parser.go", "internal/verdict/parser_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/verdict/...", "acceptanceCriteria": ["Parse accepts correctness and test_adequacy with the reviewer's severity preserved", "schema.json enum lists both; the five other schemas are unchanged", "an unknown category is still rejected with 'invalid category'", "CHANGELOG.md has a [0.27.0] heading with an Added bullet"], "modelTier": "mechanical"}
```

---

### Task 2: Completion prompt asks for correctness and test adequacy

**Goal:** `post.tmpl` tells the reviewer to look for defects and inadequate tests, before the comment and over-building sections, and lets such a finding be critical or major.

**Files:**
- Modify: `internal/prompts/templates/post.tmpl`
- Modify: `internal/prompts/testdata/post_basic.golden`, `post_with_codescene.golden`, `post_with_context_files.golden`, `post_with_exit_contracts.golden`, `post_with_exit_contracts_inferred.golden`, `post_with_stale_comment_hint.golden` (regenerated)
- Modify: `CHANGELOG.md`
- Test: `internal/prompts/prompts_test.go`

**Acceptance Criteria:**
- [ ] The rendered completion prompt contains the headings `### Correctness` and `### Test adequacy`, in that order, and both come before `### Comment hygiene`.
- [ ] The rendered prompt names `category: correctness` and `category: test_adequacy`, and contains the sentence fragment `Do not speculate about code that was not submitted`.
- [ ] The severity sentence admits the new categories: the rendered prompt contains ``OR for a `correctness` or `test_adequacy` finding that meets the severity bar in its own section below``, and still contains `left unaddressed by any of the provided evidence` and ``prefer `verdict: pass` with a `category: quality` finding``.
- [ ] `git diff --stat internal/prompts/testdata/` after `-update` lists only `post_*.golden` files; `pre_*`, `mid_*`, `plan_*`, `prime_*`, `extract_*`, `lean_*` and `worker_*` goldens are byte-identical.
- [ ] `CHANGELOG.md` `[0.27.0]` has a `### Changed` bullet describing the completion review.

**Verify:** `go test -race ./internal/prompts/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing test**

Append to `internal/prompts/prompts_test.go`:

```go
func TestRenderPost_AsksForCorrectnessBeforeCommentHygiene(t *testing.T) {
	out, err := RenderPost(PostInput{
		Spec:         sampleSpec(),
		Summary:      "Implemented the handler.",
		FinalDiff:    "--- a/h.go\n+++ b/h.go\n@@ -1 +1 @@\n-old\n+new\n",
		TestEvidence: "go test ./... PASS",
	})
	require.NoError(t, err)
	correctness := strings.Index(out.User, "### Correctness")
	tests := strings.Index(out.User, "### Test adequacy")
	comments := strings.Index(out.User, "### Comment hygiene")
	require.NotEqual(t, -1, correctness, "prompt must have a Correctness section")
	require.NotEqual(t, -1, tests, "prompt must have a Test adequacy section")
	require.NotEqual(t, -1, comments)
	assert.Less(t, correctness, tests)
	assert.Less(t, tests, comments)
	assert.Contains(t, out.User, "`category: correctness`")
	assert.Contains(t, out.User, "`category: test_adequacy`")
	assert.Contains(t, out.User, "Do not speculate about code that was not submitted")
	assert.Contains(t, out.User, "OR for a `correctness` or `test_adequacy` finding that meets the severity bar in its own section below")
}
```

`strings` is already imported by this test file; if the compiler says otherwise, add it to the import block.

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/prompts/... -run TestRenderPost_AsksForCorrectnessBeforeCommentHygiene`
Expected: FAIL, `prompt must have a Correctness section`.

- [ ] **Step 3: Widen the severity sentence**

In `internal/prompts/templates/post.tmpl`, replace this paragraph (one line in the file):

```text
When the provided evidence addresses every AC and the implementer's narrative is internally consistent with it, prefer `verdict: pass` with a `category: quality` finding for nit-level concerns over `verdict: fail`. Reserve `severity: critical` and `severity: major` for evidence that affirmatively contradicts an AC, OR for an AC that is left unaddressed by any of the provided evidence. The bias toward `pass` applies only when every AC has been addressed — not when evidence is absent for an AC.
```

with:

```text
When the provided evidence addresses every AC and the implementer's narrative is internally consistent with it, prefer `verdict: pass` with a `category: quality` finding for nit-level concerns over `verdict: fail`. Reserve `severity: critical` and `severity: major` for evidence that affirmatively contradicts an AC, OR for an AC that is left unaddressed by any of the provided evidence, OR for a `correctness` or `test_adequacy` finding that meets the severity bar in its own section below. The bias toward `pass` applies only when every AC has been addressed — not when evidence is absent for an AC, and not when the evidence shows a defect.
```

- [ ] **Step 4: Add the two sections**

In the same file, find the line `### Comment hygiene` and insert the following block immediately above it, leaving one blank line between the block and `### Comment hygiene`:

```text
### Correctness

The acceptance-criteria walk above asks whether each criterion is met. This section asks a different question: is the code right? A criterion can be met by code that is still wrong. Read what this change adds or modifies and look for defects, whether or not a criterion mentions them:

- a wrong result for an input the task covers: empty, zero, negative, duplicate, missing, or at a boundary;
- an error, a failure return or a nil / None / undefined value that is dropped, or handled so that the caller carries on with bad data;
- state shared between threads, goroutines or requests that this change reads or writes without the protection the surrounding code uses;
- a file, lock, connection or transaction that is not released on every path;
- a caller and a callee that this change updates on one side only;
- behaviour the summary claims that the code shown does not have.

Report only what the evidence shows. Quote the offending lines in `evidence` with their path, and with a line number when a diff hunk header gives one, never an invented one; then state the input or sequence that triggers the defect and the wrong outcome it produces. Give the fix in `suggestion`. Do not speculate about code that was not submitted: "this may break callers elsewhere" is not a finding, and neither is a defect in code this change does not touch.

Emit each defect as its own finding with `category: correctness`. Set `criterion` to the verbatim AC text when the defect breaks that criterion, and to `correctness` otherwise. Severity: `critical` for data loss, a security hole, or a crash on the main path; `major` for a wrong result or an unhandled failure on a path the task's Goal or acceptance criteria cover; `minor` for a defect on a path the task does not exercise.

### Test adequacy

Apply this when the change adds or modifies tests, or when test evidence was provided. For each acceptance criterion that describes behaviour, ask whether a test in the evidence would fail if that behaviour were broken. Flag:

- a test that asserts nothing about the behaviour: it only checks that nothing was thrown, only checks a mock it configured itself, or restates the implementation;
- a criterion that names a failure or boundary case and is tested only on the happy path;
- an individual test the evidence shows was skipped or deselected.

Emit `category: test_adequacy` with `criterion` set to the verbatim AC text. Severity: `major` when a criterion has no test that would catch its breakage and the task's acceptance criteria or Verification call for tests; `minor` otherwise. Do not flag tests the task does not call for, and do not use this category for evidence that is merely absent: that is `insufficient_evidence`.

```

The block contains no template actions (`{{`), so it renders verbatim.

- [ ] **Step 5: Run the new test**

Run: `go test ./internal/prompts/... -run TestRenderPost_AsksForCorrectnessBeforeCommentHygiene`
Expected: PASS.

- [ ] **Step 6: Regenerate the goldens and read the diff**

```bash
go test ./internal/prompts/... -update
git diff --stat internal/prompts/testdata/
```

Expected: exactly the six `post_*.golden` files listed under **Files** change, each gaining the two sections and the widened sentence. If any other golden changed, the edit landed in the wrong template: revert and redo Steps 3 and 4.

- [ ] **Step 7: Run the package**

Run: `go test -race ./internal/prompts/...`
Expected: `ok`. `TestRenderPost_IncludesEvidenceToleranceGuidance` must still pass: it pins two phrases of the paragraph edited in Step 3, and both were kept.

- [ ] **Step 8: Changelog**

Under `## [0.27.0]` in `CHANGELOG.md`, add after the `### Added` list:

```markdown
### Changed

- `validate_completion` now asks the reviewer to look for defects in the submitted change (wrong results on boundary inputs, dropped failures, unprotected shared state, unreleased resources, one-sided caller/callee changes) and for tests that would not fail if the behaviour broke, reported as `correctness` and `test_adequacy`. A finding of either kind can be critical or major, so it moves the verdict; the reviewer is told to report only what the submitted evidence shows.

```

- [ ] **Step 9: Commit**

```bash
git add internal/prompts/templates/post.tmpl internal/prompts/testdata internal/prompts/prompts_test.go CHANGELOG.md
git commit -m "feat(prompts): completion review asks for correctness and test adequacy"
```

```json:metadata
{"files": ["internal/prompts/templates/post.tmpl", "internal/prompts/prompts_test.go", "internal/prompts/testdata/post_basic.golden", "internal/prompts/testdata/post_with_codescene.golden", "internal/prompts/testdata/post_with_context_files.golden", "internal/prompts/testdata/post_with_exit_contracts.golden", "internal/prompts/testdata/post_with_exit_contracts_inferred.golden", "internal/prompts/testdata/post_with_stale_comment_hint.golden", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/prompts/...", "acceptanceCriteria": ["### Correctness then ### Test adequacy, both before ### Comment hygiene", "prompt names both categories and forbids speculation about unsubmitted code", "severity sentence admits the new categories and keeps the two pinned phrases", "only post_*.golden files change", "CHANGELOG [0.27.0] has a Changed bullet"], "modelTier": "mechanical"}
```

---

### Task 3: Record finding categories and diff size per task

**Goal:** Every `validate_completion` call adds its per-category finding counts to the task's row and records the size of its diff, and both reach `runs.jsonl`.

**Files:**
- Modify: `internal/planrun/planrun.go` (`TaskRow`, `cloneRow`)
- Modify: `scorecard/records.go` (`TaskSnapshot`)
- Modify: `internal/mcpsrv/plan_run_rows.go` (`completionRowUpdate`, `recordCompletionRow`, `recordLightweightCompletionRow`, new `diffLineCounts`)
- Modify: `internal/mcpsrv/run_snapshots.go` (`snapshotRow`)
- Modify: `internal/mcpsrv/handlers.go` (the one `recordCompletionRow` call)
- Modify: `CHANGELOG.md`
- Test: `internal/mcpsrv/completion_row_test.go` (create)

**Acceptance Criteria:**
- [ ] `planrun.TaskRow` and `scorecard.TaskSnapshot` each have `Categories map[string]int` (`json:"categories,omitempty"`), `LinesAdded int` (`json:"lines_added,omitempty"`) and `LinesRemoved int` (`json:"lines_removed,omitempty"`).
- [ ] Applying `completionRowUpdate` for two calls to the same row sums `Categories` across the calls, while `LinesAdded` / `LinesRemoved` hold the second call's counts only.
- [ ] `diffLineCounts` counts lines starting `+` and `-`, does not count a `--- ` line that is immediately followed by a `+++ ` line nor that `+++ ` line, does count a removed line whose content starts with `-- `, and returns `0, 0` for an empty string.
- [ ] `cloneRow` returns a row whose `Categories` map is a copy: mutating the clone's map leaves the original unchanged.
- [ ] `snapshotRow` writes the three fields, and a row with no categories and no diff serialises without the three keys.
- [ ] `go build ./...` and `cd gnome-topbar/daemon && go build ./...` both succeed.

**Verify:** `go test -race ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing tests**

Create `internal/mcpsrv/completion_row_test.go`:

```go
package mcpsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func TestDiffLineCounts(t *testing.T) {
	diff := "diff --git a/q.sql b/q.sql\n" +
		"--- a/q.sql\n" +
		"+++ b/q.sql\n" +
		"@@ -1,3 +1,3 @@\n" +
		" select 1;\n" +
		"--- an old sql comment\n" +
		"-select 2;\n" +
		"+select 3;\n" +
		"+select 4;\n"
	added, removed := diffLineCounts(diff)
	assert.Equal(t, 2, added)
	assert.Equal(t, 2, removed, "a removed line whose content starts with '-- ' is a removal, not a file header")

	added, removed = diffLineCounts("")
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, removed)
}

func TestCompletionRowUpdate_SumsCategoriesAndKeepsLatestDiffSize(t *testing.T) {
	first := Envelope{Verdict: "warn", Findings: []verdict.Finding{
		{Severity: verdict.SeverityMajor, Category: verdict.CategoryCorrectness, Criterion: "c", Evidence: "e", Suggestion: "s"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "c", Evidence: "e", Suggestion: "s"},
	}}
	second := Envelope{Verdict: "pass", Findings: []verdict.Finding{
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "c", Evidence: "e", Suggestion: "s"},
	}}
	var row planrun.TaskRow
	completionRowUpdate(first, nil, "--- a/x\n+++ b/x\n@@ -1 +1,3 @@\n-a\n+b\n+c\n+d\n")(&row)
	completionRowUpdate(second, nil, "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n")(&row)

	assert.Equal(t, map[string]int{"correctness": 1, "quality": 2}, row.Categories)
	assert.Equal(t, 1, row.LinesAdded)
	assert.Equal(t, 1, row.LinesRemoved)
	assert.Equal(t, "pass", row.PostVerdict)
}

func TestCompletionRowUpdate_NoFindingsNoDiffLeavesFieldsEmpty(t *testing.T) {
	var row planrun.TaskRow
	completionRowUpdate(Envelope{Verdict: "pass"}, nil, "")(&row)
	assert.Nil(t, row.Categories)
	assert.Zero(t, row.LinesAdded)
	assert.Zero(t, row.LinesRemoved)
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/mcpsrv/... -run 'DiffLineCounts|CompletionRowUpdate'`
Expected: compile errors: `undefined: diffLineCounts`, too many arguments to `completionRowUpdate`.

- [ ] **Step 3: Add the row fields**

In `internal/planrun/planrun.go`, in `type TaskRow struct`, after the `CallsDropped` field add:

```go
	// Categories counts, per finding category, the findings every
	// validate_completion call on the task returned, summed over the calls.
	Categories map[string]int `json:"categories,omitempty"`
	// LinesAdded and LinesRemoved are the size of the latest
	// validate_completion call's final_diff. Both are zero when that call
	// sent no diff.
	LinesAdded   int `json:"lines_added,omitempty"`
	LinesRemoved int `json:"lines_removed,omitempty"`
```

In the same file, in `cloneRow`, add one line after `row.Severity = cloneIntMap(row.Severity)`:

```go
	row.Categories = cloneIntMap(row.Categories)
```

and widen its doc comment's first sentence to name the new map:

```go
// cloneRow deep-copies row: Severity, Categories and Codescene would
// otherwise still alias the live row's maps and digest.
```

- [ ] **Step 4: Add the snapshot fields**

In `scorecard/records.go`, in `type TaskSnapshot struct`, after `CallsDropped` add:

```go
	// Categories counts the findings the task's validate_completion calls
	// returned, per finding category, summed over the calls.
	Categories   map[string]int `json:"categories,omitempty"`
	LinesAdded   int            `json:"lines_added,omitempty"`
	LinesRemoved int            `json:"lines_removed,omitempty"`
```

- [ ] **Step 5: Count the diff and write the row**

In `internal/mcpsrv/plan_run_rows.go`, add `"strings"` to the import block if it is not there, and replace the whole `completionRowUpdate` function (and its doc comment) with:

```go
// diffLineCounts returns how many lines a unified diff adds and removes. A
// "--- " line counts as a file header only when a "+++ " line follows it
// directly, so a removed line whose own text begins with "-- " is still
// counted as a removal.
func diffLineCounts(diff string) (added, removed int) {
	if diff == "" {
		return 0, 0
	}
	lines := strings.Split(diff, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ") {
			i++
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

// completionRowUpdate is the plan-run row write for one validate_completion
// result. finalDiff is the diff the call submitted, or "" when it sent none.
func completionRowUpdate(env Envelope, cs *codescene.Digest, finalDiff string) func(*planrun.TaskRow) {
	sev, cats, _, _ := stats.CountFindings(env.Findings)
	added, removed := diffLineCounts(finalDiff)
	state := planrun.StateMissing
	if cs != nil {
		if cs.Ran {
			state = planrun.StateRan
		} else {
			state = planrun.StateSkipped
		}
	}
	completedAt := time.Now().UTC()
	call := callFromEnvelope("validate_completion", env)
	return func(row *planrun.TaskRow) {
		row.PostVerdict = env.Verdict
		row.Severity = sev
		for c, n := range cats {
			if row.Categories == nil {
				row.Categories = map[string]int{}
			}
			row.Categories[c] += n
		}
		row.LinesAdded, row.LinesRemoved = added, removed
		row.SubmissionOnly = env.SubmissionDefectOnly
		row.Codescene = cs
		row.CodesceneState = state
		row.CompletedAt = completedAt
		row.Waived = len(env.WaivedFindings)
		row.Escalated = row.Escalated || env.Escalate
		row.AppendCall(call)
	}
}
```

In the same file change the two callers. `recordCompletionRow` gains a parameter:

```go
func (h *handlers) recordCompletionRow(sess *session.Session, env Envelope, cs *codescene.Digest, finalDiff string) {
	if sess.PlanRunID == "" {
		return
	}
	if row, ok := h.deps.PlanRuns.UpdateRow(sess.PlanRunID, sess.ID, completionRowUpdate(env, cs, finalDiff)); ok {
```

(the rest of the function is unchanged), and in `recordLightweightCompletionRow` change

```go
	if row, ok := h.deps.PlanRuns.UpsertLite(args.PlanRunID, ref, completionRowUpdate(env, args.Codescene)); ok {
```

to

```go
	if row, ok := h.deps.PlanRuns.UpsertLite(args.PlanRunID, ref, completionRowUpdate(env, args.Codescene, args.FinalDiff)); ok {
```

- [ ] **Step 6: Pass the diff from the handler**

In `internal/mcpsrv/handlers.go`, in `ValidateCompletion`, change

```go
		h.recordCompletionRow(sess, env, args.Codescene)
```

to

```go
		h.recordCompletionRow(sess, env, args.Codescene, args.FinalDiff)
```

`args.FinalDiff` already holds the content of `final_diff_path` at this point: the handler resolves the path into it before the review runs.

- [ ] **Step 7: Copy the fields into the snapshot**

In `internal/mcpsrv/run_snapshots.go`, in `snapshotRow`, inside the `&scorecard.TaskSnapshot{...}` literal add after `CallsDropped:   row.CallsDropped,`:

```go
			Categories:     row.Categories,
			LinesAdded:     row.LinesAdded,
			LinesRemoved:   row.LinesRemoved,
```

- [ ] **Step 8: Find any other caller**

Run: `grep -rn "completionRowUpdate(\|recordCompletionRow(" internal/`
Expected: only the definitions, the three call sites edited above, and the new test. Fix any other caller the same way (pass the call's diff, or `""` in a test that has none).

- [ ] **Step 9: Run the tests**

Run: `go test -race ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go build ./...)`
Expected: `ok` for the three packages and a silent build.

- [ ] **Step 10: Changelog**

Under `## [0.27.0]` → `### Added` in `CHANGELOG.md`:

```markdown
- `runs.jsonl` task snapshots carry `categories` (findings per category, summed over the task's `validate_completion` calls) and `lines_added` / `lines_removed` (the size of the latest call's `final_diff`; absent when the call sent no diff). `plan-runs.jsonl` rows carry the same three fields.
```

- [ ] **Step 11: Commit**

```bash
git add internal/planrun/planrun.go scorecard/records.go internal/mcpsrv/plan_run_rows.go internal/mcpsrv/run_snapshots.go internal/mcpsrv/handlers.go internal/mcpsrv/completion_row_test.go CHANGELOG.md
git commit -m "feat(stats): record finding categories and diff size per task"
```

```json:metadata
{"files": ["internal/planrun/planrun.go", "scorecard/records.go", "internal/mcpsrv/plan_run_rows.go", "internal/mcpsrv/run_snapshots.go", "internal/mcpsrv/handlers.go", "internal/mcpsrv/completion_row_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/...", "acceptanceCriteria": ["TaskRow and TaskSnapshot have categories, lines_added, lines_removed with omitempty tags", "categories sum across calls; line counts are the latest call's", "diffLineCounts skips real file headers and counts a removed '-- ' line", "cloneRow copies the Categories map", "snapshotRow writes the three fields", "root and daemon modules build"], "modelTier": "standard"}
```

---

### Task 4: Normalise model ids in cohorts and prefer a same-model baseline

**Goal:** A dated and an undated id of the same model score as one cohort, and a cohort's regression baseline is the earlier cohort with the same review and implementer models when one exists.

**Files:**
- Modify: `scorecard/records.go` (new `NormalizeModel`)
- Modify: `scorecard/assemble.go` (`reviewModel`, `implementerModel`)
- Modify: `scorecard/toolmodel.go` (`tmCtx.acc`)
- Modify: `scorecard/compute.go` (`assignRegression`)
- Modify: `CHANGELOG.md`
- Test: `scorecard/normalize_test.go` (create)

**Acceptance Criteria:**
- [ ] `NormalizeModel` returns `anthropic:claude-haiku-4-5` for `anthropic:claude-haiku-4-5-20251001`, for `  Anthropic:Claude-Haiku-4-5 ` and for `anthropic:claude-haiku-4-5`; leaves `anthropic:claude-sonnet-5-5` and `openai:gpt-5.6-terra` unchanged; and returns `""` for `""`.
- [ ] Two runs whose outcomes name `anthropic:claude-haiku-4-5` and `anthropic:claude-haiku-4-5-20251001` produce one group in `Scorecard.Cohorts`, with `runs == 2`.
- [ ] Two tasks reviewed by `openai:gpt-x` and `openai:gpt-x-20260101` produce one `ByReviewModel` group and one `validate_completion` row in `ByToolModel`.
- [ ] With three time-disjoint cohorts in order (A: implementer `impl-a`, version `0.26.0`; B: implementer `impl-b`, version `0.26.0`; C: implementer `impl-a`, version `0.27.0`), C's `Baseline.ImplementerModel` is `impl-a` and its `ServerVersion` is `0.26.0`, although B ended later than A.
- [ ] When no earlier cohort shares both models, the baseline is the most recently ended earlier cohort, as before: the existing `TestRegressionFlag` passes unmodified.
- [ ] The stored records are not rewritten: no code in `internal/stats` or `internal/mcpsrv` changes in this task.

**Verify:** `go test -race ./scorecard/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing tests**

Create `scorecard/normalize_test.go`:

```go
package scorecard

import (
	"testing"
	"time"
)

func TestNormalizeModel(t *testing.T) {
	cases := map[string]string{
		"anthropic:claude-haiku-4-5-20251001": "anthropic:claude-haiku-4-5",
		"  Anthropic:Claude-Haiku-4-5 ":       "anthropic:claude-haiku-4-5",
		"anthropic:claude-haiku-4-5":          "anthropic:claude-haiku-4-5",
		"anthropic:claude-sonnet-5-5":         "anthropic:claude-sonnet-5-5",
		"openai:gpt-5.6-terra":                "openai:gpt-5.6-terra",
		"":                                    "",
	}
	for in, want := range cases {
		if got := NormalizeModel(in); got != want {
			t.Errorf("NormalizeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCohortsMergeDatedAndUndatedImplementerModel(t *testing.T) {
	lines := []RunLine{
		taskLine("r1", t0, 1, "pass", completion("m", "pass")),
		taskLine("r2", t0.Add(time.Minute), 1, "pass", completion("m", "pass")),
	}
	o1 := outcome("r1", SourceFinalReview, t0.Add(time.Hour))
	o1.ImplementerModels = []ImplementerModel{{TaskIndex: 1, Model: "anthropic:claude-haiku-4-5"}}
	o2 := outcome("r2", SourceFinalReview, t0.Add(time.Hour))
	o2.ImplementerModels = []ImplementerModel{{TaskIndex: 1, Model: "anthropic:claude-haiku-4-5-20251001"}}
	g := only(t, Compute(lines, []OutcomeLine{o1, o2}, Options{}).Cohorts, SourceFinalReview)
	if g.Runs != 2 || g.Key.ImplementerModel != "anthropic:claude-haiku-4-5" {
		t.Fatalf("cohort = %+v runs=%d", g.Key, g.Runs)
	}
}

func TestReviewModelIsNormalisedInEveryView(t *testing.T) {
	lines := []RunLine{
		taskLine("r1", t0, 1, "pass", completion("openai:gpt-x", "pass")),
		taskLine("r1", t0, 2, "pass", completion("openai:gpt-x-20260101", "pass")),
	}
	sc := Compute(lines, []OutcomeLine{outcome("r1", SourceFinalReview, t0.Add(time.Hour))}, Options{})
	g := only(t, sc.ByReviewModel, SourceFinalReview)
	if g.Key.ReviewModel != "openai:gpt-x" || g.Tasks != 2 {
		t.Fatalf("by review model = %+v tasks=%d", g.Key, g.Tasks)
	}
	rows := 0
	for _, r := range sc.ByToolModel {
		if r.Tool == "validate_completion" {
			rows++
			if r.Model != "openai:gpt-x" || r.Calls != 2 {
				t.Fatalf("tool-model row = %+v", r)
			}
		}
	}
	if rows != 1 {
		t.Fatalf("want 1 validate_completion row, got %d", rows)
	}
}

// versionedCohort builds n one-task runs for one implementer model on one
// server version, a minute apart from start.
func versionedCohort(prefix, version, impl string, start time.Time, n int) ([]RunLine, []OutcomeLine) {
	var lines []RunLine
	var outs []OutcomeLine
	for i := 0; i < n; i++ {
		h := prefix + string(rune('a'+i))
		ts := start.Add(time.Duration(i) * time.Minute)
		l := taskLine(h, ts, 1, "pass", completion("m", "pass"))
		l.ServerVersion = version
		lines = append(lines, l)
		o := outcome(h, SourceFinalReview, ts)
		o.ImplementerModels = []ImplementerModel{{TaskIndex: 1, Model: impl}}
		outs = append(outs, o)
	}
	return lines, outs
}

func TestBaselinePrefersSameModels(t *testing.T) {
	la, oa := versionedCohort("a", "0.26.0", "impl-a", t0, 3)
	lb, ob := versionedCohort("b", "0.26.0", "impl-b", t0.Add(24*time.Hour), 3)
	lc, oc := versionedCohort("c", "0.27.0", "impl-a", t0.Add(48*time.Hour), 3)
	lines := append(append(la, lb...), lc...)
	outs := append(append(oa, ob...), oc...)
	for _, g := range Compute(lines, outs, Options{MinRuns: 1}).Cohorts {
		if g.Key.ServerVersion != "0.27.0" {
			continue
		}
		if g.Baseline == nil || g.Baseline.ImplementerModel != "impl-a" || g.Baseline.ServerVersion != "0.26.0" {
			t.Fatalf("baseline = %+v, want the impl-a 0.26.0 cohort", g.Baseline)
		}
		return
	}
	t.Fatal("no 0.27.0 cohort")
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./scorecard/... -run 'NormalizeModel|CohortsMerge|ReviewModelIsNormalised|BaselinePrefers'`
Expected: compile error, `undefined: NormalizeModel`.

- [ ] **Step 3: Add `NormalizeModel`**

In `scorecard/records.go`, add `"regexp"` to the import block and add after `NormalizeCategory`:

```go
var modelDateSuffix = regexp.MustCompile(`-\d{8}$`)

// NormalizeModel is the form a model id takes in every cohort key:
// lower-cased, trimmed, and without one trailing -YYYYMMDD date stamp, so a
// dated id and its undated alias land in the same cohort. It is applied when
// records are read, never when they are written, so records stored before it
// existed group the same way as new ones.
func NormalizeModel(s string) string {
	return modelDateSuffix.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "")
}
```

- [ ] **Step 4: Use it for the review and implementer models**

In `scorecard/assemble.go`, replace `reviewModel` and `implementerModel` with:

```go
func (t *task) reviewModel() string {
	if c, ok := t.latestCall("validate_completion"); ok {
		if m := NormalizeModel(c.Model); m != "" {
			return m
		}
	}
	return "unknown"
}
```

```go
// implementerModel prefers the final review's report, because the controller
// that files it is the one that dispatched each task.
func (r *run) implementerModel(index int) string {
	for _, src := range Sources {
		for _, m := range r.outcomes[src].ImplementerModels {
			if m.TaskIndex != index {
				continue
			}
			if model := NormalizeModel(m.Model); model != "" {
				return model
			}
		}
	}
	return "unknown"
}
```

- [ ] **Step 5: Use it for tool-model rows**

In `scorecard/toolmodel.go`, find the method `func (c *tmCtx) acc(tool, model string) *tmAcc` and add as its first statement:

```go
	model = NormalizeModel(model)
```

Every row is keyed through this accessor, so no other line in the file changes.

- [ ] **Step 6: Prefer a same-model baseline**

In `scorecard/compute.go`, replace the body of the candidate loop in `assignRegression`. Change

```go
		var base *Group
		for j := range groups {
			c := &groups[j]
			if j == i || c.Source != g.Source || c.Publisher != g.Publisher || !c.last.Before(g.first) {
				continue
			}
			if base == nil || c.last.After(base.last) {
				base = c
			}
		}
```

to

```go
		var base, sameModels *Group
		for j := range groups {
			c := &groups[j]
			if j == i || c.Source != g.Source || c.Publisher != g.Publisher || !c.last.Before(g.first) {
				continue
			}
			if base == nil || c.last.After(base.last) {
				base = c
			}
			if c.Key.ReviewModel == g.Key.ReviewModel && c.Key.ImplementerModel == g.Key.ImplementerModel &&
				(sameModels == nil || c.last.After(sameModels.last)) {
				sameModels = c
			}
		}
		if sameModels != nil {
			base = sameModels
		}
```

and replace the function's doc comment with:

```go
// assignRegression compares each group with its baseline. Candidates are the
// groups of the same source and publisher whose last scored task came before
// this group's first. Among them, the most recent one reviewed and implemented
// by the same models is the baseline, so a release is measured against the
// same models on the release before; when no candidate shares both models,
// the most recent candidate of any key is used. groups arrives sorted, and the
// strict After below keeps the first-sorted group on a tie. Only a disjoint
// interval counts as a regression, and only once both sides have minRuns runs.
```

- [ ] **Step 7: Run the package**

Run: `go test -race ./scorecard/...`
Expected: `ok`, including the unmodified `TestRegressionFlag` and `TestCohortKeyCarriesVersionAndImplementer`.

- [ ] **Step 8: Changelog**

Under `## [0.27.0]` → `### Changed` in `CHANGELOG.md`:

```markdown
- Scorecard cohorts key on a normalised model id: lower-cased, trimmed, and without a trailing `-YYYYMMDD` date stamp, so `claude-haiku-4-5` and `claude-haiku-4-5-20251001` are one cohort. Stored records are unchanged; the normalisation is applied when they are read.
- A cohort's regression baseline is now the most recent earlier cohort with the same review model and implementer model, falling back to the most recent earlier cohort of any key when there is none.
```

- [ ] **Step 9: Commit**

```bash
git add scorecard/records.go scorecard/assemble.go scorecard/toolmodel.go scorecard/compute.go scorecard/normalize_test.go CHANGELOG.md
git commit -m "feat(scorecard): normalise model ids and prefer a same-model baseline"
```

```json:metadata
{"files": ["scorecard/records.go", "scorecard/assemble.go", "scorecard/toolmodel.go", "scorecard/compute.go", "scorecard/normalize_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./scorecard/...", "acceptanceCriteria": ["NormalizeModel strips one trailing -YYYYMMDD, lower-cases and trims", "dated and undated implementer ids form one cohort", "review model is normalised in ByReviewModel and ByToolModel", "baseline prefers the earlier cohort with the same review and implementer models", "existing TestRegressionFlag passes unmodified", "no stored record is rewritten"], "modelTier": "standard"}
```

---

### Task 5: Correctness rates and code size in the scorecard

**Goal:** `scorecard.json` reports how often a correctness defect escaped a pass, how often anti-tangent had raised a correctness finding on a task where the review found one, and the median lines added per task.

**Files:**
- Modify: `scorecard/metrics.go`
- Modify: `scorecard/assemble.go` (new `outcomeHasHigh`)
- Modify: `CHANGELOG.md`
- Test: `scorecard/correctness_test.go` (create)

**Acceptance Criteria:**
- [ ] `Metrics` has `CorrectnessEscapeRate Rate` (`json:"correctness_escape_rate"`), `CorrectnessFlagRecall Rate` (`json:"correctness_flag_recall"`) and `LinesAddedP50 int64` (`json:"lines_added_p50,omitempty"`).
- [ ] `correctness_escape_rate` is `{num: passed tasks with a critical or major outcome finding whose category is "correctness", n: passed tasks}`. A passed task whose only major outcome finding is `tests` does not count.
- [ ] `correctness_flag_recall` is `{num: tasks with such an outcome finding whose snapshot has categories["correctness"] > 0, n: tasks with such an outcome finding}`, counting tasks of every final verdict.
- [ ] `lines_added_p50` is the nearest-rank median of `lines_added` over tasks whose snapshot has `lines_added > 0`, and is omitted when no task has one.
- [ ] A snapshot with no `categories` and no line counts (a 0.26.0 record) scores without error: recall `num` is 0 for it.

**Verify:** `go test -race ./scorecard/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing tests**

Create `scorecard/correctness_test.go`:

```go
package scorecard

import (
	"testing"
	"time"
)

func snapLine(hash string, idx int, post string, cats map[string]int, added int) RunLine {
	l := taskLine(hash, t0, idx, post, completion("m", post))
	l.Task.Categories = cats
	l.Task.LinesAdded = added
	return l
}

func TestCorrectnessRates(t *testing.T) {
	lines := []RunLine{
		snapLine("r1", 1, "pass", nil, 10),                               // escaped, never flagged
		snapLine("r1", 2, "pass", map[string]int{"correctness": 2}, 30),  // escaped, was flagged
		snapLine("r1", 3, "warn", map[string]int{"correctness": 1}, 20),  // not passed, flagged
		snapLine("r1", 4, "pass", nil, 0),                                // major tests finding only
		snapLine("r1", 5, "pass", map[string]int{"quality": 1}, 0),       // clean
	}
	outs := []OutcomeLine{outcome("r1", SourceFinalReview, t0.Add(time.Hour),
		OutcomeFinding{TaskIndex: 1, Severity: "major", Category: "correctness"},
		OutcomeFinding{TaskIndex: 2, Severity: "critical", Category: "correctness"},
		OutcomeFinding{TaskIndex: 3, Severity: "major", Category: "correctness"},
		OutcomeFinding{TaskIndex: 4, Severity: "major", Category: "tests"},
		OutcomeFinding{TaskIndex: 5, Severity: "minor", Category: "correctness"})}
	g := only(t, Compute(lines, outs, Options{}).ByReviewModel, SourceFinalReview)

	if g.CorrectnessEscapeRate.Num != 2 || g.CorrectnessEscapeRate.N != 4 {
		t.Fatalf("correctness escape = %+v, want 2/4", g.CorrectnessEscapeRate)
	}
	if g.CorrectnessFlagRecall.Num != 2 || g.CorrectnessFlagRecall.N != 3 {
		t.Fatalf("correctness recall = %+v, want 2/3", g.CorrectnessFlagRecall)
	}
	if g.EscapeRate.Num != 3 {
		t.Fatalf("escape = %+v, want 3 (tasks 1, 2, 4)", g.EscapeRate)
	}
	if g.LinesAddedP50 != 20 {
		t.Fatalf("lines added p50 = %d, want 20", g.LinesAddedP50)
	}
}

func TestCorrectnessRatesOnRecordsWithoutCategories(t *testing.T) {
	lines := []RunLine{taskLine("r1", t0, 1, "pass", completion("m", "pass"))}
	outs := []OutcomeLine{outcome("r1", SourceFinalReview, t0.Add(time.Hour),
		OutcomeFinding{TaskIndex: 1, Severity: "major", Category: "correctness"})}
	g := only(t, Compute(lines, outs, Options{}).ByReviewModel, SourceFinalReview)
	if g.CorrectnessFlagRecall.Num != 0 || g.CorrectnessFlagRecall.N != 1 || g.LinesAddedP50 != 0 {
		t.Fatalf("recall = %+v lines = %d", g.CorrectnessFlagRecall, g.LinesAddedP50)
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./scorecard/... -run CorrectnessRates`
Expected: compile error, `g.CorrectnessEscapeRate undefined`. (If `l.Task.Categories` is reported undefined instead, Task 3 has not been applied: stop and apply it first.)

- [ ] **Step 3: Add the outcome helper**

In `scorecard/assemble.go`, add after `outcomeCounts`:

```go
// correctnessCategory is the outcome category a correctness defect is filed
// under. It is the same word as the reviewer's own correctness category, which
// is what lets a task's outcome be matched against its snapshot's categories.
const correctnessCategory = "correctness"

// outcomeHasHigh reports whether o attributes a critical or major finding of
// category to task index.
func outcomeHasHigh(o OutcomeLine, index int, category string) bool {
	for _, f := range o.Findings {
		if f.TaskIndex == index && f.Category == category && (f.Severity == "critical" || f.Severity == "major") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Accumulate and report**

In `scorecard/metrics.go`:

Add to `type Metrics struct`, after `CaughtAndFixed`:

```go
	// CorrectnessEscapeRate counts passed tasks in which the review found a
	// critical or major correctness problem. CorrectnessFlagRecall is, of the
	// tasks with such a problem, the share on which anti-tangent raised a
	// correctness finding at any completion call. Records hold no finding
	// text, so the two are matched by task and not by defect: recall is an
	// upper bound.
	CorrectnessEscapeRate Rate `json:"correctness_escape_rate"`
	CorrectnessFlagRecall Rate `json:"correctness_flag_recall"`
```

and after `ReviewMSP95`:

```go
	LinesAddedP50 int64 `json:"lines_added_p50,omitempty"`
```

Add to `type acc struct`, after the `caught` field:

```go
	corrEsc, corrN, corrFlagged int
	lines                       []int64
```

In `addTask`, directly after the line `high, minor := outcomeCounts(o, t.snap.Index)`, add:

```go
	if outcomeHasHigh(o, t.snap.Index, correctnessCategory) {
		a.corrN++
		if t.snap.Categories[correctnessCategory] > 0 {
			a.corrFlagged++
		}
		if t.snap.PostVerdict == "pass" {
			a.corrEsc++
		}
	}
	if t.snap.LinesAdded > 0 {
		a.lines = append(a.lines, int64(t.snap.LinesAdded))
	}
```

In `metrics()`, add to the `Metrics{...}` literal:

```go
		CorrectnessEscapeRate: wilson(a.corrEsc, a.passN),
		CorrectnessFlagRecall: wilson(a.corrFlagged, a.corrN),
		LinesAddedP50:         percentile(a.lines, 50),
```

`percentile` returns 0 for an empty slice, which `omitempty` then drops. Reading a nil `Categories` map returns 0, so a record without the field needs no guard.

- [ ] **Step 5: Run the package**

Run: `go test -race ./scorecard/... && (cd gnome-topbar/daemon && go test -race ./...)`
Expected: `ok` in both modules. If a daemon test compares a whole `scorecard.Metrics` value against a literal, it now sees two extra zero-valued `Rate` fields; `wilson(0, 0)` is `Rate{Lo: 0, Hi: 1}`, so update that literal to include `CorrectnessEscapeRate` and `CorrectnessFlagRecall` with that value rather than loosening the assertion.

- [ ] **Step 6: Changelog**

Under `## [0.27.0]` → `### Added` in `CHANGELOG.md`:

```markdown
- `scorecard.json` groups report `correctness_escape_rate` (passed tasks in which the review found a critical or major `correctness` problem), `correctness_flag_recall` (of the tasks with such a problem, the share where anti-tangent raised a `correctness` finding on any completion call; matched by task, so an upper bound) and `lines_added_p50`.
```

- [ ] **Step 7: Commit**

```bash
git add scorecard/metrics.go scorecard/assemble.go scorecard/correctness_test.go CHANGELOG.md
git commit -m "feat(scorecard): correctness escape rate, flag recall and lines added"
```

```json:metadata
{"files": ["scorecard/metrics.go", "scorecard/assemble.go", "scorecard/correctness_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./scorecard/...", "acceptanceCriteria": ["Metrics has correctness_escape_rate, correctness_flag_recall, lines_added_p50", "escape rate counts only passed tasks with a critical or major correctness outcome finding", "recall counts tasks of every verdict, matched by task", "lines_added_p50 is the nearest-rank median and omitted when empty", "a record without categories scores without error"], "modelTier": "mechanical"}
```

---

### Task 6: `record_review_outcome` names the tasks with no implementer model

**Goal:** The outcome tool still records the outcome, and its result and summary list the tasks that have a final verdict and no implementer model, so the controller can resend and the `unknown` cohort empties.

**Files:**
- Modify: `scorecard/runescapes.go` (new `TasksWithVerdict`)
- Modify: `internal/mcpsrv/outcome_handler.go`
- Modify: `docs/protocol/outcome.md`
- Modify: `plugin/anti-tangent-protocol/protocol/outcome.md` (resynced copy)
- Modify: `CHANGELOG.md`
- Test: `internal/mcpsrv/outcome_handler_test.go`, `scorecard/compute_test.go`

**Acceptance Criteria:**
- [ ] `scorecard.TasksWithVerdict(lines)` returns, ascending, the indexes of tasks whose latest snapshot has a non-empty `post_verdict`, and an empty non-nil slice for no lines.
- [ ] `RecordReviewOutcomeResult` has `MissingImplementerModels []int` with tag `json:"missing_implementer_models"`; it marshals as `[]`, never `null`, including when the call is not recorded.
- [ ] For a run with tasks 1, 2 and 3 holding final verdicts and a call sending a model for task 2 only, the result has `recorded: true` and `missing_implementer_models: [1, 3]`, and the summary block contains the line `implementer model missing for tasks: 1, 3`.
- [ ] When every task with a verdict has a model, the list is empty and the summary block has no `implementer model missing` line.
- [ ] `docs/protocol/outcome.md` tells the controller to send one entry per task and names `missing_implementer_models`; it stays under 16,000 bytes and `diff -r docs/protocol plugin/anti-tangent-protocol/protocol` prints nothing.
- [ ] The `implementer_models` argument's `jsonschema` description says one entry per dispatched task.

**Verify:** `go test -race ./internal/mcpsrv/... ./scorecard/...` → `ok`; `diff -r docs/protocol plugin/anti-tangent-protocol/protocol` → no output

**Steps:**

- [ ] **Step 1: Write the failing tests**

Append to `scorecard/compute_test.go`:

```go
func TestTasksWithVerdict(t *testing.T) {
	lines := []RunLine{
		taskLine("r1", t0, 3, "pass", completion("m", "pass")),
		taskLine("r1", t0, 1, "warn", completion("m", "warn")),
		taskLine("r1", t0, 2, ""),
	}
	got := TasksWithVerdict(lines)
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("TasksWithVerdict = %v, want [1 3]", got)
	}
	if got := TasksWithVerdict(nil); got == nil || len(got) != 0 {
		t.Fatalf("TasksWithVerdict(nil) = %#v, want an empty non-nil slice", got)
	}
}
```

Append to `internal/mcpsrv/outcome_handler_test.go`:

```go
// outcomeRun mints a run of n tasks, each with the final verdict "pass" and a
// snapshot line, and returns its id.
func outcomeRun(t *testing.T, h *handlers, n int) string {
	t.Helper()
	run := h.deps.PlanRuns.Create("pass", "rigorous", n)
	for i := 0; i < n; i++ {
		sid := "s" + string(rune('1'+i))
		_, ok := h.deps.PlanRuns.Attach(run.ID, sid, planrun.TaskRef{Index: i + 1}, "pass")
		require.True(t, ok)
		row, _ := h.deps.PlanRuns.UpdateRow(run.ID, sid, func(r *planrun.TaskRow) { r.PostVerdict = "pass" })
		h.snapshotRow(run.ID, row)
	}
	return run.ID
}

func TestRecordReviewOutcome_ListsTasksMissingAnImplementerModel(t *testing.T) {
	h, _, dir := outcomeHandlers(t)
	runID := outcomeRun(t, h, 3)
	res := recordOutcome(t, h, RecordReviewOutcomeArgs{
		PlanRunID: runID, Source: "final_review",
		ImplementerModels: []OutcomeImplementerModelArg{{TaskIndex: 2, Model: "anthropic:claude-sonnet-5"}},
		Findings:          []OutcomeFindingArg{},
	})
	require.True(t, res.Recorded, res.Reason)
	assert.Equal(t, []int{1, 3}, res.MissingImplementerModels)
	assert.Contains(t, res.SummaryBlock, "implementer model missing for tasks: 1, 3")
	waitForScorecard(t, dir)
}

func TestRecordReviewOutcome_NoMissingModelsLeavesSummaryQuiet(t *testing.T) {
	h, _, dir := outcomeHandlers(t)
	runID := outcomeRun(t, h, 2)
	res := recordOutcome(t, h, RecordReviewOutcomeArgs{
		PlanRunID: runID, Source: "final_review",
		ImplementerModels: []OutcomeImplementerModelArg{
			{TaskIndex: 1, Model: "anthropic:claude-sonnet-5"},
			{TaskIndex: 2, Model: "anthropic:claude-sonnet-5"},
		},
		Findings: []OutcomeFindingArg{},
	})
	require.True(t, res.Recorded, res.Reason)
	assert.Equal(t, []int{}, res.MissingImplementerModels)
	assert.NotContains(t, res.SummaryBlock, "implementer model missing")
	waitForScorecard(t, dir)
}

func TestRecordReviewOutcome_MissingModelsIsAnArrayWhenNotRecorded(t *testing.T) {
	h := &handlers{deps: newDeps(t, &fakeReviewer{name: "anthropic", resp: passResp("m")})}
	res := recordOutcome(t, h, RecordReviewOutcomeArgs{PlanRunID: "pr_x", Source: "final_review"})
	require.False(t, res.Recorded)
	assert.NotNil(t, res.MissingImplementerModels)
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./scorecard/... ./internal/mcpsrv/... -run 'TasksWithVerdict|MissingAnImplementerModel|NoMissingModels|MissingModelsIsAnArray'`
Expected: compile errors, `undefined: TasksWithVerdict` and `res.MissingImplementerModels undefined`.

- [ ] **Step 3: Add `TasksWithVerdict`**

Append to `scorecard/runescapes.go`:

```go
// TasksWithVerdict returns, ascending, the index of every task in lines whose
// latest snapshot carries a final verdict.
func TasksWithVerdict(lines []RunLine) []int {
	out := []int{}
	for _, r := range assemble(lines, nil, "") {
		for _, t := range r.tasks {
			if t.snap.PostVerdict != "" {
				out = append(out, t.snap.Index)
			}
		}
	}
	sort.Ints(out)
	return out
}
```

- [ ] **Step 4: Report the gap from the handler**

In `internal/mcpsrv/outcome_handler.go`:

Change the argument description:

```go
	ImplementerModels []OutcomeImplementerModelArg `json:"implementer_models,omitempty" jsonschema:"The model each task was dispatched on, one entry per dispatched task. The controller knows this; the server cannot see it. Tasks left out are listed in missing_implementer_models and scored in an unknown cohort."`
```

Add the result field after `Escapes`:

```go
	// MissingImplementerModels lists the tasks that have a final verdict and
	// that this call named no implementer model for.
	MissingImplementerModels []int `json:"missing_implementer_models"`
```

In `recordReviewOutcome`, change the first statement to initialise it:

```go
	res := RecordReviewOutcomeResult{Escapes: []scorecard.Escape{}, MissingImplementerModels: []int{}}
```

and, directly after the line `res.Escapes, res.TasksScored, snapshotted = scorecard.RunEscapes(lines, o)`, add:

```go
	res.MissingImplementerModels = missingImplementerModels(scorecard.TasksWithVerdict(lines), args.ImplementerModels)
```

Add the helper after `outcomeLineFromArgs`:

```go
// missingImplementerModels returns the entries of tasks, in order, that models
// names no model for.
func missingImplementerModels(tasks []int, models []OutcomeImplementerModelArg) []int {
	named := make(map[int]bool, len(models))
	for _, m := range models {
		named[m.TaskIndex] = true
	}
	out := []int{}
	for _, idx := range tasks {
		if !named[idx] {
			out = append(out, idx)
		}
	}
	return out
}
```

In `formatOutcomeSummary`, after the `for _, e := range res.Escapes { ... }` loop and before `return b.String()`, add:

```go
	if len(res.MissingImplementerModels) > 0 {
		idx := make([]string, len(res.MissingImplementerModels))
		for i, n := range res.MissingImplementerModels {
			idx[i] = strconv.Itoa(n)
		}
		fmt.Fprintf(&b, "implementer model missing for tasks: %s — call again with implementer_models for every task\n", strings.Join(idx, ", "))
	}
```

and add `"strconv"` to the import block. The line carries integers only, so it adds no caller-supplied text to the block and needs no `escapeBlockValue`.

- [ ] **Step 5: Run the packages**

Run: `go test -race ./internal/mcpsrv/... ./scorecard/...`
Expected: `ok`. Two existing tests may need attention:
- `summary_forgery_test.go` drives forged text through every string field of the summary inputs. The new field is `[]int`, so it has nothing to forge; if the test enumerates result fields by reflection and rejects an unlisted one, add `MissingImplementerModels: []int{}` to its `RecordReviewOutcomeResult` literal.
- If a test pins the tool catalog or a tool's input schema text, regenerate or update it for the new `implementer_models` description, and read the diff: only that description may change.

- [ ] **Step 6: Update the protocol part and resync the bundle**

In `docs/protocol/outcome.md`, replace the paragraph

```markdown
**Implementer models.** Pass the model you dispatched each task on. With model routing this
differs per task, and the server cannot see it. Omit a task you do not know.
```

with

```markdown
**Implementer models.** Pass the model you dispatched each task on, one entry per task: with
model routing it differs per task, and the server cannot see it. The response lists the tasks you
left out in `missing_implementer_models`; call again with the full list, because a task without a
model is scored in an `unknown` cohort. A dated id and its undated form count as one model.
```

Then:

```bash
rm -f plugin/anti-tangent-protocol/protocol/*.md
cp docs/protocol/*.md plugin/anti-tangent-protocol/protocol/
wc -c docs/protocol/*.md INTEGRATION.md
diff -r docs/protocol plugin/anti-tangent-protocol/protocol
```

Expected: every `docs/protocol/*.md` under 16000, `INTEGRATION.md` under 2000, and `diff` prints nothing. Section numbers are untouched.

- [ ] **Step 7: Changelog**

Under `## [0.27.0]` → `### Added` in `CHANGELOG.md`:

```markdown
- `record_review_outcome` returns `missing_implementer_models`: the tasks that have a final verdict and that the call named no implementer model for. The outcome is still recorded; the summary block names the tasks so the controller can call again with the full list.
```

- [ ] **Step 8: Commit**

```bash
git add scorecard/runescapes.go scorecard/compute_test.go internal/mcpsrv/outcome_handler.go internal/mcpsrv/outcome_handler_test.go docs/protocol/outcome.md plugin/anti-tangent-protocol/protocol CHANGELOG.md
git add -u internal/mcpsrv
git commit -m "feat(outcome): report tasks with no implementer model"
```

```json:metadata
{"files": ["scorecard/runescapes.go", "scorecard/compute_test.go", "internal/mcpsrv/outcome_handler.go", "internal/mcpsrv/outcome_handler_test.go", "docs/protocol/outcome.md", "plugin/anti-tangent-protocol/protocol/outcome.md", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/mcpsrv/... ./scorecard/... && diff -r docs/protocol plugin/anti-tangent-protocol/protocol", "acceptanceCriteria": ["TasksWithVerdict returns ascending indexes and an empty non-nil slice for no lines", "result has missing_implementer_models, always an array", "tasks 1 and 3 are listed when only task 2 has a model, and the summary names them", "no summary line when nothing is missing", "outcome.md updated, under budget, bundle identical", "argument description says one entry per dispatched task"], "modelTier": "standard"}
```

---

### Task 7: Count `plan_run_id` advisories in the stats

**Goal:** `events.jsonl` records how often a call was told it named no plan run, so the cause of the task-row coverage gap can be measured.

**Files:**
- Modify: `internal/stats/event.go`
- Modify: `CHANGELOG.md`
- Test: `internal/stats/event_test.go`

**Acceptance Criteria:**
- [ ] `stats.CountFindings` returns `criterion["plan_run_id"] == 1` for one finding whose criterion is `plan_run_id`, and counts ` Plan_Run_ID ` in the same bucket.
- [ ] A finding whose criterion is free text (an acceptance criterion) is still not counted: `TestCountFindings_CriterionAllowlistOnly` passes unmodified.

**Verify:** `go test -race ./internal/stats/...` → `ok`

**Steps:**

- [ ] **Step 1: Write the failing test**

Append to `internal/stats/event_test.go`:

```go
func TestCountFindings_PlanRunIDAdvisoryIsCounted(t *testing.T) {
	findings := []verdict.Finding{
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryOther,
			Criterion: "plan_run_id", Evidence: "e", Suggestion: "s"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryOther,
			Criterion: " Plan_Run_ID ", Evidence: "e", Suggestion: "s"},
	}
	_, _, crit, total := CountFindings(findings)
	require.Equal(t, 2, total)
	assert.Equal(t, 2, crit["plan_run_id"])
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/stats/... -run TestCountFindings_PlanRunIDAdvisoryIsCounted`
Expected: FAIL, `crit["plan_run_id"]` is 0.

- [ ] **Step 3: Add the sentinel**

In `internal/stats/event.go`, in `countedCriteria`, add after the `"max_tokens_override": true,` line:

```go
	"plan_run_id":                  true,
```

The value is a fixed server-written criterion on the advisories that tell a caller it named no plan run, an unknown one, or no task; it is never reviewer free text.

- [ ] **Step 4: Run the package**

Run: `go test -race ./internal/stats/...`
Expected: `ok`.

- [ ] **Step 5: Changelog**

Under `## [0.27.0]` → `### Added` in `CHANGELOG.md`:

```markdown
- `events.jsonl` counts the `plan_run_id` criterion, so the stats show how often a `validate_task_spec` or lightweight `validate_completion` call named no plan run and therefore left its task out of the run records.
```

- [ ] **Step 6: Commit**

```bash
git add internal/stats/event.go internal/stats/event_test.go CHANGELOG.md
git commit -m "feat(stats): count plan_run_id advisories"
```

```json:metadata
{"files": ["internal/stats/event.go", "internal/stats/event_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/stats/...", "acceptanceCriteria": ["CountFindings counts the plan_run_id criterion, case- and whitespace-insensitively", "free-text criteria are still not counted"], "modelTier": "mechanical"}
```

---

### Task 8: Documentation and whole-repo check

**Goal:** The README describes the new categories, record fields and metrics, the changelog entry is complete, and every module's tests and the protocol invariants are green.

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md` (only if a bullet from Tasks 1–7 is missing)

**Acceptance Criteria:**
- [ ] The README's `#### Scorecard` section names `categories`, `lines_added` / `lines_removed` in the `runs.jsonl` bullet, and `correctness_escape_rate`, `correctness_flag_recall`, `lines_added_p50`, model-id normalisation and the same-model baseline in the `scorecard.json` bullet.
- [ ] The README's `### validate_completion arguments` section has a paragraph naming the `correctness` and `test_adequacy` categories and stating that either can be critical or major.
- [ ] `CHANGELOG.md` `## [0.27.0] - 2026-10-04` has, under `### Added`, bullets for: the two categories, the snapshot fields, the scorecard metrics, `missing_implementer_models`, the `plan_run_id` criterion; and under `### Changed`, bullets for: the completion review, model-id normalisation, the baseline choice.
- [ ] `VERSION` still reads `0.26.0`.
- [ ] `go build ./... && go test -race ./...` passes at the root, `go test -race ./...` passes in `gnome-topbar/daemon`, every `docs/protocol/*.md` is under 16,000 bytes, `INTEGRATION.md` is under 2,000 bytes and `diff -r docs/protocol plugin/anti-tangent-protocol/protocol` prints nothing.
- [ ] No file added or changed on the branch contains a comment with a task, issue, PR or version reference, or the words "previously" or "no longer".

**Verify:** `go build ./... && go test -race ./... && (cd gnome-topbar/daemon && go test -race ./...) && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && cat VERSION` → all `ok`, no diff output, `0.26.0`

**Steps:**

- [ ] **Step 1: README, scorecard section**

In `README.md`, under `#### Scorecard`, replace the `runs.jsonl` bullet's ending and the `scorecard.json` bullet.

Change the end of the `runs.jsonl` bullet from

```markdown
plus `calls_dropped` counting any older calls the cap evicted; and a header per run with the configured model per role.
```

to

```markdown
plus `calls_dropped` counting any older calls the cap evicted, `categories` (findings per category, summed over the task's `validate_completion` calls) and `lines_added` / `lines_removed` (the size of the latest call's diff); and a header per run with the configured model per role.
```

Change the `scorecard.json` bullet from

```markdown
- `scorecard.json`: escape rate (tasks anti-tangent passed that the independent review found a critical or major problem in), unconfirmed-flag rate, waive rate and cost, by review-model cohort and by anti-tangent tool × model, each rate with its n and 90% interval. `regression` stays `insufficient_data` until a cohort and its baseline each have `ANTI_TANGENT_SCORECARD_MIN_RUNS` runs (default 10).
```

to

```markdown
- `scorecard.json`: escape rate (tasks anti-tangent passed that the independent review found a critical or major problem in), unconfirmed-flag rate, waive rate and cost, by review-model cohort and by anti-tangent tool × model, each rate with its n and 90% interval. `correctness_escape_rate` narrows the escape rate to `correctness` problems; `correctness_flag_recall` is, of the tasks with such a problem, the share where anti-tangent raised a `correctness` finding on any completion call (matched by task, so an upper bound); `lines_added_p50` is the median diff size. Model ids are compared lower-cased and without a trailing `-YYYYMMDD` stamp. `regression` compares a cohort with the most recent earlier cohort of the same review and implementer models (any earlier cohort when there is none), and stays `insufficient_data` until both have `ANTI_TANGENT_SCORECARD_MIN_RUNS` runs (default 10).
```

- [ ] **Step 2: README, completion section**

In `README.md`, under `### validate_completion arguments`, add this paragraph directly below the heading, before the line that begins `In addition to`:

```markdown
The completion review walks every acceptance criterion and then reads the submitted change for defects of its own: a finding the evidence shows is reported as `correctness`, and a test that would not fail if the behaviour broke as `test_adequacy`. Either can be critical or major and so move the verdict. The reviewer sees only what the call submits, so a diff with little context limits what it can find.

```

- [ ] **Step 3: Check the changelog against the acceptance criterion**

Run: `awk '/^## \[0.27.0\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.md`
Expected: five `### Added` bullets and three `### Changed` bullets, as listed in the acceptance criteria. Add any that is missing, using the wording from the task that owns it.

- [ ] **Step 4: Comment-policy sweep over the branch's additions**

Run:

```bash
git diff main...HEAD -- '*.go' '*.py' '*.sh' | grep '^+' | grep -nE '(//|#).*(previously|no longer|this replaced|Task [0-9]+|v0\.[0-9]+\.[0-9]+|#[0-9]+)' || echo "clean"
```

Expected: `clean`. Rewrite any hit so the comment states present behaviour.

- [ ] **Step 5: Whole-repo verification**

Run:

```bash
go build ./... && go test -race ./...
(cd gnome-topbar/daemon && go test -race ./...)
wc -c docs/protocol/*.md INTEGRATION.md
diff -r docs/protocol plugin/anti-tangent-protocol/protocol
cat VERSION
```

Expected: every package `ok`; each protocol part under 16000 and `INTEGRATION.md` under 2000; no `diff` output; `0.26.0`.

- [ ] **Step 6: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: correctness review, run-record fields and scorecard metrics"
```

```json:metadata
{"files": ["README.md", "CHANGELOG.md"], "verifyCommand": "go build ./... && go test -race ./... && (cd gnome-topbar/daemon && go test -race ./...) && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && cat VERSION", "acceptanceCriteria": ["README scorecard section names the new fields, metrics, normalisation and baseline", "README completion section names correctness and test_adequacy", "CHANGELOG [0.27.0] has five Added and three Changed bullets", "VERSION still 0.26.0", "all tests green in both modules and protocol invariants hold", "no change-history comment on the branch"], "modelTier": "mechanical"}
```

---

## Task order

Tasks 1 → 2 (the prompt names the categories Task 1 adds). Task 3 → 5 (the metrics read the snapshot fields). Tasks 4, 6 and 7 are independent of the others. Task 8 is last.

## After the plan

- The final whole-plan review is the gate before the PR. Because the plan is not run under anti-tangent, there is no `plan_run_id` and no `record_review_outcome` call for it.
- The effect of Task 2 cannot be shown by a test. It is read from the field after release, against the measures in the spec's §3.1: `escape_rate` and `correctness_flag_recall` for the 0.27.0 cohorts, with `unconfirmed_flag_rate` and `calls_per_task` as the guard rails.
- Parts 2 and 3 of the spec each need their own plan and their own `version/X.Y.Z` branch.
