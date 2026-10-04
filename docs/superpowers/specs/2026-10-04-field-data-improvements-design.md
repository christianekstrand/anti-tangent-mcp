# Field-data improvements — design

**Status:** design (brainstorming output), pre-implementation. The decisions in §1.3 are the
maintainer's and are settled; §9 lists what still needs a ruling.
**Date:** 2026-10-04
**Input:** [`2026-10-04-field-data-findings.md`](2026-10-04-field-data-findings.md) — 18 days of
opt-in stats. This spec verifies those findings against the code and the raw files (§2) before
designing from them.
**Release vehicle:** three parts, three releases. Part 1 is `version/0.27.0` (backward-compatible
minor; released is 0.26.0, and no `version/*` branch above 0.25.0 is in flight). Parts 2 and 3
are expected to be 0.28.0 and 0.29.0, each with its own plan.

---

## 1. Summary

The final whole-plan review finds a critical or major problem in about one task in five, and
anti-tangent's own verdict on that task does not predict which: the rate is 18% after a `pass`,
21% after a `warn`, 23% after a `fail`. The reviewer spends its findings on the shape of the
submission (evidence, CodeScene, spec wording, comments); the final review finds correctness
defects. The completion review is not asked to look for them and has no category to report one in.

This design changes what the completion review looks for, fixes the measurements that will show
whether that worked, and then reduces the cost of the gates around it.

### 1.1 Goals

- Lower the escape rate: the share of tasks anti-tangent passed in which the final review finds a
  critical or major problem.
- Make that rate comparable across releases and models, so a change can be judged.
- Stop retries that cannot resolve anything, without discouraging the retries that fix findings.
- Make `validate_plan` converge instead of re-reviewing an edited plan from scratch.

### 1.2 Non-goals

- No blocking in the server. Every server change here is advisory; the one enforcing change
  (automatic `check_progress`) is a plugin hook with its own kill switch.
- No language-specific analysis. The correctness review is the reviewer LLM reading the submitted
  evidence, not a linter.
- No new persistent store. New measurements are fields on records the opt-in stats subsystem
  already writes, and stay content-free: counts, verdicts, model ids and anti-tangent's own
  category names.
- No control arm. The stats cannot say what a task's outcome would have been without a retry;
  nothing here tries to.

### 1.3 Decisions already made

- `check_progress` is triggered automatically, from the `anti-tangent-guard` plugin, as a hook with
  its own kill switch. The threshold is proposed here (§5.1).
- `codescene_not_run` stays verdict-driving. It clears on the next call 136 of 138 times.
- Agents circling back to fix findings is the wanted behaviour. The target is a finding that
  repeats without resolving, not a retry.
- Lean guidance gets a measurement (code size per task) and teeth (an `over_building` finding that
  survives a retry needs an explicit implementer ruling).

---

## 2. What the code and the data say about each finding

Every number below was re-derived on 2026-10-04 from `events.jsonl` (1,886 calls), `runs.jsonl`
(753 lines) and `outcomes.jsonl` (33 records, 32 distinct runs). Where a number differs from the
findings file, this section's number is the one the rest of the spec uses. Escape counts use the
latest outcome per run, which is what the scorecard does; the findings file counted a superseded
outcome record as well.

