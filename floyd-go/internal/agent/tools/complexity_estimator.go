package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const ComplexityEstimatorToolName = "complexity_estimator"

type ComplexityEstimatorParams struct {
	Input string `json:"input" jsonschema:"description=Code or pseudocode to analyze for structural complexity"`
}

type ComplexityEstimatorResponse struct {
	CyclomaticEstimate int    `json:"cyclomatic_estimate"`
	BranchCount        int    `json:"branch_count"`
	LoopCount          int    `json:"loop_count"`
	ConditionCount     int    `json:"condition_count"`
	MaxNestingDepth    int    `json:"max_nesting_depth"`
	LineCount          int    `json:"line_count"`
	Risk               string `json:"risk"`
}

func NewComplexityEstimatorTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ComplexityEstimatorToolName,
		"Estimates structural complexity of code/pseudocode with a deterministic algorithm (branches, loops, nesting, and cyclomatic estimate).",
		func(ctx context.Context, params ComplexityEstimatorParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_ = ctx
			input := strings.TrimSpace(params.Input)
			if input == "" {
				return fantasy.NewTextErrorResponse("input is required"), nil
			}

			lines := strings.Split(input, "\n")
			branches := 0
			loops := 0
			conditions := 0
			depth := 0
			maxDepth := 0

			for _, ln := range lines {
				line := strings.TrimSpace(strings.ToLower(ln))
				if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
					continue
				}
				if strings.Contains(line, "if ") || strings.Contains(line, " if(") || strings.Contains(line, " if (") {
					branches++
					conditions++
				}
				if strings.Contains(line, "else if") {
					branches++
				}
				if strings.Contains(line, "switch ") || strings.HasPrefix(line, "switch(") {
					branches++
				}
				if strings.HasPrefix(line, "case ") || strings.Contains(line, " case ") {
					branches++
				}
				if strings.Contains(line, " for ") || strings.HasPrefix(line, "for ") || strings.HasPrefix(line, "for(") {
					loops++
				}
				if strings.Contains(line, " while ") || strings.HasPrefix(line, "while ") || strings.HasPrefix(line, "while(") {
					loops++
				}
				if strings.Contains(line, "&&") || strings.Contains(line, "||") {
					conditions += strings.Count(line, "&&") + strings.Count(line, "||")
				}

				for _, ch := range line {
					if ch == '{' {
						depth++
						if depth > maxDepth {
							maxDepth = depth
						}
					}
					if ch == '}' && depth > 0 {
						depth--
					}
				}
			}

			cyclomatic := 1 + branches + loops + max(0, conditions-branches)
			risk := "low"
			switch {
			case cyclomatic >= 30 || maxDepth >= 6:
				risk = "high"
			case cyclomatic >= 15 || maxDepth >= 4:
				risk = "medium"
			}

			resp := ComplexityEstimatorResponse{
				CyclomaticEstimate: cyclomatic,
				BranchCount:        branches,
				LoopCount:          loops,
				ConditionCount:     conditions,
				MaxNestingDepth:    maxDepth,
				LineCount:          len(lines),
				Risk:               risk,
			}

			summary := fmt.Sprintf(
				"Complexity estimate: %d (%s risk)\nBranches: %d, Loops: %d, Conditions: %d, Max nesting: %d, Lines: %d",
				resp.CyclomaticEstimate,
				resp.Risk,
				resp.BranchCount,
				resp.LoopCount,
				resp.ConditionCount,
				resp.MaxNestingDepth,
				resp.LineCount,
			)
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(summary), resp), nil
		},
	)
}
