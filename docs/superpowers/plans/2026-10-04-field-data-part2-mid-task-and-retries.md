# Field-Data Part 2: Mid-Task Checks and Retries That Resolve — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Get `check_progress` called on tasks long enough to drift, make that checkpoint look for defects, and stop `validate_completion` retries being judged on findings that cannot resolve.

**Architecture:** A fourth `anti-tangent-guard` hook (`PostToolUse` on `Edit|Write|NotebookEdit`, bash wrapper plus Python body, like the start gate) counts a task's edits in the session's own transcript and asks once for `check_progress`. In the server, `validate_completion` marks a carried-over minor with `repeat_of` and leaves it out of the minor rung, returns an evidenced CodeScene skip once per session, and adds a major companion to an `over_building` finding raised twice with no answer. `mid.tmpl` gains a short Correctness section; the comment guard scans three more extensions.

**Tech Stack:** Go 1.25 (`text/template` prompts with golden files, `testify` in `internal/` tests), bash and Python 3 standard library for the plugin hooks, `jq` in the eval harness.

**Spec:** `docs/superpowers/specs/2026-10-04-field-data-improvements-design.md` (§5 is this plan; §9 holds the maintainer's rulings; §2 is the evidence). Parts 1 and 3 of that spec are **not** in this plan.

## Global Constraints

- **Branch and release.** Work is on `feature/field-data-part-2-mid-task-and-retries`, cut from Part 1's tip. The pull request targets `version/0.27.0`, never `main`. Do **not** edit `VERSION`. CodeRabbit does not auto-review a pull request whose base is not `main`: ask it with a top-level `@coderabbitai review` comment each round, through `gh` only.
- **Do not gate this plan with anti-tangent.** It changes the tool. Skip `validate_plan`, `validate_task_spec`, `check_progress` and `validate_completion` for every task here; there is no `plan_run_id` and no `record_review_outcome` call. Subagent-driven spec and code-quality review per task, then a final whole-branch review, is the gate. A dispatch prompt for these tasks must **not** contain the heading `## Drift-protection protocol (anti-tangent-mcp)`: the installed guard would then refuse the implementer's edits.
- **The server stays advisory.** No server change here rejects a call. The one enforcing change is the plugin hook, it has its own kill switch, and it refuses nothing: it runs after the edit.
- **The hook fails open.** Every error path in `check-progress-nudge` and its Python body exits 0. It must never be the reason an edit is reported as failed, and it must never ask more than once per task.
- **Exact strings.** Hook files `hooks/check-progress-nudge`, `hooks/check_progress_nudge.py`. Environment `ANTI_TANGENT_PROGRESS_GUARD` (`0` turns the hook off), `ANTI_TANGENT_PROGRESS_EDITS` (default `10`). Trace tag `progress`; events `pass`, `nudge`, `skip`, `error`; detail `edits=N`. Message heading `CHECKPOINT DUE: call check_progress`. State file `progress-asked-<16 hex>` beside the trace log. Finding companion: `category: unaddressed_finding`, `criterion: over_building`, `severity: major`. Record key `over_building_ruled`. Session flags `CodesceneSkipReported`, `OverBuildingAnswered`.
- **Additive wire changes only.** Every new JSON field is `omitempty`, so records written by 0.26.0 still decode. `scorecard/` imports only the standard library; the gnome-topbar daemon builds against it through a `replace` directive and must keep compiling.
- **Content-free records.** `runs.jsonl` and `plan-runs.jsonl` hold counts, verdicts, model ids and anti-tangent's own category names. Nothing here adds a title, a path, finding text or a raw id to them.
- **Public repo.** No consumer ticket id, file name, plan title or task title from the field data appears in code, tests, docs or commit messages. Fixtures use invented names.
- **Protocol byte budgets (CI-enforced).** Each `docs/protocol/*.md` strictly under 16,000 bytes; `INTEGRATION.md` under 2,000; `plugin/anti-tangent-protocol/protocol/` identical to `docs/protocol/`. Only Task 8 edits protocol files; its replacements shrink `implementer.md` (15,954 → 15,879) and `core.md` (15,979 → 15,970). Resync the bundle in the same commit. Do not renumber sections.
- **Changelog.** `CHANGELOG.md` already has `## [0.27.0] - 2026-10-04`, shared by all three parts. Each task appends its bullet as the **last** bullet of the named subsection (`### Added` or `### Changed`) of that entry, in the same commit as the code.
- **Comments.** Comments explain non-obvious behaviour or an invariant. No task, issue, PR or version references, no "previously" / "no longer" / "this replaced". The guard plugin enforces this on `Edit`/`Write`.
- **Prompt goldens.** After editing a template run `go test ./internal/prompts/... -update`, then read `git diff --stat internal/prompts/testdata/`: only `mid_basic.golden` may change in this plan.
- **Tests.** `go test -race ./...` from the repo root, and `cd gnome-topbar/daemon && go test -race ./...`. Guard plugin: `bash plugin/anti-tangent-guard/evals/run.sh` (runs every `hooks/*_test.py`, then the eval table) and `bash plugin/anti-tangent-guard/evals/fp-report.sh`. No test touches the network.
- **Verified by dry run.** Every edit, new file and command in this plan was applied to a throwaway worktree at `ff1a166` and the suites above passed. An `old` block that does not match exactly once means the file has moved: stop and report it instead of adapting the edit.

**User decisions (already made):**
- `check_progress` is triggered automatically from the guard plugin, once per task (spec §9.4).
- An unevidenced CodeScene skip stays major; only an evidenced skip is accepted once (spec §9.1).
- `codescene_not_run` stays verdict-driving.
- Circling back to fix findings is wanted; the target is findings that repeat without resolving.
- `mid.tmpl` asks for correctness defects (spec §9.5).
- 4.0 calls per task is the limit before the correctness prompt is tightened.
- Still unruled, built as designed: the hook asks main sessions as well as dispatched subagents (spec §9, "Still open").

**Rulings this plan makes (reported to the maintainer with the PR):**
- "Once per task" is enforced with a state file created exclusively, not by comparing the count to N. Edits sent in one assistant turn run their hooks concurrently and all see the same count, so an equality test would ask several times or never.
- "The same skip" is any evidenced skip on the session: reason and evidence text are not compared.
- A carried minor is a minor finding that raises a **minor** finding of the previous call again, matched by `same_as` or by fingerprint (category plus criterion), each earlier finding matched once. The server keeps no finding text to compare, and the completion prompt pins some criteria (`comment_hygiene`, `correctness`), so matching once is what stops several new findings hiding behind one old one. An unanswered critical or major repeat is left unmarked.
- An unchanged resubmission of three minors now passes where it used to warn. That is what spec §5.2 asks for ("never pushed back to `warn` only by nits it already reported"); it is listed for the maintainer as a consequence, not changed.
- "The row counts it as ruled" is one counter, `over_building_ruled`, on the task row and snapshot.
- A `finding_responses` answer to an `over_building` finding is remembered by the session: the protocol tells implementers to answer once, so the answer must still stand on later calls.
- An `over_building` finding the reviewer links to a pre-task finding with `same_as` draws no companion: `post.tmpl` addresses that finding to the plan author and says the implementer is not expected to act on it.
- A controller ruling on the companion finding's own id stops the companion, as well as a ruling on the `over_building` finding.
- The companion carries `criterion: over_building` as the spec says, so `events.jsonl` counts that criterion twice on such a call; the §3.1 measure reads presence per call, not the count.
- Inside a subagent the hook's trace line carries `s=<session>.<agent>`, so per-task edit counts can be told apart when subagents run in parallel.
- The "false-positive evals" for the three extensions: this repository has no file of those types, so `fp-report.sh` cannot exercise them. At plan time the extended scanner was run over every distinct `.vue`, `.kts` and `.mjs` file on the maintainer's machine outside `node_modules` (4,243, 801 and 121 files; about 589,000 lines): 23 hits, all of them comments that do cite a pull request, an issue or a task, and no false positive. Only those counts are recorded. The plan adds scanner unit tests on ordinary comments and four eval cases.
- `anti-tangent-guard` goes to 0.7.0; its `marketplace.json` entry, which still said 0.5.0, is brought to 0.7.0 too.

---

### Task 1: Carried minor findings leave the verdict ladder

**Goal:** A minor finding the reviewer raises again from the previous `validate_completion` call is returned with `repeat_of` and does not count toward the `minor >= 3` rung.

**Files:**
- Modify: `internal/verdict/finalize.go`
- Modify: `internal/verdict/verdict.go`
- Modify: `internal/mcpsrv/finding_rulings.go`
- Modify: `CHANGELOG.md`
- Test: `internal/verdict/finalize_test.go`
- Test (create): `internal/mcpsrv/carried_minors_test.go`

**Acceptance Criteria:**
- [ ] `verdict.FinalizeVerdict` does not count a minor finding whose `RepeatOf` is non-empty: three such minors give `pass` with no `noise_cluster` finding; one carried plus three new minors give `warn` whose `noise_cluster` evidence says `3 minor findings`.
- [ ] `markRepeats` sets `RepeatOf` on a minor finding that matches a prior **minor** finding by a shown `same_as` or by fingerprint, answered or not, and matches each prior finding once: of two findings sharing one prior finding's fingerprint only the first is marked. A minor that shares a fingerprint with a prior major is not marked. An unanswered major or critical repeat keeps an empty `RepeatOf`, and only answered critical and major repeats are returned for escalation.
- [ ] Through the handler: a second `validate_completion` returning the same three minors as the first gives `pass`, three findings, each with `RepeatOf` equal to the first call's id. A call returning one earlier `comment_hygiene` minor and three new minors with that same criterion gives `warn`, with `RepeatOf` on the first only.
- [ ] `TestMarkRepeats_EscalatesOnlyAnAnsweredCriticalOrMajorRepeat` and `TestValidateCompletion_OnlyAnAnsweredCriticalOrMajorRepeatEscalates` pass unmodified.
- [ ] `CHANGELOG.md` `[0.27.0]` `### Changed` has the bullet below.

**Verify:** `go test -race ./internal/verdict/... ./internal/mcpsrv/...` → `ok` for both packages

**Steps:**

- [x] **Step 1: Write the failing tests**

Append to `internal/verdict/finalize_test.go` (the file imports `require` only):

```go
func TestFinalizeVerdict_RepeatedMinorsDoNotCountTowardTheMinorRung(t *testing.T) {
	minor := func(criterion, repeatOf string) Finding {
		return Finding{Severity: SeverityMinor, Category: CategoryQuality, Criterion: criterion, RepeatOf: repeatOf}
	}
	carried := FinalizeVerdict(Result{Findings: []Finding{
		minor("a", "f_00000001"), minor("b", "f_00000002"), minor("c", "f_00000003"),
	}})
	require.Equal(t, VerdictPass, carried.Verdict, "three minors already reported must not lift the verdict")
	require.Len(t, carried.Findings, 3, "no noise_cluster advisory for carried minors")

	mixed := FinalizeVerdict(Result{Findings: []Finding{
		minor("a", "f_00000001"), minor("b", ""), minor("c", ""),
	}})
	require.Equal(t, VerdictPass, mixed.Verdict, "two new minors and one carried is below the rung")

	fresh := FinalizeVerdict(Result{Findings: []Finding{
		minor("a", "f_00000001"), minor("b", ""), minor("c", ""), minor("d", ""),
	}})
	require.Equal(t, VerdictWarn, fresh.Verdict, "three new minors still lift the verdict")
	require.Len(t, fresh.Findings, 5)
	require.Contains(t, fresh.Findings[4].Evidence, "3 minor findings")
}
```

Create `internal/mcpsrv/carried_minors_test.go`:

```go
package mcpsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/prompts"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func TestMarkRepeats_CarriesAMinorOnceAndLeavesMajorsAlone(t *testing.T) {
	nitID := verdict.Fingerprint(verdict.CategoryQuality, "", "comment_hygiene")
	majorID := verdict.Fingerprint(verdict.CategoryScopeDrift, "", "AC 1")
	otherID := verdict.Fingerprint(verdict.CategoryQuality, "", "naming")
	fixedID := verdict.Fingerprint(verdict.CategoryCorrectness, "", "correctness")
	prior := []prompts.PriorFinding{
		{Finding: verdict.Finding{ID: nitID, Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"}},
		{Finding: verdict.Finding{ID: majorID, Severity: verdict.SeverityMajor, Category: verdict.CategoryScopeDrift, Criterion: "AC 1"}},
		{Finding: verdict.Finding{ID: otherID, Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "naming"}},
		{Finding: verdict.Finding{ID: fixedID, Severity: verdict.SeverityMajor, Category: verdict.CategoryCorrectness, Criterion: "correctness"}},
	}
	fs := []verdict.Finding{
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"},
		{Severity: verdict.SeverityMajor, Category: verdict.CategoryScopeDrift, Criterion: "AC 1"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryOther, Criterion: "reworded", SameAs: strPtr(otherID)},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryCorrectness, Criterion: "correctness"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "brand new"},
	}
	shown := map[string]bool{nitID: true, majorID: true, otherID: true, fixedID: true}
	escalate := markRepeats(fs, prior, shown)

	assert.Equal(t, nitID, fs[0].RepeatOf, "a minor with a prior minor's fingerprint is carried")
	assert.Empty(t, fs[1].RepeatOf, "a second finding with that fingerprint is new: one prior finding is carried once")
	assert.Empty(t, fs[2].RepeatOf, "an unanswered major is an open finding, not a repeat")
	assert.Equal(t, otherID, fs[3].RepeatOf, "a minor whose same_as names a prior minor is carried")
	assert.Empty(t, fs[4].RepeatOf, "a minor that shares a fingerprint with a prior major is a new finding")
	assert.Empty(t, fs[5].RepeatOf)
	assert.Empty(t, escalate)
}

func TestValidateCompletion_CarriedMinorsDoNotLiftARetryToWarn(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	nits := []string{
		findingObj("minor", "quality", "comment_hygiene", "a history comment", ""),
		findingObj("minor", "quality", "naming", "a vague name", ""),
		findingObj("minor", "quality", "dead_code", "an unused helper", ""),
	}
	first := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(nits...))
	require.Equal(t, "warn", first.Verdict, "three new minors lift the first call to warn")

	second := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(nits...))
	assert.Equal(t, "pass", second.Verdict)
	require.Len(t, second.Findings, 3, "no noise_cluster advisory")
	for i, f := range second.Findings {
		assert.Equal(t, first.Findings[i].ID, f.RepeatOf)
	}
}

func TestValidateCompletion_NewMinorsSharingACriterionStillCount(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	hygiene := func(evidence string) string {
		return findingObj("minor", "quality", "comment_hygiene", evidence, "")
	}
	first := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(hygiene("a.go: a history comment")))
	require.Equal(t, "pass", first.Verdict)

	second := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(
		hygiene("a.go: a history comment"), hygiene("b.go: a ticket reference"),
		hygiene("c.go: a version reference"), hygiene("d.go: a previously comment")))
	assert.Equal(t, "warn", second.Verdict, "one carried and three new: the new ones share its criterion and still count")
	assert.Equal(t, first.Findings[0].ID, second.Findings[0].RepeatOf)
	for _, f := range second.Findings[1:4] {
		assert.Empty(t, f.RepeatOf)
	}
}
```

`findingObj`, `newRulingsHandlers`, `startTask`, `completeWith` are in `handlers_rulings_test.go`; `completionCallArgs`, `reviewerFindingsResp` in `handlers_truncation_test.go`; `strPtr` is an existing test helper.

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/verdict/... -run RepeatedMinors; go test ./internal/mcpsrv/... -run 'CarriedMinor|CarriesAMinor|NewMinorsSharing'`
Expected: FAIL. The verdict test reports `warn` where it wants `pass`; the mcpsrv tests report an empty `RepeatOf` and a `warn` second call.

- [x] **Step 3: Leave carried minors out of the ladder**

In `internal/verdict/finalize.go`, replace

```go
//	otherwise                    → pass
//
// and appends
```

with

```go
//	otherwise                    → pass
//
// A minor finding that carries RepeatOf is left out of the minor count: it was
// already reported on an earlier call, so it must not be what lifts a retry
// back to warn. Only validate_completion sets RepeatOf, so every other tool's
// minors all count. The ladder then appends
```

In `internal/verdict/finalize.go`, replace

```go
		case SeverityMinor:
			minor++
```

with

```go
		case SeverityMinor:
			if f.RepeatOf == "" {
				minor++
			}
```

In `internal/verdict/verdict.go`, replace

```go
jsonschema:"Server-set: the id of an earlier finding the implementer answered that this finding raises again."
```

with

```go
jsonschema:"Server-set: the id of an earlier finding this finding raises again: one the implementer answered, or a minor finding carried over from the previous validate_completion call. A minor finding that carries it does not count toward the verdict."
```

- [x] **Step 4: Mark carried minors**

In `internal/mcpsrv/finding_rulings.go`, replace

```go
// markRepeats sets RepeatOf on every finding that raises again a prior
// finding this call answered — matched by fingerprint, or by a same_as naming
// it — and returns the prior IDs its critical and major repeats raise again,
// each once, in order. It clears same_as on every finding once read.
func markRepeats(fs []verdict.Finding, prior []prompts.PriorFinding, shown map[string]bool) []string {
	answered := map[string]bool{}
	answeredByFingerprint := map[string]string{}
	for _, p := range prior {
		if p.Response == "" {
			continue
		}
		answered[p.ID] = true
		if fp := fingerprintOf(p.Finding); answeredByFingerprint[fp] == "" {
			answeredByFingerprint[fp] = p.ID
		}
	}
	var escalate []string
	for i := range fs {
		f := &fs[i]
		if id := sameAsID(*f, shown); id != "" && answered[id] {
			f.RepeatOf = id
		} else if id := answeredByFingerprint[fingerprintOf(*f)]; id != "" {
			f.RepeatOf = id
		}
		f.SameAs = nil
```

with

```go
// carriedMinors is the previous call's minor findings that a minor finding
// on this call can raise again. Each is matched once: a fingerprint is only a
// category and a criterion, and the completion prompt pins some criteria
// (comment_hygiene, correctness), so two new findings can share a fingerprint
// with one old one. Matching each old finding once keeps the second from
// being read as already reported.
type carriedMinors struct {
	left []prompts.PriorFinding
}

// take returns the ID of the prior minor finding f raises again — the one its
// same_as names, else the first with its fingerprint — and removes it, or
// returns "" when none is left.
func (c *carriedMinors) take(f verdict.Finding, shown map[string]bool) string {
	named, fp, pick := sameAsID(f, shown), fingerprintOf(f), -1
	for i, p := range c.left {
		if named != "" && p.ID == named {
			pick = i
			break
		}
		if pick < 0 && fingerprintOf(p.Finding) == fp {
			pick = i
		}
	}
	if pick < 0 {
		return ""
	}
	id := c.left[pick].ID
	c.left = append(c.left[:pick], c.left[pick+1:]...)
	return id
}

// markRepeats sets RepeatOf on every finding that raises a prior finding
// again — matched by a same_as naming it, or by fingerprint — in two cases: a
// finding of any severity when this call answered the prior one, and a minor
// finding that raises a prior minor finding, answered or not. The second case
// is what lets FinalizeVerdict leave a carried-over nit out of the minor
// count. An unanswered critical or major repeat stays unmarked: it is an open
// finding, not a dispute. Returns the prior IDs the critical and major repeats
// raise again, each once, in order, and clears same_as on every finding once
// read.
func markRepeats(fs []verdict.Finding, prior []prompts.PriorFinding, shown map[string]bool) []string {
	answered := map[string]bool{}
	answeredByFingerprint := map[string]string{}
	var carried carriedMinors
	for _, p := range prior {
		if p.Severity == verdict.SeverityMinor {
			carried.left = append(carried.left, p)
		}
		if p.Response == "" {
			continue
		}
		answered[p.ID] = true
		if fp := fingerprintOf(p.Finding); answeredByFingerprint[fp] == "" {
			answeredByFingerprint[fp] = p.ID
		}
	}
	var escalate []string
	for i := range fs {
		f := &fs[i]
		if id := sameAsID(*f, shown); id != "" && answered[id] {
			f.RepeatOf = id
		} else if id := answeredByFingerprint[fingerprintOf(*f)]; id != "" {
			f.RepeatOf = id
		} else if f.Severity == verdict.SeverityMinor {
			f.RepeatOf = carried.take(*f, shown)
		}
		f.SameAs = nil
```

The two answered-repeat branches, the clearing of `same_as`, the escalation check and the return are unchanged: the edit adds the `carriedMinors` type, collects prior minors in the first loop, and adds the third branch.

- [x] **Step 5: Run the packages**

Run: `go test -race ./internal/verdict/... ./internal/mcpsrv/...`
Expected: `ok` twice. If `TestToolSchema...` contract tests fail on the `repeat_of` description, the description edit in `verdict.go` was not applied exactly.

- [x] **Step 6: Changelog**

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- `validate_completion` marks a minor finding that raises a minor finding of the previous call again with `repeat_of`, whether or not the implementer answered it, and such a finding no longer counts toward the three-minor rung that lifts a verdict to `warn`. A retry made to fix a major finding is no longer pushed back to `warn` by nits it already reported. Findings are matched by category and criterion (or the reviewer's `same_as`), and each earlier finding is matched once, so new findings that share a criterion with an old one still count. An unanswered critical or major finding raised again is unchanged: it carries no `repeat_of` and still counts.
```

- [x] **Step 7: Commit**

```bash
git add internal/verdict/finalize.go internal/verdict/verdict.go internal/verdict/finalize_test.go internal/mcpsrv/finding_rulings.go internal/mcpsrv/carried_minors_test.go CHANGELOG.md
git commit -m "feat(completion): carried minor findings no longer lift a retry to warn"
```

```json:metadata
{"files": ["internal/verdict/finalize.go", "internal/verdict/verdict.go", "internal/verdict/finalize_test.go", "internal/mcpsrv/finding_rulings.go", "internal/mcpsrv/carried_minors_test.go", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/verdict/... ./internal/mcpsrv/...", "acceptanceCriteria": ["FinalizeVerdict does not count a minor finding that carries RepeatOf", "markRepeats marks a minor that raises a prior minor again, each prior finding once, and leaves an unanswered major unmarked", "a second validate_completion returning the same three minors passes with repeat_of set on each, and three new minors sharing a carried one's criterion still warn", "the two existing escalation tests pass unmodified", "CHANGELOG.md [0.27.0] Changed has the bullet"], "modelTier": "standard"}
```

---

### Task 2: An evidenced CodeScene skip is reported once per session

**Goal:** With `ANTI_TANGENT_CODESCENE=required`, the minor finding for a skip that carries a reason and evidence is returned on the session's first `validate_completion` call and not on later ones.

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/store.go`
- Modify: `internal/mcpsrv/submission_defect.go`
- Modify: `internal/mcpsrv/handlers.go` (`ValidateCompletion`)
- Modify: `README.md`, `CHANGELOG.md`
- Test: `internal/session/store_test.go`
- Test (create): `internal/mcpsrv/codescene_skip_once_test.go`

**Acceptance Criteria:**
- [ ] A session's first `validate_completion` with `{"ran": false, "skip_reason": …, "skip_evidence": …}` returns one minor `codescene_skipped` finding; the second call on that session with the same argument returns none.
- [ ] A skip with a reason and no evidence returns a major `codescene_skipped` on every call, and does not use up the one report: an evidenced skip sent afterwards is still reported.
- [ ] After an evidenced skip was reported, a call with no `codescene` argument still returns `codescene_not_run`.
- [ ] A lightweight call (empty `session_id`) returns the evidenced-skip finding on every call.
- [ ] `session.Store.ApplyReview` sets `CodesceneSkipReported` when the update carries it and never clears it; `ReviewState` returns it.
- [ ] `README.md`'s skip-ladder paragraph and `CHANGELOG.md` `[0.27.0]` `### Changed` describe the change.

**Verify:** `go test -race ./internal/session/... ./internal/mcpsrv/...` → `ok` for both packages

**Steps:**

- [x] **Step 1: Write the failing tests**

Append to `internal/session/store_test.go`:

```go
func TestStore_CodesceneSkipReportedIsSticky(t *testing.T) {
	s := NewStore(time.Hour)
	sess := s.Create(TaskSpec{Title: "t"}, "")
	st, _ := s.ReviewState(sess.ID)
	assert.False(t, st.CodesceneSkipReported)

	require.True(t, s.ApplyReview(sess.ID, ReviewUpdate{CodesceneSkipReported: true}))
	require.True(t, s.ApplyReview(sess.ID, ReviewUpdate{}))
	st, _ = s.ReviewState(sess.ID)
	assert.True(t, st.CodesceneSkipReported, "a later review that reports no skip must not clear the flag")
}
```

Create `internal/mcpsrv/codescene_skip_once_test.go`:

```go
package mcpsrv

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
	"github.com/patiently/anti-tangent-mcp/internal/session"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func findingsOf(env Envelope, category verdict.Category) []verdict.Finding {
	var out []verdict.Finding
	for _, f := range env.Findings {
		if f.Category == category {
			out = append(out, f)
		}
	}
	return out
}

func skipArgs(sid string, evidence string) ValidateCompletionArgs {
	args := completionCallArgs(sid)
	args.Codescene = &codescene.Digest{Ran: false, SkipReason: "docs-only task", SkipEvidence: evidence}
	return args
}

func TestValidateCompletion_EvidencedSkipIsReportedOncePerSession(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	_, first, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	require.Len(t, findingsOf(first, verdict.CategoryCodesceneSkipped), 1)

	_, second, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	assert.Empty(t, findingsOf(second, verdict.CategoryCodesceneSkipped), "an evidenced skip is reported once")
}

func TestValidateCompletion_UnevidencedSkipRepeatsOnEveryCall(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	for call := 1; call <= 2; call++ {
		_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, ""))
		require.NoError(t, err)
		got := findingsOf(env, verdict.CategoryCodesceneSkipped)
		require.Len(t, got, 1, "call %d", call)
		assert.Equal(t, verdict.SeverityMajor, got[0].Severity)
	}

	_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "MCP error: tool not found"))
	require.NoError(t, err)
	got := findingsOf(env, verdict.CategoryCodesceneSkipped)
	require.Len(t, got, 1, "an unevidenced skip must not use up the one report of an evidenced one")
	assert.Equal(t, verdict.SeverityMinor, got[0].Severity)
}

func TestValidateCompletion_MissingCodesceneAfterAnEvidencedSkipIsStillNotRun(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	_, _, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	_, env, err := h.ValidateCompletion(context.Background(), nil, completionCallArgs(sess.ID))
	require.NoError(t, err)
	assert.Len(t, findingsOf(env, verdict.CategoryCodesceneNotRun), 1)
}

func TestValidateCompletion_LightweightEvidencedSkipIsReportedEveryCall(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	for call := 1; call <= 2; call++ {
		_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs("", "no source files in diff"))
		require.NoError(t, err)
		assert.Len(t, findingsOf(env, verdict.CategoryCodesceneSkipped), 1, "call %d", call)
	}
}
```

`newTestHandlersWithCodescene` is an existing helper in `handlers_test.go`. `findingsOf` is new here and Task 3's tests use it.

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/session/... ./internal/mcpsrv/... -run 'CodesceneSkipReported|EvidencedSkip|UnevidencedSkip|MissingCodescene'`
Expected: compile error in `internal/session` (`unknown field CodesceneSkipReported`); once that is fixed, `TestValidateCompletion_EvidencedSkipIsReportedOncePerSession` fails with one finding where it wants none.

- [x] **Step 3: The session flag**

In `internal/session/session.go`, replace

```go
	Escalated bool
}
```

with

```go
	Escalated bool
	// CodesceneSkipReported is set once a validate_completion on the session
	// has returned the finding for an evidenced CodeScene skip, and never
	// cleared. Nothing the implementer does clears that finding, so it is
	// returned once.
	CodesceneSkipReported bool
}
```

In `internal/session/store.go`, replace

```go
	// CheckpointFindings is one copy of Findings per checkpoint, in order.
	CheckpointFindings [][]verdict.Finding
}
```

with

```go
	// CheckpointFindings is one copy of Findings per checkpoint, in order.
	CheckpointFindings [][]verdict.Finding
	// CodesceneSkipReported mirrors Session.CodesceneSkipReported.
	CodesceneSkipReported bool
}
```

In `internal/session/store.go`, replace

```go
		CheckpointFindings: make([][]verdict.Finding, len(sess.Checkpoints)),
	}
