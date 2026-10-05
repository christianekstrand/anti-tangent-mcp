package mcpsrv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/planparser"
	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/providers"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

// roundTask is one task's scripted reviewer result: its heading and its
// findings, each a JSON object literal.
type roundTask struct {
	title    string
	findings []string
}

func roundTasksJSON(tasks []roundTask) string {
	items := make([]string, len(tasks))
	for i, task := range tasks {
		title, _ := json.Marshal(task.title)
		items[i] = fmt.Sprintf(`{"task_index":%d,"task_title":%s,"verdict":"warn","findings":[%s],"suggested_header_block":"","suggested_header_reason":""}`,
			i+1, title, strings.Join(task.findings, ","))
	}
	return strings.Join(items, ",")
}

// roundSingleResp is a whole-plan reviewer response: the first round of a
// plan small enough for one call.
func roundSingleResp(planFindings string, tasks ...roundTask) providers.Response {
	return providers.Response{Model: "claude-sonnet-4-6", RawJSON: []byte(
		`{"plan_verdict":"warn","plan_quality":"actionable","plan_findings":[` + planFindings +
			`],"tasks":[` + roundTasksJSON(tasks) + `],"next_action":"round one"}`)}
}

// roundPlanLevelResp is a plan-level pass response.
func roundPlanLevelResp(planFindings, nextAction string) providers.Response {
	return providers.Response{Model: "claude-sonnet-4-6", RawJSON: []byte(
		`{"plan_verdict":"warn","plan_quality":"actionable","plan_findings":[` + planFindings +
			`],"next_action":"` + nextAction + `"}`)}
}

// roundChunkResp is one task chunk's response.
func roundChunkResp(tasks ...roundTask) providers.Response {
	return providers.Response{Model: "claude-sonnet-4-6", RawJSON: []byte(`{"tasks":[` + roundTasksJSON(tasks) + `]}`)}
}

const (
	roundMajor = `{"severity":"major","category":"ambiguous_spec","criterion":"ac1","evidence":"vague","suggestion":"rewrite"}`
	roundMinor = `{"severity":"minor","category":"quality","criterion":"naming","evidence":"unclear name","suggestion":"rename"}`
	roundOrder = `{"severity":"major","category":"ambiguous_spec","criterion":"task order","evidence":"Task 2 needs Task 3","suggestion":"reorder"}`
)

func roundTitles(n int) []roundTask {
	out := make([]roundTask, n)
	for i, title := range titlesRange(1, n) {
		out[i] = roundTask{title: title}
	}
	return out
}

// planWithEditedTask is buildPlanWithNTasks(n) with task k's acceptance
// criterion reworded.
func planWithEditedTask(n, k int) string {
	return strings.Replace(buildPlanWithNTasks(n), fmt.Sprintf("- ac%d\n", k), fmt.Sprintf("- ac%d, measured\n", k), 1)
}

// buildRawTasks returns one parsed task per title, each with its heading and
// a one-line body.
func buildRawTasks(titles ...string) []planparser.RawTask {
	out := make([]planparser.RawTask, len(titles))
	for i, title := range titles {
		out[i] = planparser.RawTask{Title: title, Body: "### " + title + "\n\nbody\n"}
	}
	return out
}

func roundHandlers(t *testing.T, chunkSize int, responses ...providers.Response) (*handlers, *scriptedReviewer) {
	t.Helper()
	sr := &scriptedReviewer{responses: responses}
	return &handlers{deps: newDepsWithScripted(t, sr, chunkSize)}, sr
}

func validatePlanRound(t *testing.T, h *handlers, args ValidatePlanArgs) verdict.PlanResult {
	t.Helper()
	_, pr, err := h.ValidatePlan(context.Background(), nil, args)
	require.NoError(t, err)
	pr.PlanFindings = stripPlanDeprecationFinding(pr.PlanFindings)
	return pr
}

func TestValidatePlan_AnUnchangedRoundMakesNoReviewerCall(t *testing.T) {
	tasks := roundTitles(3)
	tasks[1].findings = []string{roundMajor}
	h, sr := roundHandlers(t, 8, roundSingleResp(roundMinor, tasks...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})
	require.Equal(t, 1, sr.calls)
	require.NotEmpty(t, first.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 1, TasksReviewed: 3, TasksCarried: 0, PlanLevelReviewed: true}, first.ReviewScope)

	// The pass cache holds nothing for a non-pass result, and would be stale
	// anyway: the run is what answers.
	h.planCache().expireForTest()
	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3), PlanRunID: first.PlanRunID})

	assert.Equal(t, 1, sr.calls, "an unchanged plan must not reach the reviewer")
	assert.Equal(t, first.PlanRunID, second.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 0, TasksCarried: 3, PlanLevelReviewed: false}, second.ReviewScope)
	assert.Equal(t, first.PlanVerdict, second.PlanVerdict)
	assert.Equal(t, first.PlanFindings, second.PlanFindings)
	assert.Equal(t, first.Tasks, second.Tasks)
	assert.Equal(t, first.NextAction, second.NextAction)
	assert.Contains(t, second.SummaryBlock, "  review_scope:  revision 2: 0 task(s) reviewed, 3 carried; plan-level findings carried\n")

	_, revision, ok := h.deps.PlanRuns.Review(first.PlanRunID)
	require.True(t, ok)
	assert.Equal(t, 2, revision)
}

