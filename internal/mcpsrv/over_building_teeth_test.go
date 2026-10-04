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
