// Package mcpsrv: plan-result normalization — the unverifiable-claim
// checklist and its verdict calibration, controller-verified references, and
// controller rulings. No I/O.
package mcpsrv

import (
	"fmt"
	"strings"

	"github.com/patiently/anti-tangent-mcp/internal/planparser"
	"github.com/patiently/anti-tangent-mcp/internal/session"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

// rollupEvidencePerTaskMax bounds each per-task entry in the rolled-up
// codebase_reference_checklist evidence. It is wider than summary.go's
// summaryEvidenceMax (120) so the checklist has room for about two compact
// lines of paths and symbols without letting one task dominate.
const rollupEvidencePerTaskMax = 240

// splitTaskUnverifiable separates a task's findings into the ones that stay
// attached (kept) and the evidence strings that roll up to plan level
// (perTaskEvidence). The kept slice is freshly allocated so the caller's
// backing array is not aliased.
func splitTaskUnverifiable(findings []verdict.Finding) (kept []verdict.Finding, perTaskEvidence []string) {
	kept = make([]verdict.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category != verdict.CategoryUnverifiableCodebaseClaim {
			kept = append(kept, f)
			continue
		}
		perTaskEvidence = append(perTaskEvidence, f.Evidence)
	}
	return kept, perTaskEvidence
}

// stripTaskUnverifiableFindings removes every task-level
// unverifiable_codebase_claim finding and returns one checklist line per
// affected task, with that task's evidence joined by "; " and truncated at
// rollupEvidencePerTaskMax. A claim that only names paths the task's own
// Files: section lists is dropped and reaches no line. Reviewer-emitted
// plan-level unverifiable findings stay where they are. Each task's Findings
// is reassigned to a fresh slice.
//
// The label numbers by the PARSED plan position, never by the reviewer's own
// task_index: validateChunkIdentity checks a chunk's titles and order but not
// task_index, so a chunk-local index (e.g. the second chunk's first task
// reporting task_index: 1) survives into the merged response and would
// mislabel it as Task 1. parsedIdx holds, per result, the index of the
// parsed task it actually reports on (see parsedTaskIndexes, and
// planRound.planPositions for a round cut short); the merged-list position
// (i+1) is used only where it holds -1.
func stripTaskUnverifiableFindings(pr *verdict.PlanResult, tasks []planparser.RawTask, parsedIdx []int) []string {
	var lines []string
	for i := range pr.Tasks {
		idx := parsedIdx[i]
		findings := pr.Tasks[i].Findings
		if idx >= 0 {
			findings = dropListedFileClaims(findings, planparser.ListedPaths(tasks[idx].Body))
		}
		kept, perTask := splitTaskUnverifiable(findings)
		pr.Tasks[i].Findings = kept
		if len(perTask) == 0 {
			continue
		}
		taskNum := i + 1
		if idx >= 0 {
			taskNum = idx + 1
		}
		lines = append(lines, fmt.Sprintf("Task %d: %s",
			taskNum,
			truncate(strings.Join(perTask, "; "), rollupEvidencePerTaskMax)))
	}
	return lines
}

// calibratePlanVerdictForUnverifiableOnly treats a plan whose only findings
// are minor unverifiable_codebase_claim entries as a checklist rather than a
// blocker: plan_quality rises to at least actionable, unless the reviewer said
// rigorous, and next_action says so. The checklist is a list of references to
// pre-flight before dispatch, not work the plan owes. stripped reports whether
// task-level unverifiable findings were removed for the checklist, which counts
// as one such finding although the checklist is not among the findings. The
// ladder that runs next derives the verdict from the findings either way.
func calibratePlanVerdictForUnverifiableOnly(pr *verdict.PlanResult, stripped bool) {
	if !allPlanFindingsAreMinorUnverifiable(*pr, stripped) {
		return
	}
	pr.PlanVerdict = verdict.VerdictPass
	if pr.PlanQuality != verdict.PlanQualityRigorous {
		pr.PlanQuality = verdict.PlanQualityActionable
	}
	pr.NextAction = "Plan passes: dispatch."
	if stripped {
		pr.NextAction += " `codebase_reference_checklist` lists references the " +
			"reviewer could not verify: pre-flight any you have not already checked, or list them in " +
			"controller_verified_references on the next call."
		return
	}
	pr.NextAction += " The remaining unverifiable_codebase_claim finding(s) list references the " +
		"reviewer could not verify: pre-flight any you have not already checked, or list them in " +
		"controller_verified_references on the next call."
}

// isMinorUnverifiable reports whether f is a minor-severity
// unverifiable_codebase_claim finding — the only shape that the
// unverifiable-only calibration is willing to force-pass.
func isMinorUnverifiable(f verdict.Finding) bool {
	return f.Severity == verdict.SeverityMinor &&
		f.Category == verdict.CategoryUnverifiableCodebaseClaim
}

// allPlanFindingsAreMinorUnverifiable reports whether every finding across
// pr.PlanFindings and pr.Tasks[].Findings is a minor
// unverifiable_codebase_claim and at least one such finding exists, counting
// a stripped checklist as one. With nothing stripped and no findings it
// returns false: calibration only fires when there is something to calibrate.
func allPlanFindingsAreMinorUnverifiable(pr verdict.PlanResult, stripped bool) bool {
	found := stripped
	for _, f := range pr.PlanFindings {
		if !isMinorUnverifiable(f) {
			return false
		}
		found = true
	}
	for _, task := range pr.Tasks {
		for _, f := range task.Findings {
			if !isMinorUnverifiable(f) {
				return false
			}
			found = true
		}
	}
	return found
}

// suppressPlanVerifiedReferences drops every unverifiable_codebase_claim, at
// plan level or on a task, that a controller_verified_references entry
// matches; see suppressUnverifiableCodebaseClaim for the match.
func suppressPlanVerifiedReferences(pr *verdict.PlanResult, refs []string) {
	pr.PlanFindings = suppressUnverifiableCodebaseClaim(pr.PlanFindings, refs)
	for i := range pr.Tasks {
		pr.Tasks[i].Findings = suppressUnverifiableCodebaseClaim(pr.Tasks[i].Findings, refs)
	}
}

// waivePlanFindings moves every finding a ruling covers into
// WaivedFindings, plan-level and per task, fingerprinting plan-level findings
// under planScopeKey and a task's findings under the task key planTaskKeys
// derives from tasks, the parsed plan. The assignment replaces any waived
// entries the parsed response carried, since only the server fills them.
func waivePlanFindings(pr *verdict.PlanResult, rulings map[string]session.Ruling, tasks []planparser.RawTask) {
	pr.PlanFindings, pr.WaivedFindings = waiveRuled(pr.PlanFindings, planScopeKey, rulings, nil)
	keys := planTaskKeys(*pr, tasks)
	for i := range pr.Tasks {
		t := &pr.Tasks[i]
		t.Findings, t.WaivedFindings = waiveRuled(t.Findings, keys[i], rulings, nil)
	}
}
