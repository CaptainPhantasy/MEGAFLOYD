package tools

import (
	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
)

// JobsTools returns the job-record tool set. Coordinator.buildTools should
// append the result inside its first additive block so the tools flow through
// the existing allowlist filter and hook wrappers:
//
//	allTools = append(allTools, tools.JobsTools(
//		c.cfg.Config().Options.DataDirectory, c.permissions, c.cfg.WorkingDir(),
//	)...)
//
// The constructors take the data directory, the permission service, and the
// working directory, matching the other built-in tool factories.
func JobsTools(dataDir string, permissions permission.Service, workingDir string) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		NewContractTool(dataDir, permissions, workingDir),
		NewProveTool(dataDir, permissions, workingDir),
		NewDriftTool(dataDir, workingDir),
		NewWhyTool(dataDir, workingDir),
		NewRecoverTool(dataDir, workingDir),
		NewHandoffTool(dataDir, permissions, workingDir),
		NewReplayTool(dataDir, workingDir),
	}
}