func TestValidatePlan_AChangedTaskIsTheOnlyTaskReviewed(t *testing.T) {
	tasks := roundTitles(3)
	tasks[0].findings = []string{roundMinor}
	tasks[1].findings = []string{roundMajor}
	h, sr := roundHandlers(t, 8,
		roundSingleResp(roundOrder, tasks...),
		roundPlanLevelResp("", "round two"),
		roundChunkResp(roundTask{title: "Task 2: t2"}),
	)

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})
	require.Len(t, first.Tasks[1].Findings, 1)

	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(3, 2), PlanRunID: first.PlanRunID})

	require.Equal(t, 3, sr.calls, "one plan-level call and one chunk call")
	assert.Equal(t, first.PlanRunID, second.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 1, TasksCarried: 2, PlanLevelReviewed: true}, second.ReviewScope)
	assert.Contains(t, second.SummaryBlock, "  review_scope:  revision 2: 1 task(s) reviewed, 2 carried; plan-level findings reviewed\n")

	planLevel, chunk := sr.requests[1], sr.requests[2]
	assert.Contains(t, planLevel.User, "## Earlier plan-level findings")
	assert.Contains(t, planLevel.User, "- [major][ambiguous_spec] task order — Task 2 needs Task 3\n")
	assert.Contains(t, chunk.User, "- Task 2: t2\n")
	assert.NotContains(t, chunk.User, "- Task 1: t1\n", "an unchanged task is not sent for review")
	assert.NotContains(t, chunk.User, "- Task 3: t3\n")
	assert.Contains(t, chunk.CachePrefix, "- ac2, measured", "the reviewer sees the edited plan")

	assert.Empty(t, second.PlanFindings, "the plan-level pass dropped the resolved finding")
	assert.Equal(t, "round two", second.NextAction)
	require.Len(t, second.Tasks, 3)
	assert.Equal(t, first.Tasks[0], second.Tasks[0], "an unchanged task keeps its result")
	assert.Empty(t, second.Tasks[1].Findings, "the edited task takes its new result")
	assert.Equal(t, first.Tasks[2], second.Tasks[2])
	for i, task := range second.Tasks {
		assert.Equal(t, i+1, task.TaskIndex)
	}
}

func TestValidatePlan_AChangeOutsideEveryTaskReRunsOnlyThePlanLevelPass(t *testing.T) {
	h, sr := roundHandlers(t, 8,
		roundSingleResp(roundOrder, roundTitles(2)...),
		roundPlanLevelResp(roundOrder, "still open"),
	)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})

	edited := strings.Replace(buildPlanWithNTasks(2), "# Plan\n", "# Plan\n\nTasks run in order.\n", 1)
	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: edited, PlanRunID: first.PlanRunID})

	assert.Equal(t, 2, sr.calls)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 0, TasksCarried: 2, PlanLevelReviewed: true}, second.ReviewScope)
	require.Len(t, second.PlanFindings, 1)
	assert.Equal(t, first.PlanFindings[0].ID, second.PlanFindings[0].ID, "a finding raised again keeps its id")
	assert.Equal(t, first.Tasks, second.Tasks)
}

func TestValidatePlan_AnInsertedTaskIsReviewedAndTheRestCarried(t *testing.T) {
	tasks := roundTitles(2)
	tasks[1].findings = []string{roundMinor}
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", tasks...),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 3: t3", findings: []string{roundMajor}}),
	)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3), PlanRunID: first.PlanRunID})

	assert.Equal(t, 3, sr.calls)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 1, TasksCarried: 2, PlanLevelReviewed: true}, second.ReviewScope)
	require.Len(t, second.Tasks, 3)
	assert.Equal(t, first.Tasks[1].Findings, second.Tasks[1].Findings)
	assert.Equal(t, 3, second.Tasks[2].TaskIndex, "a chunk's task_index is replaced by the plan position")
	assert.Len(t, second.Tasks[2].Findings, 1)

	count, ok := h.deps.PlanRuns.PlanTaskCount(first.PlanRunID)
	require.True(t, ok)
	assert.Equal(t, 3, count, "the run takes the revised task list")
}

func TestValidatePlan_ARulingAppliesToACarriedTaskWithoutAReviewerCall(t *testing.T) {
	tasks := roundTitles(1)
	tasks[0].findings = []string{roundMajor}
	h, sr := roundHandlers(t, 8, roundSingleResp("", tasks...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(1)})
	require.Equal(t, verdict.VerdictWarn, first.Tasks[0].Verdict)
	require.Len(t, first.Tasks[0].Findings, 1)

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText:  buildPlanWithNTasks(1),
		PlanRunID: first.PlanRunID,
		ControllerRulings: []ControllerRulingArg{{
			FindingID: first.Tasks[0].Findings[0].ID, Ruling: "ac1 is pinned by the test in step 1",
		}},
	})

	assert.Equal(t, 1, sr.calls, "a ruling is applied to the stored reviewer output")
	assert.Empty(t, second.Tasks[0].Findings)
	require.Len(t, second.Tasks[0].WaivedFindings, 1)
	assert.Equal(t, verdict.VerdictPass, second.Tasks[0].Verdict)

	third := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(1), PlanRunID: first.PlanRunID})
	assert.Equal(t, 1, sr.calls)
	assert.Len(t, third.Tasks[0].Findings, 1, "the run keeps the reviewer's finding, not the waived result")
}

func TestValidatePlan_AnUnknownRunIDReviewsEverythingUnderANewRun(t *testing.T) {
	h, sr := roundHandlers(t, 8, roundSingleResp("", roundTitles(2)...))

	pr := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2), PlanRunID: "pr_0123456789ab"})

	assert.Equal(t, 1, sr.calls)
	assert.NotEqual(t, "pr_0123456789ab", pr.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 1, TasksReviewed: 2, TasksCarried: 0, PlanLevelReviewed: true}, pr.ReviewScope)
	assert.Equal(t, verdict.VerdictPass, pr.PlanVerdict, "the advisory describes the call, not the plan")
	require.Len(t, pr.PlanFindings, 1)
	advisory := pr.PlanFindings[0]
	assert.Equal(t, "plan_run_id", advisory.Criterion)
	assert.Equal(t, verdict.SeverityMinor, advisory.Severity)
	assert.Contains(t, advisory.Evidence, "pr_0123456789ab names no live plan run")
	assert.Contains(t, advisory.Suggestion, "plan_run_id="+pr.PlanRunID)
}

