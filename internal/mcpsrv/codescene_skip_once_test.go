package mcpsrv

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
	"github.com/patiently/anti-tangent-mcp/internal/session"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func findingsOf(env Envelope, category verdict.Category) []verdict.Finding {
	var out []verdict.Finding
	for _, f := range env.Findings {
		if f.Category == category {
			out = append(out, f)
		}
	}
	return out
}

func skipArgs(sid string, evidence string) ValidateCompletionArgs {
	args := completionCallArgs(sid)
	args.Codescene = &codescene.Digest{Ran: false, SkipReason: "docs-only task", SkipEvidence: evidence}
	return args
}

func TestValidateCompletion_EvidencedSkipIsReportedOncePerSession(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	_, first, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	require.Len(t, findingsOf(first, verdict.CategoryCodesceneSkipped), 1)

	_, second, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	assert.Empty(t, findingsOf(second, verdict.CategoryCodesceneSkipped), "an evidenced skip is reported once")
}

func TestValidateCompletion_UnevidencedSkipRepeatsOnEveryCall(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	for call := 1; call <= 2; call++ {
		_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, ""))
		require.NoError(t, err)
		got := findingsOf(env, verdict.CategoryCodesceneSkipped)
		require.Len(t, got, 1, "call %d", call)
		assert.Equal(t, verdict.SeverityMajor, got[0].Severity)
	}

	_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "MCP error: tool not found"))
	require.NoError(t, err)
	got := findingsOf(env, verdict.CategoryCodesceneSkipped)
	require.Len(t, got, 1, "an unevidenced skip must not use up the one report of an evidenced one")
	assert.Equal(t, verdict.SeverityMinor, got[0].Severity)
}

func TestValidateCompletion_MissingCodesceneAfterAnEvidencedSkipIsStillNotRun(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	sess := h.deps.Sessions.Create(session.TaskSpec{Title: "t", Goal: "g"}, "")

	_, _, err := h.ValidateCompletion(context.Background(), nil, skipArgs(sess.ID, "no source files in diff"))
	require.NoError(t, err)
	_, env, err := h.ValidateCompletion(context.Background(), nil, completionCallArgs(sess.ID))
	require.NoError(t, err)
	assert.Len(t, findingsOf(env, verdict.CategoryCodesceneNotRun), 1)
}

func TestValidateCompletion_LightweightEvidencedSkipIsReportedEveryCall(t *testing.T) {
	h := newTestHandlersWithCodescene(t, "required")
	for call := 1; call <= 2; call++ {
		_, env, err := h.ValidateCompletion(context.Background(), nil, skipArgs("", "no source files in diff"))
		require.NoError(t, err)
		assert.Len(t, findingsOf(env, verdict.CategoryCodesceneSkipped), 1, "call %d", call)
	}
}
