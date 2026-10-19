package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const FailureModeEnumeratorToolName = "failure_mode_enumerator"

type FailureModeEnumeratorParams struct {
	SystemDescription string   `json:"system_description" jsonschema:"description=Short description of system/component under analysis"`
	ChangeIntent      string   `json:"change_intent,omitempty" jsonschema:"description=Planned change to evaluate"`
	Domains           []string `json:"domains,omitempty" jsonschema:"description=Domains like api, db, ui, infra"`
}

type FailureModeEnumeratorResponse struct {
	FailureModes []string `json:"failure_modes"`
	Detectors    []string `json:"detectors"`
	Mitigations  []string `json:"mitigations"`
	Severity     string   `json:"severity"`
}

func NewFailureModeEnumeratorTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		FailureModeEnumeratorToolName,
		"Enumerates likely failure modes for a planned change with detectors and mitigations.",
		func(ctx context.Context, params FailureModeEnumeratorParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_ = ctx
			sys := strings.TrimSpace(params.SystemDescription)
			if sys == "" {
				return fantasy.NewTextErrorResponse("system_description is required"), nil
			}

			severity := "medium"
			if len(params.Domains) >= 3 {
				severity = "high"
			}
			if len(params.Domains) == 0 {
				severity = "low"
			}

			modes := []string{
				"Configuration drift between environments causes startup/runtime mismatch",
				"Backward-incompatible assumptions break existing scripts or automation",
				"Timeout/retry strategy amplifies transient failure into hard outage",
				"Partial rollout leaves mixed behavior across lanes/binaries",
				"Missing observability hides regression until user-facing impact",
			}
			detectors := []string{
				"Startup invariant checks with explicit fatal/warn mode",
				"Canary smoke command against representative working directory",
				"Error hash/circuit-breaker counters over rolling 10-minute window",
				"Scoreboard trend deltas (latency/tokens/failure spikes)",
			}
			mitigations := []string{
				"Introduce strict compatibility shims and explicit deprecation windows",
				"Gate risky paths behind env-guarded feature flags",
				"Prepare reversible install path and rollback command",
				"Require focused test + smoke receipt before promotion",
			}

			resp := FailureModeEnumeratorResponse{
				FailureModes: modes,
				Detectors:    detectors,
				Mitigations:  mitigations,
				Severity:     severity,
			}

			var b strings.Builder
			b.WriteString("Failure-mode analysis\n")
			b.WriteString(fmt.Sprintf("System: %s\n", sys))
			if strings.TrimSpace(params.ChangeIntent) != "" {
				b.WriteString(fmt.Sprintf("Change: %s\n", strings.TrimSpace(params.ChangeIntent)))
			}
			if len(params.Domains) > 0 {
				b.WriteString("Domains: " + strings.Join(params.Domains, ", ") + "\n")
			}
			b.WriteString("\nLikely failure modes:\n")
			for _, m := range modes {
				b.WriteString("- " + m + "\n")
			}
			b.WriteString("\nSeverity: " + severity)

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(b.String()), resp), nil
		},
	)
}