func TestValidatePlan_ChangedInputsReviewEveryTaskUnderTheSameRun(t *testing.T) {
	h, sr := roundHandlers(t, 8,
		roundSingleResp(roundOrder, roundTitles(2)...),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTitles(2)...),
	)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: buildPlanWithNTasks(2), PlanRunID: first.PlanRunID,
		ProjectKnowledge: "Decision: tasks run in order.",
	})

	assert.Equal(t, 3, sr.calls)
	assert.Equal(t, first.PlanRunID, second.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 2, TasksCarried: 0, PlanLevelReviewed: true}, second.ReviewScope)
	assert.NotContains(t, sr.requests[1].User, "## Earlier plan-level findings",
		"findings raised under other inputs are not shown as this plan's earlier findings")
}

func TestValidatePlan_ATruncatedRoundKeepsTheRunAndItsStoredReview(t *testing.T) {
	tasks := roundTitles(2)
	tasks[0].findings = []string{roundMinor}
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", tasks...),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 2: t2"}),
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})

	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)

	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(2, 2), PlanRunID: first.PlanRunID})
	assert.True(t, cut.Partial)
	assert.Equal(t, first.PlanRunID, cut.PlanRunID, "a truncated round still answers under the run it named")
	assert.Nil(t, cut.ReviewScope)
	require.Len(t, cut.Tasks, 1, "the carried task survives; the truncated one has no result")
	assert.Equal(t, "Task 1: t1", cut.Tasks[0].TaskTitle)
	assert.Equal(t, verdict.VerdictWarn, cut.PlanVerdict, "a round that did not review the edited task must not pass")
	require.Len(t, cut.PlanFindings, 1)
	assert.Equal(t, verdict.SeverityMajor, cut.PlanFindings[0].Severity)
	assert.Equal(t, "reviewer_response", cut.PlanFindings[0].Criterion)
	assert.Contains(t, cut.PlanFindings[0].Evidence, "1 of the plan's 2 task(s) have no result")
	assert.NotContains(t, cut.PlanFindings[0].Evidence, "plan-level pass did not finish")
	assert.Contains(t, cut.NextAction, "max_tokens_override")
	_, revision, _ := h.deps.PlanRuns.Review(first.PlanRunID)
	assert.Equal(t, 1, revision, "a truncated round does not revise the run")

	retry := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(2, 2), PlanRunID: first.PlanRunID})
	assert.Equal(t, 5, sr.calls, "the retry reviews the changed task again")
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 1, TasksCarried: 1, PlanLevelReviewed: true}, retry.ReviewScope)
	assert.Len(t, retry.Tasks, 2)
}

func TestValidatePlan_AChunkedFirstRoundCarriesByTaskTextNotByChunk(t *testing.T) {
	h, sr := roundHandlers(t, 2,
		passOneResp(),
		chunkResp(t, titlesRange(1, 2)),
		chunkResp(t, titlesRange(3, 3)),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 1: t1"}, roundTask{title: "Task 3: t3"}),
	)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})
	require.Equal(t, 3, sr.calls)

	edited := strings.Replace(planWithEditedTask(3, 1), "- ac3\n", "- ac3, measured\n", 1)
	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: edited, PlanRunID: first.PlanRunID})

	assert.Equal(t, 5, sr.calls, "the two changed tasks share one chunk call")
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 2, TasksCarried: 1, PlanLevelReviewed: true}, second.ReviewScope)
	require.Len(t, second.Tasks, 3)
	assert.Equal(t, []string{"Task 1: t1", "Task 2: t2", "Task 3: t3"},
		[]string{second.Tasks[0].TaskTitle, second.Tasks[1].TaskTitle, second.Tasks[2].TaskTitle})
}

