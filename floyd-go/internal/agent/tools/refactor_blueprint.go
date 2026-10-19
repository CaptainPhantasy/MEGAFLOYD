package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const RefactorBlueprintToolName = "refactor_blueprint"

type RefactorBlueprintParams struct {
	Goal        string   `json:"goal" jsonschema:"description=Desired outcome of the refactor"`
	CodeSmells  []string `json:"code_smells,omitempty" jsonschema:"description=Known smells or pain points"`
	Constraints []string `json:"constraints,omitempty" jsonschema:"description=Constraints like API compatibility, deadlines, risk limits"`
}

type RefactorBlueprintResponse struct {
	Phases        []string `json:"phases"`
	Guardrails    []string `json:"guardrails"`
	TestPlan      []string `json:"test_plan"`
	RollbackPlan  []string `json:"rollback_plan"`
	EstimatedRisk string   `json:"estimated_risk"`
}

func NewRefactorBlueprintTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		RefactorBlueprintToolName,
		"Generates a deterministic phased refactor plan with test and rollback guardrails.",
		func(ctx context.Context, params RefactorBlueprintParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_ = ctx
			goal := strings.TrimSpace(params.Goal)
			if goal == "" {
				return fantasy.NewTextErrorResponse("goal is required"), nil
			}

			risk := "low"
			if len(params.CodeSmells) >= 3 || len(params.Constraints) >= 3 {
				risk = "medium"
			}
			if len(params.CodeSmells) >= 6 || containsAny(params.Constraints, []string{"no downtime", "backward compatibility", "safety-critical"}) {
				risk = "high"
			}

			resp := RefactorBlueprintResponse{
				Phases: []string{
					"1) Baseline: capture current behavior and failing/passing tests",
					"2) Isolation: introduce seam/interfaces around volatile logic",
					"3) Migration: move logic incrementally behind seam",
					"4) Verification: run focused + integration tests",
					"5) Cleanup: remove dead paths and document deltas",
				},
				Guardrails: []string{
					"Keep externally visible contracts stable unless explicitly approved",
					"One logical change-set per commit",
					"Abort migration step if golden tests regress",
				},
				TestPlan: []string{
					"Run unit tests around touched modules",
					"Run integration smoke for user-facing flows",
					"Add regression tests for every bug discovered during refactor",
				},
				RollbackPlan: []string{
					"Retain old path behind feature flag until verification complete",
					"Prepare reversible commits/cherry-picks",
					"If production issue appears: disable flag and redeploy last green build",
				},
				EstimatedRisk: risk,
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("Refactor blueprint for: %s\n", goal))
			if len(params.CodeSmells) > 0 {
				b.WriteString("Smells: " + strings.Join(params.CodeSmells, ", ") + "\n")
			}
			if len(params.Constraints) > 0 {
				b.WriteString("Constraints: " + strings.Join(params.Constraints, ", ") + "\n")
			}
			b.WriteString("\nPhases:\n")
			for _, p := range resp.Phases {
				b.WriteString("- " + p + "\n")
			}
			b.WriteString("\nRisk: " + resp.EstimatedRisk)

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(b.String()), resp), nil
		},
	)
}

func containsAny(items []string, needles []string) bool {
	for _, item := range items {
		s := strings.ToLower(item)
		for _, n := range needles {
			if strings.Contains(s, strings.ToLower(n)) {
				return true
			}
		}
	}
	return false
}
