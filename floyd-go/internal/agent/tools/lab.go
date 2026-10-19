package tools

import (
	"context"
	"fmt"
	"os/exec"

	"charm.land/fantasy"
)

const (
	SpawnLabToolName    = "mcp_floyd-lab_spawn_lab"
	ExecuteLabToolName  = "mcp_floyd-lab_execute_in_lab"
	TeardownLabToolName = "mcp_floyd-lab_teardown_lab"
)

type SpawnLabParams struct {
	SessionID string `json:"session_id" jsonschema:"Unique identifier for this session to track the lab"`
	Image     string `json:"image" jsonschema:"The Docker image to use (default: floyd-lab:full)"`
}

func NewSpawnLabTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SpawnLabToolName,
		"Spawns a persistent, privileged Docker lab using ORBSTACK live mounts. The current host workspace is mirrored into the container at /workspace instantly.",
		func(ctx context.Context, params SpawnLabParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			image := params.Image
			if image == "" {
				image = "floyd-lab:full"
			}

			// Force remove existing instance with context awareness
			rmCmd := exec.CommandContext(ctx, "docker", "rm", "-f", params.SessionID)
			_ = rmCmd.Run()

			args := []string{
				"run", "-d",
				"--name", params.SessionID,
				"-v", workingDir + ":/workspace",
				"-v", "/var/run/docker.sock:/var/run/docker.sock", // GOD MODE: OrbStack Daemon access
				"-e", "DOCKER_HOST=unix:///var/run/docker.sock",
				"-w", "/workspace",
				image,
				"tail", "-f", "/dev/null",
			}
			
			if err := exec.CommandContext(ctx, "docker", args...).Run(); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to spawn lab via OrbStack: %v", err)), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Persistent Lab [%s] Online.\n- OrbStack native host workspace '%s' live-mounted to /workspace.\n- GOD MODE ENABLED: Docker socket bound. You can run docker CLI commands INSIDE the lab to spawn sibling containers.\n- Because of OrbStack, DO NOT migrate files. Code changes are instantaneous.", params.SessionID, workingDir)), nil
		})
}

type ExecuteLabParams struct {
	SessionID string `json:"session_id" jsonschema:"Unique identifier matching the spawned lab"`
	Command   string `json:"command" jsonschema:"The shell command to execute inside the persistent lab"`
}

func NewExecuteLabTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExecuteLabToolName,
		"Executes a command inside the active persistent lab and returns the output.",
		func(ctx context.Context, params ExecuteLabParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" || params.Command == "" {
				return fantasy.NewTextErrorResponse("session_id and command are required"), nil
			}

			args := []string{"exec", params.SessionID, "bash", "-c", params.Command}
			out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
			
			resultStr := string(out)
			if err != nil {
				resultStr = fmt.Sprintf("Execution Error:\n%s\n%v", resultStr, err)
			}
			return fantasy.NewTextResponse(resultStr), nil
		})
}

type TeardownLabParams struct {
	SessionID string `json:"session_id" jsonschema:"Unique identifier matching the spawned lab"`
}

func NewTeardownLabTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeardownLabToolName,
		"Destroys the persistent lab container and cleans up resources.",
		func(ctx context.Context, params TeardownLabParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			
			if err := exec.CommandContext(ctx, "docker", "rm", "-f", params.SessionID).Run(); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Teardown failed: %v", err)), nil
			}
			
			return fantasy.NewTextResponse(fmt.Sprintf("Successfully tore down lab for session %s", params.SessionID)), nil
		})
}