func TestValidatePlan_ACacheHitReportsThatNothingWasReviewed(t *testing.T) {
	h, sr := roundHandlers(t, 8, roundSingleResp("", roundTitles(2)...), roundSingleResp("", roundTitles(2)...))
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)

	hit := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, first.PlanRunID, hit.PlanRunID)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 1, TasksReviewed: 0, TasksCarried: 2, PlanLevelReviewed: false}, hit.ReviewScope)

	// A cached entry whose run a later round revised to another plan text
	// describes a plan the run does not hold, so the call is a miss.
	h2, sr2 := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(2)...),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 2: t2"}),
		roundSingleResp("", roundTitles(2)...),
	)
	one := validatePlanRound(t, h2, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	validatePlanRound(t, h2, ValidatePlanArgs{PlanText: planWithEditedTask(2, 2), PlanRunID: one.PlanRunID})
	again := validatePlanRound(t, h2, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	assert.Equal(t, 4, sr2.calls, "the stale entry is not served")
	assert.NotEqual(t, one.PlanRunID, again.PlanRunID)
}

func TestPlanRound_CarryMatchesADuplicatedTaskOncePerOccurrence(t *testing.T) {
	h := newTestPlanHandlers(t)
	tasks := buildRawTasks("Task 1: a", "Task 1: a", "Task 2: b")
	run := h.deps.PlanRuns.Create("pass", "rigorous", 3)
	inputs := planInputs{Model: "m"}

	first := h.newPlanRound(run.ID, "pre", tasks, inputs)
	require.Equal(t, run.ID, first.RunID)
	assert.Len(t, first.changed, 3, "a run with no stored review carries nothing")
	raw := verdict.PlanResult{Tasks: []verdict.PlanTaskResult{
		{TaskIndex: 1, TaskTitle: "Task 1: a"}, {TaskIndex: 2, TaskTitle: "Task 1: a"}, {TaskIndex: 3, TaskTitle: "Task 2: b"},
	}}
	require.True(t, h.deps.PlanRuns.SetReview(run.ID, first.review(raw, tasks, "m")))

	same := h.newPlanRound(run.ID, "pre", tasks, inputs)
	assert.Empty(t, same.changed)
	assert.False(t, same.planLevel)

	grown := h.newPlanRound(run.ID, "pre", append(buildRawTasks("Task 1: a"), tasks...), inputs)
	assert.Len(t, grown.changed, 1, "a third copy of a task the earlier plan held twice has no result to carry")
	assert.True(t, grown.planLevel)

	otherInputs := h.newPlanRound(run.ID, "pre", tasks, planInputs{Model: "other"})
	assert.Len(t, otherInputs.changed, 3)
	assert.Nil(t, otherInputs.prior)
}

func TestValidatePlan_EachRoundWritesAHeaderUnderTheSameRunHash(t *testing.T) {
	dir := t.TempDir()
	h, _ := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(3)...),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 2: t2"}),
	)
	rec := newStatsRecorder(t, dir)
	h.deps.Stats = rec
	h.deps.PlanLedger = &planrun.Ledger{Dir: dir}

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})
	validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(3, 2), PlanRunID: first.PlanRunID})

	lines, err := rec.RunLines(rec.RunHash(first.PlanRunID))
	require.NoError(t, err)
	require.Len(t, lines, 2, "one header per round, both under the run's hash")
	assert.True(t, lines[0].Header && lines[1].Header)
	assert.Equal(t, []int{1, 2}, []int{lines[0].Revision, lines[1].Revision})
	assert.Equal(t, []int{0, 2}, []int{lines[0].TasksCarried, lines[1].TasksCarried})
	assert.Equal(t, 3, lines[1].TaskCount)

	b, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	require.NoError(t, err)
	events := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	require.Len(t, events, 2)
	assert.NotContains(t, events[0], "tasks_carried")
	assert.Contains(t, events[1], `"tasks_carried":2`)

	loaded, ok := h.deps.PlanLedger.Load(first.PlanRunID)
	require.True(t, ok)
	assert.Equal(t, 2, loaded.Revision, "the ledger keeps the latest round's header")
}

func TestValidatePlan_ATruncatedPlanLevelPassDoesNotReturnTheEarlierPass(t *testing.T) {
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(2)...),
		providers.Response{RawJSON: []byte(`{"plan_verdict":"pass","plan_findings":[`)},
	)
	sr.errors = []error{nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)

	edited := strings.Replace(buildPlanWithNTasks(2), "# Plan\n", "# Plan\n\nTasks run in order.\n", 1)
	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: edited, PlanRunID: first.PlanRunID})

	assert.Equal(t, verdict.VerdictWarn, cut.PlanVerdict)
	assert.True(t, cut.Partial)
	assert.Len(t, cut.Tasks, 2, "both tasks are unchanged and carried")
	require.Len(t, cut.PlanFindings, 1)
	assert.Contains(t, cut.PlanFindings[0].Evidence, "0 of the plan's 2 task(s) have no result")
	assert.Contains(t, cut.PlanFindings[0].Evidence, "The plan-level pass did not finish either")
	assert.NotEqual(t, first.NextAction, cut.NextAction, "the earlier round's next_action is not this round's")
}

// A chunk cut short after one complete task result: the complete one numbers
// itself from the chunk, which collides with a carried task's plan position.
// The round reads nothing from the partial bytes, so no result can be dropped
// in favour of, or filed under, another task.
func TestValidatePlan_ATruncatedChunkFilesNothingUnderAnotherTask(t *testing.T) {
	partial := `{"tasks":[{"task_index":1,"task_title":"Task 2: t2","verdict":"warn","findings":[` + roundMajor +
		`],"suggested_header_block":"","suggested_header_reason":""},{"task_index":2,"task_title":"Task 4: t4","verdict":"wa`
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(3)...),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(partial)},
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})

	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(4, 2), PlanRunID: first.PlanRunID})

	assert.Equal(t, verdict.VerdictWarn, cut.PlanVerdict)
	require.Len(t, cut.Tasks, 2, "only the carried tasks have results")
	assert.Equal(t, []string{"Task 1: t1", "Task 3: t3"}, []string{cut.Tasks[0].TaskTitle, cut.Tasks[1].TaskTitle})
	assert.Equal(t, []int{1, 3}, []int{cut.Tasks[0].TaskIndex, cut.Tasks[1].TaskIndex})
	for _, task := range cut.Tasks {
		assert.Empty(t, task.Findings)
	}
	assert.Contains(t, cut.PlanFindings[len(cut.PlanFindings)-1].Evidence, "2 of the plan's 4 task(s) have no result")
}

