package prompts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/planparser"
	"github.com/patiently/anti-tangent-mcp/internal/session"
)

var sampleBoundaryRules = []string{
	"Code never inspects reply or driver words: no regex or phrase matching over them.",
	"The LLM writes every reply; the only code-written text is verbatim legal text.",
}

func boundarySpec() session.TaskSpec {
	s := sampleSpec()
	s.BoundaryRules = sampleBoundaryRules
	return s
}

const agentNetworkPlan = "# Plan\n\n**Plan kind:** agent-network\n\n" +
	"### Task 1: Zip mapper\n\n**Goal:** map the ZIP field\n\n" +
	"### Task 2: Zip prompt\n\n**Kind:** experiment\n**Rung:** prompt\n\n**Goal:** ask for a missing ZIP\n"

func agentNetworkTasks(t *testing.T) []planparser.RawTask {
	t.Helper()
	tasks, _ := planparser.SplitTasks(agentNetworkPlan)
	require.Len(t, tasks, 2)
	return tasks
}

func TestRenderPre_WithBoundaryRules_Golden(t *testing.T) {
	out, err := RenderPre(PreInput{Spec: boundarySpec()})
	require.NoError(t, err)
	golden(t, "pre_with_boundary_rules", out.System+"\n---USER---\n"+out.User)
}

func TestRenderMid_WithBoundaryRules_Golden(t *testing.T) {
	out, err := RenderMid(MidInput{
		Spec:      boundarySpec(),
		WorkingOn: "routing the ZIP question",
		Files:     []File{{Path: "bot/zip.kt", Content: "fun ask(reply: String) = reply.contains(\"zip\")\n"}},
	})
	require.NoError(t, err)
	golden(t, "mid_with_boundary_rules", out.System+"\n---USER---\n"+out.User)
}

func TestRenderPost_WithBoundaryRules_Golden(t *testing.T) {
	out, err := RenderPost(PostInput{
		Spec:      boundarySpec(),
		Summary:   "Added the ZIP question.",
		FinalDiff: "--- a/bot/zip.kt\n+++ b/bot/zip.kt\n@@ -1 +1 @@\n-fun ask() = null\n+fun ask(reply: String) = reply.contains(\"zip\")\n",
	})
	require.NoError(t, err)
	golden(t, "post_with_boundary_rules", out.System+"\n---USER---\n"+out.User)
}

func TestRenderPlan_WithBoundaryRules_Golden(t *testing.T) {
	out, err := RenderPlan(PlanInput{PlanText: agentNetworkPlan, PlanKind: planparser.PlanKindAgentNetwork, BoundaryRules: sampleBoundaryRules})
	require.NoError(t, err)
	golden(t, "plan_with_boundary_rules", out.System+"\n---USER---\n"+out.User)
}

func TestRenderPlanChunked_WithBoundaryRules_SharePrefix(t *testing.T) {
	findingsOnly, err := RenderPlanFindingsOnly(PlanInput{PlanText: agentNetworkPlan, PlanKind: planparser.PlanKindAgentNetwork, BoundaryRules: sampleBoundaryRules})
	require.NoError(t, err)
	chunk, err := RenderPlanTasksChunk(PlanChunkInput{PlanText: agentNetworkPlan, ChunkTasks: agentNetworkTasks(t), PlanKind: planparser.PlanKindAgentNetwork, BoundaryRules: sampleBoundaryRules})
	require.NoError(t, err)
	require.Equal(t, findingsOnly.UserPrefix, chunk.UserPrefix, "the rules render in the cached prefix every call of a review shares")
	require.Contains(t, chunk.UserPrefix, "## Boundary rules (caller-supplied, authoritative)")
	golden(t, "plan_findings_only_with_boundary_rules", findingsOnly.System+"\n---USER---\n"+findingsOnly.User)
	golden(t, "plan_tasks_chunk_with_boundary_rules", chunk.System+"\n---USER---\n"+chunk.User)
}

func TestBoundaryRules_FenceOutrunsARuleBacktickRun(t *testing.T) {
	spec := sampleSpec()
	spec.BoundaryRules = []string{"never write ````` in a reply"}
	out, err := RenderPre(PreInput{Spec: spec})
	require.NoError(t, err)
	require.Contains(t, out.User, "``````text\n1. never write ````` in a reply\n``````")
}

func TestRigidityTag_OnlyInAgentNetworkOrRulesMode(t *testing.T) {
	plain := sampleSpec()
	agent := sampleSpec()
	agent.PlanKind = planparser.PlanKindAgentNetwork
	for _, c := range []struct {
		spec session.TaskSpec
		want bool
	}{{plain, false}, {agent, true}, {boundarySpec(), true}} {
		mid, err := RenderMid(MidInput{Spec: c.spec, WorkingOn: "w"})
		require.NoError(t, err)
		post, err := RenderPost(PostInput{Spec: c.spec, Summary: "s", FinalDiff: "+x\n"})
		require.NoError(t, err)
		pre, err := RenderPre(PreInput{Spec: c.spec})
		require.NoError(t, err)
		for _, body := range []string{mid.User, post.User, pre.User} {
			require.Equal(t, c.want, strings.Contains(body, "`rigidity:`"))
		}
	}
}
