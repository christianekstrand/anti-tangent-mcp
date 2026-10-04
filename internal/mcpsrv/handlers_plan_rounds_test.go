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
