package stats

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
)

func TestComputeCodescene(t *testing.T) {
	base := time.Unix(1700000000, 0).UTC()
	events := []CodesceneEvent{
		{Ts: base, Digest: codescene.Digest{Tool: "analyze_change_set", QualityGate: "passed", FilesAnalyzed: 3,
			Verdicts: &Verdicts{Improved: 2, Stable: 1}, Trend: "improvement", NetPP: -1.5,
			CategoryCounts: map[string]int{"Complex Method": 1}}},
		{Ts: base.Add(time.Hour), Digest: codescene.Digest{Tool: "analyze_change_set", QualityGate: "failed", FilesAnalyzed: 5,
			Verdicts: &Verdicts{Degraded: 4, Stable: 1}, Trend: "regression", NetPP: 2.3,
			CategoryCounts: map[string]int{"Complex Method": 2, "Bumpy Road Ahead": 1}}},
	}
	cr := computeCodescene(events)
	if cr == nil {
		t.Fatal("expected non-nil rollup")
	}
	if cr.Runs != 2 || cr.GatesPassed != 1 || cr.GatesFailed != 1 {
		t.Errorf("runs/gates = %d/%d/%d", cr.Runs, cr.GatesPassed, cr.GatesFailed)
	}
	if cr.LatestGate != "failed" || cr.LatestTrend != "regression" || cr.LatestNetPP != 2.3 {
		t.Errorf("latest = %v/%v/%v", cr.LatestGate, cr.LatestTrend, cr.LatestNetPP)
	}
	if cr.Regressions != 1 || cr.Improvements != 1 || cr.Neutral != 0 {
		t.Errorf("trend counts = %d/%d/%d", cr.Regressions, cr.Improvements, cr.Neutral)
	}
	if cr.FilesAnalyzed != 8 {
		t.Errorf("FilesAnalyzed = %d, want 8", cr.FilesAnalyzed)
	}
	if cr.NetPPP50 != -1.5 { // {-1.5,2.3}→{-150,230}; percentile(50) ceil-rank picks lower → -1.5
		t.Errorf("NetPPP50 = %v, want -1.5", cr.NetPPP50)
	}
	if cr.CategoryHistogram["Complex Method"] != 3 || cr.CategoryHistogram["Bumpy Road Ahead"] != 1 {
		t.Errorf("category histogram = %v", cr.CategoryHistogram)
	}
	if !cr.WindowStart.Equal(base) || !cr.WindowEnd.Equal(base.Add(time.Hour)) {
		t.Errorf("window = %v..%v", cr.WindowStart, cr.WindowEnd)
	}
}

func TestComputeCodesceneEmptyIsNil(t *testing.T) {
	if cr := computeCodescene(nil); cr != nil {
		t.Errorf("empty input must yield nil (omitted key), got %+v", cr)
	}
}

func TestCodesceneRollupJSONContract(t *testing.T) {
	cr := CodesceneRollup{CategoryHistogram: map[string]int{"Complex Method": 1}}
	b, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, key := range []string{
		"runs", "gates_passed", "gates_failed", "latest_gate", "latest_trend",
		"latest_net_pp", "net_pp_p50", "regressions", "improvements", "neutral",
		"files_analyzed", "category_histogram", "window_start", "window_end",
	} {
		if !strings.Contains(s, `"`+key+`"`) {
			t.Errorf("missing json key %q in marshaled CodesceneRollup", key)
		}
	}
}

