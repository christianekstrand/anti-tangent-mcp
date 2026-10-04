package stats

import (
	"encoding/json"
	"math"
	"regexp"
	"time"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
)

const codesceneFile = "codescene-events.jsonl"

// Verdicts is re-exported so existing consumers of stats.Verdicts keep
// compiling; the canonical definition lives in internal/codescene.
type Verdicts = codescene.Verdicts

// CodesceneEvent is one CodeScene run as codescene-events.jsonl holds it (see
// docs/team-setup/codescene-stats.md). The server appends one for a
// validate_completion call whose `codescene` argument reports a run. Counts
// and metadata only: no file path, no ref name, no skip text.
// analyze_change_set is categorical (verdicts / quality-gate / problem-points),
// not a 1-10 score.
type CodesceneEvent struct {
	Ts time.Time `json:"ts"`
	codescene.Digest
}

// UnmarshalJSON decodes Ts, then delegates the rest to Digest's own
// UnmarshalJSON. Anonymously embedding Digest promotes its UnmarshalJSON to
// CodesceneEvent, so without this override the promoted method would run
// alone and Ts would never be set — see codescene.Digest.UnmarshalJSON's
// doc comment.
func (e *CodesceneEvent) UnmarshalJSON(b []byte) error {
	var ts struct {
		Ts time.Time `json:"ts"`
	}
	if err := json.Unmarshal(b, &ts); err != nil {
		return err
	}
	if err := json.Unmarshal(b, &e.Digest); err != nil {
		return err
	}
	e.Ts = ts.Ts
	return nil
}

// CodesceneRollup is the nested `codescene` block in rollup.json.
type CodesceneRollup struct {
	Runs              int            `json:"runs"`
	GatesPassed       int            `json:"gates_passed"`
	GatesFailed       int            `json:"gates_failed"`
	LatestGate        string         `json:"latest_gate"`
	LatestTrend       string         `json:"latest_trend"`
	LatestNetPP       float64        `json:"latest_net_pp"`
	NetPPP50          float64        `json:"net_pp_p50"`
	Regressions       int            `json:"regressions"`
	Improvements      int            `json:"improvements"`
	Neutral           int            `json:"neutral"`
	FilesAnalyzed     int            `json:"files_analyzed"`
	CategoryHistogram map[string]int `json:"category_histogram"`
	WindowStart       time.Time      `json:"window_start"`
	WindowEnd         time.Time      `json:"window_end"`
}

// codesceneRunTool is the one tool name a run record carries.
const codesceneRunTool = "analyze_change_set"

// codesceneOtherCategory is the key that takes the count of every category
// key a record does not keep.
const codesceneOtherCategory = "other"

// plainCategoryName matches a key that reads as a category name and nothing
// more: letters and the punctuation a name uses, with no digit, path
// separator, dot or colon, so it cannot carry a file path, a line number or a
// function name.
var plainCategoryName = regexp.MustCompile(`^[A-Za-z][A-Za-z ,'-]{0,39}$`)

// plainCategoryCounts returns a copy of counts holding only the keys that
// read as plain category names, with every other key's count added to
// "other". The keys are caller text and there is no list of CodeScene's
// category names to check them against. It returns nil for no counts.
func plainCategoryCounts(counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	out := make(map[string]int, len(counts))
	for k, n := range counts {
		if !plainCategoryName.MatchString(k) {
			k = codesceneOtherCategory
		}
		out[k] += n
	}
	return out
}

// RunRecord reduces d to the fields a CodesceneEvent may hold. The caller's
// free text — skip reason, skip evidence, base ref — is left out, and so is
// Ran: every record is a run. Tool is caller text too, so a record names the
// tool only when it is the expected one, and says "other" for anything else.
// Category keys are caller text as well: see plainCategoryCounts.
func RunRecord(d codescene.Digest) codescene.Digest {
	tool := codesceneRunTool
	if d.Tool != "" && d.Tool != codesceneRunTool {
		tool = "other"
	}
	return codescene.Digest{
		Tool:           tool,
		QualityGate:    d.QualityGate,
		FilesAnalyzed:  d.FilesAnalyzed,
		Verdicts:       d.Verdicts,
		Trend:          d.Trend,
		NetPP:          d.NetPP,
		CategoryCounts: plainCategoryCounts(d.CategoryCounts),
	}
}

// RecordCodescene appends one CodeScene run, reduced by RunRecord, to
// codescene-events.jsonl. Best-effort, and safe on a nil Recorder.
func (r *Recorder) RecordCodescene(d codescene.Digest) {
	if r == nil {
		return
	}
	ev := CodesceneEvent{Ts: r.clock().Truncate(time.Second), Digest: RunRecord(d)}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := appendJSONL(r.dir, codesceneFile, ev); err != nil {
		r.logger.Warn("stats codescene append failed", "err", err)
	}
}

func readCodescene(dir string) ([]CodesceneEvent, error) {
	return readJSONL[CodesceneEvent](dir, codesceneFile)
}

// pruneCodescene rewrites codescene-events.jsonl keeping only records at/after cutoff.
func pruneCodescene(dir string, cutoff time.Time) error {
	events, err := readCodescene(dir)
	if err != nil {
		return err
	}
	kept := events[:0]
	for _, e := range events {
		if !e.Ts.Before(cutoff) {
			kept = append(kept, e)
		}
	}
	return rewriteJSONL(dir, codesceneFile, kept)
}

// computeCodescene aggregates per-run records. Returns nil when there are none,
// so the rollup's `codescene` key is omitted entirely (absence == no data).
func computeCodescene(events []CodesceneEvent) *CodesceneRollup {
	if len(events) == 0 {
		return nil
	}
	cr := &CodesceneRollup{
		CategoryHistogram: map[string]int{},
		WindowStart:       events[0].Ts,
		WindowEnd:         events[0].Ts,
		Runs:              len(events),
	}
	nps := make([]int64, 0, len(events)) // net_pp*100 as int64 for percentile()
	latest := events[0]
	for _, e := range events {
		if e.Ts.Before(cr.WindowStart) {
			cr.WindowStart = e.Ts
		}
		if !e.Ts.Before(cr.WindowEnd) {
			cr.WindowEnd = e.Ts
			latest = e
		}
		switch e.QualityGate {
		case "passed":
			cr.GatesPassed++
		case "failed":
			cr.GatesFailed++
		}
		switch e.Trend {
		case "regression":
			cr.Regressions++
		case "improvement":
			cr.Improvements++
		default:
			cr.Neutral++
		}
		cr.FilesAnalyzed += e.FilesAnalyzed
		for k, v := range e.CategoryCounts {
			cr.CategoryHistogram[k] += v
		}
		nps = append(nps, int64(math.Round(e.NetPP*100)))
	}
	cr.LatestGate = latest.QualityGate
	cr.LatestTrend = latest.Trend
	cr.LatestNetPP = latest.NetPP
	cr.NetPPP50 = float64(percentile(nps, 50)) / 100
	return cr
}