| # | Finding | Result | What the code or data shows |
|---|---|---|---|
| 1 | Verdicts do not predict escapes | Confirmed | pass 27/152 (18%), warn 5/24 (21%), fail 3/13 (23%). By number of `validate_completion` calls: one 14/59, two 9/68, three 7/36, four or more 5/26. By spec verdict: pass 13/76, warn 13/64, fail 9/34. Lightweight 0/15; escalated 9/35. |
| 2 | The reviewer flags process; the final review finds correctness | Confirmed, with one correction | 46 of the 73 critical or major outcome findings are `correctness`. The correction: `post.tmpl` **does** tell the reviewer to judge the change against each acceptance criterion ("Walk every acceptance criterion explicitly"). What it lacks is §2.1. |
| 3 | Some findings repeat without resolving | Confirmed; cause differs per category | The cleared-on-next-call table reproduces exactly. Causes are in §2.2. Non-pass completion calls inside a session: 559, of which 482 were followed by another call and 77 were not (31 on a `fail`). 294 of 455 passes (65%, not 76%) carry findings. |
| 4 | `validate_plan` loops and is slow | Confirmed, with one correction | p50 95.8s, p95 244.8s, max 359.0s, 102 calls over 120s; 99 of 127 run headers have no task rows. The correction is in §2.3: the loop is not the same plan re-submitted. |
| 5 | `codebase_reference_checklist` fires on most specs | Confirmed | 420 of 570 spec calls, 128 of them passes, and the only finding on 56. Cause in §2.4. |
| 6 | `over_building` rarely clears | Confirmed | 49 of 1,042 completion calls (20 pass, 16 fail, 13 warn); 6 of 20 cleared on the next call. Pinned to minor in `post.tmpl`, `mid.tmpl` and `lean.tmpl`. |
| 7 | `check_progress` is almost never called | Confirmed; the cause is the protocol | 6 calls; 4 of 193 task rows have a checkpoint. `docs/protocol/implementer.md` tells implementers the call is "OPTIONAL", to be made "ONLY if you suspect drift", and calls it "low-signal in field data". Implementers are following the instruction. |
| 8a | Cohorts are fragmented and an `unknown` cohort exists | Confirmed | §2.5. |
| 8b | Every cohort reads `no_baseline` | Confirmed; structural, not a defect | §2.5. |
| 8c | 193 of 260 planned tasks have rows | Confirmed; cause not measurable today | §2.5. |
| 8d | `codescene-events.jsonl` is empty although rows say CodeScene ran | **Contradicted as a defect** | §2.6. The server never writes that file. |
| 9 | Guard plugin | Confirmed, with one addition | 38 comment-guard blocks; `.mjs` 405, `.vue` 151, `.kts` 42 writes skipped by extension. The start gate recorded 720 passes and no block, and also 6,253 `not-gated` evaluations: it decided on about one write in ten. §5.1 depends on this. |
| 10 | No trend | Not re-derived | Not used by this design. |

### 2.1 What the completion review is asked to judge

`internal/prompts/templates/post.tmpl` asks for, in order: an exit-contract check, a check of
pre-task findings, a walk of every acceptance criterion against the evidence, a walk of the
non-goals, a cross-check of test evidence against the ACs ("does it actually exercise each AC?"),
then three sections, each longer than the AC walk: comment hygiene, stale comments, over-building.

Three things are missing, and together they explain the category mix:

1. **No instruction to look for defects.** Nothing asks whether the code is right beyond whether
   each AC has "a passing implementation". An AC can be met by code that drops an error, mishandles
   an empty input, or races.
2. **No category for one.** The reviewer schema's category enum (`internal/verdict/schema.json`,
   mirrored by `validCategory` in `parser.go`) is closed and has no correctness or test category.
   A reviewer that sees a bug can only file it as `quality`, which the same prompt steers to
   "nit-level concerns", or as `missing_acceptance_criterion`, which requires an AC to be
   contradicted.
3. **A severity bar that excludes it.** "Reserve `severity: critical` and `severity: major` for
   evidence that affirmatively contradicts an AC, OR for an AC that is left unaddressed." A defect
   the ACs do not mention can never be major, so it can never move a verdict.

So improvement 1 is **both** a prompt change and a schema change, plus the severity sentence.
Judging against the acceptance criteria, which the agreed improvement list also asks for, is
already there and is not the gap.

### 2.2 Why findings repeat

- **`codescene_skipped` (7 of 62 cleared).** `codesceneFindings` in
  `internal/mcpsrv/submission_defect.go` derives its findings from the `codescene` argument on
  every call and keeps no state. Two different cases share the category:
  - A skip with a reason **and** `skip_evidence` produces a minor finding whose own suggestion is
    "No action needed if the evidence holds." It is re-emitted on every call by construction and
    there is nothing the implementer can do to clear it. 47 of the 118 calls carrying the category
    had no critical or major finding at all, and on 30 passes it was the only finding. This case
    is an artifact.
  - A skip with a reason and **no** evidence is graded major, deliberately equal to not running
    CodeScene. The code comment gives the reason: the server cannot verify a reason, so if a bare
    reason cost less than silence, composing one would be the cheapest route to a pass. This case
    repeats because implementers resend the reason without evidence. It is working as designed;
    whether to change it is question 1 in §9.
