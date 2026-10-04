# Field-data findings, 2026-09-16 to 2026-10-03

Input to the field-data improvements spec. Every number below was computed from the opt-in stats
files in `ANTI_TANGENT_STATS_DIR` (`events.jsonl`, `runs.jsonl`, `plan-runs.jsonl`,
`outcomes.jsonl`, `scorecard.json`, `rollup.json`) and from the guard trace log. The files hold
counts, verdicts, model ids and hashed session ids only.

Amended 2026-10-04: every number was re-derived against the raw files and the code while writing
`2026-10-04-field-data-improvements-design.md`, and the figures and causes below are the corrected
ones. Escape counts use the latest outcome per run, as the scorecard does. The files keep growing.

## Dataset

- 1,886 reviewer calls: `validate_completion` 1,042, `validate_task_spec` 570, `validate_plan` 268,
  `check_progress` 6.
- 193 task records across 28 runs (`runs.jsonl`, all server version 0.26.0, from 2026-09-24).
- 33 `record_review_outcome` records (all `source: final_review`, 32 distinct runs) carrying 281
  findings: 208 minor, 71 major, 2 critical.
- Reviewer models: `openai:gpt-5.6-terra` 1,062 calls, `openai:gpt-5.6-sol` 818, `openai:gpt-5.6-luna` 6.

## 1. Verdicts do not predict final-review escapes

Share of tasks in which the final review found a major or critical problem:

| Signal | Value | Tasks | Major+ escape |
|---|---|---|---|
| Final completion verdict | pass | 152 | 18% |
| | warn | 24 | 21% |
| | fail | 13 | 23% |
| Spec verdict | pass | 76 | 17% |
| | warn | 64 | 20% |
| | fail | 34 | 26% |
| `validate_completion` calls | 1 | 59 | 24% |
| | 2 | 68 | 13% |
| | 3 | 36 | 19% |
| | 4+ | 26 | 19% |

- Scorecard, all runs reviewed by terra: escape rate 27/152, minor escape rate 50/152,
  unconfirmed flag rate 29/37, waive rate 3/241, caught-and-fixed 78.
- There is no control arm. This shows the final verdict does not separate good tasks from bad
  ones; it does not show the retries fixed nothing.
- 15 "lite" tasks had 0 major escapes; 35 "escalated" tasks had 9. Both samples are small.

## 2. The reviewer flags process; the final review finds correctness

- Final-review finding categories: correctness 115 (46 critical or major, of 73), docs 67, tests 36,
  maintainability 12, security 10, accessibility 7.
- Reviewer categories, all tools: `ambiguous_spec` 1,316, `quality` 1,312,
  `insufficient_evidence` 726, `unverifiable_codebase_claim` 699,
  `missing_acceptance_criterion` 518.
- The reviewer's category set has no correctness-type category.

Checked against `internal/prompts/templates/post.tmpl`: the prompt does ask the reviewer to judge
every acceptance criterion against the evidence. It does not ask for defects the criteria do not
state, has no category for one, and reserves critical and major for a contradicted or unaddressed
criterion.

## 3. Completion retries: agents do circle back, but some findings never resolve

- Only 60 of 193 tasks closed in one `validate_completion` call. Mean 3.2 reviewer calls and 66s of
  reviewer time per task.
- Of 559 non-pass completion calls made inside a session, 482 were followed by another call in the
  same session; 77 ended there (31 on a fail).
- After a substantive non-pass, payload size changed on 220 of 341 retries; median gap to the retry
  100s. After a process-only non-pass (only evidence or CodeScene categories), payload size was
  unchanged on 86 of 118 retries; median gap 45s.
- 65% of pass verdicts (294 of 455) still carry findings.

Finding present on a non-pass call and absent on the very next call:

| Finding | Cleared |
|---|---|
| `codescene_not_run` | 136 of 138 |
| `insufficient_evidence` | 112 of 204 |
| `missing_acceptance_criterion` | 98 of 187 |
| `comment_hygiene` | 61 of 127 |
| `ambiguous_spec` | 43 of 92 |
| `over_building` | 6 of 20 |
| `quality` | 52 of 206 |
| `codescene_skipped` | 7 of 62 |

- `codescene_not_run` never appears on a pass (84 warn, 78 fail) and works as enforcement.
- `codescene_skipped` is derived from the `codescene` argument on every call, with no memory. A skip
  with reason and evidence returns a minor "no action needed" finding each time (47 of the 118
  calls carrying the category had no critical or major finding). A skip with a reason and no
  evidence is graded major on purpose, equal to not running CodeScene.
- "Cleared" means the reviewer stopped raising the category, not that the fix was verified.

## 4. `validate_plan` loops and is slow