```

with

```go
		CheckpointFindings: make([][]verdict.Finding, len(sess.Checkpoints)),

		CodesceneSkipReported: sess.CodesceneSkipReported,
	}
```

In `internal/session/store.go`, replace

```go
	Rulings   map[string]Ruling
	Escalated bool
}
```

with

```go
	Rulings   map[string]Ruling
	Escalated bool
	// CodesceneSkipReported sets the session's flag; false leaves it as it is.
	CodesceneSkipReported bool
}
```

In `internal/session/store.go`, replace

```go
	if u.Escalated {
		sess.Escalated = true
	}
```

with

```go
	if u.Escalated {
		sess.Escalated = true
	}
	if u.CodesceneSkipReported {
		sess.CodesceneSkipReported = true
	}
```

- [x] **Step 4: Report the skip once**

In `internal/mcpsrv/submission_defect.go`, replace

```go
// codesceneFindings returns the findings implied by an inbound digest.
```

with

```go
// isEvidencedSkip reports whether d is the one CodeScene skip the server
// accepts: declared, with a reason and with evidence. Its finding asks for
// nothing, so a session returns it once and not on every later call.
func isEvidencedSkip(mode string, d *codescene.Digest) bool {
	return mode == "required" && d != nil && !d.Ran &&
		strings.TrimSpace(d.SkipReason) != "" && strings.TrimSpace(d.SkipEvidence) != ""
}

// codesceneFindings returns the findings implied by an inbound digest.
```

In `internal/mcpsrv/handlers.go`, replace

```go
	var lightweightMalformedRulingIDs []string
	if lightweight {
```

with

```go
	var lightweightMalformedRulingIDs []string
	skipReported := false
	if lightweight {
```

In `internal/mcpsrv/handlers.go`, replace

```go
		review = buildCompletionReview(state, state.PreFindings, knownSessionFindings(state), responses, rulingArgs)
	}
```

with

```go
		review = buildCompletionReview(state, state.PreFindings, knownSessionFindings(state), responses, rulingArgs)
		skipReported = state.CodesceneSkipReported
	}
```

In `internal/mcpsrv/handlers.go`, replace

```go
	head = append(head, codesceneFindings(h.deps.Cfg.Codescene, args.Codescene)...)
```

with

```go
	// An evidenced skip's finding asks for nothing, so a session returns it
	// once. A lightweight call has no session to remember it in and returns it
	// every time.
	evidencedSkip := isEvidencedSkip(h.deps.Cfg.Codescene, args.Codescene)
	if !evidencedSkip || !skipReported {
		head = append(head, codesceneFindings(h.deps.Cfg.Codescene, args.Codescene)...)
	}
```

In `internal/mcpsrv/handlers.go`, replace

```go
			IssuedIDs: envelopeIDs(env),
			Rulings:   review.newRulings,
			Escalated: env.Escalate,
		}
```

with

```go
			IssuedIDs:             envelopeIDs(env),
			Rulings:               review.newRulings,
			Escalated:             env.Escalate,
			CodesceneSkipReported: evidencedSkip,
		}
```

The flag is written by the one locked `ApplyReview` the call already makes, so two concurrent calls on a session cannot lose it. `gofmt` realigns the struct literal as shown.

- [x] **Step 5: Run the packages**

Run: `gofmt -l internal/ && go test -race ./internal/session/... ./internal/mcpsrv/...`
Expected: no `gofmt` output; `ok` twice. `TestValidateCompletion_CodesceneRequired_DeclaredSkipWithEvidence` (a first call) still passes.

- [x] **Step 6: Documentation and changelog**

In `README.md`, replace

```markdown
and `skip_evidence` is not read at all.
```

with

```markdown
and `skip_evidence` is not read at all. From 0.27.0 a skip that carries `skip_evidence` is reported once per session: later `validate_completion` calls on that session return no `codescene_skipped` finding for it, while a skip without evidence, and a missing `codescene` argument, are returned on every call. A lightweight call has no session, so it is reported each time.
```

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- With `ANTI_TANGENT_CODESCENE=required`, a CodeScene skip that carries both `skip_reason` and `skip_evidence` is reported once per session: the first `validate_completion` call returns the minor `codescene_skipped` finding and later calls on that session return none. A skip without evidence stays `major` on every call, as does a missing `codescene` argument (`codescene_not_run`). A lightweight call has no session and reports the skip each time.
```

- [x] **Step 7: Commit**

```bash
git add internal/session/session.go internal/session/store.go internal/session/store_test.go internal/mcpsrv/submission_defect.go internal/mcpsrv/handlers.go internal/mcpsrv/codescene_skip_once_test.go README.md CHANGELOG.md
git commit -m "feat(completion): report an evidenced CodeScene skip once per session"
```

```json:metadata
{"files": ["internal/session/session.go", "internal/session/store.go", "internal/session/store_test.go", "internal/mcpsrv/submission_defect.go", "internal/mcpsrv/handlers.go", "internal/mcpsrv/codescene_skip_once_test.go", "README.md", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/session/... ./internal/mcpsrv/...", "acceptanceCriteria": ["an evidenced skip is returned on a session's first validate_completion and not on later ones", "an unevidenced skip stays major on every call and does not use up the one report", "codescene_not_run is unchanged after an evidenced skip", "a lightweight call reports the skip every time", "ApplyReview sets CodesceneSkipReported and never clears it", "README and CHANGELOG describe the change"], "modelTier": "standard"}
```

---

### Task 3: `over_building` raised twice without an answer gains a major companion

**Goal:** An `over_building` finding the reviewer raises again on a later `validate_completion` call, with no `finding_responses` answer and no controller ruling, is accompanied by a major `unaddressed_finding`, and the task row counts the calls on which one was settled by an answer or a ruling.

**Files:**
- Modify: `internal/session/session.go`, `internal/session/store.go`
- Modify: `internal/mcpsrv/finding_rulings.go`
- Modify: `internal/mcpsrv/handlers.go` (`ValidateCompletion`)
- Modify: `internal/mcpsrv/plan_run_rows.go`, `internal/mcpsrv/run_snapshots.go`
- Modify: `internal/planrun/planrun.go`, `scorecard/records.go`
- Modify: `README.md`, `CHANGELOG.md`
- Test: `internal/session/store_test.go`
- Test (create): `internal/mcpsrv/over_building_teeth_test.go`