- **`quality` (52 of 206), of which `comment_hygiene` 61 of 127 and `over_building` 6 of 20.**
  These are minor by template. `post.tmpl` tells the reviewer to raise a prior finding again
  unless the evidence now satisfies it, and nothing obliges the implementer to act on a minor. A
  retry made to fix a major therefore carries the same minors along, and three of them lift a
  clean retry back to `warn` through the `minor >= 3` rung of `FinalizeVerdict`.
- **`insufficient_evidence` (112 of 204) and `missing_acceptance_criterion` (98 of 187).** These
  clear about half the time, which is the circling-back the maintainer wants. Not a target.

### 2.3 Why `validate_plan` loops

`planCallContext.mintPlanRunID` (`internal/mcpsrv/review_error.go`) creates a run whenever the
result carries no id, and the result carries one only on a cache hit. The cache
(`plan_cache.go`) stores `pass` results only, for three minutes, keyed on the exact plan text and
rendered prompts. `ValidatePlanArgs` has no field naming an earlier run and the plan prompts are
given no earlier findings. Every call that is not a byte-identical repeat inside three minutes is
therefore a full review of the whole plan and a new run header. The comment on
`planRunIDAdvisory` states it: "every validate_plan round mints a new id and supersedes the
previous one."

The data adds what the findings file did not have. Grouping calls by task count within an hour
gives 64 plan sessions; 52 of them reached a `pass`, and 120 calls were made **after** the first
pass. On all 97 calls that directly follow a pass, the payload size differs from the pass's. So
controllers are not re-submitting a passed plan; they edit it (a pass carries findings) and
re-submit, the unchanged tasks are reviewed again by a non-deterministic reviewer, and 20
sessions saw a non-pass after a pass. Only 4 of 268 calls were cache hits. (Payload size includes
attached context files, so some of the 97 may be a changed attachment set rather than a changed
plan.)

A delta re-review therefore needs: an argument naming the earlier run; the run keeping each
task's text hash and last result; and a prompt that reviews only what changed.

### 2.4 Why `codebase_reference_checklist` fires

`pre.tmpl` instructs the reviewer to emit `unverifiable_codebase_claim` whenever the spec "asserts
something about the codebase that you cannot verify from the text alone — a field name, function
signature, file path". `validate_task_spec` never sends code, so every spec that names a file or a
symbol qualifies unless `pinned_by`, `controller_verified_references` or project knowledge covers
the claim. `normalizeTaskSpecUnverifiableFindings` rolls the claims into one minor
`codebase_reference_checklist` finding **before** `FinalizeVerdict` runs, so the checklist counts
as one of the three minors that make a `noise_cluster` warn. It is a to-do list for the controller
that the server reports as a defect in the spec.

### 2.5 Scorecard inputs

- **Implementer model.** `run.implementerModel` (`scorecard/assemble.go`) reads it only from the
  outcome record's `implementer_models`, an optional argument of `record_review_outcome`, and
  returns `unknown` otherwise. Of the 38 scored tasks in the `unknown` cohort, 34 belong to outcome
  records that sent no list and 4 to lists that left a task out. The handler trims the string and
  stores it verbatim, so `anthropic:claude-haiku-4-5` (18 tasks) and
  `anthropic:claude-haiku-4-5-20251001` (8 tasks) are separate cohorts.
- **Baseline.** `assignRegression` gives a cohort a baseline only when another cohort's last task
  precedes this cohort's first. All records are server 0.26.0 and the cohorts overlap in time, so
  none has one. The first release after 0.26.0 creates cohorts that do. One weakness is real: the
  baseline is the most recently ended cohort of **any** key, so a 0.27.0 sonnet cohort could be
  compared with a 0.26.0 haiku cohort.
