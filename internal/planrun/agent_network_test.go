package planrun

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func agentNetworkRun(t *testing.T, s *Store) *Run {
	t.Helper()
	return s.CreateWithTasks("pass", "rigorous", []PlanTask{
		{Index: 1, Title: "Task 1: Mapper", Kind: "build"},
		{Index: 2, Title: "Task 2: Zip prompt", Kind: "experiment", Rung: "prompt"},
	})
}

func TestTaskAgentNetwork_ByIndexAndByTitle(t *testing.T) {
	s := NewStore(time.Hour)
	run := agentNetworkRun(t, s)
	require.True(t, s.SetAgentNetwork(run.ID, "agent-network", []string{"no regex over reply text"}))

	got, ok := s.TaskAgentNetwork(run.ID, TaskRef{Index: 2})
	require.True(t, ok)
	require.Equal(t, AgentNetwork{PlanKind: "agent-network", BoundaryRules: []string{"no regex over reply text"}, TaskKind: "experiment", Rung: "prompt"}, got)

	got, ok = s.TaskAgentNetwork(run.ID, TaskRef{Title: "Zip prompt"})
	require.True(t, ok)
	require.Equal(t, "experiment", got.TaskKind)

	got, ok = s.TaskAgentNetwork(run.ID, TaskRef{Title: "Not in the plan"})
	require.True(t, ok)
	require.Equal(t, "agent-network", got.PlanKind)
	require.Empty(t, got.TaskKind)
	require.Empty(t, got.Rung)

	got, ok = s.TaskAgentNetwork(run.ID, TaskRef{})
	require.True(t, ok)
	require.Empty(t, got.TaskKind)

	_, ok = s.TaskAgentNetwork("pr_unknown", TaskRef{Index: 1})
	require.False(t, ok)
	require.False(t, s.SetAgentNetwork("pr_unknown", "agent-network", nil))
}

func TestTaskAgentNetwork_ReviseReplacesKinds(t *testing.T) {
	s := NewStore(time.Hour)
	run := agentNetworkRun(t, s)
	require.True(t, s.SetAgentNetwork(run.ID, "agent-network", []string{"rule"}))
	_, ok := s.Revise(run.ID, "pass", "rigorous", []PlanTask{
		{Index: 1, Title: "Task 1: Mapper", Kind: "build"},
		{Index: 2, Title: "Task 2: Zip prompt", Kind: "build"},
	}, nil)
	require.True(t, ok)
	require.True(t, s.SetAgentNetwork(run.ID, "", nil))

	got, ok := s.TaskAgentNetwork(run.ID, TaskRef{Index: 2})
	require.True(t, ok)
	require.Equal(t, AgentNetwork{TaskKind: "build"}, got)
}

func TestTaskAgentNetwork_ReturnsCopies(t *testing.T) {
	s := NewStore(time.Hour)
	run := agentNetworkRun(t, s)
	rules := []string{"rule one"}
	require.True(t, s.SetAgentNetwork(run.ID, "agent-network", rules))
	rules[0] = "changed by caller"

	got, _ := s.TaskAgentNetwork(run.ID, TaskRef{Index: 1})
	require.Equal(t, []string{"rule one"}, got.BoundaryRules)
	got.BoundaryRules[0] = "changed by reader"

	snap, ok := s.Snapshot(run.ID)
	require.True(t, ok)
	require.Equal(t, []string{"rule one"}, snap.BoundaryRules)
}

func TestLedgerHeader_CarriesNoAgentNetworkFields(t *testing.T) {
	s := NewStore(time.Hour)
	run := agentNetworkRun(t, s)
	require.True(t, s.SetAgentNetwork(run.ID, "agent-network", []string{"secret rule text"}))
	snap, _ := s.Snapshot(run.ID)
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	require.NotContains(t, string(b), "secret rule text")
	require.NotContains(t, string(b), "experiment")
	require.NotContains(t, string(b), "agent-network")
}
