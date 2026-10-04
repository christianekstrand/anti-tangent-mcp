# Field-data findings, 2026-09-16 to 2026-10-03

Input to the field-data improvements spec. Every number below was computed from the opt-in stats
files in `ANTI_TANGENT_STATS_DIR` (`events.jsonl`, `runs.jsonl`, `plan-runs.jsonl`,
`outcomes.jsonl`, `scorecard.json`, `rollup.json`) and from the guard trace log. The files hold
counts, verdicts, model ids and hashed session ids only.

Re-derive any number before relying on it in the spec; the files keep growing.

## Dataset

- 1,885 reviewer calls: `validate_completion` 1,042, `validate_task_spec` 570, `validate_plan` 268,
  `check_progress` 6.
- 193 task records across 28 runs (`runs.jsonl`, all server version 0.26.0, from 2026-09-24).
- 33 `record_review_outcome` records (all `source: final_review`) carrying 281 findings:
  208 minor, 71 major, 2 critical.
- Reviewer models: `openai:gpt-5.6-terra` 1,061 calls, `openai:gpt-5.6-sol` 818, `openai:gpt-5.6-luna` 6.

## 1. Verdicts do not predict final-review escapes

Share of tasks in which the final review found a major or critical problem:

| Signal | Value | Tasks | Major+ escape |
|---|---|---|---|
| Final completion verdict | pass | 152 | 20% |
| | warn | 24 | 21% |
| | fail | 13 | 23% |
| Spec verdict | pass | 76 | 17% |
| | warn | 64 | 25% |
| | fail | 34 | 26% |
| Completion attempts | 1 | 59 | 24% |
| | 2 | 68 | 15% |
| | 3 | 36 | 25% |
| | 4+ | 26 | 19% |

- Scorecard, all runs reviewed by terra: escape rate 27/152, minor escape rate 50/152,
  unconfirmed flag rate 29/37, waive rate 3/241, caught-and-fixed 78.
- There is no control arm. This shows the final verdict does not separate good tasks from bad
  ones; it does not show the retries fixed nothing.
- 15 "lite" tasks had 0 major escapes; 35 "escalated" tasks had 10. Both samples are small.

## 2. The reviewer flags process; the final review finds correctness

- Final-review finding categories: correctness 115 (45 major), docs 67, tests 36,
  maintainability 12, security 10, accessibility 7.
- Reviewer categories, all tools: `ambiguous_spec` 1,316, `quality` 1,312,
  `insufficient_evidence` 726, `unverifiable_codebase_claim` 699,
  `missing_acceptance_criterion` 518.
- The reviewer's category set has no correctness-type category.

Not yet checked: what the completion prompt (`internal/prompts/templates/post.tmpl`) actually asks
the reviewer to judge. The mismatch above is inferred from the category mix alone.

## 3. Completion retries: agents do circle back, but some findings never resolve

- Only 60 of 193 tasks closed in one `validate_completion` call. Mean 3.2 reviewer calls and 66s of
  reviewer time per task.
- Of 529 non-pass completion calls, 452 were followed by another call in the same session; 77 ended
  there (31 on a fail).
- After a substantive non-pass, payload size changed on 220 of 341 retries; median gap to the retry
  100s. After a process-only non-pass (only evidence or CodeScene categories), payload size was
  unchanged on 86 of 118 retries; median gap 45s.
- 76% of pass verdicts still carry findings.

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
- `codescene_skipped` repeats after the implementer has already stated a skip reason.
- "Cleared" means the reviewer stopped raising the category, not that the fix was verified.

## 4. `validate_plan` loops and is slow

- 268 calls collapse to about 60 plan sessions (heuristic: same task count within one hour).
  Calls per session: 12 sessions with 1, 10 with 2, 21 with 4 or 5, 12 with 6 to 14.
- Latency p50 95s, p95 245s, max 359s; 102 calls over 120s. Sessions with three or more calls used
  about 7.8 hours of reviewer time.
- Verdicts oscillate: `w>p>w>p`, `f>p>w>p`, `f>p>p>w>p>p>p`.
- Each call records a new run header: 99 of 127 headers have no task rows.
- Mean findings fell from 9.7 on the first call to 4.2 on the last in multi-call sessions.

## 5. The spec gate

- `codebase_reference_checklist` fired on 420 of 570 `validate_task_spec` calls, 128 of them passes.
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

- 6 calls in 18 days. 4 of 193 tasks had a checkpoint.

## 8. Scorecard inputs and coverage

- Cohorts are fragmented by model id spelling: `anthropic:claude-haiku-4-5` (10 runs) and
  `anthropic:claude-haiku-4-5-20251001` (6 runs) are separate cohorts. An `unknown` implementer
  cohort holds 38 tasks.
- Every cohort reads `regression: no_baseline`.
- 193 of 260 planned tasks have rows; 11 of 28 runs are fully covered.
- 86 of 281 final-review findings land on a task with no row (28 of those are unattributed,
  `task_index` 0), so they go unscored. 6 outcome runs have no task rows at all.
- `codescene-events.jsonl` is empty although 163 task rows report CodeScene as `ran`.

## 9. Guard plugin

- Comment guard: 38 blocks out of about 1,680 scanned writes. The semantic tier was off for 3,464
  writes (`jev-skip | setting`).
- Skipped by extension, though they can carry comments: `.mjs` 405 writes, `.vue` 151, `.kts` 42.
- Start gate: 720 passes, no blocks recorded.

## 10. No trend

Daily fail rate and findings per call are flat across the window. Median latency fell from about
35s to about 20s as calls moved from sol to terra.

## Proposed improvements, as agreed with the maintainer

1. Point `validate_completion` at correctness: add correctness and test-adequacy categories and have
   the reviewer judge the change against the acceptance criteria. Confirm against the prompt first.
2. Stop findings that repeat without resolving. Keep `codescene_not_run` verdict-driving. Accept a
   stated CodeScene skip reason once. Address repeating `quality` findings.
3. Make `validate_plan` converge: review the delta on a re-call, treat pass as terminal, keep one
   run id across revisions of the same plan.
4. Fix scorecard inputs: normalize implementer model ids, close the `unknown` cohort, establish a
   baseline, close the task-row coverage gap.
5. Trim `codebase_reference_checklist` so it stops firing on three-quarters of specs.
6. Trigger `check_progress` automatically, from the `anti-tangent-guard` plugin: a `PostToolUse`
   hook on `Edit`/`Write` that instructs the implementer to call it after a set number of edits
   since the last check, with its own kill switch. Starting point: once per task after about 10
   edits; tune from the stats.
7. Extend the comment guard to `.mjs`, `.vue`, `.kts`.
8. Lean guidance: record lines added and removed per task so it can be measured, and make an
   `over_building` finding that survives a retry require an explicit implementer ruling.