- **Coverage.** A task gets a row only when `validate_task_spec` (or a lightweight
  `validate_completion`) passes `plan_run_id`. Since run records began there were 246 spec calls
  and about 191 attached to a row, so roughly 55 calls named no run. The server already answers
  such a call with an advisory (`criterion: plan_run_id`), but that criterion is not in
  `countedCriteria` (`internal/stats/event.go`), so the stats cannot count it and the cause of the
  gap is inferred, not measured. 52 outcome findings name a task with no row and 34 are
  unattributed (`task_index` 0; the findings file says 28).

### 2.6 `codescene-events.jsonl`

Nothing in the server writes this file. It is appended by an optional `PostToolUse` hook the
operator registers (`examples/hooks/codescene-log.sh`, documented in
`docs/team-setup/codescene-stats.md`). The `ran` state on a task row comes from a different
channel, the `codescene` argument of `validate_completion`. An empty file next to 163 `ran` rows
means the hook is not registered on this host, not that records were lost.

---

## 3. The split

Ordered by expected effect on the escape rate.

| Part | Release | Contents (improvement numbers from the findings file) | Why here |
|---|---|---|---|
| 1 | 0.27.0 | 1 (correctness review); 4 (scorecard inputs); the measurement half of 8 (code size per task) | The only change aimed directly at the escapes, shipped with the measurements needed to judge it. Recording code size now gives Part 2 a before-number. |
| 2 | 0.28.0 | 6 (automatic `check_progress`); 2 (non-resolving repeats); the teeth half of 8; 7 (comment guard extensions) | Indirect effect: defects caught mid-task, and reviewer output not spent on findings that cannot resolve. |
| 3 | 0.29.0 | 3 (`validate_plan` convergence); 5 (checklist); 8d (CodeScene event channel) | Cost and latency. No expected effect on escapes. |

Each part is planned and released on its own, so the scorecard's regression flag compares one
change at a time. Part 1 has a plan: `docs/superpowers/plans/2026-10-04-field-data-part1-correctness-and-scorecard.md`.

### 3.1 Success measures

Baselines are the 0.26.0 numbers in §2.

| Part | Measure, as read from the stats files | Baseline | Target |
|---|---|---|---|
| 1 | `escape_rate` of the 0.27.0 cohorts in `scorecard.json` | 27/152 (18%) | Regression flag `ok` and a lower point estimate once a cohort has 10 runs. The 90% interval at this sample is about ±5 points, so only a drop to roughly 8% or below would separate the two intervals. |
| 1 | `correctness_flag_recall` (new, §4.4) | 0 by construction | Above zero; the number to watch as prompts change. |
| 1 | Share of scored tasks in the `unknown` implementer cohort | 38/189 (20%) | Under 5%. |
| 1 | Guard rails: `unconfirmed_flag_rate`, `calls_per_task` | 29/37; 3.24 | No rise in `calls_per_task` above 4.0. A rise in flags that the final review does not confirm means the correctness prompt is speculating. |
| 2 | Tasks with at least one checkpoint | 4/193 | Over half of the tasks that reach the edit threshold. |
| 2 | `codescene_skipped` on a call after the first in a session | 55 of 62 | None for an evidenced skip. |
| 2 | `over_building` cleared or ruled on the next call | 6/20 | Over 80%. |
| 2 | Lines added per task (recorded from Part 1) | first recorded in 0.27.0 | Direction only. |
| 3 | `validate_plan` calls per plan session; run headers without rows | median 4; 99/127 | Median 2 or fewer; near zero. |
| 3 | Spec calls where the checklist is a finding | 420/570 | Zero (it moves to its own field). |

---

## 4. Part 1 — correctness review and the scorecard that judges it

### 4.1 Two reviewer categories

`correctness` and `test_adequacy` join `verdict.Category`, the enum in
`internal/verdict/schema.json`, and `validCategory`. Neither is severity-floored: the reviewer's
severity stands, as with `attestation_contradiction`. Neither is a submission defect, so
`blockingCodeFindingIDs` treats a major one as an open code finding and `next_action` already says
to fix and re-validate. The plan, prime and extract schemas gain the same two enum entries, because a CI test keeps the six
reviewer schemas' category enums identical; no prompt for those tools asks for them.