func TestCodesceneEvent_UnmarshalsHookWrittenRecord(t *testing.T) {
	// Exactly what examples/hooks/codescene-log.sh appends: no "ran" key.
	line := `{"ts":"2026-07-07T13:11:28Z","tool":"analyze_change_set",` +
		`"quality_gate":"failed","files_analyzed":2,` +
		`"verdicts":{"improved":0,"degraded":2,"stable":0},` +
		`"trend":"regression","net_pp":2.0,"category_counts":{"Complex Method":2}}`

	var ev CodesceneEvent
	require.NoError(t, json.Unmarshal([]byte(line), &ev))
	assert.Equal(t, "failed", ev.QualityGate)
	assert.Equal(t, 2, ev.FilesAnalyzed)
	require.NotNil(t, ev.Verdicts)
	assert.Equal(t, 2, ev.Verdicts.Degraded)
	assert.Equal(t, 2.0, ev.NetPP)
	assert.False(t, ev.Ran, "hook records carry no ran field")

	out, err := json.Marshal(ev)
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"ran"`)
}

// TestCodesceneEvent_UnmarshalJSON_DecodesTsAlongsideDigestFields pins
// CodesceneEvent's UnmarshalJSON override. codescene.Digest's own
// UnmarshalJSON is promoted onto CodesceneEvent through the anonymous
// embed; without CodesceneEvent's override, that promoted method runs
// alone, decodes the Digest fields, and never touches Ts, which is why this
// test checks both in one decode.
func TestCodesceneEvent_UnmarshalJSON_DecodesTsAlongsideDigestFields(t *testing.T) {
	line := `{"ts":"2026-07-07T13:11:28Z","quality_gate":"failed"}`
	var ev CodesceneEvent
	require.NoError(t, json.Unmarshal([]byte(line), &ev))
	assert.True(t, ev.Ts.Equal(time.Date(2026, 7, 7, 13, 11, 28, 0, time.UTC)), "Ts = %v", ev.Ts)
	assert.Equal(t, "failed", ev.QualityGate)
}

func TestPruneCodescene(t *testing.T) {
	dir := t.TempDir()
	base := time.Unix(1700000000, 0).UTC()
	old := CodesceneEvent{Ts: base.Add(-48 * time.Hour), Digest: codescene.Digest{Tool: "analyze_change_set"}}
	fresh := CodesceneEvent{Ts: base, Digest: codescene.Digest{Tool: "analyze_change_set"}}
	if err := appendJSONL(dir, codesceneFile, old); err != nil {
		t.Fatal(err)
	}
	if err := appendJSONL(dir, codesceneFile, fresh); err != nil {
		t.Fatal(err)
	}
	if err := pruneCodescene(dir, base.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := readCodescene(dir)
	if len(got) != 1 || !got[0].Ts.Equal(base) {
		t.Fatalf("after prune got %d events, want 1 (the fresh one)", len(got))
	}
}

func TestRecordCodescene_WritesAContentFreeRunRecord(t *testing.T) {
	r := newTestRecorder(t, 1000)
	r.RecordCodescene(codescene.Digest{
		Ran: true, Tool: "analyze_change_set", QualityGate: "failed", FilesAnalyzed: 2,
		Verdicts: &Verdicts{Degraded: 2}, Trend: "regression", NetPP: 2,
		CategoryCounts: map[string]int{"Complex Method": 2},
		SkipReason:     "a reason", SkipEvidence: "error text", BaseRef: "origin/feature-branch",
	})

	b, err := os.ReadFile(filepath.Join(r.dir, codesceneFile))
	require.NoError(t, err)
	line := strings.TrimSpace(string(b))
	for _, absent := range []string{"skip_reason", "skip_evidence", "base_ref", "feature-branch", `"ran"`} {
		assert.NotContains(t, line, absent)
	}

	events, err := readCodescene(r.dir)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.False(t, events[0].Ts.IsZero())
	assert.Equal(t, codescene.Digest{
		Tool: "analyze_change_set", QualityGate: "failed", FilesAnalyzed: 2,
		Verdicts: &Verdicts{Degraded: 2}, Trend: "regression", NetPP: 2,
		CategoryCounts: map[string]int{"Complex Method": 2},
	}, events[0].Digest)

	cr := computeCodescene(events)
	require.NotNil(t, cr)
	assert.Equal(t, 1, cr.Runs)
	assert.Equal(t, 1, cr.GatesFailed)

	var nilRecorder *Recorder
	nilRecorder.RecordCodescene(codescene.Digest{Ran: true})
}

func TestRunRecord_NamesOnlyTheExpectedTool(t *testing.T) {
	assert.Equal(t, "analyze_change_set", RunRecord(codescene.Digest{Tool: "analyze_change_set"}).Tool)
	assert.Equal(t, "analyze_change_set", RunRecord(codescene.Digest{}).Tool, "a digest reduced from raw output may carry no tool")
	assert.Equal(t, "other", RunRecord(codescene.Digest{Tool: "ran it by hand on src/billing/invoice.go"}).Tool,
		"tool is caller text and must not reach the record")
}

func TestRunRecord_CountsAKeyThatIsNotAPlainCategoryNameAsOther(t *testing.T) {
	long := strings.Repeat("a", 41)
	in := map[string]int{
		"Complex Method":   2,
		"Bumpy Road Ahead": 1,
		"Complex Method in internal/billing/invoice.go": 3,
		"Complex Method 2":   4,
		long:                 5,
		"Large Method: Load": 6,
	}
	sent := maps.Clone(in)

	got := RunRecord(codescene.Digest{CategoryCounts: in}).CategoryCounts

	assert.Equal(t, map[string]int{"Complex Method": 2, "Bumpy Road Ahead": 1, "other": 18}, got)
	assert.Equal(t, sent, in, "the caller's map is left as sent")
}

func TestRunRecord_KeepsTheLongestPlainCategoryNameAndAddsToASentOther(t *testing.T) {
	longest := strings.Repeat("a", 40)
	got := RunRecord(codescene.Digest{CategoryCounts: map[string]int{longest: 1, "other": 2, "a/b": 3}}).CategoryCounts
	assert.Equal(t, map[string]int{longest: 1, "other": 5}, got)
}

func TestRunRecord_NoCategoryCountsStaysNil(t *testing.T) {
	assert.Nil(t, RunRecord(codescene.Digest{}).CategoryCounts)
	assert.Nil(t, RunRecord(codescene.Digest{CategoryCounts: map[string]int{}}).CategoryCounts)
}