func TestValidatePlan_AnUnchangedPassingRoundMakesNoReviewerCallHoweverOld(t *testing.T) {
	dir := t.TempDir()
	h, sr := roundHandlers(t, 8, roundSingleResp("", roundTitles(2)...))
	rec := newStatsRecorder(t, dir)
	h.deps.Stats = rec
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)

	h.planCache().expireForTest()
	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2), PlanRunID: first.PlanRunID})

	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, verdict.VerdictPass, second.PlanVerdict)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 0, TasksCarried: 2}, second.ReviewScope)

	lines, err := rec.RunLines(rec.RunHash(first.PlanRunID))
	require.NoError(t, err)
	require.Len(t, lines, 2)
	require.NotNil(t, lines[1].PlanCall, "a round with no reviewer call keeps the last real plan call on its header")
	assert.Equal(t, *lines[0].PlanCall, *lines[1].PlanCall)
	assert.Equal(t, "anthropic:claude-sonnet-4-6", lines[1].PlanCall.Model)
}

func TestValidatePlan_AnUnknownRunIDAnsweredFromThePassCacheSaysSoTruthfully(t *testing.T) {
	h, sr := roundHandlers(t, 8, roundSingleResp("", roundTitles(2)...))
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})

	hit := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2), PlanRunID: "pr_0123456789ab"})

	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, first.PlanRunID, hit.PlanRunID)
	require.Len(t, hit.PlanFindings, 1)
	assert.Contains(t, hit.PlanFindings[0].Evidence, "this response belongs to plan run "+first.PlanRunID)
	assert.NotContains(t, hit.PlanFindings[0].Evidence, "was reviewed")
}

func TestValidatePlanTool_DescribesPlanRunID(t *testing.T) {
	descs := allPropertyDescriptions(t)
	assert.Contains(t, descs["validate_plan.plan_run_id"], "only the tasks whose text changed")
	assert.Contains(t, validatePlanTool().Description, "plan_run_id")
}

func TestValidatePlan_ARulingDoesNotWaiveATruncatedRoundsFinding(t *testing.T) {
	cutChunk := providers.Response{RawJSON: []byte(`{"tasks":[`)}
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(2)...),
		roundPlanLevelResp("", "n"), cutChunk,
		roundPlanLevelResp("", "n"), cutChunk,
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)

	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(2, 2), PlanRunID: first.PlanRunID})
	require.Len(t, cut.PlanFindings, 1)
	require.NotEmpty(t, cut.PlanFindings[0].ID)

	ruled := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: planWithEditedTask(2, 2), PlanRunID: first.PlanRunID,
		ControllerRulings: []ControllerRulingArg{{FindingID: cut.PlanFindings[0].ID, Ruling: "the truncation is fine"}},
	})

	assert.Equal(t, verdict.VerdictWarn, ruled.PlanVerdict, "a ruling must not turn an unfinished round into a pass")
	assert.True(t, ruled.Partial)
	assert.Empty(t, ruled.WaivedFindings)
	require.Len(t, ruled.PlanFindings, 1)
	assert.Equal(t, "reviewer_response", ruled.PlanFindings[0].Criterion)
	assert.Equal(t, cut.PlanFindings[0].ID, ruled.PlanFindings[0].ID)
}

func TestPlanRound_HashesKeepBytesJSONWouldReplace(t *testing.T) {
	a := planparser.RawTask{Title: "Task 1: a", Body: "### Task 1: a\n\nbody \xff\n"}
	b := planparser.RawTask{Title: "Task 1: a", Body: "### Task 1: a\n\nbody \xfe\n"}
	assert.NotEqual(t, planTaskHash(a), planTaskHash(b), "two bodies that differ only in invalid UTF-8 are two tasks")
	assert.NotEqual(t,
		planTaskHash(planparser.RawTask{Title: "ab", Body: "c"}),
		planTaskHash(planparser.RawTask{Title: "a", Body: "bc"}), "a field boundary is part of the hash")

	h := newTestPlanHandlers(t)
	tasks := buildRawTasks("Task 1: a")
	assert.NotEqual(t,
		h.newPlanRound("", "pre \xff", tasks, planInputs{}).planKey,
		h.newPlanRound("", "pre \xfe", tasks, planInputs{}).planKey)
	assert.NotEqual(t, planInputs{ProjectKnowledge: "k \xff"}.key(), planInputs{ProjectKnowledge: "k \xfe"}.key())
}

func TestPlanInputs_KeyIgnoresHowTheSameReviewWasAskedFor(t *testing.T) {
	assert.Equal(t, planInputs{Model: "m"}.key(), planInputs{Model: "m", Mode: "thorough"}.key(),
		"an omitted mode is the thorough review")
	assert.NotEqual(t, planInputs{Model: "m"}.key(), planInputs{Model: "m", Mode: "quick"}.key())

	x := fileSource{Path: "/r/x.go", Bytes: 3, SHA256: "aa"}
	y := fileSource{Path: "/r/y.go", Bytes: 4, SHA256: "bb"}
	inOrder := planInputs{ContextFiles: []fileSource{x, y}}
	reversed := planInputs{ContextFiles: []fileSource{y, x}}
	assert.Equal(t, inOrder.key(), reversed.key(), "the same attached files in another order are the same inputs")
	assert.Equal(t, []fileSource{y, x}, reversed.ContextFiles, "the caller's slice is not reordered")
	assert.NotEqual(t, inOrder.key(), planInputs{ContextFiles: []fileSource{x, {Path: "/r/y.go", Bytes: 4, SHA256: "cc"}}}.key())
	assert.NotEqual(t, inOrder.key(), planInputs{ContextFiles: []fileSource{x}}.key())
}

func TestUnknownPlanRunAdvisory_CapsTheEchoedID(t *testing.T) {
	long := strings.Repeat("x", 5000)
	f := unknownPlanRunAdvisory(long, "pr_0123456789ab")
	assert.Contains(t, f.Evidence, "plan_run_id "+strings.Repeat("x", 64)+"… names no live plan run")
	assert.Less(t, len(f.Evidence), 400)
	assert.Contains(t, unknownPlanRunAdvisory("pr_short", "pr_0123456789ab").Evidence, "plan_run_id pr_short names no live plan run")
}

