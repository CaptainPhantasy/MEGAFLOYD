package tools

import (
	"context"
	"fmt"
	"os/exec"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
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

func NewSpawnLabTool(permissions permission.Service, workingDir string) fantasy.AgentTool {
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

			// The lab mounts the host workspace and the Docker socket,
			// so spawning it needs explicit permission like any other
			// privileged execution.
			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   GetSessionFromContext(ctx),
					ToolCallID:  call.ID,
					ToolName:    SpawnLabToolName,
					Action:      "execute",
					Description: fmt.Sprintf("Spawn privileged Docker lab %q (image %s) mounting %s and the Docker socket", params.SessionID, image, workingDir),
					Params:      params,
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
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

func NewExecuteLabTool(permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExecuteLabToolName,
		"Executes a command inside the active persistent lab and returns the output.",
		func(ctx context.Context, params ExecuteLabParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" || params.Command == "" {
				return fantasy.NewTextErrorResponse("session_id and command are required"), nil
			}

			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   GetSessionFromContext(ctx),
					ToolCallID:  call.ID,
					ToolName:    ExecuteLabToolName,
					Action:      "execute",
					Description: fmt.Sprintf("Execute in lab %s: %s", params.SessionID, params.Command),
					Params:      params,
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
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

func NewTeardownLabTool(permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeardownLabToolName,
		"Destroys the persistent lab container and cleans up resources.",
		func(ctx context.Context, params TeardownLabParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}

			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   GetSessionFromContext(ctx),
					ToolCallID:  call.ID,
					ToolName:    TeardownLabToolName,
					Action:      "execute",
					Description: fmt.Sprintf("Tear down lab container %s", params.SessionID),
					Params:      params,
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			if err := exec.CommandContext(ctx, "docker", "rm", "-f", params.SessionID).Run(); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Teardown failed: %v", err)), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Successfully tore down lab for session %s", params.SessionID)), nil
		})
}
