package mcpsrv

import (
	"strings"

	"github.com/patiently/anti-tangent-mcp/internal/planparser"
	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

// taskSpecListedFiles returns the paths the task's own Files: section lists:
// the ones a Files: section in the caller's Context names, and the ones plan
// run planRunID recorded for the task from the plan.
func (h *handlers) taskSpecListedFiles(contextText, planRunID string, ref planrun.TaskRef) []string {
	files := planparser.ListedPaths(contextText)
	if planRunID == "" {
		return files
	}
	return append(files, h.deps.PlanRuns.TaskFiles(planRunID, ref)...)
}

// taskSpecChecklistNextAction is appended to validate_task_spec's next_action
// when the envelope carries a codebase_reference_checklist.
const taskSpecChecklistNextAction = " `codebase_reference_checklist` lists references the reviewer could not verify: " +
	"pre-flight any that were not already checked. It is a to-do list, not a defect in the spec."

// splitTaskSpecChecklist takes every unverifiable_codebase_claim out of a
// validate_task_spec review's findings. The claims are references for the
// controller to pre-flight, not defects in the spec, so they are returned as
// the envelope's checklist, one entry per claim, and never reach the verdict
// ladder.
func splitTaskSpecChecklist(findings []verdict.Finding) (kept []verdict.Finding, checklist []string) {
	kept, evidence := splitTaskUnverifiable(findings)
	for _, e := range evidence {
		if e = strings.TrimSpace(e); e != "" {
			checklist = append(checklist, truncate(e, rollupEvidencePerTaskMax))
		}
	}
	return kept, checklist
}

// suppressUnverifiableCodebaseClaim drops any unverifiable_codebase_claim
// finding whose evidence OR criterion substring-matches any CVR entry (either
// direction: entry-is-substring-of-text OR text-is-substring-of-entry). It
// mirrors the prompt-side instruction in pre.tmpl §48 but provides
// deterministic, reviewer-compliance-independent behavior.
//
// Defensive guards mirror suppressTestabilityExtractionScopeDrift:
//   - empty CVR or empty findings short-circuits to the input slice
//   - empty/whitespace-only evidence AND criterion is treated as non-match
//     (avoids the strings.Contains(non_empty, "") trap)
//   - CVR entries shorter than 4 code points are skipped (avoid single-letter
//     false matches like a CVR entry of "T" swallowing every claim)
//
// Findings whose category is NOT unverifiable_codebase_claim pass through
// unchanged.
func suppressUnverifiableCodebaseClaim(findings []verdict.Finding, cvr []string) []verdict.Finding {
	if len(cvr) == 0 || len(findings) == 0 {
		return findings
	}
	usable := make([]string, 0, len(cvr))
	for _, e := range cvr {
		if len([]rune(e)) >= 4 {
			usable = append(usable, e)
		}
	}
	if len(usable) == 0 {
		return findings
	}
	out := make([]verdict.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category != verdict.CategoryUnverifiableCodebaseClaim {
			out = append(out, f)
			continue
		}
		evidence := strings.TrimSpace(f.Evidence)
		criterion := strings.TrimSpace(f.Criterion)
		if evidence == "" && criterion == "" {
			out = append(out, f)
			continue
		}
		matched := false
		for _, e := range usable {
			if evidence != "" && (strings.Contains(evidence, e) || strings.Contains(e, evidence)) {
				matched = true
				break
			}
			if criterion != "" && (strings.Contains(criterion, e) || strings.Contains(e, criterion)) {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, f)
		}
	}
	return out
}

// suppressTestabilityExtractionScopeDrift drops any scope_drift finding whose
// evidence matches a testability_extractions entry by substring in either
// direction (entry is substring of evidence OR evidence is substring of
// entry). Non-scope_drift findings pass through unchanged. Empty extractions
// short-circuits to the input slice (no allocation).
//
// Empty/whitespace-only evidence on a scope_drift finding is treated as
// non-match: `strings.Contains(non_empty, "")` returns true, which would
// otherwise let a non-compliant reviewer that emits empty Evidence suppress
// every scope_drift finding whenever extractions is non-empty.
func suppressTestabilityExtractionScopeDrift(findings []verdict.Finding, extractions []string) []verdict.Finding {
	if len(extractions) == 0 || len(findings) == 0 {
		return findings
	}
	out := make([]verdict.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category != verdict.CategoryScopeDrift {
			out = append(out, f)
			continue
		}
		if strings.TrimSpace(f.Evidence) == "" {
			out = append(out, f)
			continue
		}
		matched := false
		for _, e := range extractions {
			if e == "" {
				continue
			}
			if strings.Contains(f.Evidence, e) || strings.Contains(e, f.Evidence) {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, f)
		}
	}
	return out
}