func TestPlanCallContext_ARunThatExpiredDuringItsRoundIsReported(t *testing.T) {
	h := newTestPlanHandlers(t)
	tasks := buildRawTasks("Task 1: a")
	call := planCallContext{
		PlanRuns: h.deps.PlanRuns,
		Round:    planRound{RunID: "pr_gone00000000", Revision: 1},
		Tasks:    tasks,
	}
	pr := verdict.PlanResult{
		PlanVerdict: verdict.VerdictPass,
		Tasks:       []verdict.PlanTaskResult{{TaskIndex: 1, TaskTitle: "Task 1: a", Verdict: verdict.VerdictPass}},
		ReviewScope: &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 1},
	}

	call.settlePlanRun(&pr, &planReview{})
	call.finish(&pr)

	assert.NotEqual(t, "pr_gone00000000", pr.PlanRunID)
	_, _, ok := h.deps.PlanRuns.Review(pr.PlanRunID)
	assert.True(t, ok, "the review is stored on the run minted in its place")
	assert.Equal(t, 1, pr.ReviewScope.Revision)
	require.Len(t, pr.PlanFindings, 1)
	assert.Equal(t, "plan_run_id", pr.PlanFindings[0].Criterion)
	assert.Equal(t, verdict.SeverityMinor, pr.PlanFindings[0].Severity)
	assert.Contains(t, pr.PlanFindings[0].Evidence, "plan_run_id pr_gone00000000 names no live plan run")
	assert.Contains(t, pr.PlanFindings[0].Suggestion, "plan_run_id="+pr.PlanRunID)
	assert.Equal(t, verdict.VerdictPass, pr.PlanVerdict)
}

func TestValidatePlan_ATruncatedRoundRecordsItsCarriedTasks(t *testing.T) {
	dir := t.TempDir()
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(3)...),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	h.deps.Stats = newStatsRecorder(t, dir)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(3)})

	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: planWithEditedTask(3, 2), PlanRunID: first.PlanRunID})
	require.True(t, cut.Partial)

	b, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	require.NoError(t, err)
	events := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	require.Len(t, events, 2)
	assert.Contains(t, events[1], `"partial":true`)
	assert.Contains(t, events[1], `"tasks_carried":2`)
}

func TestValidatePlan_ACachedPassIsNotServedOnceItsRunWasRevisedUnderOtherInputs(t *testing.T) {
	h, sr := roundHandlers(t, 8,
		roundSingleResp("", roundTitles(2)...),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTitles(2)...),
		roundSingleResp("", roundTitles(2)...),
	)
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)
	validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: buildPlanWithNTasks(2), PlanRunID: first.PlanRunID, ProjectKnowledge: "Decision: tasks run in order.",
	})
	require.Equal(t, 3, sr.calls)

	again := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})

	assert.Equal(t, 4, sr.calls, "the run now holds a review made under other inputs, so the entry is not served")
	assert.NotEqual(t, first.PlanRunID, again.PlanRunID)
}

const roundUnverifiable = `{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"Registry.Lookup is assumed to exist","suggestion":"verify"}`

func TestValidatePlan_ATruncatedRoundLabelsACarriedTaskByItsPlanPosition(t *testing.T) {
	plan := "# Plan\n\n" +
		"### Task 1: Setup\n\n**Goal:** g1\n\n**Acceptance criteria:**\n- ac1\n\n" +
		"### Task 2: Add tests\n\n**Goal:** g2\n\n**Acceptance criteria:**\n- ac2\n\n" +
		"### Task 3: Add tests\n\n**Goal:** g3\n\n**Acceptance criteria:**\n- ac3\n\n"
	h, sr := roundHandlers(t, 8,
		roundSingleResp("",
			roundTask{title: "Task 1: Setup"},
			roundTask{title: "Task 2: Add tests"},
			roundTask{title: "Task 3: Add tests", findings: []string{roundUnverifiable}}),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan})
	require.Len(t, first.CodebaseReferenceChecklist, 1)
	require.True(t, strings.HasPrefix(first.CodebaseReferenceChecklist[0], "Task 3: "))

	cut := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: strings.Replace(plan, "- ac2\n", "- ac2, measured\n", 1), PlanRunID: first.PlanRunID,
	})

	require.True(t, cut.Partial)
	require.Len(t, cut.Tasks, 2)
	assert.Equal(t, []int{1, 3}, []int{cut.Tasks[0].TaskIndex, cut.Tasks[1].TaskIndex})
	require.Len(t, cut.CodebaseReferenceChecklist, 1)
	assert.Equal(t, first.CodebaseReferenceChecklist[0], cut.CodebaseReferenceChecklist[0],
		"the carried task's line keeps its plan number, not its position among the results returned")
}