The category enum is shared by the three per-task tools. Only `post.tmpl` asks for the new
categories in Part 1; a `check_progress` clause is a Part 2 question (§9, question 6).

### 4.2 The completion prompt

Two sections go into `post.tmpl` directly after the evidence rules and **before** comment hygiene,
so the reviewer meets them first.

**Correctness.** After the AC walk, read the change looking for defects, independent of the AC
list. Named classes: a wrong result for a boundary input the task covers (empty, zero, negative,
duplicate, missing); an error or failure return that is dropped, or handled so the caller carries
on with bad data; shared state changed without the protection the surrounding code uses; a
resource not released on every path; a caller and callee changed on one side only; behaviour the
summary claims that the shown code does not have. Rules: report only what the evidence shows, quote
the lines, name the triggering input and the wrong outcome, give the fix. Speculation about code
that was not submitted is not a finding. `criterion` is the verbatim AC text when the defect breaks
one, otherwise `correctness`. Severity: critical for data loss, a security hole or a crash on the
main path; major for a wrong result or unhandled failure on a path the task's Goal or ACs cover;
minor for a defect on a path the task does not exercise.

**Test adequacy.** When the change adds or changes tests, or test evidence was sent: would the
tests fail if the behaviour an AC describes were broken? Flag a test that asserts nothing about
the behaviour, an AC that names a failure or boundary case and is tested only on the happy path,
and a test the evidence shows was skipped. Major when an AC has no test that would catch its
breakage and the task calls for tests; minor otherwise. Absent tests are not flagged when the task
does not call for them (the lean ruleset says so), and missing evidence stays
`insufficient_evidence`.

The severity sentence is widened to admit both: critical and major are reserved for an AC
contradicted or unaddressed, **or** a `correctness` or `test_adequacy` finding that meets its own
bar above.

The review is bounded by what is submitted. A diff with one line of context hides the callers; the
prompt's "only what the evidence shows" rule is what keeps that from turning into guesses, and the
unconfirmed-flag rate is what will show if it fails.

### 4.3 Protocol and documentation

`docs/protocol/core.md` is 27 bytes under its 16,000-byte limit and `implementer.md` 46 bytes
under. Part 1 adds nothing to either: a `correctness` finding is an ordinary code finding and the
envelope's `next_action` already carries the instruction. `outcome.md` (2,976 bytes) takes the one
protocol change, in §4.5. The category list in the README and in the authoritative design spec
gains the two categories.

### 4.4 Measurement on the run records

`planrun.TaskRow` and `scorecard.TaskSnapshot` gain three content-free fields, written at
`validate_completion`:

- `categories`: for each category, how many findings of it the task's completion calls returned,
  summed over calls. Category names come from the server's fixed set.
- `lines_added`, `lines_removed`: counted from the latest call's `final_diff` (lines starting `+`
  or `-`, excluding the `+++`/`---` file headers). Omitted when the call sent no diff.

`scorecard.Metrics` gains:

- `correctness_escape_rate`: passed tasks in which the outcome has a critical or major
  `correctness` finding, over passed tasks.
- `correctness_flag_recall`: of the tasks in which the outcome has a critical or major
  `correctness` finding, the share where anti-tangent raised a `correctness` finding on any
  completion call. This is an upper bound on real recall: the two findings are matched by task,
  not by defect, because the records hold no finding text.
- `lines_added_p50`: median `lines_added` over tasks that recorded one.

All additions are `omitempty` or zero-safe, so records written by 0.26.0 still load and the
gnome-topbar daemon, which builds against `scorecard` through a `replace` directive, keeps
compiling. Showing the new metrics on `/ui/runs` is not part of Part 1.

### 4.5 Cohort hygiene

- **Model id normalisation.** `scorecard.NormalizeModel` trims, lower-cases and strips one trailing
  `-YYYYMMDD` date stamp. It is applied when records are **read** into cohort keys (implementer
  model, review model, tool-model rows), not when they are written, so the 0.26.0 records merge
  without a migration.
- **Missing implementer models.** `record_review_outcome` still records the outcome, and its result
  gains `missing_implementer_models`: the indexes of tasks that have a final verdict and no model
  in the call. The summary block names them. `outcome.md` tells the controller to send one entry
  per dispatched task. The server stays advisory: nothing is rejected.
