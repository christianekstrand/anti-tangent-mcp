package mcpsrv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
	"github.com/patiently/anti-tangent-mcp/internal/session"
)

// codesceneEventLines returns the lines of codescene-events.jsonl in dir, or
// none when the file was never written.
func codesceneEventLines(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "codescene-events.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func ranArgs(sid string, netPP float64) ValidateCompletionArgs {
	args := completionCallArgs(sid)
	args.Codescene = &codescene.Digest{
		Ran: true, Tool: "analyze_change_set", QualityGate: "passed", FilesAnalyzed: 3,
		Verdicts: &codescene.Verdicts{Stable: 3}, NetPP: netPP, BaseRef: "origin/some-branch",
	}
	return args
}

func newCodesceneEventHandlers(t *testing.T) (*handlers, string) {
	t.Helper()
	dir := t.TempDir()
	h := newTestHandlersWithCodescene(t, "required")
	h.deps.Stats = newStatsRecorder(t, dir)
	return h, dir
}

func TestCodesceneRunKey(t *testing.T) {
	assert.Empty(t, codesceneRunKey(nil))
	assert.Empty(t, codesceneRunKey(&codescene.Digest{Ran: false, SkipReason: "r"}))

	a := codesceneRunKey(&codescene.Digest{Ran: true, QualityGate: "passed", FilesAnalyzed: 3, BaseRef: "one"})
	b := codesceneRunKey(&codescene.Digest{Ran: true, QualityGate: "passed", FilesAnalyzed: 3, BaseRef: "two"})
	c := codesceneRunKey(&codescene.Digest{Ran: true, QualityGate: "failed", FilesAnalyzed: 3})
	assert.NotEmpty(t, a)
	assert.Equal(t, a, b, "the key covers only what the record holds")
	assert.NotEqual(t, a, c)
}

func TestValidateCompletion_AReportedCodesceneRunIsWrittenOncePerResult(t *testing.T) {
	h, dir := newCodesceneEventHandlers(t)
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	for call := 1; call <= 2; call++ {
		_, _, err := h.ValidateCompletion(context.Background(), nil, ranArgs(sess.ID, 0))
		require.NoError(t, err)
	}
	lines := codesceneEventLines(t, dir)
	require.Len(t, lines, 1, "a retry that resends the same result is not a second run")
	assert.Contains(t, lines[0], `"quality_gate":"passed"`)
	assert.Contains(t, lines[0], `"files_analyzed":3`)
	assert.NotContains(t, lines[0], "some-branch", "the base ref is a name the caller chose and stays out of the record")

	_, _, err := h.ValidateCompletion(context.Background(), nil, ranArgs(sess.ID, 1.5))
	require.NoError(t, err)
	assert.Len(t, codesceneEventLines(t, dir), 2, "a different result is a new run")
}

func TestValidateCompletion_ASkipOrAMissingArgumentWritesNoCodesceneEvent(t *testing.T) {
	h, dir := newCodesceneEventHandlers(t)
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	_, _, err := h.ValidateCompletion(context.Background(), nil, completionCallArgs(sess.ID))
	require.NoError(t, err)
	_, _, err = h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "MCP error: tool not found"))
	require.NoError(t, err)
	assert.Empty(t, codesceneEventLines(t, dir))

	_, _, err = h.ValidateCompletion(context.Background(), nil, ranArgs(sess.ID, 0))
	require.NoError(t, err)
	assert.Len(t, codesceneEventLines(t, dir), 1, "a skip before the run does not stop the run being recorded")
}

func TestValidateCompletion_ALightweightCallRecordsItsCodesceneRunEveryTime(t *testing.T) {
	h, dir := newCodesceneEventHandlers(t)
	for call := 1; call <= 2; call++ {
		_, env, err := h.ValidateCompletion(context.Background(), nil, ranArgs("", 0))
		require.NoError(t, err)
		require.True(t, env.Lightweight)
	}
	assert.Len(t, codesceneEventLines(t, dir), 2, "a lightweight call has no session to remember the run in")
}

func TestValidateCompletion_CodesceneRunWithStatsOffWritesNothing(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")
	_, env, err := h.ValidateCompletion(context.Background(), nil, ranArgs(sess.ID, 0))
	require.NoError(t, err)
	assert.Equal(t, "pass", env.Verdict)
}
