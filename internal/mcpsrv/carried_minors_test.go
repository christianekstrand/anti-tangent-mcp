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

func TestMarkRepeats_AnAnsweredPriorMinorIsCarriedOnce(t *testing.T) {
	nitID := verdict.Fingerprint(verdict.CategoryQuality, "", "comment_hygiene")
	downgradedID := verdict.Fingerprint(verdict.CategoryScopeDrift, "", "AC 2")
	prior := []prompts.PriorFinding{
		{Finding: verdict.Finding{ID: nitID, Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"}, Response: "fixed"},
		{Finding: verdict.Finding{ID: downgradedID, Severity: verdict.SeverityMajor, Category: verdict.CategoryScopeDrift, Criterion: "AC 2"}, Response: "ruled"},
	}
	fs := []verdict.Finding{
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryQuality, Criterion: "comment_hygiene"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryOther, Criterion: "downgraded", SameAs: strPtr(downgradedID)},
	}
	shown := map[string]bool{nitID: true, downgradedID: true}
	markRepeats(fs, prior, shown)

	assert.Equal(t, nitID, fs[0].RepeatOf, "an answered prior minor is carried by the first match")
	assert.Empty(t, fs[1].RepeatOf, "a second minor with that fingerprint is new")
	assert.Equal(t, downgradedID, fs[2].RepeatOf, "a minor naming an answered prior major keeps its repeat mark")
}

func TestMarkRepeats_AnAnsweredPriorMajorIsCarriedByOneMinor(t *testing.T) {
	majorID := verdict.Fingerprint(verdict.CategoryCorrectness, "", "correctness")
	prior := []prompts.PriorFinding{
		{Finding: verdict.Finding{ID: majorID, Severity: verdict.SeverityMajor, Category: verdict.CategoryCorrectness, Criterion: "correctness"}, Response: "ruled"},
	}
	fs := []verdict.Finding{
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryCorrectness, Criterion: "correctness"},
		{Severity: verdict.SeverityMinor, Category: verdict.CategoryCorrectness, Criterion: "correctness"},
		{Severity: verdict.SeverityMajor, Category: verdict.CategoryCorrectness, Criterion: "correctness"},
	}
	escalate := markRepeats(fs, prior, map[string]bool{majorID: true})

	assert.Equal(t, majorID, fs[0].RepeatOf, "the first minor takes the answered match")
	assert.Empty(t, fs[1].RepeatOf, "a second minor is new")
	assert.Equal(t, majorID, fs[2].RepeatOf, "a major may still take the answered match")
	assert.Equal(t, []string{majorID}, escalate)
}

func TestValidateCompletion_AnAnsweredMajorDoesNotHideFourNewMinors(t *testing.T) {
	h, rv := newRulingsHandlers(t)
	sid := startTask(t, h, rv)
	first := completeWith(t, h, rv, completionCallArgs(sid), reviewerFindingsResp(
		findingObj("major", "correctness", "correctness", "a wrong result", "")))
	require.Len(t, first.Findings, 1)

	minor := func(evidence string) string {
		return findingObj("minor", "correctness", "correctness", evidence, "")
	}
	args := completionCallArgs(sid)
	args.FindingResponses = []FindingResponseArg{{FindingID: first.Findings[0].ID, Response: "fixed"}}
	second := completeWith(t, h, rv, args, reviewerFindingsResp(
		minor("a.go: one"), minor("b.go: two"), minor("c.go: three"), minor("d.go: four")))

	assert.Equal(t, "warn", second.Verdict)
	repeats := 0
	for _, f := range second.Findings {
		if f.RepeatOf != "" {
			repeats++
		}
	}
	assert.Equal(t, 1, repeats)
}
