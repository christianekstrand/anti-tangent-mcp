// Package mcpsrv: agent-network mode. A plan that declares
// `**Plan kind:** agent-network`, a task that declares `**Kind:** experiment`,
// and a call that sends boundary_rules are reviewed for work that belongs to
// the model rather than to code. This file holds the server's side of that:
// argument limits, the findings the server adds, and the ones it removes.
package mcpsrv

import (
	"fmt"
	"strings"

	"github.com/patiently/anti-tangent-mcp/internal/planparser"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

const (
	maxBoundaryRules     = 20
	maxBoundaryRuleChars = 1000
	// fixLadderCriterion marks the boundary_violation an agent-network plan's
	// review raises for a fix on the wrong rung. It needs no boundary rule.
	fixLadderCriterion = "fix_ladder"
	// experimentLightweightReason is the lightweight_reason the server sets
	// when it forces an experiment task out of lightweight mode.
	experimentLightweightReason = "experiment task"
	// agentNoteValueMax bounds, in runes, how much of an unknown header or
	// argument value a note repeats back.
	agentNoteValueMax = 64
)

func normalizeBoundaryRules(rules []string) ([]string, error) {
	return normalizeBoundedStringList("boundary_rules", rules, maxBoundaryRules, maxBoundaryRuleChars)
}

// dropUnrequestedBoundaryViolations removes the boundary_violation findings a
// call without boundary rules cannot have asked for: the reviewer had no rule
// to judge against. On an agent-network plan the fix_ladder finding is kept,
// since the fix ladder is the plan kind's own rule.
func dropUnrequestedBoundaryViolations(fs []verdict.Finding, hasRules, agentNetwork bool) []verdict.Finding {
	if hasRules || len(fs) == 0 {
		return fs
	}
	out := make([]verdict.Finding, 0, len(fs))
	for _, f := range fs {
		if f.Category == verdict.CategoryBoundaryViolation && !(agentNetwork && f.Criterion == fixLadderCriterion) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// applyPlanAgentNetwork runs on a plan review's merged results before the
// verdict ladder. It removes unrequested boundary findings, records each
// task's kind, rung and plan kind from the parsed plan, and takes every
// experiment task out of lightweight mode, whatever the reviewer returned.
func applyPlanAgentNetwork(pr *verdict.PlanResult, tasks []planparser.RawTask, planKind string, rules []string) {
	agentNetwork := planKind == planparser.PlanKindAgentNetwork
	hasRules := len(rules) > 0
	pr.PlanFindings = dropUnrequestedBoundaryViolations(pr.PlanFindings, hasRules, agentNetwork)
	for i, idx := range parsedTaskIndexes(pr.Tasks, tasks) {
		t := &pr.Tasks[i]
		t.Findings = dropUnrequestedBoundaryViolations(t.Findings, hasRules, agentNetwork)
		t.PlanKind = planKind
		if idx < 0 {
			continue
		}
		t.TaskKind, t.Rung = declaredKind(planKind, tasks[idx]), tasks[idx].Rung
		if t.TaskKind == planparser.TaskKindExperiment {
			t.LightweightEligible = false
			t.LightweightReason = experimentLightweightReason
		}
	}
}

// declaredKind is the task kind the plan declares for t. On a plan with no
// **Plan kind:** header the parser's default, build, declares nothing, so a
// plain plan's results and run carry no task kind and a per-task call's own
// task_kind still counts.
func declaredKind(planKind string, t planparser.RawTask) string {
	if planKind == "" && t.Kind == planparser.TaskKindBuild {
		return ""
	}
	return t.Kind
}

// addPlanAgentNetworkNotes adds the minor notes about how the plan and this
// call declared their kind. They describe the declarations, not the plan's
// quality, so they join after the verdict ladder and never move the verdict.
func addPlanAgentNetworkNotes(pr *verdict.PlanResult, tasks []planparser.RawTask, planKind, unknownPlanKind string, rules []string) {
	if unknownPlanKind != "" {
		pr.PlanFindings = append(pr.PlanFindings, unknownPlanKindNote(unknownPlanKind))
	}
	pr.PlanFindings = append(pr.PlanFindings, declarationNotes(planKind, unknownPlanKind, len(rules) > 0)...)
	for i, idx := range parsedTaskIndexes(pr.Tasks, tasks) {
		if idx < 0 {
			continue
		}
		pr.Tasks[i].Findings = append(pr.Tasks[i].Findings, taskHeaderNotes(tasks[idx])...)
	}
}

// declarationNotes returns boundary_rules_missing for an agent-network plan
// reviewed without rules, and plan_kind_missing for rules sent for a plan
// with no **Plan kind:** header.
func declarationNotes(planKind, unknownPlanKind string, hasRules bool) []verdict.Finding {
	switch {
	case planKind == planparser.PlanKindAgentNetwork && !hasRules:
		return []verdict.Finding{boundaryRulesMissingNote()}
	case planKind == "" && unknownPlanKind == "" && hasRules:
		return []verdict.Finding{planKindMissingNote()}
	}
	return nil
}

// taskHeaderNotes returns a parsed task's notes: an unknown **Kind:** or
// **Rung:** value, and an experiment with no rung.
func taskHeaderNotes(t planparser.RawTask) []verdict.Finding {
	var out []verdict.Finding
	if t.UnknownKind != "" {
		out = append(out, unknownTaskKindNote(t.UnknownKind))
	}
	if t.UnknownRung != "" {
		out = append(out, unknownRungNote(t.UnknownRung))
	}
	if t.Kind == planparser.TaskKindExperiment && t.Rung == "" {
		out = append(out, rungMissingNote())
	}
	return out
}

func agentNote(cat verdict.Category, criterion, evidence, suggestion string) verdict.Finding {
	return verdict.Finding{
		Severity:   verdict.SeverityMinor,
		Category:   cat,
		Criterion:  criterion,
		Evidence:   evidence,
		Suggestion: suggestion,
	}
}

func quoteValue(v string) string {
	return fmt.Sprintf("%q", truncate(strings.TrimSpace(v), agentNoteValueMax))
}

func unknownPlanKindNote(v string) verdict.Finding {
	return agentNote(verdict.CategoryUnknownKind, "plan_kind",
		fmt.Sprintf("**Plan kind:** %s is not a plan kind anti-tangent knows, so the plan is reviewed as an ordinary plan.", quoteValue(v)),
		"Write `**Plan kind:** agent-network` above the first task heading, or remove the line.")
}

func unknownTaskKindNote(v string) verdict.Finding {
	return agentNote(verdict.CategoryUnknownKind, "task_kind",
		fmt.Sprintf("**Kind:** %s is not a task kind anti-tangent knows, so the task is reviewed as a build task.", quoteValue(v)),
		"Write `**Kind:** experiment` or `**Kind:** build`.")
}

func unknownRungNote(v string) verdict.Finding {
	return agentNote(verdict.CategoryUnknownKind, "rung",
		fmt.Sprintf("**Rung:** %s is not a fix-ladder rung, so the task is treated as naming none.", quoteValue(v)),
		"Name one of: "+strings.Join(planparser.Rungs, ", ")+".")
}

func rungMissingNote() verdict.Finding {
	return agentNote(verdict.CategoryRungMissing, "rung",
		"This experiment task names no fix-ladder rung, so the plan's fix ladder cannot place it.",
		"Add a `**Rung:**` line naming the rung the experiment works at: "+strings.Join(planparser.Rungs, ", ")+".")
}

func boundaryRulesMissingNote() verdict.Finding {
	return agentNote(verdict.CategoryBoundaryRulesMissing, "boundary_rules",
		"This is agent-network work, but the call sent no boundary_rules, so nothing is checked against a boundary rule.",
		"Pass the project's boundary rules as boundary_rules on validate_plan and on every per-task call.")
}

func planKindMissingNote() verdict.Finding {
	return agentNote(verdict.CategoryPlanKindMissing, "plan_kind",
		"This call sent boundary_rules for work with no plan kind. The rules are checked, but the determinism, rate, experiment and fix-ladder checks stay off.",
		"If this is agent-network work, add `**Plan kind:** agent-network` above the plan's first task heading, or pass plan_kind.")
}
