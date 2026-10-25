package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/skills"
)

const RunSkillToolName = "run_skill"

type RunSkillParams struct {
	Name string `json:"name" description:"Exact name of an available executable skill"`
}

// NewRunSkillTool runs discovered skills through the normal permission service.
func NewRunSkillTool(permissions permission.Service, workingDir string, available []*skills.Skill) fantasy.AgentTool {
	return fantasy.NewAgentTool(RunSkillToolName,
		"Execute a skill listed in available_skills with a runtime configuration. Supply its exact name. Uses the registered command, workspace directory, and normal tool permissions; returns stdout, stderr, exit_code and duration.",
		func(ctx context.Context, params RunSkillParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var skill *skills.Skill
			for _, candidate := range available {
				if candidate.Name == params.Name && !candidate.DisableModelInvocation {
					skill = candidate
					break
				}
			}
			if skill == nil || skill.Runtime == nil {
				return fantasy.NewTextErrorResponse("No active executable skill named " + params.Name), nil
			}
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for executable skills")
			}
			if permissions == nil {
				return fantasy.ToolResponse{}, fmt.Errorf("permission service is required for executable skills")
			}
			approved, err := permissions.Request(ctx, permission.CreatePermissionRequest{
				SessionID: sessionID, ToolCallID: call.ID, ToolName: RunSkillToolName,
				Action: "execute", Path: workingDir,
				Description: "Execute skill " + skill.Name + ": " + skill.Runtime.Command,
				Params:      map[string]string{"name": skill.Name, "command": skill.Runtime.Command, "working_dir": workingDir},
			})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !approved {
				return NewPermissionDeniedResponse(), nil
			}
			executable := &skills.ExecutableSkill{Name: skill.Name, Description: skill.Description, Runtime: skill.Runtime}
			result, err := skills.NewRuntimeDispatcherForDirectory(workingDir).Dispatch(ctx, executable)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if result.ExitCode != 0 {
				return fantasy.NewTextErrorResponse(string(encoded)), nil
			}
			return fantasy.NewTextResponse(string(encoded)), nil
		})
}