func TestValidatePlan_ARoundCutInItsSecondChunkKeepsTheFirstChunksResults(t *testing.T) {
	h, sr := roundHandlers(t, 2,
		passOneResp(),
		chunkResp(t, titlesRange(1, 2)),
		chunkResp(t, titlesRange(3, 4)),
		roundPlanLevelResp("", "n"),
		roundChunkResp(roundTask{title: "Task 1: t1", findings: []string{roundMajor}}, roundTask{title: "Task 2: t2"}),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
	)
	sr.errors = []error{nil, nil, nil, nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(4)})
	require.Equal(t, 3, sr.calls)

	edited := buildPlanWithNTasks(4)
	for _, k := range []int{1, 2, 4} {
		edited = strings.Replace(edited, fmt.Sprintf("- ac%d\n", k), fmt.Sprintf("- ac%d, measured\n", k), 1)
	}
	cut := validatePlanRound(t, h, ValidatePlanArgs{PlanText: edited, PlanRunID: first.PlanRunID})

	assert.Equal(t, 6, sr.calls, "the plan-level pass and both chunks were sent")
	assert.True(t, cut.Partial)
	assert.Equal(t, verdict.VerdictWarn, cut.PlanVerdict)
	require.Len(t, cut.Tasks, 3, "the first chunk's two tasks and the carried one")
	assert.Equal(t, []string{"Task 1: t1", "Task 2: t2", "Task 3: t3"},
		[]string{cut.Tasks[0].TaskTitle, cut.Tasks[1].TaskTitle, cut.Tasks[2].TaskTitle})
	assert.Equal(t, []int{1, 2, 3}, []int{cut.Tasks[0].TaskIndex, cut.Tasks[1].TaskIndex, cut.Tasks[2].TaskIndex})
	assert.Len(t, cut.Tasks[0].Findings, 1, "a task reviewed before the cut keeps its new result")
	unfinished := cut.PlanFindings[len(cut.PlanFindings)-1]
	assert.Equal(t, "reviewer_response", unfinished.Criterion)
	assert.Contains(t, unfinished.Evidence, "1 of the plan's 4 task(s) have no result")
}

func TestValidatePlan_AnUnchangedRoundOnAFailingReviewMakesNoReviewerCall(t *testing.T) {
	critical := `{"severity":"critical","category":"ambiguous_spec","criterion":"ac1","evidence":"contradicts the goal","suggestion":"rewrite"}`
	h, sr := roundHandlers(t, 8, roundSingleResp(critical, roundTitles(2)...))
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictFail, first.PlanVerdict)

	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2), PlanRunID: first.PlanRunID})

	assert.Equal(t, 1, sr.calls, "a stored fail answers an unchanged plan as a stored pass does")
	assert.Equal(t, verdict.VerdictFail, second.PlanVerdict)
	assert.Equal(t, first.PlanFindings, second.PlanFindings)
	assert.Equal(t, &verdict.PlanReviewScope{Revision: 2, TasksReviewed: 0, TasksCarried: 2}, second.ReviewScope)
}

func TestValidatePlan_ARulingOnACarriedPlanLevelFindingReplacesTheStoredNextAction(t *testing.T) {
	h, sr := roundHandlers(t, 8, roundSingleResp(roundOrder, roundTitles(2)...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Equal(t, verdict.VerdictWarn, first.PlanVerdict)
	require.Len(t, first.PlanFindings, 1)
	require.Equal(t, "round one", first.NextAction)

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText:  buildPlanWithNTasks(2),
		PlanRunID: first.PlanRunID,
		ControllerRulings: []ControllerRulingArg{{
			FindingID: first.PlanFindings[0].ID, Ruling: "Task 3 does not exist; the order is right",
		}},
	})

	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, verdict.VerdictPass, second.PlanVerdict)
	assert.Empty(t, second.PlanFindings)
	require.Len(t, second.WaivedFindings, 1)
	assert.Equal(t, "Plan passes: dispatch.", second.NextAction,
		"the stored next_action was written for a finding this round waived")
}

func TestValidatePlan_ARulingThatLeavesCarriedPlanLevelFindingsSaysTheyWereCarried(t *testing.T) {
	h, sr := roundHandlers(t, 8, roundSingleResp(roundOrder+","+roundMajor, roundTitles(2)...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(2)})
	require.Len(t, first.PlanFindings, 2)

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText:  buildPlanWithNTasks(2),
		PlanRunID: first.PlanRunID,
		ControllerRulings: []ControllerRulingArg{{
			FindingID: first.PlanFindings[0].ID, Ruling: "Task 3 does not exist; the order is right",
		}},
	})

	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, verdict.VerdictWarn, second.PlanVerdict)
	require.Len(t, second.PlanFindings, 1)
	assert.NotEqual(t, "round one", second.NextAction)
	assert.Contains(t, second.NextAction, "This round made no reviewer call")
	assert.Contains(t, second.NextAction, "plan_run_id")
}

func TestValidatePlan_ATruncatedRoundGivesACarriedTaskItsOwnNormativeTestBodies(t *testing.T) {
	task := func(n int, title string) string {
		return fmt.Sprintf("### Task %d: %s\n\n**Goal:** g%d\n\n**Acceptance criteria:**\n- ac%d\n\n"+
			"**NORMATIVE TEST BODIES (verbatim):**\n\n```go\nfunc TestTask%d(t *testing.T) {}\n```\n\n", n, title, n, n, n)
	}
	plan := "# Plan\n\n" + task(1, "Setup") + task(2, "Add tests") + task(3, "Add tests")
	h, sr := roundHandlers(t, 8,
		roundSingleResp("",
			roundTask{title: "Task 1: Setup"},
			roundTask{title: "Task 2: Add tests"},
			roundTask{title: "Task 3: Add tests"}),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan})
	require.Len(t, first.Tasks, 3)

	cut := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: strings.Replace(plan, "- ac2\n", "- ac2, measured\n", 1), PlanRunID: first.PlanRunID,
	})

	require.True(t, cut.Partial)
	require.Len(t, cut.Tasks, 2)
	require.Equal(t, 3, cut.Tasks[1].TaskIndex)
	assert.Equal(t, []string{"func TestTask1(t *testing.T) {}"}, cut.Tasks[0].NormativeTestBodies)
	assert.Equal(t, []string{"func TestTask3(t *testing.T) {}"}, cut.Tasks[1].NormativeTestBodies,
		"a carried task takes the bodies of the task at its plan position, not of the task that shares its title")
}