- **Baseline choice.** `assignRegression` prefers, among the earlier time-disjoint cohorts of the
  same source and publisher, one with the same review model and implementer model; it falls back
  to today's rule when there is none. A release is then compared with the same models on the
  previous release.
- **Coverage cause.** `plan_run_id` joins `countedCriteria`, so the stats count how often a spec
  call named no run. The fix for the gap itself needs one run per plan and is in Part 3 (§6.1).

---

## 5. Part 2 — mid-task checks and retries that resolve

Designed here to the level a plan can start from; the Part 2 plan fixes the remaining detail.

### 5.1 Automatic `check_progress`

A fourth hook in `anti-tangent-guard`, built like the start gate: a bash wrapper
(`hooks/check-progress-nudge`) owning the kill switch, trace line and exit-code mapping, and a
Python body (`hooks/check_progress_nudge.py`) owning the transcript work.

- **Event:** `PostToolUse` on `Edit|Write|NotebookEdit`.
- **Scope:** the session's own transcript: the subagent's when the payload carries `agent_id`,
  otherwise the main one. This is deliberately wider than the start gate, which decided on one
  write in ten (§2, row 9), because a task run under executing-plans has no dispatched subagent.
- **Window:** from the last `validate_task_spec` call in that transcript. No such call means no
  task and no nudge, which also exempts lightweight tasks. A `validate_completion` call after it
  means the task is in its completion loop, and fix-up edits are not nudged.
- **Rule:** count the gated edits since the later of that `validate_task_spec` and the last
  `check_progress`. When the count is a multiple of N, exit 2 with a message telling the
  implementer to call `check_progress` with the files changed so far. The edit has already
  happened; exit 2 on `PostToolUse` returns the message to the model without undoing anything.
- **Switches:** `ANTI_TANGENT_PROGRESS_GUARD=0` turns the hook off.
  `ANTI_TANGENT_PROGRESS_EDITS` sets N; an unusable value falls back to the default.
- **Failure:** every error allows. No transcript, no python, malformed payload: exit 0.
- **Trace:** `progress | pass|nudge|skip | edits=N` in the existing trace log.

**Threshold.** N = 10, counted since the last checkpoint, so a long task is asked again every ten
edits and a task under ten edits never is. The stats tune it two ways. The trace's largest
`edits=` per window gives the distribution of edits per task: N should sit near its median, so
about half of tasks get one checkpoint. And `checkpoints` on the task rows, joined to outcomes,
gives the escape rate with and without a checkpoint, while the `check_progress` events give its
verdict mix: if more than four in five nudged checkpoints return a clean pass, raise N.

**Protocol.** `implementer.md` currently calls `check_progress` optional and low-signal (§2,
row 7). That text changes to "when the guard asks, or when you suspect drift", as a replacement
within the file's 46 bytes of headroom, with the bundle resynced.

### 5.2 Findings that repeat without resolving

- **Evidenced CodeScene skip.** The session remembers that a skip with reason and evidence was
  reported. The first call returns the minor finding; later calls in the session with the same
  skip return none. The row still records `skipped`.
- **Carried minors.** A minor finding the reviewer raises again from the previous call
  (fingerprint or `same_as`) is returned with `repeat_of` set and does not count toward the
  `minor >= 3` rung on that call. A retry is then never pushed back to `warn` only by nits it
  already reported.
- **`codescene_not_run`** is unchanged.

### 5.3 Teeth for `over_building`

An `over_building` finding raised on one completion call and again on the next, with no
`finding_responses` entry answering it and no controller ruling covering it, gains a companion
major `unaddressed_finding` (`criterion: over_building`): cut the structure, or answer the finding
with the reason it stays. With an answer it remains minor and the row counts it as ruled. This is
verdict-driving but advisory, the same standing as `codescene_not_run`.

### 5.4 Comment guard extensions

`SCAN_EXTS` gains `.mjs` (JavaScript rules, interpolating and backtick-aware), `.kts` (Kotlin
rules) and `.vue` (JavaScript rules; `<!-- -->` template comments are not recognised, a documented
limit). The plugin's false-positive evals run against the three before release.

