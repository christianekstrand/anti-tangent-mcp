package mcpsrv

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
	"github.com/patiently/anti-tangent-mcp/scorecard"
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

func TestSnapshotRow_CarriesCategoriesAndDiffSize(t *testing.T) {
	h, rec, _ := outcomeHandlers(t)
	run := h.deps.PlanRuns.Create("pass", "rigorous", 1)
	_, ok := h.deps.PlanRuns.Attach(run.ID, "s1", planrun.TaskRef{Index: 1}, "pass")
	require.True(t, ok)
	env := Envelope{Verdict: "warn", Findings: []verdict.Finding{
		{Severity: verdict.SeverityMajor, Category: verdict.CategoryCorrectness, Criterion: "c", Evidence: "e", Suggestion: "s"},
	}}
	row, ok := h.deps.PlanRuns.UpdateRow(run.ID, "s1", completionRowUpdate(env, nil, "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"))
	require.True(t, ok)
	h.snapshotRow(run.ID, row)

	lines, err := rec.RunLines(rec.RunHash(run.ID))
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	snap := lines[len(lines)-1].Task
	require.NotNil(t, snap)
	assert.Equal(t, map[string]int{"correctness": 1}, snap.Categories)
	assert.Equal(t, 1, snap.LinesAdded)
	assert.Equal(t, 1, snap.LinesRemoved)
}

func TestTaskSnapshot_OmitsEmptyCategoriesAndDiffSize(t *testing.T) {
	raw, err := json.Marshal(scorecard.TaskSnapshot{Index: 1})
	require.NoError(t, err)
	for _, key := range []string{"categories", "lines_added", "lines_removed"} {
		assert.NotContains(t, string(raw), key)
	}
}

func TestDiffLineCounts_IgnoresLinesBeforeFirstHunk(t *testing.T) {
	diff := "commit abc\n" +
		"- a bullet in the commit message\n" +
		"-- \n" +
		"--- a/x\n" +
		"+++ b/x\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"
	added, removed := diffLineCounts(diff)
	assert.Equal(t, 1, added)
	assert.Equal(t, 1, removed)
}

func TestCompletionRowUpdate_NoDiffRetryKeepsEarlierDiffSize(t *testing.T) {
	var row planrun.TaskRow
	completionRowUpdate(Envelope{Verdict: "pass"}, nil, "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n")(&row)
	completionRowUpdate(Envelope{Verdict: "pass"}, nil, "")(&row)
	assert.Equal(t, 1, row.LinesAdded)
	assert.Equal(t, 1, row.LinesRemoved)
}

func TestDiffLineCounts_FileBoundaryEndsTheHunk(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/logo.png b/logo.png\n" +
		"--- a/logo.png\n" +
		"Binary files differ\n"
	added, removed := diffLineCounts(diff)
	assert.Equal(t, 1, added)
	assert.Equal(t, 1, removed, "a later file's metadata is outside every hunk")
}

func TestDiffLineCounts_HeaderShapedLinesInsideAHunkAreCounted(t *testing.T) {
	diff := "--- a/q.sql\n" +
		"+++ b/q.sql\n" +
		"@@ -1,2 +1,2 @@\n" +
		" select 1;\n" +
		"--- old comment\n" +
		"+++ new marker\n" +
		"--- a/r.sql\n" +
		"+++ b/r.sql\n" +
		"@@ -1 +1 @@\n" +
		"-x\n" +
		"+y\n"
	added, removed := diffLineCounts(diff)
	assert.Equal(t, 2, added, "the in-hunk '++ ' line and the second file's addition")
	assert.Equal(t, 2, removed, "the in-hunk '-- ' line and the second file's removal")
}
