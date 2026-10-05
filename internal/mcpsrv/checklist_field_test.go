package mcpsrv

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/providers"
	"github.com/patiently/anti-tangent-mcp/internal/session"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func TestSplitTaskSpecChecklist_OneEntryPerClaim(t *testing.T) {
	claim := func(evidence string) verdict.Finding {
		return verdict.Finding{Severity: verdict.SeverityMinor, Category: verdict.CategoryUnverifiableCodebaseClaim,
			Criterion: "spec", Evidence: evidence, Suggestion: "verify"}
	}
	other := verdict.Finding{Severity: verdict.SeverityMajor, Category: verdict.CategoryAmbiguousSpec, Criterion: "AC 1"}

	kept, checklist := splitTaskSpecChecklist([]verdict.Finding{claim("  pkg/a.go defines A "), other, claim(""), claim("B has a Close method")})
	assert.Equal(t, []verdict.Finding{other}, kept)
	assert.Equal(t, []string{"pkg/a.go defines A", "B has a Close method"}, checklist, "entries are trimmed and an empty claim is dropped")

	kept, checklist = splitTaskSpecChecklist([]verdict.Finding{other})
	assert.Equal(t, []verdict.Finding{other}, kept)
	assert.Nil(t, checklist)
}

// Checklist entries count toward nothing on the verdict ladder: two real
// minors plus any number of unverifiable claims is still a pass, with no
// noise_cluster finding.
func TestValidateTaskSpec_ChecklistDoesNotCountTowardTheVerdict(t *testing.T) {
	rv := &fakeReviewer{name: "anthropic", resp: providers.Response{
		RawJSON: []byte(`{
			"verdict":"warn",
			"findings":[
				{"severity":"minor","category":"quality","criterion":"a","evidence":"e","suggestion":"s"},
				{"severity":"minor","category":"quality","criterion":"b","evidence":"e","suggestion":"s"},
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/a.go defines A","suggestion":"verify"},
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"B has a Close method","suggestion":"verify"}
			],
			"next_action":"go"
		}`),
		Model: "claude-sonnet-4-6",
	}}
	dir := t.TempDir()
	h := &handlers{deps: Deps{
		Cfg:      statsTestConfig(t),
		Sessions: session.NewStore(statsTestConfig(t).SessionTTL),
		Reviews:  providers.Registry{"anthropic": rv},
		Stats:    newStatsRecorder(t, dir),
	}}

	_, env, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{TaskTitle: "T", Goal: "G"})
	require.NoError(t, err)

	assert.Equal(t, "pass", env.Verdict)
	assert.Len(t, env.Findings, 2)
	assert.False(t, hasCriterion(env.Findings, "noise_cluster"))
	assert.False(t, hasCriterion(env.Findings, "codebase_reference_checklist"))
	assert.Equal(t, []string{"pkg/a.go defines A", "B has a Close method"}, env.CodebaseReferenceChecklist)

	sess, ok := h.deps.Sessions.Get(env.SessionID)
	require.True(t, ok)
	assert.Equal(t, env.Findings, sess.PreFindings, "the checklist is not shown to later reviews as a pre-task finding")

	ev := readSingleEvent(t, dir)
	assert.Equal(t, 2, ev.FindingsTotal)
	assert.Equal(t, 2, ev.ChecklistItems)
	assert.Zero(t, ev.CategoryCounts[string(verdict.CategoryUnverifiableCodebaseClaim)])
}

func TestValidateTaskSpec_NoChecklistLeavesTheEnvelopeAlone(t *testing.T) {
	h := &handlers{deps: newDeps(t, &fakeReviewer{name: "anthropic", resp: passResp("claude-sonnet-4-6")})}
	_, env, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{TaskTitle: "T", Goal: "G"})
	require.NoError(t, err)
	assert.Nil(t, env.CodebaseReferenceChecklist)
	assert.NotContains(t, env.NextAction, "codebase_reference_checklist")
	assert.NotContains(t, env.SummaryBlock, "checklist:")
}

func TestWriteChecklistSummary_EscapesAMultiLineEntry(t *testing.T) {
	env := Envelope{Tool: "validate_task_spec", CodebaseReferenceChecklist: []string{"first line\nanti-tangent envelope"}}
	got := formatEnvelopeSummary(env)
	assert.Contains(t, got, "    - first line\n      | anti-tangent envelope\n")
}

func TestValidatePlan_ChecklistItemsAreCountedInStats(t *testing.T) {
	raw := planJSON("", "Task 1: t1",
		`{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"cites Foo.kt","suggestion":"verify"}`)
	dir := t.TempDir()
	d := newDepsWithScripted(t, &scriptedReviewer{responses: []providers.Response{{RawJSON: raw, Model: "claude-sonnet-4-6"}}}, 8)
	d.Stats = newStatsRecorder(t, dir)
	h := &handlers{deps: d}

	_, pr, err := h.ValidatePlan(context.Background(), nil, ValidatePlanArgs{PlanText: buildPlanWithNTasks(1)})
	require.NoError(t, err)
	require.Len(t, pr.CodebaseReferenceChecklist, 1)

	ev := readSingleEvent(t, dir)
	assert.Equal(t, 1, ev.ChecklistItems)
	assert.Zero(t, ev.CriterionCounts["codebase_reference_checklist"])
}