- 268 calls collapse to 64 plan sessions (heuristic: same task count within one hour).
  Calls per session: 7 sessions with 1, 15 with 2, 28 with 3 to 5, 14 with 6 or more.
- Latency p50 95s, p95 245s, max 359s; 102 calls over 120s. Sessions with three or more calls used
  about 7.5 hours of reviewer time.
- Verdicts oscillate: `w>p>w>p`, `f>p>w>p`, `f>p>p>w>p>p>p`.
- Each call that is not a cache hit records a new run header: 99 of 127 headers have no task rows.
  Only 4 calls were cache hits.
- 52 sessions reached a pass and 120 calls came after the first pass. The payload size differed on
  all 97 calls that directly followed a pass: controllers edit a passed plan and re-submit it.
- Mean findings fell from 10.6 on the first call to 4.8 on the last in multi-call sessions.

## 5. The spec gate

- `codebase_reference_checklist` fired on 420 of 570 `validate_task_spec` calls, 128 of them passes,
  and was the only finding on 56. `pre.tmpl` asks for a finding on every codebase claim the
  reviewer cannot verify, and this tool never sends code.
- 34 tasks got a spec fail. Of 178 tasks with a spec call, 168 made exactly one, so most failed
  specs proceeded without re-validation.
- `noise_cluster` fired on 75 spec calls, all warn.

## 6. Lean guidance (`over_building`)

- `over_building` appears on 49 of 1,042 completion calls; 20 of those are passes. It is pinned to
  minor severity in the templates.
- Of 24 followed by a retry, 6 cleared. Payload shrank on 5 of 23 of those retries and grew on 15.
- The stats record no code-size measure, so the preventive effect of `implementation_guidance`
  cannot be measured.

## 7. `check_progress`

- 6 calls in 18 days. 4 of 193 tasks had a checkpoint. The protocol tells implementers the call is
  optional and to make it only when they suspect drift.

## 8. Scorecard inputs and coverage

- Cohorts are fragmented by model id spelling: `anthropic:claude-haiku-4-5` (10 runs) and
  `anthropic:claude-haiku-4-5-20251001` (6 runs) are separate cohorts. An `unknown` implementer
  cohort holds 38 tasks: the model comes only from an optional `record_review_outcome` argument,
  and 34 of the 38 belong to outcome records that sent none.
- Every cohort reads `regression: no_baseline`. This is structural: a baseline is an earlier,
  time-disjoint cohort, and every record is server 0.26.0.
- 193 of 260 planned tasks have rows; 11 of 28 runs are fully covered.
- 86 of 281 final-review findings land on a task with no row (34 of those are unattributed,
  `task_index` 0), so they go unscored. 6 outcome runs have no task rows at all.
- `codescene-events.jsonl` is empty although 163 task rows report CodeScene as `ran`. Not a defect:
  the server never writes that file (an optional operator hook does), and the row state comes from
  the `codescene` argument.

## 9. Guard plugin

- Comment guard: 38 blocks out of about 1,680 scanned writes. The semantic tier was off for 3,468
  writes (`jev-skip | setting`).
- Skipped by extension, though they can carry comments: `.mjs` 405 writes, `.vue` 151, `.kts` 42.
- Start gate: 720 passes, no blocks recorded, and 6,253 evaluations that were not gated (not a
  dispatched full-protocol subagent).

## 10. No trend

Daily fail rate and findings per call are flat across the window. Median latency fell from about
35s to about 20s as calls moved from sol to terra.

## Proposed improvements, as agreed with the maintainer

Amended with the maintainer's rulings of 2026-10-04. All eight ship in one release, built as three
parts.

1. Point `validate_completion` at correctness: add correctness and test-adequacy categories and have
   the reviewer look for defects the acceptance criteria do not state. A prompt and a schema change.
2. Stop findings that repeat without resolving. Keep `codescene_not_run` verdict-driving. Report a
   CodeScene skip that carries a reason and evidence once per task; a reason without evidence stays
   major. Address repeating `quality` findings.
3. Make `validate_plan` converge: review the delta on a re-call, treat pass as terminal, keep one
   run id across revisions of the same plan.
4. Fix scorecard inputs: normalize implementer model ids, close the `unknown` cohort, establish a
   baseline, close the task-row coverage gap.
5. Move `codebase_reference_checklist` out of the findings into its own response field.
6. Trigger `check_progress` automatically, from the `anti-tangent-guard` plugin: a `PostToolUse`
   hook on `Edit`/`Write` that instructs the implementer to call it once per task, after 10 edits,
   with its own kill switch; tune the threshold from the stats. `check_progress` also looks for
   correctness defects.
7. Extend the comment guard to `.mjs`, `.vue`, `.kts`.
8. Lean guidance: record lines added and removed per task so it can be measured, and make an
   `over_building` finding that survives a retry require an explicit implementer ruling.
