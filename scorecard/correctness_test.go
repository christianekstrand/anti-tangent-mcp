package scorecard

import (
	"testing"
	"time"
)

func snapLine(hash string, idx int, post string, cats map[string]int, added int) RunLine {
	l := taskLine(hash, t0, idx, post, completion("m", post))
	l.Task.Categories = cats
	l.Task.LinesAdded = added
	return l
}

func TestCorrectnessRates(t *testing.T) {
	lines := []RunLine{
		snapLine("r1", 1, "pass", nil, 10),                              // escaped, never flagged
		snapLine("r1", 2, "pass", map[string]int{"correctness": 2}, 30), // escaped, was flagged
		snapLine("r1", 3, "warn", map[string]int{"correctness": 1}, 20), // not passed, flagged
		snapLine("r1", 4, "pass", nil, 0),                               // major tests finding only
		snapLine("r1", 5, "pass", map[string]int{"quality": 1}, 0),      // clean
	}
	outs := []OutcomeLine{outcome("r1", SourceFinalReview, t0.Add(time.Hour),
		OutcomeFinding{TaskIndex: 1, Severity: "major", Category: "correctness"},
		OutcomeFinding{TaskIndex: 2, Severity: "critical", Category: "correctness"},
		OutcomeFinding{TaskIndex: 3, Severity: "major", Category: "correctness"},
		OutcomeFinding{TaskIndex: 4, Severity: "major", Category: "tests"},
		OutcomeFinding{TaskIndex: 5, Severity: "minor", Category: "correctness"})}
	g := only(t, Compute(lines, outs, Options{}).ByReviewModel, SourceFinalReview)

	if g.CorrectnessEscapeRate.Num != 2 || g.CorrectnessEscapeRate.N != 4 {
		t.Fatalf("correctness escape = %+v, want 2/4", g.CorrectnessEscapeRate)
	}
	if g.CorrectnessFlagRecall.Num != 2 || g.CorrectnessFlagRecall.N != 3 {
		t.Fatalf("correctness recall = %+v, want 2/3", g.CorrectnessFlagRecall)
	}
	if g.EscapeRate.Num != 3 {
		t.Fatalf("escape = %+v, want 3 (tasks 1, 2, 4)", g.EscapeRate)
	}
	if g.LinesAddedP50 != 20 {
		t.Fatalf("lines added p50 = %d, want 20", g.LinesAddedP50)
	}
}

func TestCorrectnessRatesOnRecordsWithoutCategories(t *testing.T) {
	lines := []RunLine{taskLine("r1", t0, 1, "pass", completion("m", "pass"))}
	outs := []OutcomeLine{outcome("r1", SourceFinalReview, t0.Add(time.Hour),
		OutcomeFinding{TaskIndex: 1, Severity: "major", Category: "correctness"})}
	g := only(t, Compute(lines, outs, Options{}).ByReviewModel, SourceFinalReview)
	if g.CorrectnessFlagRecall.Num != 0 || g.CorrectnessFlagRecall.N != 1 || g.LinesAddedP50 != 0 {
		t.Fatalf("recall = %+v lines = %d", g.CorrectnessFlagRecall, g.LinesAddedP50)
	}
}