Depends on Task 1 (a carried minor carries `RepeatOf`) and Task 2 (`findingsOf`, and the session edits anchor on Task 2's `CodesceneSkipReported` lines).

**Acceptance Criteria:**
- [ ] First call returning a `quality` / `over_building` minor: verdict `pass`, no `unaddressed_finding`. Second call returning it again with no answer: one extra finding with `severity: major`, `category: unaddressed_finding`, `criterion: over_building`, whose evidence names the first call's finding id and whose suggestion names `finding_responses`; verdict `warn`; `next_action` starts `Do not report DONE`; `escalate` is false. A third unanswered call has the companion again; a call whose review returns no `over_building` finding has neither.
- [ ] The companion is not stored as a prior finding: after the second call the session's prior findings hold the one `quality` finding.
- [ ] With a `finding_responses` entry for the first call's id, the second call has no companion, verdict `pass`, and the finding carries `repeat_of`; a third call that sends no `finding_responses` still has no companion (`Session.OverBuildingAnswered` is sticky).
- [ ] With a `controller_rulings` entry for the first call's id, the finding is waived and there is no companion. With a `controller_rulings` entry for the **companion's** id, a third call has no companion and verdict `pass`.
- [ ] An `over_building` finding whose `same_as` names a pre-task `over_building` finding draws no companion on any call.
- [ ] `overBuildingReview.ruled` is true when a waived finding is `over_building` and when the review is open and settled; false when open and unsettled, and when settled but no longer raised. `isOverBuilding` ignores case and surrounding space in the criterion and is false for the companion's category.
- [ ] `countOverBuildingRuled` increments `TaskRow.OverBuildingRuled` only when told the call was ruled, and still runs the wrapped update; the run snapshot carries the count as `over_building_ruled`; a zero value is omitted from JSON.
- [ ] `README.md` and `CHANGELOG.md` describe the change; `cd gnome-topbar/daemon && go build ./...` succeeds.

**Verify:** `go test -race ./internal/session/... ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go build ./...)` → `ok`, no build output

**Steps:**

- [x] **Step 1: Write the failing tests**

Append to `internal/session/store_test.go`:

```go
func TestStore_OverBuildingAnsweredIsSticky(t *testing.T) {
	s := NewStore(time.Hour)
	sess := s.Create(TaskSpec{Title: "t"}, "")
	require.True(t, s.ApplyReview(sess.ID, ReviewUpdate{OverBuildingAnswered: true}))
	require.True(t, s.ApplyReview(sess.ID, ReviewUpdate{}))
	st, _ := s.ReviewState(sess.ID)
	assert.True(t, st.OverBuildingAnswered, "an answer sent once must still stand after later reviews")
}
```

Create `internal/mcpsrv/over_building_teeth_test.go`:

```go
package mcpsrv

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

var overBuilt = findingObj("minor", "quality", "over_building", "x.go: yagni: a factory for one product. net: -20 lines", "")

func TestValidateCompletion_OverBuildingRaisedTwiceUnansweredGainsAMajorCompanion(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	first := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	require.Equal(t, "pass", first.Verdict)
	require.Empty(t, findingsOf(first, verdict.CategoryUnaddressed), "one call is not a repeat")
	id := first.Findings[0].ID

	second := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	companions := findingsOf(second, verdict.CategoryUnaddressed)
	require.Len(t, companions, 1)
	assert.Equal(t, verdict.SeverityMajor, companions[0].Severity)
	assert.Equal(t, "over_building", companions[0].Criterion)
	assert.Contains(t, companions[0].Evidence, id)
	assert.Contains(t, companions[0].Suggestion, "finding_responses")
	assert.Equal(t, "warn", second.Verdict)
	assert.True(t, strings.HasPrefix(second.NextAction, "Do not report DONE"), second.NextAction)
	assert.False(t, second.Escalate)

	st, _ := h.deps.Sessions.ReviewState(sid)
	require.Len(t, st.PriorFindings, 1, "the companion is not stored as a prior finding")
	assert.Equal(t, verdict.CategoryQuality, st.PriorFindings[0].Category)

	third := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	assert.Len(t, findingsOf(third, verdict.CategoryUnaddressed), 1, "still unanswered on the third call")

	fixed := completeWith(t, h, rv, completionCallArgs(sid), passResp("claude-opus-4-7"))
	assert.Empty(t, fixed.Findings, "cutting the structure clears both findings")
}

func TestValidateCompletion_AnOverBuildingAnswerStandsOnLaterCalls(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	id := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt)).Findings[0].ID

	args := completionCallArgs(sid)
	args.FindingResponses = []FindingResponseArg{{FindingID: id, Response: "the second product lands in the next task"}}
	second := completeWith(t, h, rv, args, reviewerFindingsResp(overBuilt))
	assert.Empty(t, findingsOf(second, verdict.CategoryUnaddressed))
	assert.Equal(t, "pass", second.Verdict)
	assert.Equal(t, id, second.Findings[0].RepeatOf)

	third := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	assert.Empty(t, findingsOf(third, verdict.CategoryUnaddressed), "the answer is sent once and still stands")
	assert.Equal(t, "pass", third.Verdict)
}

func TestValidateCompletion_RuledOverBuildingIsWaivedWithNoCompanion(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	id := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt)).Findings[0].ID

	args := completionCallArgs(sid)
	args.ControllerRulings = []ControllerRulingArg{{FindingID: id, Ruling: "keep the factory"}}
	env := completeWith(t, h, rv, args, reviewerFindingsResp(overBuilt))

	assert.Empty(t, env.Findings)
	require.Len(t, env.WaivedFindings, 1)
}

func TestValidateCompletion_ARulingOnTheCompanionStopsIt(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	second := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(overBuilt))
	companion := findingsOf(second, verdict.CategoryUnaddressed)
	require.Len(t, companion, 1)

	args := completionCallArgs(sid)
	args.ControllerRulings = []ControllerRulingArg{{FindingID: companion[0].ID, Ruling: "the structure stays"}}
	third := completeWith(t, h, rv, args, reviewerFindingsResp(overBuilt))
	assert.Empty(t, findingsOf(third, verdict.CategoryUnaddressed))
	assert.Equal(t, "pass", third.Verdict)
}

func TestValidateCompletion_OverBuildingAddressedToThePlanAuthorDrawsNoCompanion(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	preID := verdict.Fingerprint(verdict.CategoryQuality, "", "over_building")
	require.True(t, h.deps.Sessions.SetPreFindings(sid, []verdict.Finding{{
		ID: preID, Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "over_building",
		Evidence: "AC 1 mandates an interface with one implementation.", Suggestion: "Drop it from the AC.",
	}}))
	mandated := findingObj("minor", "quality", "over_building", "x.go: yagni: the interface AC 1 mandates", preID)

	completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(mandated))
	second := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(mandated))
	assert.Empty(t, findingsOf(second, verdict.CategoryUnaddressed),
		"a finding the prompt addresses to the plan author asks nothing of the implementer")
	assert.Equal(t, "pass", second.Verdict)
}

func TestReviewOverBuilding_RuledAndCriterionMatching(t *testing.T) {
	assert.True(t, isOverBuilding(verdict.CategoryQuality, " Over_Building "))
	assert.False(t, isOverBuilding(verdict.CategoryUnaddressed, "over_building"), "the companion is not an over_building finding")

	waived := []verdict.WaivedFinding{{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "over_building"}}
	assert.True(t, overBuildingReview{}.ruled(waived), "a ruling waived it")
	assert.True(t, overBuildingReview{open: true, settled: true}.ruled(nil), "raised and answered for")
	assert.False(t, overBuildingReview{open: true}.ruled(nil), "raised with no answer")
	assert.False(t, overBuildingReview{settled: true}.ruled(nil), "answered once, and since cut")
}

func TestCompletionRows_CountRuledOverBuilding(t *testing.T) {
	update := completionRowUpdate(Envelope{Verdict: "pass"}, nil, "")
	var row planrun.TaskRow
	countOverBuildingRuled(update, false)(&row)
	assert.Zero(t, row.OverBuildingRuled)
	countOverBuildingRuled(update, true)(&row)
	countOverBuildingRuled(update, true)(&row)
	assert.Equal(t, 2, row.OverBuildingRuled)
	assert.Equal(t, "pass", row.PostVerdict, "the wrapped update still runs")
}

func TestSnapshotRow_CarriesOverBuildingRuled(t *testing.T) {
	h, rec, _ := outcomeHandlers(t)
	run := h.deps.PlanRuns.Create("pass", "rigorous", 1)
	_, ok := h.deps.PlanRuns.Attach(run.ID, "s1", planrun.TaskRef{Index: 1}, "pass")
	require.True(t, ok)
	update := countOverBuildingRuled(completionRowUpdate(Envelope{Verdict: "pass"}, nil, ""), true)
	row, ok := h.deps.PlanRuns.UpdateRow(run.ID, "s1", update)
	require.True(t, ok)
	h.snapshotRow(run.ID, row)

	lines, err := rec.RunLines(rec.RunHash(run.ID))
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	snap := lines[len(lines)-1].Task
	require.NotNil(t, snap)
	assert.Equal(t, 1, snap.OverBuildingRuled)
}
```

`outcomeHandlers` is an existing helper used the same way by `TestSnapshotRow_CarriesCategoriesAndDiffSize` in `completion_row_test.go`; `findingsOf` is in Task 2's `codescene_skip_once_test.go`; `Sessions.SetPreFindings` is an existing store method.

- [x] **Step 2: Run them and see them fail**

Run: `go test ./internal/session/... ./internal/mcpsrv/... -run 'OverBuilding|CompletionRows_CountRuled'`
Expected: compile errors: `unknown field OverBuildingAnswered` in `internal/session`, and `undefined: isOverBuilding`, `undefined: overBuildingReview`, `undefined: countOverBuildingRuled` in `internal/mcpsrv`.

- [x] **Step 3: The session flag and the record fields**

In `internal/session/session.go`, replace

```go
	CodesceneSkipReported bool
}
```

with

```go
	CodesceneSkipReported bool
	// OverBuildingAnswered is set once a validate_completion on the session
	// carried a finding_responses answer to an over_building finding, and
	// never cleared: an answer is sent once, and must still stand on the
	// calls after it.
	OverBuildingAnswered bool
}
```

In `internal/session/store.go`, replace

```go
	// CodesceneSkipReported mirrors Session.CodesceneSkipReported.
	CodesceneSkipReported bool
}
```

with

```go
	// CodesceneSkipReported mirrors Session.CodesceneSkipReported.
	CodesceneSkipReported bool
	// OverBuildingAnswered mirrors Session.OverBuildingAnswered.
	OverBuildingAnswered bool
}
```

In `internal/session/store.go`, replace

```go
		CodesceneSkipReported: sess.CodesceneSkipReported,
	}
```

with

```go
		CodesceneSkipReported: sess.CodesceneSkipReported,
		OverBuildingAnswered:  sess.OverBuildingAnswered,
	}
```

In `internal/session/store.go`, replace

```go
	// CodesceneSkipReported sets the session's flag; false leaves it as it is.
	CodesceneSkipReported bool
}
```

with

```go
	// CodesceneSkipReported sets the session's flag; false leaves it as it is.
	CodesceneSkipReported bool
	// OverBuildingAnswered sets the session's flag; false leaves it as it is.
	OverBuildingAnswered bool
}
```

In `internal/session/store.go`, replace

```go
	if u.CodesceneSkipReported {
		sess.CodesceneSkipReported = true
	}
```

with

```go
	if u.CodesceneSkipReported {
		sess.CodesceneSkipReported = true
	}
	if u.OverBuildingAnswered {
		sess.OverBuildingAnswered = true
	}
```

In `internal/planrun/planrun.go`, replace

```go
	LinesAdded   int `json:"lines_added,omitempty"`
	LinesRemoved int `json:"lines_removed,omitempty"`
}
```

with

```go
	LinesAdded   int `json:"lines_added,omitempty"`
	LinesRemoved int `json:"lines_removed,omitempty"`
	// OverBuildingRuled counts the validate_completion calls on the task whose
	// over_building finding was settled by an answer or a controller ruling
	// instead of by cutting the structure.
	OverBuildingRuled int `json:"over_building_ruled,omitempty"`
}
```

In `scorecard/records.go`, replace

```go
	LinesAdded   int            `json:"lines_added,omitempty"`
	LinesRemoved int            `json:"lines_removed,omitempty"`
}
```

with

```go
	LinesAdded   int            `json:"lines_added,omitempty"`
	LinesRemoved int            `json:"lines_removed,omitempty"`
	// OverBuildingRuled counts the task's validate_completion calls whose
	// over_building finding was answered or ruled on instead of fixed.
	OverBuildingRuled int `json:"over_building_ruled,omitempty"`
}
```

In `internal/mcpsrv/run_snapshots.go`, replace

```go
			LinesRemoved:   row.LinesRemoved,
```

with

```go
			LinesRemoved:   row.LinesRemoved,

			OverBuildingRuled: row.OverBuildingRuled,
```

- [x] **Step 4: The over-building review**

In `internal/mcpsrv/finding_rulings.go`, replace

```go
	if f.Category == verdict.CategoryQuality && strings.ToLower(strings.TrimSpace(f.Criterion)) == "over_building" {
		return true
	}
	return false
}
```

with

```go
	return isOverBuilding(f.Category, f.Criterion)
}

// overBuildingCriterion is the criterion every over-building finding carries.
const overBuildingCriterion = "over_building"

// isOverBuilding reports whether a category and criterion are the reviewer's
// over-building finding. Criterion is free text on the wire, so case and
// surrounding space are ignored.
func isOverBuilding(category verdict.Category, criterion string) bool {
	return category == verdict.CategoryQuality &&
		strings.ToLower(strings.TrimSpace(criterion)) == overBuildingCriterion
}

// overBuildingCompanion is the major finding that accompanies an
// over_building finding raised again with nobody having answered it. An
// over_building finding is minor by template, so without the companion
// nothing obliges an implementer to act on one. priorID is the earlier
// finding it names.
func overBuildingCompanion(priorID string) verdict.Finding {
	return verdict.Finding{
		Severity:  verdict.SeverityMajor,
		Category:  verdict.CategoryUnaddressed,
		Criterion: overBuildingCriterion,
		Evidence: "The over_building finding " + priorID + " from an earlier validate_completion call is raised " +
			"again on this one, and no finding_responses entry has answered it.",
		Suggestion: "Cut the structure that finding names, or answer " + priorID + " in finding_responses with " +
			"the reason it stays. A controller ruling on " + priorID + " also settles it.",
	}
}

// overBuildingReview is what one validate_completion review shows about
// over-building.
type overBuildingReview struct {
	// priorID is the over_building finding the previous complete review
	// raised, or "" when it raised none.
	priorID string
	// open reports whether this review raises an over_building finding the
	// implementer is expected to act on. One that names a pre-task finding in
	// same_as is addressed to the plan author and is not open.
	open bool
	// settled reports whether the implementer or the controller has answered
	// for the structure: a finding_responses answer on this call or an earlier
	// one, or a ruling on the companion finding.
	settled bool
}

// reviewOverBuilding reads the over-building state of one review. reviewer is
// the reviewer's findings after the ruling waiver; preTaskLinks is, by index
// into reviewer, the pre-task finding each one's same_as names.
// answeredBefore is the session's memory of an earlier answer.
func reviewOverBuilding(reviewer []verdict.Finding, preTaskLinks map[int]string, cr completionReview, answeredBefore bool) overBuildingReview {
	ob := overBuildingReview{settled: answeredBefore}
	for _, p := range cr.prior {
		if !isOverBuilding(p.Category, p.Criterion) {
			continue
		}
		if ob.priorID == "" {
			ob.priorID = p.ID
		}
		ob.settled = ob.settled || p.Response != ""
	}
	if _, ruled := cr.rulings[fingerprintOf(overBuildingCompanion(""))]; ruled {
		ob.settled = true
	}
	for i, f := range reviewer {
		if isOverBuilding(f.Category, f.Criterion) && preTaskLinks[i] == "" {
			ob.open = true
		}
	}
	return ob
}

// answersOverBuilding reports whether this call's finding_responses answered
// an over_building finding, which the session then remembers.
func answersOverBuilding(prior []prompts.PriorFinding) bool {
	for _, p := range prior {
		if isOverBuilding(p.Category, p.Criterion) && p.Response != "" {
			return true
		}
	}
	return false
}

// companion returns the finding to add when an open over_building finding is
// raised again and nobody has answered for it.
func (ob overBuildingReview) companion() (verdict.Finding, bool) {
	if !ob.open || ob.settled || ob.priorID == "" {
		return verdict.Finding{}, false
	}
	return overBuildingCompanion(ob.priorID), true
}

// ruled reports whether the call settled an over_building finding without
// cutting the structure: a controller ruling waived it, or it is open and
// answered for.
func (ob overBuildingReview) ruled(waived []verdict.WaivedFinding) bool {
	for _, w := range waived {
		if isOverBuilding(w.Category, w.Criterion) {
			return true
		}
	}
	return ob.open && ob.settled
}
```

`verifiedAtCompletion` keeps its two earlier `if` branches; only its last branch changes to the helper call. `completionReview` is the existing struct in this file: `prior` carries each stored prior finding with this call's answer, `rulings` every ruling in force by fingerprint.

- [x] **Step 5: Wire it into `validate_completion` and the row**

In `internal/mcpsrv/handlers.go`, replace

```go
	skipReported := false
	if lightweight {
```

with

```go
	skipReported, overBuildingAnswered := false, false
	if lightweight {
```

In `internal/mcpsrv/handlers.go`, replace

```go
		skipReported = state.CodesceneSkipReported
	}
```

with

```go
		skipReported = state.CodesceneSkipReported
		overBuildingAnswered = state.OverBuildingAnswered
	}
```

In `internal/mcpsrv/handlers.go`, replace

```go
	escalateIDs := markRepeats(reviewer, review.prior, review.shown)
	findings := make([]verdict.Finding, 0, len(head)+len(reviewer)+len(out.Server))
	findings = append(findings, head...)
	findings = append(findings, reviewer...)
	findings = append(findings, out.Server...)
```

with

```go
	escalateIDs := markRepeats(reviewer, review.prior, review.shown)
	// The companion sits after the reviewer's block, so it is never stored as
	// a prior finding and never shown to the next review as one.
	overBuilding := reviewOverBuilding(reviewer, preTaskLinks, review, overBuildingAnswered)
	tail := out.Server
	if companion, ok := overBuilding.companion(); ok {
		tail = append([]verdict.Finding{companion}, tail...)
	}
	findings := make([]verdict.Finding, 0, len(head)+len(reviewer)+len(tail))
	findings = append(findings, head...)
	findings = append(findings, reviewer...)
	findings = append(findings, tail...)
```

In `internal/mcpsrv/handlers.go`, replace

```go
			CodesceneSkipReported: evidencedSkip,
		}
```

with

```go
			CodesceneSkipReported: evidencedSkip,
			OverBuildingAnswered:  answersOverBuilding(review.prior),
		}
```

In `internal/mcpsrv/handlers.go`, replace

```go
		h.recordLightweightCompletionRow(args, env)
	} else {
		h.recordCompletionRow(sess, env, args.Codescene, args.FinalDiff)
	}
```

with

```go
		h.recordLightweightCompletionRow(args, env, overBuilding.ruled(waived))
	} else {
		h.recordCompletionRow(sess, env, args.Codescene, args.FinalDiff, overBuilding.ruled(waived))
	}
```

`preTaskLinks` is the existing map built just above `markRepeats`, and `waived` the existing result of `waiveRuled`. The session's stored prior findings are still `env.Findings[len(head) : len(head)+len(reviewer)]`, and the pre-task `same_as` links are still restored at `len(head)+i`: the companion sits after that block, so neither index moves. In lightweight mode `review.prior` is empty and `overBuildingAnswered` false, so there is never a companion and only a waived finding counts as ruled.

In `internal/mcpsrv/plan_run_rows.go`, replace

```go
// recordCheckpointRow increments
```

with

```go
// countOverBuildingRuled wraps a row update so it also counts a call whose
// over_building finding was settled by an answer or a ruling.
func countOverBuildingRuled(update func(*planrun.TaskRow), ruled bool) func(*planrun.TaskRow) {
	if !ruled {
		return update
	}
	return func(row *planrun.TaskRow) {
		update(row)
		row.OverBuildingRuled++
	}
}

// recordCheckpointRow increments
```

In `internal/mcpsrv/plan_run_rows.go`, replace

```go
func (h *handlers) recordCompletionRow(sess *session.Session, env Envelope, cs *codescene.Digest, finalDiff string) {
	if sess.PlanRunID == "" {
		return
	}
	if row, ok := h.deps.PlanRuns.UpdateRow(sess.PlanRunID, sess.ID, completionRowUpdate(env, cs, finalDiff)); ok {
```

with

```go
func (h *handlers) recordCompletionRow(sess *session.Session, env Envelope, cs *codescene.Digest, finalDiff string, overBuildingRuled bool) {
	if sess.PlanRunID == "" {
		return
	}
	update := countOverBuildingRuled(completionRowUpdate(env, cs, finalDiff), overBuildingRuled)
	if row, ok := h.deps.PlanRuns.UpdateRow(sess.PlanRunID, sess.ID, update); ok {
```

In `internal/mcpsrv/plan_run_rows.go`, replace

```go
func (h *handlers) recordLightweightCompletionRow(args ValidateCompletionArgs, env Envelope) {
```

with

```go
func (h *handlers) recordLightweightCompletionRow(args ValidateCompletionArgs, env Envelope, overBuildingRuled bool) {
```

In `internal/mcpsrv/plan_run_rows.go`, replace

```go
	if row, ok := h.deps.PlanRuns.UpsertLite(args.PlanRunID, ref, completionRowUpdate(env, args.Codescene, args.FinalDiff)); ok {
```

with

```go
	update := countOverBuildingRuled(completionRowUpdate(env, args.Codescene, args.FinalDiff), overBuildingRuled)
	if row, ok := h.deps.PlanRuns.UpsertLite(args.PlanRunID, ref, update); ok {
```

- [x] **Step 6: Run the packages**

Run: `gofmt -l internal/ scorecard/ && go test -race ./internal/session/... ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go build ./...)`
Expected: no `gofmt` output; `ok` four times; no build output.

- [x] **Step 7: Documentation and changelog**

In `README.md`, replace

```markdown
holds the diff to as `quality` / `over_building` (always `minor`).
```

with

```markdown
holds the diff to as `quality` / `over_building` (always `minor`). From 0.27.0 an `over_building` finding raised on two consecutive `validate_completion` calls, with no `finding_responses` answer and no controller ruling, gains a `major` `unaddressed_finding` companion (`criterion: over_building`): cut the structure, or answer the finding with the reason it stays.
```

In `README.md`, replace

```markdown
 and `lines_added` / `lines_removed` (the size of the most recent diff a call sent); and a header
```

with

```markdown
, `lines_added` / `lines_removed` (the size of the most recent diff a call sent) and `over_building_ruled` (how many of those calls had an `over_building` finding settled by an answer or a ruling instead of a fix); and a header
```

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- An `over_building` finding raised on two consecutive `validate_completion` calls, with no `finding_responses` entry answering it and no controller ruling covering it, gains a companion `major` `unaddressed_finding` with `criterion: over_building`, which moves the verdict and tells the implementer to cut the structure or answer the finding with the reason it stays. With an answer the finding stays `minor`, on that call and on the session's later ones. A finding the reviewer links to a pre-task `over_building` finding is addressed to the plan author and draws no companion.
```

In `CHANGELOG.md`, append as the last bullet under `### Added` of `## [0.27.0] - 2026-10-04`:

```markdown
- `runs.jsonl` task snapshots and `plan-runs.jsonl` rows carry `over_building_ruled`: how many of the task's `validate_completion` calls had an `over_building` finding settled by an answer or a controller ruling instead of by a fix.
```

- [x] **Step 8: Commit**

```bash
git add internal/session/session.go internal/session/store.go internal/session/store_test.go internal/mcpsrv/finding_rulings.go internal/mcpsrv/handlers.go internal/mcpsrv/plan_run_rows.go internal/mcpsrv/run_snapshots.go internal/mcpsrv/over_building_teeth_test.go internal/planrun/planrun.go scorecard/records.go README.md CHANGELOG.md
git commit -m "feat(completion): an over_building finding raised again unanswered gains a major companion"
```

```json:metadata
{"files": ["internal/session/session.go", "internal/session/store.go", "internal/session/store_test.go", "internal/mcpsrv/finding_rulings.go", "internal/mcpsrv/handlers.go", "internal/mcpsrv/plan_run_rows.go", "internal/mcpsrv/run_snapshots.go", "internal/mcpsrv/over_building_teeth_test.go", "internal/planrun/planrun.go", "scorecard/records.go", "README.md", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/session/... ./internal/mcpsrv/... ./internal/planrun/... ./scorecard/... && (cd gnome-topbar/daemon && go build ./...)", "acceptanceCriteria": ["an over_building finding raised again unanswered gains a major unaddressed_finding companion that moves the verdict to warn, on every such call", "the companion is not stored as a prior finding", "an answer keeps the finding minor on that call and on later calls that do not resend it", "a ruling on the finding or on the companion stops the companion", "a finding linked by same_as to a pre-task over_building finding draws no companion", "overBuildingReview.ruled and isOverBuilding behave as specified", "the task row and snapshot count over_building_ruled, omitted when zero", "README and CHANGELOG describe it and the daemon builds"], "modelTier": "standard"}
```

---

### Task 4: `check_progress` asks for correctness defects

**Goal:** `mid.tmpl` carries a short form of the completion prompt's Correctness section, so a checkpoint reports defects in the code written so far, while keeping its rule against style findings.

**Files:**
- Modify: `internal/prompts/templates/mid.tmpl`
- Modify: `internal/prompts/testdata/mid_basic.golden` (regenerated)
- Modify: `README.md`, `CHANGELOG.md`
- Test: `internal/prompts/prompts_test.go`

**Acceptance Criteria:**
- [ ] The rendered `check_progress` prompt contains `### Correctness` after the sentence `DO NOT critique code style or polish at this stage.` and before `### Over-building`.
- [ ] It names `` `category: correctness` ``, and contains `Do not speculate about code that was not submitted` and `unfinished, not wrong`.
- [ ] It does not contain `test_adequacy`.
- [ ] `git diff --stat internal/prompts/testdata/` after `-update` lists only `mid_basic.golden`.
- [ ] `README.md`'s `check_progress` bullet and `CHANGELOG.md` `[0.27.0]` `### Changed` describe the change.

**Verify:** `go test -race ./internal/prompts/...` → `ok`

**Steps:**

- [x] **Step 1: Write the failing test**

Append to `internal/prompts/prompts_test.go`:

```go
func TestRenderMid_AsksForCorrectnessAndKeepsTheStyleRule(t *testing.T) {
	out, err := RenderMid(MidInput{
		Spec:      sampleSpec(),
		WorkingOn: "writing the handler",
		Files:     []File{{Path: "handlers/health.go", Content: "package handlers\n"}},
	})
	require.NoError(t, err)
	style := strings.Index(out.User, "DO NOT critique code style or polish at this stage.")
	correctness := strings.Index(out.User, "### Correctness")
	overBuilding := strings.Index(out.User, "### Over-building")
	require.NotEqual(t, -1, style, "the rule against style findings must stay")
	require.NotEqual(t, -1, correctness, "prompt must have a Correctness section")
	require.NotEqual(t, -1, overBuilding)
	assert.Less(t, style, correctness)
	assert.Less(t, correctness, overBuilding)
	assert.Contains(t, out.User, "`category: correctness`")
	assert.Contains(t, out.User, "Do not speculate about code that was not submitted")
	assert.Contains(t, out.User, "unfinished, not wrong")
	assert.NotContains(t, out.User, "test_adequacy", "test adequacy is judged at completion only")
}
```

- [x] **Step 2: Run it and see it fail**

Run: `go test ./internal/prompts/... -run TestRenderMid_AsksForCorrectness`
Expected: FAIL, `prompt must have a Correctness section`.

- [x] **Step 3: Add the section**

In `internal/prompts/templates/mid.tmpl`, replace

```text
DO NOT critique code style or polish at this stage. Style is noise mid-task.
```

with

```text
DO NOT critique code style or polish at this stage. Style is noise mid-task.

### Correctness

Drift is one question; whether the code written so far is right is another, and a defect is cheapest to fix now. In the files below, look for:

- a wrong result for an input the task covers: empty, zero, negative, duplicate, missing, or at a boundary;
- an error, a failure return or a nil / None / undefined value that is dropped, or handled so that the caller carries on with bad data;
- shared state read or written without the protection the surrounding code uses;
- a file, lock, connection or transaction that is not released on every path;
- a caller and a callee changed on one side only.

Report only what the submitted files show: quote the offending lines in `evidence` with their path, state the input or sequence that triggers the defect and the wrong outcome, and give the fix in `suggestion`. Do not speculate about code that was not submitted. The files are whole files, so report a defect only in code the `Working on` summary or an acceptance criterion ties to this task. This is a checkpoint: a function not yet written or a case not yet handled is unfinished, not wrong, unless `Working on` says it is done. A defect is not style; the rule above still holds.

Emit each defect as its own finding with `category: correctness`. Set `criterion` to the verbatim AC text when the defect breaks that criterion, and to `correctness` otherwise. Severity: `critical` for data loss, a security hole, or a crash on the main path; `major` for a wrong result or an unhandled failure on a path the task's Goal or acceptance criteria cover; `minor` for a defect on a path the task does not exercise.
```

- [x] **Step 4: Regenerate the golden and read the diff**

Run: `go test ./internal/prompts/... -update && git diff --stat internal/prompts/testdata/`
Expected: one file, `mid_basic.golden`. Any `post_*`, `pre_*`, `plan_*`, `prime_*`, `extract_*`, `lean_*` or `worker_*` golden in the list means the edit landed in the wrong template: revert and redo.

Run: `go test -race ./internal/prompts/...`
Expected: `ok`.

- [x] **Step 5: Documentation and changelog**

In `README.md`, replace

```markdown
Catches scope drift, untouched ACs, and unaddressed prior findings.
```

with

```markdown
Catches scope drift, untouched ACs, unaddressed prior findings and, in the code written so far, correctness defects.
```

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- `check_progress` asks the reviewer for correctness defects in the files submitted so far, reported as `correctness`, using a short form of the completion review's section. Unfinished work is not a defect, and the rule against style findings mid-task stays.
```

- [x] **Step 6: Commit**

```bash
git add internal/prompts/templates/mid.tmpl internal/prompts/testdata/mid_basic.golden internal/prompts/prompts_test.go README.md CHANGELOG.md
git commit -m "feat(prompts): check_progress looks for correctness defects"
```

```json:metadata
{"files": ["internal/prompts/templates/mid.tmpl", "internal/prompts/testdata/mid_basic.golden", "internal/prompts/prompts_test.go", "README.md", "CHANGELOG.md"], "verifyCommand": "go test -race ./internal/prompts/...", "acceptanceCriteria": ["the mid prompt has a Correctness section between the style rule and Over-building", "it names category: correctness and the no-speculation and unfinished-not-wrong rules", "it does not mention test_adequacy", "only mid_basic.golden changes", "README and CHANGELOG describe the change"], "modelTier": "mechanical"}
```

---

### Task 5: The `check-progress-nudge` hook

**Goal:** A `PostToolUse` hook on `Edit|Write|NotebookEdit` asks a task, once, to call `check_progress` when it reaches its edit threshold without one, and allows on every error.

**Files:**
- Create: `plugin/anti-tangent-guard/hooks/check_progress_nudge.py`
- Create: `plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py`
- Create: `plugin/anti-tangent-guard/hooks/check-progress-nudge` (mode 755)
- Modify: `plugin/anti-tangent-guard/hooks/hooks.json`
- Modify: `.github/workflows/ci.yml`
- Modify: `CHANGELOG.md`

**Acceptance Criteria:**
- [ ] `own_transcript` returns `transcript_path` for a payload with no `agent_id`; for a subagent, `<parent stem>/subagents/agent-<id>.jsonl` when that file exists, else `transcript_path` itself when its basename starts with `agent-`, else `""`; and `""` for a missing, non-string or unusable path. It never returns the parent's transcript for a subagent.
- [ ] `scan(lines)` returns `None` when the transcript has no `mcp__anti-tangent__validate_task_spec` tool_use, and otherwise the window after the **last** one: the count of `Edit` / `Write` / `NotebookEdit` tool_uses, whether a `check_progress` call and whether a `validate_completion` call follow it. Text that names a tool is not a call; malformed lines and entries are skipped.
- [ ] `decide` returns exit 3 `no-task` with no window, exit 3 `completing` after a `validate_completion` call, exit 0 `edits=N` below the threshold or once `check_progress` was called, exit 2 `edits=N` when the task is due and the claim is new, exit 0 when the task was already asked, and exit 3 `no-state` when the claim raises `OSError`.
- [ ] `claim` creates `progress-asked-<16 hex>` with `O_CREAT|O_EXCL|O_NOFOLLOW`: true for the first caller per (transcript path, spec call id), false for every later one, false without creating anything when a symlink sits at the path, `OSError` when the directory is missing.
- [ ] `threshold` returns the value for a whole number above zero and `10` for `None`, `""`, `abc`, `0`, `-4`, `2.5`.
- [ ] The wrapper exits 0 without starting Python when `ANTI_TANGENT_PROGRESS_GUARD=0`, when `python3` is absent, and when the body is unreadable; maps body exit 2 to exit 2, with the body's stderr passed through, only when the body's stdout starts `edits=` (an interpreter that itself exits 2 is traced `error | python-exit=2` and allowed), and every other exit to 0; and writes one trace line `<ts> | s=<session>[.<agent>] | progress | <event> | <detail>`.
- [ ] Run by hand against a subagent transcript holding a spec call and ten edits: first run exits 2 and prints `CHECKPOINT DUE: call check_progress`; second run exits 0; the trace shows `s=parent.a1 | progress | nudge | edits=10` then `s=parent.a1 | progress | pass | edits=10`.
- [ ] `hooks.json` registers the hook under `PostToolUse` with matcher `Edit|Write|NotebookEdit` and `"timeout": 10`, and is valid JSON. CI's `python-suites` job runs the new test file.

**Verify:** `python3 -B plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py && jq -e . plugin/anti-tangent-guard/hooks/hooks.json >/dev/null` → `Ran 27 tests` … `OK`

**Steps:**

- [x] **Step 1: Write the failing tests**

Create `plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py`:

```python
import json
import os
import tempfile
import unittest

from check_progress_nudge import (
    COMPLETION_TOOL, DEFAULT_EDITS, PROGRESS_TOOL, SPEC_TOOL, claim, decide, own_transcript, scan, threshold)


def tool_use(name, call_id="t1"):
    return json.dumps({"type": "assistant", "message": {"role": "assistant", "content": [
        {"type": "tool_use", "id": call_id, "name": name, "input": {}}]}})


def user(text):
    return json.dumps({"type": "user", "message": {"role": "user", "content": text}})


def edits(n):
    return [tool_use("Edit", "e%d" % i) for i in range(n)]


class ThresholdTest(unittest.TestCase):
    def test_a_whole_number_above_zero_is_used(self):
        self.assertEqual(threshold("3"), 3)
        self.assertEqual(threshold(" 25 "), 25)

    def test_an_unusable_value_falls_back_to_the_default(self):
        for raw in (None, "", "abc", "0", "-4", "2.5"):
            self.assertEqual(threshold(raw), DEFAULT_EDITS, raw)


class ScanTest(unittest.TestCase):
    def test_no_spec_call_is_no_task(self):
        self.assertIsNone(scan(edits(12)))
        self.assertIsNone(scan([]))

    def test_counts_gated_edits_after_the_spec_call_only(self):
        lines = edits(4) + [tool_use(SPEC_TOOL, "spec1")] + [
            tool_use("Edit"), tool_use("Read"), tool_use("Write"), tool_use("NotebookEdit"), tool_use("Bash")]
        win = scan(lines)
        self.assertEqual(win.edits, 3)
        self.assertEqual(win.key, "spec1")
        self.assertFalse(win.checked)
        self.assertFalse(win.completing)

    def test_the_last_spec_call_opens_a_new_window(self):
        lines = ([tool_use(SPEC_TOOL, "spec1")] + edits(7) + [tool_use(PROGRESS_TOOL), tool_use(COMPLETION_TOOL)]
                 + [tool_use(SPEC_TOOL, "spec2")] + edits(2))
        win = scan(lines)
        self.assertEqual((win.key, win.edits, win.checked, win.completing), ("spec2", 2, False, False))

    def test_check_progress_and_validate_completion_are_seen(self):
        win = scan([tool_use(SPEC_TOOL)] + edits(2) + [tool_use(PROGRESS_TOOL)])
        self.assertTrue(win.checked)
        win = scan([tool_use(SPEC_TOOL)] + edits(2) + [tool_use(COMPLETION_TOOL)] + edits(1))
        self.assertTrue(win.completing)
        self.assertEqual(win.edits, 3)

    def test_text_naming_a_tool_is_not_a_call(self):
        lines = [user('call ' + SPEC_TOOL + ' via "tool_use"')] + edits(3)
        self.assertIsNone(scan(lines))
        win = scan([tool_use(SPEC_TOOL)] + [user('"tool_use" ' + PROGRESS_TOOL)])
        self.assertFalse(win.checked)

    def test_a_tool_result_is_not_a_call(self):
        result = json.dumps({"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "t1", "content": PROGRESS_TOOL}]}})
        win = scan([tool_use(SPEC_TOOL), result])
        self.assertFalse(win.checked)

    def test_several_tool_uses_in_one_entry_all_count(self):
        batch = json.dumps({"type": "assistant", "message": {"content": [
            {"type": "tool_use", "id": "a", "name": "Edit", "input": {}},
            {"type": "text", "text": "and"},
            {"type": "tool_use", "id": "b", "name": "Write", "input": {}}]}})
        self.assertEqual(scan([tool_use(SPEC_TOOL), batch]).edits, 2)

    def test_malformed_lines_and_entries_are_skipped(self):
        lines = ['not json "tool_use"', "{", tool_use(SPEC_TOOL),
                 json.dumps({"type": "assistant", "message": "tool_use"}),
                 json.dumps({"type": "assistant", "message": {"content": "tool_use"}}),
                 json.dumps(["tool_use"]),
                 tool_use("Edit")]
        self.assertEqual(scan(lines).edits, 1)

    def test_a_spec_call_without_an_id_is_keyed_by_its_line(self):
        entry = json.dumps({"type": "assistant", "message": {"content": [
            {"type": "tool_use", "name": SPEC_TOOL, "input": {}}]}})
        self.assertEqual(scan([user("x"), entry]).key, "line-1")


class DecideTest(unittest.TestCase):
    def win(self, lines):
        return scan(lines)

    def never(self):
        raise AssertionError("claim must not be tried")

    def test_no_task_skips(self):
        self.assertEqual(decide(None, 10, self.never), (3, "no-task"))

    def test_a_task_in_its_completion_loop_skips(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(12) + [tool_use(COMPLETION_TOOL)])
        self.assertEqual(decide(win, 10, self.never), (3, "completing"))

    def test_below_the_threshold_passes(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(9))
        self.assertEqual(decide(win, 10, self.never), (0, "edits=9"))

    def test_a_checked_task_passes_however_many_edits(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(5) + [tool_use(PROGRESS_TOOL)] + edits(20))
        self.assertEqual(decide(win, 10, self.never), (0, "edits=25"))

    def test_the_threshold_edit_asks_when_the_claim_is_new(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(10))
        self.assertEqual(decide(win, 10, lambda: True), (2, "edits=10"))

    def test_an_edit_past_the_threshold_still_asks_when_nothing_asked_yet(self):
        # Edits sent in one turn can carry the count past the threshold
        # without any hook having seen it exactly.
        win = self.win([tool_use(SPEC_TOOL)] + edits(13))
        self.assertEqual(decide(win, 10, lambda: True), (2, "edits=13"))

    def test_a_task_already_asked_passes(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(11))
        self.assertEqual(decide(win, 10, lambda: False), (0, "edits=11"))

    def test_an_ask_that_cannot_be_recorded_is_not_made(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(10))

        def broken():
            raise OSError("read-only")
        self.assertEqual(decide(win, 10, broken), (3, "no-state"))


class OwnTranscriptTest(unittest.TestCase):
    def test_the_main_session_reads_its_own_transcript(self):
        self.assertEqual(own_transcript({"transcript_path": "/p/s1.jsonl"}), "/p/s1.jsonl")

    def test_a_subagent_reads_the_file_beside_the_parent(self):
        with tempfile.TemporaryDirectory() as tmp:
            parent = os.path.join(tmp, "s1.jsonl")
            sub = os.path.join(tmp, "s1", "subagents", "agent-a1.jsonl")
            os.makedirs(os.path.dirname(sub))
            open(sub, "w").close()
            self.assertEqual(own_transcript({"transcript_path": parent, "agent_id": "a1"}), sub)

    def test_a_subagent_handed_its_own_transcript_reads_it(self):
        path = "/p/s1/subagents/agent-a1.jsonl"
        self.assertEqual(own_transcript({"transcript_path": path, "agent_id": "a1"}), path)

    def test_a_subagent_with_no_transcript_of_its_own_never_reads_the_parent(self):
        self.assertEqual(own_transcript({"transcript_path": "/p/s1.jsonl", "agent_id": "a1"}), "")

    def test_a_payload_without_a_usable_path_is_refused(self):
        for data in ({}, {"transcript_path": ""}, {"transcript_path": 5}, {"transcript_path": "/p/s1.jsonl", "agent_id": "../x"}):
            self.assertEqual(own_transcript(data), "", data)


class ClaimTest(unittest.TestCase):
    def test_only_the_first_claim_for_a_task_wins(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertFalse(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec2"), "a new task in the same transcript")
            self.assertTrue(claim(tmp, "/p/s2.jsonl", "spec1"), "the same call id in another transcript")

    def test_a_missing_directory_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(OSError):
                claim(os.path.join(tmp, "absent"), "/p/s1.jsonl", "spec1")

    def test_a_symlink_at_the_stamp_path_is_not_followed(self):
        if not hasattr(os, "O_NOFOLLOW"):
            self.skipTest("no O_NOFOLLOW on this platform")
        with tempfile.TemporaryDirectory() as tmp:
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec1"))
            stamp = os.path.join(tmp, os.listdir(tmp)[0])
            os.unlink(stamp)
            target = os.path.join(tmp, "target")
            os.symlink(target, stamp)
            self.assertFalse(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertFalse(os.path.exists(target))


if __name__ == "__main__":
    unittest.main()
```

- [x] **Step 2: Run them and see them fail**

Run: `python3 -B plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py`
Expected: `ModuleNotFoundError: No module named 'check_progress_nudge'`.

- [x] **Step 3: Write the body**

Create `plugin/anti-tangent-guard/hooks/check_progress_nudge.py`:

```python
"""PostToolUse body: once per task, ask for a check_progress call when the
task reaches its Nth edit without one.

Reads the hook payload as JSON on stdin; the wrapper consumes the hook's own
stdin and re-feeds it here, so nothing else in this process may read stdin.
Exit 0 = nothing to ask, 2 = ask (the message is on stderr), 3 = not decided
(no task in the transcript, a task already in its completion loop, no readable
transcript, an ungated tool, no place to record the ask). The wrapper maps
every other exit to allow. One word for the trace line goes to stdout.

The transcript read is the session's own; own_transcript says how it is found.

A task is the window after the LAST validate_task_spec call. The edit that
triggered this hook has already happened, so exit 2 undoes nothing: it hands
the message to the model.
"""
import hashlib
import json
import os
import sys

# The wrapper runs this under python3 -I, which leaves the script's own
# directory off sys.path, so the sibling module is put back by hand.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from check_task_start import as_list, subagent_transcript  # noqa: E402

SPEC_TOOL = "mcp__anti-tangent__validate_task_spec"
PROGRESS_TOOL = "mcp__anti-tangent__check_progress"
COMPLETION_TOOL = "mcp__anti-tangent__validate_completion"
GATED_TOOLS = ("Edit", "Write", "NotebookEdit")
DEFAULT_EDITS = 10

NUDGE_MESSAGE = """CHECKPOINT DUE: call check_progress

This task has made {edits} edits since validate_task_spec and has not called
check_progress. Call mcp__anti-tangent__check_progress now with the session_id
validate_task_spec returned, a one-sentence working_on, and changed_files
holding every file changed so far. Act on its findings, then carry on.

The edit you just made is kept. This is asked once per task.

(Disable: ANTI_TANGENT_PROGRESS_GUARD=0. Threshold: ANTI_TANGENT_PROGRESS_EDITS,
default {default}. Trace: {trace})
"""


def threshold(raw):
    """Return the edit count that triggers the ask: raw when it is a whole
    number above zero, else the default."""
    try:
        n = int(str(raw).strip())
    except (TypeError, ValueError):
        return DEFAULT_EDITS
    return n if n > 0 else DEFAULT_EDITS


class Window:
    """What the transcript shows since the last validate_task_spec call."""

    def __init__(self):
        self.key = ""
        self.edits = 0
        self.checked = False
        self.completing = False


def scan(lines):
    """Return the Window after the last validate_task_spec tool_use in an
    iterable of transcript lines, or None when there is no such call.

    Only assistant tool_use parts count; text naming a tool is not a call. A
    line without a quoted "tool_use" is skipped before it is parsed, which
    keeps a long transcript cheap: this runs after every edit, and the quotes
    leave out the far more common tool_use_id of a tool result. A malformed
    line or entry is skipped rather than raised on.
    """
    win = None
    for number, line in enumerate(lines):
        if '"tool_use"' not in line:
            continue
        try:
            entry = json.loads(line)
        except Exception:
            continue
        if not isinstance(entry, dict) or entry.get("type") != "assistant":
            continue
        msg = entry.get("message")
        if not isinstance(msg, dict):
            continue
        for part in as_list(msg.get("content")):
            if not isinstance(part, dict) or part.get("type") != "tool_use":
                continue
            name = part.get("name")
            if name == SPEC_TOOL:
                win = Window()
                call_id = part.get("id")
                win.key = call_id if isinstance(call_id, str) and call_id else "line-%d" % number
            elif win is None:
                continue
            elif name == PROGRESS_TOOL:
                win.checked = True
            elif name == COMPLETION_TOOL:
                win.completing = True
            elif name in GATED_TOOLS:
                win.edits += 1
    return win


def own_transcript(data):
    """Return the path of the transcript of the session the payload came from,
    or "" when it cannot be told.

    With no agent_id the payload is the main session's and transcript_path is
    its transcript. Inside a subagent, transcript_path is the parent's and the
    subagent's own sits beside it at <parent stem>/subagents/agent-<id>.jsonl.
    When that file is absent but transcript_path itself names an agent
    transcript, the host has handed over the subagent's own path and it is
    used as it is. Anything else is "": reading the parent's transcript for a
    subagent would judge the wrong session.
    """
    parent = data.get("transcript_path")
    if not isinstance(parent, str) or not parent:
        return ""
    if not data.get("agent_id"):
        return parent
    derived = subagent_transcript(data)
    if derived and os.path.isfile(derived):
        return derived
    if os.path.basename(parent).startswith("agent-"):
        return parent
    return ""


def claim(directory, transcript, key):
    """Record that this task has been asked, and return True when this call is
    the one that recorded it. False means an earlier call already asked. Raises
    OSError when the record cannot be written.

    The exclusive create is what makes the ask happen once: edits sent in one
    assistant turn run their hooks at the same time and all read the same
    count, and only one of them creates the file.
    """
    digest = hashlib.sha256(("%s\0%s" % (transcript, key)).encode("utf-8")).hexdigest()[:16]
    stamp = os.path.join(directory, "progress-asked-%s" % digest)
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    try:
        os.close(os.open(stamp, flags, 0o600))
    except FileExistsError:
        return False
    return True


def decide(win, limit, try_claim):
    """Return (exit code, trace word) for a Window. try_claim() is called only
    when the task is due, and follows claim()'s contract."""
    if win is None:
        return 3, "no-task"
    if win.completing:
        return 3, "completing"
    detail = "edits=%d" % win.edits
    if win.checked or win.edits < limit:
        return 0, detail
    try:
        first = try_claim()
    except OSError:
        # An ask that cannot be recorded would repeat after every edit.
        return 3, "no-state"
    return (2, detail) if first else (0, detail)


def main():
    try:
        data = json.loads(sys.stdin.read())
    except Exception:
        return 3
    if not isinstance(data, dict) or (data.get("tool_name") or "") not in GATED_TOOLS:
        return 3
    path = own_transcript(data)
    if not path:
        return 3
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            win = scan(fh)
    except OSError:
        return 3
    trace_log = os.environ.get("ATG_TRACE_LOG", "")
    limit = threshold(os.environ.get("ANTI_TANGENT_PROGRESS_EDITS"))

    def try_claim():
        if not trace_log:
            raise OSError("no state directory")
        return claim(os.path.dirname(trace_log), path, win.key)

    code, detail = decide(win, limit, try_claim)
    sys.stdout.write(detail)
    if code == 2:
        sys.stderr.write(NUDGE_MESSAGE.format(edits=win.edits, default=DEFAULT_EDITS, trace=trace_log))
    return code


if __name__ == "__main__":
    sys.exit(main())
```

Three things here are load-bearing and must not be simplified away:

1. The `sys.path.insert` before the sibling import. The wrapper runs the body with `python3 -I`, which drops the script's directory from `sys.path`; without the insert the import fails, the wrapper maps the failure to allow, and the hook silently never asks.
2. `decide` asks when `edits >= limit`, not `== limit`, and relies on `claim` for "once". Edits sent in one assistant turn are all in the transcript before any of their hooks run, so the count can pass the threshold without any hook seeing it equal.
3. `claim` raising makes `decide` return 3, not 2. An ask that cannot be recorded would repeat after every edit.
4. `own_transcript` returns `""` for a subagent whose own transcript cannot be found. Falling back to the parent's would count the controller's edits against the subagent.

- [x] **Step 4: Run the tests**

Run: `python3 -B plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py`
Expected: `Ran 27 tests` and `OK`.

- [x] **Step 5: Write the wrapper**

Create `plugin/anti-tangent-guard/hooks/check-progress-nudge`:

```bash
#!/usr/bin/env bash
# PostToolUse hook on Edit/Write/NotebookEdit: once per task, at the edit that
# brings the task to its threshold with no check_progress call, hand the model
# a message asking for one. The edit has already happened, so exit 2 refuses
# nothing. The Python body owns the transcript work; this wrapper owns the
# kill switch, the trace log and the exit-code mapping, the same split as
# check-task-start. Every failure allows: a hook that runs after every edit
# must never be the reason an edit is reported as failed.
set -uo pipefail

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
TRACE_LOG="${ANTI_TANGENT_GUARD_TRACE_LOG:-/tmp/claude-hooks/anti-tangent-guard.log}"
# Mode 700 on creation: the default lives in a world-writable directory.
mkdir -p -m 700 "$(dirname "$TRACE_LOG")" 2>/dev/null || true
ATG_SESSION="-"

atg_rotate_trace() {
    local max=${ANTI_TANGENT_GUARD_TRACE_MAX_BYTES:-1048576} size
    [[ "$max" =~ ^[0-9]+$ ]] || max=1048576
    [[ -L "$TRACE_LOG" ]] && return 0
    [[ -f "$TRACE_LOG" ]] || return 0
    size=$(wc -c < "$TRACE_LOG" 2>/dev/null) || return 0
    [[ "$size" -gt "$max" ]] || return 0
    mv -f "$TRACE_LOG" "$TRACE_LOG.1" 2>/dev/null || true
    return 0
}
atg_rotate_trace

trace() {
    # Never append through a symlink, and scrub the field separators out of
    # every argument at the sink: a newline or "|" reaching printf would split
    # one record into two.
    [[ -L "$TRACE_LOG" ]] && return 0
    local ev=${1:-?} detail=${2:-}
    ev=${ev//[$'\r\n|']/}
    detail=${detail//[$'\r\n|']/}
    printf '%s | s=%s | progress | %s%s\n' \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$ATG_SESSION" "${ev:-?}" "${detail:+ | $detail}" \
        >> "$TRACE_LOG" 2>/dev/null || true
}

[[ "${ANTI_TANGENT_PROGRESS_GUARD:-1}" == "0" ]] && { trace "skip" "guard=0"; exit 0; }
command -v python3 >/dev/null 2>&1 || { trace "skip" "no-python3"; exit 0; }
[[ -r "$PLUGIN_ROOT/hooks/check_progress_nudge.py" ]] || { trace "skip" "no-body"; exit 0; }

ATG_INPUT="$(cat)"
# The session column carries the subagent as well as the session: subagents of
# one session run at the same time, and their edit counts are only readable in
# the trace when each line says whose it is.
ATG_SESSION_RAW=$(printf '%s' "$ATG_INPUT" | jq -r '
    def short: tostring | gsub("[^A-Za-z0-9_-]"; "") | .[0:8];
    ((.session_id // "") | short) as $s | ((.agent_id // "") | short) as $a
    | if $a == "" then $s else $s + "." + $a end' 2>/dev/null) || ATG_SESSION_RAW=""
[[ -n "$ATG_SESSION_RAW" ]] && ATG_SESSION="$ATG_SESSION_RAW"

# The body prints one word for the trace on stdout and the message for the
# model on stderr, which passes through untouched.
ATG_DETAIL=$(printf '%s' "$ATG_INPUT" | ATG_TRACE_LOG="$TRACE_LOG" python3 -I -B "$PLUGIN_ROOT/hooks/check_progress_nudge.py")
status=$?
ATG_DETAIL=${ATG_DETAIL//[^a-z0-9=-]/}
ATG_DETAIL=${ATG_DETAIL:0:32}
case "$status" in
    0) trace "pass" "$ATG_DETAIL"; exit 0 ;;
    2)
        # The interpreter itself exits 2 when it cannot start the body. Only
        # the body's own exit 2 comes with an edit count, and only that asks.
        [[ "$ATG_DETAIL" == edits=* ]] || { trace "error" "python-exit=2"; exit 0; }
        trace "nudge" "$ATG_DETAIL"; exit 2 ;;
    3) trace "skip" "${ATG_DETAIL:-not-gated}"; exit 0 ;;
    *) trace "error" "python-exit=$status"; exit 0 ;;
esac
```

Then: `chmod 755 plugin/anti-tangent-guard/hooks/check-progress-nudge`

`atg_rotate_trace` and `trace` are the same as in `check-task-start`, with the tag `progress`. The `edits=*` test on exit 2 is what keeps an interpreter failure (Python exits 2 when it cannot open the script) from being read as an ask. `status=$?` after the command substitution is the body's exit status, because the body is the last command of the pipeline inside it.

- [x] **Step 6: Register it**

In `plugin/anti-tangent-guard/hooks/hooks.json`, replace

```json
        "matcher": "TaskUpdate",
        "hooks": [{ "type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/check-task-complete\"" }]
      }
    ],
```

with

```json
        "matcher": "TaskUpdate",
        "hooks": [{ "type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/check-task-complete\"" }]
      },
      {
        "matcher": "Edit|Write|NotebookEdit",
        "hooks": [{ "type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/check-progress-nudge\"", "timeout": 10 }]
      }
    ],
```

Run: `jq -e . plugin/anti-tangent-guard/hooks/hooks.json >/dev/null && echo valid`
Expected: `valid`.

- [x] **Step 7: Exercise the wrapper by hand**

```bash
T=$(mktemp -d)
python3 - "$T" <<'PY'
import json, os, sys
t = sys.argv[1]
def call(name, i):
    return json.dumps({"type": "assistant", "message": {"content": [
        {"type": "tool_use", "id": i, "name": name, "input": {}}]}})
os.makedirs(t + "/parent/subagents")
lines = [json.dumps({"type": "user", "message": {"content": "go"}}),
         call("mcp__anti-tangent__validate_task_spec", "s1")]
lines += [call("Edit", "e%d" % i) for i in range(10)]
open(t + "/parent/subagents/agent-a1.jsonl", "w").write("\n".join(lines) + "\n")
PY
IN='{"tool_name":"Edit","tool_input":{},"transcript_path":"'$T'/parent.jsonl","session_id":"parent","agent_id":"a1"}'
for i in 1 2; do
  printf '%s' "$IN" | ANTI_TANGENT_GUARD_TRACE_LOG="$T/tr/log" plugin/anti-tangent-guard/hooks/check-progress-nudge
  echo "exit=$?"
done
printf '%s' "$IN" | ANTI_TANGENT_PROGRESS_GUARD=0 ANTI_TANGENT_GUARD_TRACE_LOG="$T/tr/log" plugin/anti-tangent-guard/hooks/check-progress-nudge; echo "exit=$?"
printf 'garbage' | ANTI_TANGENT_GUARD_TRACE_LOG="$T/tr/log" plugin/anti-tangent-guard/hooks/check-progress-nudge; echo "exit=$?"
cat "$T/tr/log"; rm -rf "$T"
```

Expected: the `CHECKPOINT DUE: call check_progress` message and `exit=2`; then `exit=0` three times; and a trace of

```text
<ts> | s=parent.a1 | progress | nudge | edits=10
<ts> | s=parent.a1 | progress | pass | edits=10
<ts> | s=- | progress | skip | guard=0
<ts> | s=- | progress | skip | not-gated
```

- [x] **Step 8: CI and changelog**

In `.github/workflows/ci.yml`, replace

```yaml
        run: python3 -B plugin/anti-tangent-guard/hooks/check_task_start_test.py
```

with

```yaml
        run: python3 -B plugin/anti-tangent-guard/hooks/check_task_start_test.py

      - name: Progress reminder
        run: python3 -B plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py
```

In `CHANGELOG.md`, append as the last bullet under `### Added` of `## [0.27.0] - 2026-10-04`:

```markdown
- `anti-tangent-guard` 0.7.0 gains a fourth hook, `check-progress-nudge`: a `PostToolUse` hook on `Edit`/`Write`/`NotebookEdit` that asks a task for a `check_progress` call, once, when it has made ten edits since `validate_task_spec` without one. It reads the session's own transcript (a subagent's, or the main session's), refuses nothing, and is silent with no `validate_task_spec` call and after `validate_completion`. `ANTI_TANGENT_PROGRESS_EDITS` sets the threshold and `ANTI_TANGENT_PROGRESS_GUARD=0` turns the hook off; every failure allows. Trace lines read `progress | pass|nudge|skip | edits=N`, with the subagent in the session column.
```

- [x] **Step 9: Commit**

```bash
git add plugin/anti-tangent-guard/hooks/check_progress_nudge.py plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py plugin/anti-tangent-guard/hooks/check-progress-nudge plugin/anti-tangent-guard/hooks/hooks.json .github/workflows/ci.yml CHANGELOG.md
git commit -m "feat(guard): ask for check_progress once a task reaches ten edits"
```

```json:metadata
{"files": ["plugin/anti-tangent-guard/hooks/check_progress_nudge.py", "plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py", "plugin/anti-tangent-guard/hooks/check-progress-nudge", "plugin/anti-tangent-guard/hooks/hooks.json", ".github/workflows/ci.yml", "CHANGELOG.md"], "verifyCommand": "python3 -B plugin/anti-tangent-guard/hooks/check_progress_nudge_test.py && jq -e . plugin/anti-tangent-guard/hooks/hooks.json >/dev/null", "acceptanceCriteria": ["own_transcript finds the session's own transcript and never the parent's for a subagent", "scan returns the window after the last validate_task_spec call, or None", "decide maps the window to pass, nudge or skip, and never asks when the claim cannot be recorded", "claim is an exclusive create that only the first caller per task wins and that does not follow a symlink", "threshold falls back to 10 for an unusable value", "the wrapper honours the kill switch, allows on every error including an interpreter exit 2, and writes the progress trace line with the subagent in the session column", "a hand run asks once and then passes", "hooks.json registers the hook with a 10-second timeout and CI runs the new tests"], "modelTier": "frontier", "tierReason": "This hook runs after every Edit/Write in every session with the plugin installed. Its named failure modes reach a real user: an ask repeated after every edit, or a hook that stalls the editor. Getting 'once per task' right is a concurrency question (edits sent in one turn run their hooks at the same time against one transcript and one state file), which is where a cheaper model is measurably weaker; the review needs the same care."}
```

---

### Task 6: Eval cases for the progress reminder

**Goal:** The guard eval table drives the real `check-progress-nudge` wrapper through fifteen cases, and the suite checks the trace line.

**Files:**
- Modify: `plugin/anti-tangent-guard/evals/guard-evals.json` (15 cases appended, ids 180–194)
- Modify: `plugin/anti-tangent-guard/evals/run.sh`

Depends on Task 5.

**Acceptance Criteria:**
- [ ] `guard-evals.json` has 194 cases; ids 180–194 have `"hook": "check-progress-nudge"`; cases 1–179 are byte-identical to before; the description line ends `and the check-progress-nudge PostToolUse hook (194 cases)`.
- [ ] The cases cover: the tenth edit asks and a second run of the same task does not (`expected_exits: [2, 0]`); nine edits pass; a task that called `check_progress` passes; a task that called `validate_completion` is skipped; no `validate_task_spec` call is skipped; the main session is asked; a subagent's own transcript is read, not the parent's; the task after a completed one is asked on its own count; a new `validate_task_spec` call restarts the count; `ANTI_TANGENT_PROGRESS_GUARD=0`; an unusable `ANTI_TANGENT_PROGRESS_EDITS` falls back to ten and asks at the tenth edit; `python3` missing from `PATH`; a missing transcript; malformed stdin; a `Read` payload.
- [ ] `run.sh` has `EXPECTED_CASE_COUNT=194`, unsets `ANTI_TANGENT_PROGRESS_GUARD` and `ANTI_TANGENT_PROGRESS_EDITS`, and fails when the trace holds no `s=parent.a1 | progress | nudge | edits=10` line.
- [ ] `bash plugin/anti-tangent-guard/evals/run.sh` ends `194 passed, 0 failed, 194 total` and exits 0.

**Verify:** `bash plugin/anti-tangent-guard/evals/run.sh | tail -1` → `Total: 194 passed, 0 failed, 194 total`

**Steps:**

- [x] **Step 1: Append the cases**

The transcripts are escaped JSON inside JSON, so they are generated, not typed. Save this as `/tmp/add_progress_cases.py` (it is not committed), and run it **once** from the repository root:

```python
"""Append the check-progress-nudge eval cases to guard-evals.json.

Run once from the repository root. The cases are inserted as text before the
closing bracket, so the existing cases keep their bytes.
"""
import json

PATH = "plugin/anti-tangent-guard/evals/guard-evals.json"
SPEC = "mcp__anti-tangent__validate_task_spec"
PROGRESS = "mcp__anti-tangent__check_progress"
COMPLETION = "mcp__anti-tangent__validate_completion"


def call(name, call_id):
    return json.dumps({"type": "assistant", "message": {"content": [
        {"type": "tool_use", "id": call_id, "name": name, "input": {}}]}})


def transcript(*steps):
    """steps are tool names, or ("Edit", n) for n edits in a row."""
    lines = [json.dumps({"type": "user", "message": {"role": "user", "content": "Implement Task 3."}})]
    for i, step in enumerate(steps):
        if isinstance(step, tuple):
            lines += [call(step[0], "c%d-%d" % (i, j)) for j in range(step[1])]
        else:
            lines.append(call(step, "c%d" % i))
    return "\n".join(lines) + "\n"


def payload(tool="Edit", agent=True):
    data = {"tool_name": tool, "tool_input": {"file_path": "{{TMPDIR}}/x.go"},
            "transcript_path": "{{TMPDIR}}/parent.jsonl", "session_id": "parent"}
    if agent:
        data["agent_id"] = "a1"
        data["agent_type"] = "general-purpose"
    return json.dumps(data)


SUB = "parent/subagents/agent-a1.jsonl"
THREE = {"ANTI_TANGENT_PROGRESS_EDITS": "3"}
DUE = transcript(SPEC, ("Edit", 3))
ASK = ["CHECKPOINT DUE: call check_progress"]

CASES = [
    {"name": "progress-nudge-tenth-edit-asks-once", "stdin_raw": payload(),
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 10))},
     "expected_exits": [2, 0], "expected_stderr_contains": ASK,
     "reason": "ten edits after validate_task_spec with no check_progress must ask, at the default threshold, and the same task must not be asked a second time"},
    {"name": "progress-nudge-below-threshold-passes", "stdin_raw": payload(),
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 9))}, "expected_exit": 0,
     "reason": "nine edits is under the default threshold of ten, so nothing is asked"},
    {"name": "progress-nudge-check-progress-called-passes", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 2), PROGRESS, ("Edit", 3))}, "expected_exit": 0,
     "reason": "a task that already called check_progress is never asked, however many edits follow"},
    {"name": "progress-nudge-completion-loop-skips", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 3), COMPLETION, ("Edit", 1))}, "expected_exit": 0,
     "reason": "after validate_completion the task is fixing review findings, and those edits are not asked about"},
    {"name": "progress-nudge-no-task-skips", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: transcript(("Edit", 5))}, "expected_exit": 0,
     "reason": "no validate_task_spec call means no task: a lightweight task or ordinary editing is never asked"},
    {"name": "progress-nudge-main-session-asks", "stdin_raw": payload(agent=False), "env": THREE,
     "tmpdir_fixture": {"parent.jsonl": DUE}, "expected_exit": 2, "expected_stderr_contains": ASK,
     "reason": "with no agent_id the payload is the main session, whose own transcript is read: a task run without a dispatched subagent is asked too"},
    {"name": "progress-nudge-reads-subagent-not-parent", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {"parent.jsonl": DUE, SUB: transcript(SPEC, ("Edit", 1))}, "expected_exit": 0,
     "reason": "inside a subagent the count comes from the subagent's transcript; a hook that read the parent's would ask after one edit"},
    {"name": "progress-nudge-next-task-is-asked-on-its-own-count", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 3), COMPLETION, SPEC, ("Edit", 3))},
     "expected_exit": 2, "expected_stderr_contains": ASK,
     "reason": "the window is the last validate_task_spec call: a hook that read the first one would see the earlier task's validate_completion and stay silent for every task after it"},
    {"name": "progress-nudge-next-task-starts-from-zero", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 2), SPEC, ("Edit", 2))}, "expected_exit": 0,
     "reason": "a new validate_task_spec call restarts the count: four edits across two tasks is two for the current one"},
    {"name": "progress-nudge-kill-switch", "stdin_raw": payload(),
     "env": {"ANTI_TANGENT_PROGRESS_EDITS": "3", "ANTI_TANGENT_PROGRESS_GUARD": "0"},
     "tmpdir_fixture": {SUB: DUE}, "expected_exit": 0,
     "reason": "ANTI_TANGENT_PROGRESS_GUARD=0 turns the hook off for a task that is due"},
    {"name": "progress-nudge-unusable-threshold-uses-default", "stdin_raw": payload(),
     "env": {"ANTI_TANGENT_PROGRESS_EDITS": "abc"},
     "tmpdir_fixture": {SUB: transcript(SPEC, ("Edit", 10))},
     "expected_exit": 2, "expected_stderr_contains": ASK,
     "reason": "a threshold that is not a whole number above zero falls back to ten, so the tenth edit asks; a body that failed on the value would allow instead"},
    {"name": "progress-nudge-no-python3-fails-open", "stdin_raw": payload(), "env": THREE,
     "tmpdir_fixture": {SUB: DUE}, "path_stub_exclude": "python3", "expected_exit": 0,
     "reason": "with python3 missing from PATH the wrapper must allow a task that is due"},
    {"name": "progress-nudge-missing-transcript-fails-open", "stdin_raw": payload(), "env": THREE,
     "expected_exit": 0,
     "reason": "a subagent transcript that does not exist must allow, not fail the edit's hook"},
    {"name": "progress-nudge-malformed-stdin-fails-open", "stdin_raw": "not json", "env": THREE,
     "expected_exit": 0,
     "reason": "a payload that is not JSON must allow"},
    {"name": "progress-nudge-read-is-not-gated", "stdin_raw": payload(tool="Read"), "env": THREE,
     "tmpdir_fixture": {SUB: DUE}, "expected_exit": 0,
     "reason": "only Edit, Write and NotebookEdit are acted on, whatever the matcher sends"},
]


def append(cases, default_hook, old_description, new_description):
    raw = open(PATH, encoding="utf-8").read()
    tail = "\n  ]\n}\n"
    assert raw.endswith(tail), "unexpected end of guard-evals.json"
    next_id = max(c["id"] for c in json.loads(raw)["evals"]) + 1
    blocks = []
    for offset, case in enumerate(cases):
        ordered = {"id": next_id + offset, "name": case["name"], "hook": case.get("hook", default_hook)}
        ordered.update({k: v for k, v in case.items() if k not in ordered})
        text = json.dumps(ordered, indent=2, ensure_ascii=False)
        blocks.append("\n".join("    " + line for line in text.split("\n")))
    total = next_id + len(blocks) - 1
    old = old_description % (next_id - 1)
    assert raw.count(old) == 1, "description line not found"
    raw = raw.replace(old, new_description % total)
    open(PATH, "w", encoding="utf-8").write(raw[:-len(tail)] + ",\n" + ",\n".join(blocks) + tail)
    print("appended %d cases; guard-evals.json now has %d" % (len(blocks), total))


if __name__ == "__main__":
    append(CASES, "check-progress-nudge",
           "and the check-task-start PreToolUse hook (%d cases)",
           "the check-task-start PreToolUse hook, and the check-progress-nudge PostToolUse hook (%d cases)")
```

Run: `python3 -B /tmp/add_progress_cases.py`
Expected: `appended 15 cases; guard-evals.json now has 194`.

Run: `git diff --stat plugin/anti-tangent-guard/evals/guard-evals.json && jq '.evals | length' plugin/anti-tangent-guard/evals/guard-evals.json`
Expected: one file changed, `1 deletion(-)` (the description line; every other change is an insertion), and `194`. More than one deletion means an existing case was rewritten: `git checkout` the file and rerun the script unchanged.

- [x] **Step 2: Run the suite and see it fail on the count**

Run: `bash plugin/anti-tangent-guard/evals/run.sh | tail -3`
Expected: `guard-evals.json declares 194 case(s), expected exactly 179`.

- [x] **Step 3: Update the suite**

In `plugin/anti-tangent-guard/evals/run.sh`, replace

```bash
EXPECTED_CASE_COUNT=179
```

with

```bash
EXPECTED_CASE_COUNT=194
```

In `plugin/anti-tangent-guard/evals/run.sh`, replace

```bash
unset ANTI_TANGENT_COMMENT_GUARD
```

with

```bash
unset ANTI_TANGENT_COMMENT_GUARD
unset ANTI_TANGENT_PROGRESS_GUARD
unset ANTI_TANGENT_PROGRESS_EDITS
```

In `plugin/anti-tangent-guard/evals/run.sh`, replace

```bash
pattern and semantic tiers + check-task-start)"
```

with

```bash
pattern and semantic tiers + check-task-start + check-progress-nudge)"
```

In `plugin/anti-tangent-guard/evals/run.sh`, replace

```bash

echo ""
echo "════════════════
```

with

```bash
# An exit code of 2 says the hook asked; the trace line is what says it asked
# at the default threshold, recorded the count, and named the subagent.
if ! grep -qF "s=parent.a1 | progress | nudge | edits=10" "$ANTI_TANGENT_GUARD_TRACE_LOG"; then
    echo "FAIL: no progress nudge trace line with its edit count"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "════════════════
```

- [x] **Step 4: Run the suite**

Run: `bash plugin/anti-tangent-guard/evals/run.sh | tail -3`
Expected: `Total: 194 passed, 0 failed, 194 total`, exit 0. A `FAIL` on a `progress-nudge-*` case prints the case's `reason`; fix the hook only if the reason describes behaviour Task 5's acceptance criteria require, otherwise report it.

- [x] **Step 5: Commit**

```bash
git add plugin/anti-tangent-guard/evals/guard-evals.json plugin/anti-tangent-guard/evals/run.sh
git commit -m "test(guard): eval cases for the progress reminder"
```

```json:metadata
{"files": ["plugin/anti-tangent-guard/evals/guard-evals.json", "plugin/anti-tangent-guard/evals/run.sh"], "verifyCommand": "bash plugin/anti-tangent-guard/evals/run.sh | tail -1", "acceptanceCriteria": ["guard-evals.json has 194 cases with ids 180-194 on check-progress-nudge and cases 1-179 unchanged", "the fifteen cases cover ask-once, below threshold, checked, completing, no task, main session, subagent transcript, next task asked, count restart, kill switch, unusable threshold, no python3, missing transcript, malformed stdin and Read", "run.sh expects 194, unsets the two variables and checks the nudge trace line", "the suite ends 194 passed, 0 failed"], "modelTier": "mechanical"}
```

---

### Task 7: The comment guard scans `.mjs`, `.kts` and `.vue`

**Goal:** `SCAN_EXTS` covers `.mjs` and `.vue` with the JavaScript rules and `.kts` with the Kotlin rules, with tests that ordinary comments in each are not flagged.

**Files:**
- Modify: `plugin/anti-tangent-guard/hooks/comment_scan.py`
- Modify: `plugin/anti-tangent-guard/hooks/check-comment-write`
- Modify: `plugin/anti-tangent-guard/evals/guard-evals.json` (4 cases appended, ids 195–198)
- Modify: `plugin/anti-tangent-guard/evals/run.sh`
- Modify: `plugin/anti-tangent-guard/README.md`, `CHANGELOG.md`
- Test: `plugin/anti-tangent-guard/hooks/comment_scan_test.py`

Depends on Task 6 (the case count and the description line it leaves).

**Acceptance Criteria:**
- [ ] `violations("x.mjs", ["// fixes #1"])`, the same for `build.gradle.kts` and for `Widget.vue`, each return a violation.
- [ ] Three ordinary comments per extension (a `//` line, a `/* */` line or a `*` continuation) return no violation.
- [ ] In `.mjs` and `.vue`, a starred line inside a plain template literal is not a comment, and a block comment inside a `${}` hole is. In `.kts`, a stray backtick does not hide a later block comment.
- [ ] A `<!-- -->` comment in a `.vue` template returns no violation (the documented limit), in the unit test and in eval case 198.
- [ ] `check-comment-write`'s `ATG_SCAN_EXTS` and `comment_scan.py`'s `SCAN_EXTS` list the same extensions (the suite's drift check passes).
- [ ] `bash plugin/anti-tangent-guard/evals/run.sh` ends `198 passed, 0 failed, 198 total`; `bash plugin/anti-tangent-guard/evals/fp-report.sh` ends `FALSE POSITIVES: 0`; `python3 -B plugin/anti-tangent-guard/evals/build-jev-comments-test.py` ends `OK`.
- [ ] The guard README states the two `.vue` template limits (HTML comments are not recognised; template text after `//` is read as a comment) and `CHANGELOG.md` `[0.27.0]` `### Changed` has the bullet.

**Verify:** `bash plugin/anti-tangent-guard/evals/run.sh | tail -1 && bash plugin/anti-tangent-guard/evals/fp-report.sh | tail -1` → `Total: 198 passed, 0 failed, 198 total` and `FALSE POSITIVES: 0`

**Steps:**

- [x] **Step 1: Write the failing tests**

In `plugin/anti-tangent-guard/hooks/comment_scan_test.py`, insert this class directly above the final `if __name__ == "__main__":` line, keeping two blank lines before and after it:

```python
class ScriptModuleAndComponentExtensions(unittest.TestCase):
    # .mjs and .vue take the JavaScript rules, template literals included;
    # .kts takes the Kotlin rules, in which a backtick is ordinary text.
    def _violations(self):
        sys.path.insert(0, HOOKS)
        from comment_scan import violations
        return violations

    def test_the_three_extensions_are_scanned(self):
        violations = self._violations()
        for path in ("x.mjs", "build.gradle.kts", "Widget.vue"):
            self.assertNotEqual(violations(path, ["// fixes #1"]), [], path)

    def test_ordinary_comments_in_them_are_not_flagged(self):
        violations = self._violations()
        benign = {
            "x.mjs": ["// Resolve the entry point relative to this module.",
                      "/* Keys are sorted so the output is stable. */",
                      " * @param {string} name - the export to look up"],
            "build.gradle.kts": ["// The JVM target must match the toolchain below.",
                                 " * Repositories are declared in settings.gradle.kts.",
                                 "// version catalogs keep these numbers in one place"],
            "Widget.vue": ["// Emitted when the user picks a row.",
                           "/* Scoped: these rules must not leak into child components. */",
                           " * The prop is optional; the default is an empty list."],
        }
        for path, lines in benign.items():
            self.assertEqual(violations(path, lines), [], path)

    def test_mjs_and_vue_template_literals_use_the_javascript_rules(self):
        violations = self._violations()
        for path in ("x.mjs", "Widget.vue"):
            literal = 'const t = `\n * added in v1.2.3\n`;\n'
            self.assertEqual(violations(path, [" * added in v1.2.3"], literal), [],
                             "%s: text inside a template literal is not a comment" % path)
            hole = 'const t = `text ${ a /*\n * fixes #1\n */ b } more`;\n'
            self.assertNotEqual(violations(path, [" * fixes #1"], hole), [],
                                "%s: a block comment inside ${} is a comment" % path)

    def test_a_kts_backtick_does_not_blind_the_file(self):
        violations = self._violations()
        ctx = '// see `kotlin("jvm")\nplugins { }\n/**\n * fixes #1\n */\n'
        self.assertNotEqual(violations("build.gradle.kts", [" * fixes #1"], ctx), [])

    def test_a_vue_template_comment_is_not_recognised(self):
        violations = self._violations()
        ctx = "<template>\n  <!-- fixes #1 -->\n</template>\n"
        self.assertEqual(violations("Widget.vue", ["  <!-- fixes #1 -->"], ctx), [])
```

- [x] **Step 2: Run them and see them fail**

Run: `python3 -B plugin/anti-tangent-guard/hooks/comment_scan_test.py 2>&1 | tail -5`
Expected: failures in `ScriptModuleAndComponentExtensions`: `test_the_three_extensions_are_scanned` gets `[]` for `x.mjs`.

- [x] **Step 3: Extend the scanner and the wrapper's copy of the list**

In `plugin/anti-tangent-guard/hooks/comment_scan.py`, replace

```python
    ".go", ".sh", ".bash", ".py", ".ts", ".tsx", ".js", ".jsx",
    ".rs", ".java", ".kt", ".rb", ".c", ".h", ".cc", ".cpp", ".hpp",
}
```

with

```python
    ".go", ".sh", ".bash", ".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".vue",
    ".rs", ".java", ".kt", ".kts", ".rb", ".c", ".h", ".cc", ".cpp", ".hpp",
}
```

In `plugin/anti-tangent-guard/hooks/comment_scan.py`, replace

```python
_INTERPOLATING_EXTS = {".js", ".jsx", ".ts", ".tsx"}
```

with

```python
#
# A .vue single-file component is read with these rules throughout: its script
# block is JavaScript or TypeScript, and its style block's comments are the
# same `/* */` form. The template block's `<!-- -->` comments are not a form
# this scanner knows, so a comment there is never offered to the tells.
_INTERPOLATING_EXTS = {".js", ".jsx", ".mjs", ".ts", ".tsx", ".vue"}
```

In `plugin/anti-tangent-guard/hooks/check-comment-write`, replace

```bash
ATG_SCAN_EXTS=".go .sh .bash .py .ts .tsx .js .jsx .rs .java .kt .rb .c .h .cc .cpp .hpp"
```

with

```bash
ATG_SCAN_EXTS=".go .sh .bash .py .ts .tsx .js .jsx .mjs .vue .rs .java .kt .kts .rb .c .h .cc .cpp .hpp"
```

`_BACKTICK_EXTS` is derived from `_INTERPOLATING_EXTS`, so `.mjs` and `.vue` get the backtick rules with no further edit, and `.kts`, absent from both, treats a backtick as ordinary text like `.kt`.

Run: `python3 -B plugin/anti-tangent-guard/hooks/comment_scan_test.py 2>&1 | tail -3`
Expected: `Ran 72 tests` and `OK`.

- [x] **Step 4: Append the eval cases**

Save this as `/tmp/add_extension_cases.py` (not committed) and run it **once** from the repository root:

```python
"""Append the .mjs / .kts / .vue comment-guard eval cases to guard-evals.json.

Run once from the repository root. The cases are inserted as text before the
closing bracket, so the existing cases keep their bytes.
"""
import json

PATH = "plugin/anti-tangent-guard/evals/guard-evals.json"


def write(target, content):
    return json.dumps({"tool_name": "Write", "tool_input": {"file_path": "{{TMPDIR}}/" + target, "content": content}})


HISTORY = ["adds comment(s) carrying change history", "an issue or pull-request reference"]
EXT_CASES = [
    {"name": "comment-write-mjs-issue-ref-blocks", "hook": "check-comment-write",
     "stdin_raw": write("atg-eval.mjs", "// fixes #61\nexport const x = 1;\n"),
     "expected_exit": 2, "expected_stderr_contains": HISTORY,
     "reason": "an .mjs file is scanned with the JavaScript rules"},
    {"name": "comment-write-kts-issue-ref-blocks", "hook": "check-comment-write",
     "stdin_raw": write("build.gradle.kts", "// fixes #61\nplugins { kotlin(\"jvm\") }\n"),
     "expected_exit": 2, "expected_stderr_contains": HISTORY,
     "reason": "a .kts script is scanned with the Kotlin rules"},
    {"name": "comment-write-vue-script-issue-ref-blocks", "hook": "check-comment-write",
     "stdin_raw": write("Widget.vue", "<template>\n  <p>hi</p>\n</template>\n<script>\n// fixes #61\nexport default {}\n</script>\n"),
     "expected_exit": 2, "expected_stderr_contains": HISTORY,
     "reason": "a comment in a .vue file's script block is scanned with the JavaScript rules"},
    {"name": "comment-write-vue-template-comment-not-recognised", "hook": "check-comment-write",
     "stdin_raw": write("Widget.vue", "<template>\n  <!-- fixes #61 -->\n  <p>hi</p>\n</template>\n"),
     "expected_exit": 0,
     "reason": "an HTML comment in a .vue template is not a comment form the scanner recognises: a documented limit, pinned so a change to it is deliberate"},
]


def append(cases, default_hook, old_description, new_description):
    raw = open(PATH, encoding="utf-8").read()
    tail = "\n  ]\n}\n"
    assert raw.endswith(tail), "unexpected end of guard-evals.json"
    next_id = max(c["id"] for c in json.loads(raw)["evals"]) + 1
    blocks = []
    for offset, case in enumerate(cases):
        ordered = {"id": next_id + offset, "name": case["name"], "hook": case.get("hook", default_hook)}
        ordered.update({k: v for k, v in case.items() if k not in ordered})
        text = json.dumps(ordered, indent=2, ensure_ascii=False)
        blocks.append("\n".join("    " + line for line in text.split("\n")))
    total = next_id + len(blocks) - 1
    old = old_description % (next_id - 1)
    assert raw.count(old) == 1, "description line not found"
    raw = raw.replace(old, new_description % total)
    open(PATH, "w", encoding="utf-8").write(raw[:-len(tail)] + ",\n" + ",\n".join(blocks) + tail)
    print("appended %d cases; guard-evals.json now has %d" % (len(blocks), total))


if __name__ == "__main__":
    append(EXT_CASES, "check-comment-write",
           "check-progress-nudge PostToolUse hook (%d cases)",
           "check-progress-nudge PostToolUse hook (%d cases)")
```

Run: `python3 -B /tmp/add_extension_cases.py`
Expected: `appended 4 cases; guard-evals.json now has 198`.

In `plugin/anti-tangent-guard/evals/run.sh`, replace

```bash
EXPECTED_CASE_COUNT=194
```

with

```bash
EXPECTED_CASE_COUNT=198
```

- [x] **Step 5: Run the suites**

Run: `bash plugin/anti-tangent-guard/evals/run.sh | tail -1`
Expected: `Total: 198 passed, 0 failed, 198 total`.

`fp-report.sh` and the corpus test read committed content (`HEAD` blobs), so commit first (Step 7) and run them in Step 8.

- [x] **Step 6: Documentation and changelog**

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
  scripts — falls outside it and is not scanned by either layer.
```

with

```markdown
  scripts — falls outside it and is not scanned by either layer.
- A `.vue` file is scanned with the JavaScript rules throughout, its template
  block included. The `<!-- -->` comments of that block are not a comment form
  the scanner recognises, so a comment written there is never checked, and
  template text after a `//` is read as a comment.
```

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- `anti-tangent-guard`'s comment scan covers `.mjs` and `.vue` (JavaScript rules) and `.kts` (Kotlin rules). The `<!-- -->` comments of a `.vue` template block are not recognised.
```

- [x] **Step 7: Commit**

```bash
git add plugin/anti-tangent-guard/hooks/comment_scan.py plugin/anti-tangent-guard/hooks/check-comment-write plugin/anti-tangent-guard/hooks/comment_scan_test.py plugin/anti-tangent-guard/evals/guard-evals.json plugin/anti-tangent-guard/evals/run.sh plugin/anti-tangent-guard/README.md CHANGELOG.md
git commit -m "feat(guard): scan .mjs, .kts and .vue comments"
```

- [x] **Step 8: Run the false-positive gate on the committed tree**

Run: `bash plugin/anti-tangent-guard/evals/fp-report.sh | tail -2 && python3 -B plugin/anti-tangent-guard/evals/build-jev-comments-test.py 2>&1 | tail -1`
Expected: `scanned hits: 21   classified: 21`, `FALSE POSITIVES: 0`, `OK`. An `UNCLASSIFIED hit` names a comment this branch added that matches a history tell: rewrite that comment (it breaks the comment policy), amend, and rerun. Do not add it to `fp-class.tsv`.

```json:metadata
{"files": ["plugin/anti-tangent-guard/hooks/comment_scan.py", "plugin/anti-tangent-guard/hooks/check-comment-write", "plugin/anti-tangent-guard/hooks/comment_scan_test.py", "plugin/anti-tangent-guard/evals/guard-evals.json", "plugin/anti-tangent-guard/evals/run.sh", "plugin/anti-tangent-guard/README.md", "CHANGELOG.md"], "verifyCommand": "bash plugin/anti-tangent-guard/evals/run.sh | tail -1 && bash plugin/anti-tangent-guard/evals/fp-report.sh | tail -1", "acceptanceCriteria": ["a history comment in .mjs, .kts and .vue is flagged", "ordinary comments in each are not flagged", ".mjs and .vue use the JavaScript template-literal rules and a .kts backtick is ordinary text", "a .vue template comment is not recognised, in the unit test and in case 198", "the wrapper's extension list matches the scanner's", "the eval suite ends 198 passed, the false-positive gate reports 0 and the corpus test passes", "the guard README states the two .vue template limits and CHANGELOG has the bullet"], "modelTier": "mechanical"}
```

---

### Task 8: Protocol wording and documentation

**Goal:** The implementer protocol tells implementers to call `check_progress` when the guard asks or when they suspect drift, and the plugin's README, the root README, `CLAUDE.md` and the plugin manifests describe the fourth hook.

**Files:**
- Modify: `docs/protocol/implementer.md`, `docs/protocol/core.md`
- Modify: `plugin/anti-tangent-protocol/protocol/*.md` (resynced copy)
- Modify: `examples/lightweight-dispatch.md`
- Modify: `plugin/anti-tangent-guard/README.md`
- Modify: `README.md`, `CLAUDE.md`
- Modify: `plugin/anti-tangent-guard/.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`
- Modify: `CHANGELOG.md`

Depends on Tasks 5–7 (it documents them, and Task 7 leaves the eval count at 198).

**Acceptance Criteria:**
- [ ] `docs/protocol/implementer.md` no longer contains `OPTIONAL`, `low-signal` or `ONLY if you suspect`; it contains `when the guard asks, or when` twice in the dispatch clause and `When the guard asks, or when you suspect drift` in the lifecycle table. The heading `## Drift-protection protocol (anti-tangent-mcp)` and every section number are unchanged.
- [ ] `wc -c docs/protocol/implementer.md` is 15,879 and `docs/protocol/core.md` is 15,970 (both under 16,000); `diff -r docs/protocol plugin/anti-tangent-protocol/protocol` prints nothing.
- [ ] `plugin/anti-tangent-guard/README.md` has a `## Progress reminder (PostToolUse hook)` section between the start gate and the write-time comment guard, an `### ANTI_TANGENT_PROGRESS_EDITS` configuration entry, an `ANTI_TANGENT_PROGRESS_GUARD=0` kill-switch bullet, the `progress` trace events, and says `198 cases` and `four hooks`.
- [ ] `README.md`'s guard section says four hooks, describes the reminder and names its kill switch; `CLAUDE.md` describes the guard's fourth hook, which refuses nothing, and says five kill switches. `README.md` no longer calls `check_progress` optional.
- [ ] `plugin.json` and the guard entry of `marketplace.json` say `Four hooks`, describe the reminder, name `ANTI_TANGENT_PROGRESS_GUARD=0`, and carry `"version": "0.7.0"`; both files are valid JSON.
- [ ] `CHANGELOG.md` `[0.27.0]` `### Changed` has the bullet below.

**Verify:** `wc -c docs/protocol/*.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && jq -e . plugin/anti-tangent-guard/.claude-plugin/plugin.json .claude-plugin/marketplace.json >/dev/null && go test -race ./...` → every protocol file under 16000, no diff output, `ok` for every package

**Steps:**

- [x] **Step 1: Protocol wording**

In `docs/protocol/implementer.md`, replace

```markdown
| During | `check_progress` | Optional (advisory; low-signal in field data) | When you suspect drift, a test that 'should' fail doesn't, or you've spent >5 min on behavior the spec leaves under-specified |
```

with

```markdown
| During | `check_progress` | When asked | When the guard asks, or when you suspect drift: a test that 'should' fail doesn't, or >5 min spent on behavior the spec leaves under-specified |
```

In `docs/protocol/implementer.md`, replace

```markdown
`validate_completion`. Use `check_progress` only when you suspect drift.
```

with

```markdown
`validate_completion`. Use `check_progress` when the guard asks, or when
you suspect drift.
```

In `docs/protocol/implementer.md`, replace

```markdown
**2. During work (OPTIONAL).** Call `check_progress` ONLY if you suspect
you're drifting mid-task, OR a test that 'should' fail doesn't, OR
you've spent >5 min debugging behavior the spec leaves under-specified.
This call is advisory — most tasks skip it. When you do call, pass: the
session_id, a one-sentence `working_on` summary, and the changed files.
```

with

```markdown
**2. During work.** Call `check_progress` when the guard asks, or when
you suspect drift: a test that 'should' fail doesn't, or you've spent
>5 min debugging behavior the spec leaves under-specified. Pass: the
session_id, a one-sentence `working_on` summary, and the changed files.
```

In `docs/protocol/core.md`, replace

```markdown
treating mid calls as optional (call only when you suspect drift)
```

with

```markdown
with mid calls when the guard asks or drift is suspected
```

- [x] **Step 2: Check the budget and resync the bundle**

```bash
wc -c docs/protocol/implementer.md docs/protocol/core.md
rm -f plugin/anti-tangent-protocol/protocol/*.md
cp docs/protocol/*.md plugin/anti-tangent-protocol/protocol/
diff -r docs/protocol plugin/anti-tangent-protocol/protocol && echo in-sync
```

Expected: `15879 docs/protocol/implementer.md`, `15970 docs/protocol/core.md`, `in-sync`. A different byte count means a replacement was not applied exactly; a count of 16000 or more fails CI.

- [x] **Step 3: The lightweight dispatch example**

In `examples/lightweight-dispatch.md`, replace

```markdown
- **Skip** `check_progress` (already optional in full mode).
```

with

```markdown
- **Skip** `check_progress` (the guard's reminder needs a `validate_task_spec` call, so it never fires here).
```

- [x] **Step 4: The guard plugin's README**

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
Three hooks enforcing anti-tangent-mcp's conventions: a `PostToolUse` hook that
```

with

```markdown
Four hooks enforcing anti-tangent-mcp's conventions: a `PostToolUse` hook that
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
`validate_task_spec` has been called; and a `PreToolUse` hook that prevents
such comments from being written in the first place.
```

with

```markdown
`validate_task_spec` has been called; a `PreToolUse` hook that prevents
such comments from being written in the first place; and a `PostToolUse` hook
on `Edit`/`Write`/`NotebookEdit` that asks a task, once, for a
`check_progress` call when it reaches ten edits without one.
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
This plugin has three hooks and no configuration step.
```

with

```markdown
This plugin has four hooks and no configuration step.
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
comment-hygiene scan — there is nothing further to turn on.
```

with

```markdown
comment-hygiene scan, and each of those edits is counted toward the progress
reminder — there is nothing further to turn on.
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
## Write-time comment guard (PreToolUse hook)
```

with

```markdown
## Progress reminder (PostToolUse hook)

`check-progress-nudge` fires after every `Edit`, `Write` and `NotebookEdit`.
It reads the session's own transcript — the subagent's when the payload
carries `agent_id` (the file beside the parent's, or `transcript_path` itself
when that already names an agent transcript), the main session's otherwise —
and looks at the window
after the last `mcp__anti-tangent__validate_task_spec` call, which is the
current task. When that window reaches `ANTI_TANGENT_PROGRESS_EDITS` edits
(default 10) and holds no `check_progress` call, the hook exits 2 with a
message asking for one. The edit has already happened and is kept: exit 2 on
`PostToolUse` hands the message to the model and undoes nothing.

A task is asked once. The hook records the ask in a `progress-asked-<hash>`
file beside the trace log, created exclusively, so edits sent in one turn —
whose hooks run at the same time and all read the same count — produce one
message, not one each. The files are empty and are not removed; the hash is
of the transcript path and the `validate_task_spec` call's id.

It stays silent when the transcript has no `validate_task_spec` call (no
task, which also covers a lightweight task and a controller's own edits),
when the task has already called `check_progress`, and once the task has
called `validate_completion` — edits after that are fixes to review findings.

Unlike the start gate it also acts in the main session, because a task run
there with no dispatched subagent has no other transcript. Limits: edits made
through `Bash` are not counted, an edit another hook refused is counted (the
count is of attempts in the transcript), and a session that runs several
tasks is judged only on the one after its last `validate_task_spec` call. Every failure
allows: no `python3`, no readable transcript, a malformed payload, or a
directory the ask cannot be recorded in all exit 0, and the hook's entry in
`hooks.json` carries a 10-second timeout. Kill switch:
`ANTI_TANGENT_PROGRESS_GUARD=0`.

## Write-time comment guard (PreToolUse hook)
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
## Kill switches
```

with

```markdown
### `ANTI_TANGENT_PROGRESS_EDITS`

How many edits a task makes after `validate_task_spec` before the progress
reminder asks for `check_progress`. Default `10`. A value that is not a whole
number above zero is ignored and the default applies.

## Kill switches
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
- `ANTI_TANGENT_JEV` unset or not exactly `1` disables the semantic tier alone,
```

with

```markdown
- `ANTI_TANGENT_PROGRESS_GUARD=0` disables the progress reminder
  (`PostToolUse` on `Edit`/`Write`/`NotebookEdit`) and nothing else. It is not
  one of the three switches above: the reminder is a separate hook.
- `ANTI_TANGENT_JEV` unset or not exactly `1` disables the semantic tier alone,
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
and `error | python-exit=N`.
```

with

```markdown
and `error | python-exit=N`.

`check-progress-nudge` carries the literal tag `progress` in that column, and
inside a subagent its session column reads `s=<session>.<agent>` (the first 8
characters of each), because subagents of one session run at the same time
and each has its own count. Its events: `pass | edits=N` (the task has made N edits and nothing is asked: it
is under the threshold, has already called `check_progress`, or was already
asked), `nudge | edits=N` (the hook asked), `skip | no-task` (no
`validate_task_spec` call in the transcript), `skip | completing` (the task
has called `validate_completion`), `skip | no-state` (the ask could not be
recorded beside the trace log, so it was not made), `skip | not-gated` (an
unreadable payload or transcript, or a tool the hook does not act on),
`skip | guard=0`, `skip | no-python3`, `skip | no-body`, and
`error | python-exit=N` (which includes an exit 2 that came with no edit
count: the interpreter failing, not the body asking). The largest `edits=` a task reaches is how many edits
it made before completion, which is the number to tune
`ANTI_TANGENT_PROGRESS_EDITS` against.
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
suite (179 cases) against all three hooks, and exits non-zero on either — the
```

with

```markdown
suite (198 cases) against all four hooks, and exits non-zero on either — the
```

In `plugin/anti-tangent-guard/README.md`, replace

```markdown
itself, the semantic tier — and check-task-start's start gate. See
```

with

```markdown
itself, the semantic tier — check-task-start's start gate, and
check-progress-nudge's once-per-task reminder. See
```

- [x] **Step 5: The root README and CLAUDE.md**

In `README.md`, replace

```markdown
complementary to anti-tangent's optional `check_progress`.
```

with

```markdown
complementary to anti-tangent's `check_progress`.
```

In `README.md`, replace

```markdown
Three hooks that enforce anti-tangent-mcp's conventions, all of which block.
```

with

```markdown
Four hooks that enforce anti-tangent-mcp's conventions. Three block; the fourth asks.
```

In `README.md`, replace

```markdown
`Drift-protection protocol (lightweight)` exempts it.

Kill switches:
```

with

```markdown
`Drift-protection protocol (lightweight)` exempts it.

A `PostToolUse` hook on `Edit`/`Write`/`NotebookEdit` asks a task for a `check_progress` call, once, when it has made ten edits since `validate_task_spec` without one (`ANTI_TANGENT_PROGRESS_EDITS` changes the ten). It refuses nothing: the edit is kept and the message goes to the model. It is silent when the session has no `validate_task_spec` call, and after `validate_completion`.

Kill switches:
```

In `README.md`, replace

```markdown
setting all three is what silences the close-time hook entirely.
```

with

```markdown
setting all three is what silences the close-time hook entirely; `ANTI_TANGENT_PROGRESS_GUARD=0` turns off the progress reminder.
```

In `CLAUDE.md`, replace

```markdown
See the guard plugin's README.) That is not a reversal
```

with

```markdown
See the guard plugin's README.) The guard's fourth hook refuses nothing: a `PostToolUse` hook on `Edit`/`Write`/`NotebookEdit` asks a task, once, for a `check_progress` call when it reaches ten edits without one, and the edit is kept. That is not a reversal
```

In `CLAUDE.md`, replace

```markdown
`anti-tangent-guard`'s four kill switches are scoped by concern
```

with

```markdown
`anti-tangent-guard`'s five kill switches are scoped by concern
```

In `CLAUDE.md`, replace

```markdown
leaving the pattern tier, the completion gate and the start gate untouched; each governs
```

with

```markdown
leaving the pattern tier, the completion gate and the start gate untouched, and `ANTI_TANGENT_PROGRESS_GUARD=0` turns off the progress reminder; each governs
```

- [x] **Step 6: The plugin manifests**

In `plugin/anti-tangent-guard/.claude-plugin/plugin.json`, replace

```json
"description": "Three hooks enforcing
```

with

```json
"description": "Four hooks enforcing
```

In `plugin/anti-tangent-guard/.claude-plugin/plugin.json`, replace

```json
Set ANTI_TANGENT_TICKET_PATTERN to your tracker's key shape
```

with

```json
A PostToolUse hook on Edit/Write/NotebookEdit asks a task once for a check_progress call when it reaches ten edits since validate_task_spec without one; it refuses nothing. Set ANTI_TANGENT_TICKET_PATTERN to your tracker's key shape
```

In `plugin/anti-tangent-guard/.claude-plugin/plugin.json`, replace

```json
ANTI_TANGENT_COMMENT_GUARD=0 (comment scan, both hooks).",
```

with

```json
ANTI_TANGENT_COMMENT_GUARD=0 (comment scan, both hooks), ANTI_TANGENT_PROGRESS_GUARD=0 (progress reminder).",
```

In `plugin/anti-tangent-guard/.claude-plugin/plugin.json`, replace

```json
"version": "0.6.0"
```

with

```json
"version": "0.7.0"
```

In `.claude-plugin/marketplace.json`, replace

```json
"description": "Three hooks enforcing anti-tangent-mcp's conventions.
```

with

```json
"description": "Four hooks enforcing anti-tangent-mcp's conventions.
```

In `.claude-plugin/marketplace.json`, replace

```json
Set ANTI_TANGENT_TICKET_PATTERN to your tracker's key shape
```

with

```json
A PostToolUse hook on Edit/Write/NotebookEdit asks a task once for a check_progress call when it reaches ten edits since validate_task_spec without one; it refuses nothing. Set ANTI_TANGENT_TICKET_PATTERN to your tracker's key shape
```

In `.claude-plugin/marketplace.json`, replace

```json
ANTI_TANGENT_COMMENT_GUARD=0 (comment scan, both hooks).",
      "version": "0.5.0"
```

with

```json
ANTI_TANGENT_COMMENT_GUARD=0 (comment scan, both hooks), ANTI_TANGENT_PROGRESS_GUARD=0 (progress reminder).",
      "version": "0.7.0"
```

Run: `jq -e . plugin/anti-tangent-guard/.claude-plugin/plugin.json .claude-plugin/marketplace.json >/dev/null && echo valid`
Expected: `valid`.

- [x] **Step 7: Changelog**

In `CHANGELOG.md`, append as the last bullet under `### Changed` of `## [0.27.0] - 2026-10-04`:

```markdown
- The implementer protocol no longer calls `check_progress` optional and low-signal: the dispatch clause says to call it when the guard asks, or when drift is suspected.
```

- [x] **Step 8: Run everything**

```bash
go build ./... && go test -race ./...
(cd gnome-topbar/daemon && go test -race ./...)
bash plugin/anti-tangent-guard/evals/run.sh | tail -1
```

Expected: `ok` for every package in both modules; `Total: 198 passed, 0 failed, 198 total`.

- [x] **Step 9: Commit, then run the gates that read committed content**

```bash
git add docs/protocol plugin/anti-tangent-protocol/protocol examples/lightweight-dispatch.md plugin/anti-tangent-guard/README.md README.md CLAUDE.md plugin/anti-tangent-guard/.claude-plugin/plugin.json .claude-plugin/marketplace.json CHANGELOG.md
git commit -m "docs: check_progress when the guard asks; document the progress reminder"
bash plugin/anti-tangent-guard/evals/fp-report.sh | tail -1
```

Expected: `FALSE POSITIVES: 0`.

```json:metadata
{"files": ["docs/protocol/implementer.md", "docs/protocol/core.md", "plugin/anti-tangent-protocol/protocol/implementer.md", "plugin/anti-tangent-protocol/protocol/core.md", "examples/lightweight-dispatch.md", "plugin/anti-tangent-guard/README.md", "README.md", "CLAUDE.md", "plugin/anti-tangent-guard/.claude-plugin/plugin.json", ".claude-plugin/marketplace.json", "CHANGELOG.md"], "verifyCommand": "wc -c docs/protocol/*.md && diff -r docs/protocol plugin/anti-tangent-protocol/protocol && jq -e . plugin/anti-tangent-guard/.claude-plugin/plugin.json .claude-plugin/marketplace.json >/dev/null && go test -race ./...", "acceptanceCriteria": ["implementer.md says to call check_progress when the guard asks or drift is suspected, with the clause heading and section numbers unchanged", "implementer.md is 15879 bytes, core.md 15970, and the plugin bundle is identical to docs/protocol", "the guard README documents the progress reminder, its threshold, kill switch and trace events, and says 198 cases and four hooks", "the root README and CLAUDE.md describe the hook and its kill switch", "plugin.json and marketplace.json describe four hooks at version 0.7.0 and are valid JSON", "CHANGELOG has the bullet"], "modelTier": "standard"}
```