---

## 6. Part 3 — gate cost

### 6.1 `validate_plan` convergence

- `validate_plan` takes an optional `plan_run_id` naming the run an earlier round returned.
- The run keeps, per task, a hash of the task's text and its last result, and the plan-level
  findings.
- A re-call with a live `plan_run_id` keeps that id. Tasks whose hash is unchanged carry their
  result forward and are not sent to the reviewer. Changed and new tasks go through the existing
  chunked task path. The plan-level pass runs again only when a task changed or the task list
  did, and is shown the earlier plan-level findings.
- **Pass is terminal:** a re-call in which nothing changed returns the stored result with no
  reviewer call, whatever its age. This replaces the three-minute cache for callers that pass the
  id.
- The response reports how many tasks were reviewed and how many carried. The run header is
  written again under the same run hash with a revision count.
- An unknown or expired id falls back to a full review, a new run and an advisory.
- With one run per plan, a `validate_task_spec` call that names no run can be attached by title to
  the server's single live run, which closes the coverage gap of §2.5.

### 6.2 `codebase_reference_checklist`

The checklist leaves `findings` and becomes its own envelope field, so it is still handed to the
controller but no longer counts as a finding, as a minor on the verdict ladder, or toward
`noise_cluster`. Paths listed in the task's own `Files:` section are not claims and are dropped
from it.

### 6.3 CodeScene event channel

Either the server appends a record to `codescene-events.jsonl` for each in-band `ran` digest and
the hook is retired, or the two channels stay and the documentation says an empty file is normal.
Question 5 in §9.

---

## 7. Error handling

- Stats writes stay best-effort: a failed write is logged and never changes a tool result.
- A reviewer that returns one of the new categories to an older server cannot happen (the schema
  is the server's); a 0.26.0 record without the new fields reads as zero values.
- `record_review_outcome` reports missing implementer models in its result and never as an error.

## 8. Testing

- `internal/verdict`: the two categories parse; an unknown category still fails.
- `internal/prompts`: golden files regenerated for every `post_*` fixture; a test asserts the two
  sections precede comment hygiene.
- `scorecard`: normalisation table test; baseline preference; the two new rates on hand-built
  records; a 0.26.0-shaped record still scores.
- `internal/mcpsrv`: a completion call writes `categories` and line counts to the row; the outcome
  tool lists missing models.
- `go test -race ./...` at the root and in `gnome-topbar/daemon`.
- The effect of the prompt change is not unit-testable. It is judged on the field measures in
  §3.1.

## 9. Open questions for the maintainer

1. **Unevidenced CodeScene skip.** The agreed list says "accept a stated CodeScene skip reason
   once". The code grades a reason without evidence as major on purpose (§2.2). §5.2 accepts only
   an **evidenced** skip once and leaves the unevidenced case major. Accepting a bare reason
   reverses that design decision. Which is wanted?
2. **One plan per part.** The plan written with this spec covers Part 1 only, on the reading that
   each part is its own release. The earlier field-assessment work merged parts and released once.
   Confirm three releases.
3. **Baseline choice (§4.5).** Preferring a same-model baseline changes what the regression flag
   compares. Acceptable?
4. **`check_progress` cadence and scope (§5.1).** Every ten edits or once per task; and main
   sessions as well as dispatched subagents?
5. **CodeScene event channel (§6.3).** Server-written records, or document the two channels?
6. **Correctness at `check_progress`.** Should `mid.tmpl` also ask for correctness defects once
   the hook makes mid-task checks common, or stay drift-only?
7. **Retry cost.** A correctness review will produce more non-pass verdicts. §3.1 proposes 4.0
   calls per task as the limit before the prompt is tightened. Is that the right limit?
8. **Checklist (§6.2).** Move it out of findings, or keep it a finding and only narrow what
   qualifies?
9. **The findings file** is committed as written. Where its numbers differ from §2 (pass escape
   rate, passes carrying findings, unattributed findings, plan sessions), §2 is the corrected
   figure. Should the file be amended instead?