func TestValidatePlan_AVerifiedReferenceThatEmptiesTheChecklistReplacesTheStoredNextAction(t *testing.T) {
	claim := `{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/cache.go defines Evict","suggestion":"verify"}`
	tasks := roundTitles(1)
	tasks[0].findings = []string{roundMajor, claim}
	h, sr := roundHandlers(t, 8, roundSingleResp("", tasks...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: buildPlanWithNTasks(1)})
	require.Len(t, first.CodebaseReferenceChecklist, 1)
	require.Equal(t, "round one", first.NextAction)

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: buildPlanWithNTasks(1), PlanRunID: first.PlanRunID,
		ControllerVerifiedReferences: []string{"pkg/cache.go"},
	})

	assert.Equal(t, 1, sr.calls)
	assert.Empty(t, second.CodebaseReferenceChecklist)
	assert.Equal(t, first.PlanVerdict, second.PlanVerdict)
	assert.NotEqual(t, "round one", second.NextAction,
		"the stored next_action was written with the reference still unverified")
}

func TestValidatePlan_AServerFindingAddedOnACarriedRoundReplacesTheStoredNextAction(t *testing.T) {
	plan := "# Plan\n\n### Task 1: t1\n\n**Goal:** g1\n\n**Files:**\n- Modify: `pkg/missing.go`\n\n**Acceptance criteria:**\n- ac1\n\n"
	h, sr := roundHandlers(t, 8, roundSingleResp("", roundTitles(1)...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan})
	require.Equal(t, verdict.VerdictPass, first.PlanVerdict)
	require.Equal(t, "round one", first.NextAction)

	second := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan, PlanRunID: first.PlanRunID, RepoRoot: t.TempDir()})

	assert.Equal(t, 1, sr.calls)
	require.True(t, hasCriterion(second.PlanFindings, "task_order_contradiction"), "the disk tier found the missing Modify target")
	assert.Equal(t, verdict.VerdictWarn, second.PlanVerdict)
	assert.Contains(t, second.NextAction, "This round made no reviewer call",
		"the stored next_action was written before the server found the missing file")
}

// Two tasks with one title, a truncated round, and a ruling on the carried
// one's finding. A task finding's id is built from the title without its
// "Task N:" prefix, so tasks that share a title share a task key whichever of
// them a result is matched to: the ruling still reaches the carried finding.
func TestValidatePlan_ARulingReachesACarriedTaskWithADuplicateTitleOnATruncatedRound(t *testing.T) {
	plan := func(ac2 string) string {
		return "# Plan\n\n### Task 1: t1\n\n**Goal:** g\n\n**Acceptance criteria:**\n- ac1\n\n" +
			"### Task 2: Add tests\n\n**Goal:** g\n\n**Acceptance criteria:**\n- " + ac2 + "\n\n" +
			"### Task 3: Add tests\n\n**Goal:** g\n\n**Acceptance criteria:**\n- ac3\n\n"
	}
	h, sr := roundHandlers(t, 8,
		roundSingleResp("",
			roundTask{title: "Task 1: t1"},
			roundTask{title: "Task 2: Add tests"},
			roundTask{title: "Task 3: Add tests", findings: []string{roundMajor}}),
		roundPlanLevelResp("", "n"),
		providers.Response{RawJSON: []byte(`{"tasks":[`)},
	)
	sr.errors = []error{nil, nil, providers.ErrResponseTruncated}
	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan("ac2")})
	require.Len(t, first.Tasks[2].Findings, 1)
	id := first.Tasks[2].Findings[0].ID

	cut := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: plan("ac2, measured"), PlanRunID: first.PlanRunID,
		ControllerRulings: []ControllerRulingArg{{FindingID: id, Ruling: "ac1 is pinned by a test"}},
	})

	require.Len(t, cut.Tasks, 2, "Task 2 was cut short; Tasks 1 and 3 are carried")
	carried := cut.Tasks[1]
	assert.Equal(t, 3, carried.TaskIndex)
	assert.Empty(t, carried.Findings)
	require.Len(t, carried.WaivedFindings, 1)
	assert.Equal(t, id, carried.WaivedFindings[0].ID)
}

// A ruling waives the carried major finding while the file check adds another
// major one: the verdict and the number of findings are what they were, and
// the stored sentence is about the finding that was just waived.
func TestValidatePlan_AWaivedFindingReplacedByAServerFindingReplacesTheStoredNextAction(t *testing.T) {
	plan := "# Plan\n\n### Task 1: t1\n\n**Goal:** g1\n\n**Files:**\n- Modify: `pkg/missing.go`\n\n**Acceptance criteria:**\n- ac1\n\n"
	h, sr := roundHandlers(t, 8, roundSingleResp(roundOrder, roundTitles(1)...))

	first := validatePlanRound(t, h, ValidatePlanArgs{PlanText: plan})
	require.Equal(t, verdict.VerdictWarn, first.PlanVerdict)
	require.Len(t, first.PlanFindings, 1)

	second := validatePlanRound(t, h, ValidatePlanArgs{
		PlanText: plan, PlanRunID: first.PlanRunID, RepoRoot: t.TempDir(),
		ControllerRulings: []ControllerRulingArg{{FindingID: first.PlanFindings[0].ID, Ruling: "the order is intended"}},
	})

	assert.Equal(t, 1, sr.calls)
	assert.Equal(t, verdict.VerdictWarn, second.PlanVerdict)
	require.Len(t, second.PlanFindings, 1, "one waived, one added")
	assert.Equal(t, "task_order_contradiction", second.PlanFindings[0].Criterion)
	assert.Contains(t, second.NextAction, "This round made no reviewer call")
}
