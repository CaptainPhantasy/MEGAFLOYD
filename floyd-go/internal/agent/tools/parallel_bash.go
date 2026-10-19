package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/shell"
)

const (
	ParallelBashToolName         = "parallel_bash"
	DefaultMaxParallelCommands   = 4
	SuperFloydMaxParallel        = 12
	ParallelDefaultTimeout       = 60 * time.Second
	ParallelMaxOutputPerJob      = 10000
	DefaultParallelConcurrency   = 4
	SuperFloydMaxParallelCeiling = 32
)

//go:embed parallel_bash.tpl
var parallelBashDescriptionTmpl []byte

type ParallelBashCommand struct {
	Command     string `json:"command" description:"The bash command to execute"`
	Description string `json:"description" description:"Brief description of what the command does (max 30 chars)"`
}

type ParallelBashParams struct {
	Commands       []ParallelBashCommand `json:"commands" description:"Array of commands to execute in parallel (max depends on lane)"`
	WorkingDir     string                `json:"working_dir,omitempty" description:"Working directory for all commands (defaults to current directory)"`
	Timeout        int                   `json:"timeout,omitempty" description:"Timeout in seconds for all commands (default 60, max 300)"`
	MaxConcurrency int                   `json:"max_concurrency,omitempty" description:"Max concurrent commands to run at once (default 4)"`
	FailFast       bool                  `json:"fail_fast,omitempty" description:"Cancel remaining commands on first failure"`
}

type ParallelBashResult struct {
	Index       int    `json:"index"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Output      string `json:"output"`
	ExitCode    int    `json:"exit_code"`
	Duration    int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
}

type ParallelBashResponseMetadata struct {
	TotalDuration int64                `json:"total_duration_ms"`
	WorkingDir    string               `json:"working_directory"`
	Results       []ParallelBashResult `json:"results"`
	Summary       string               `json:"summary"`
}

func NewParallelBashTool(permissions permission.Service, workingDir string, attribution *config.Attribution, modelName string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ParallelBashToolName,
		string(parallelBashDescriptionTmpl),
		func(ctx context.Context, params ParallelBashParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Validate command count
			if len(params.Commands) == 0 {
				return fantasy.NewTextErrorResponse("no commands provided"), nil
			}
			maxCommands := maxParallelCommandsForLane()
			if len(params.Commands) > maxCommands {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("too many commands: %d (max %d)", len(params.Commands), maxCommands)), nil
			}

			// Validate each command has required fields
			for i, cmd := range params.Commands {
				if cmd.Command == "" {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("command %d is empty", i+1)), nil
				}
				if cmd.Description == "" {
					params.Commands[i].Description = fmt.Sprintf("Command %d", i+1)
				}
			}

			// Set timeout
			timeout := ParallelDefaultTimeout
			if params.Timeout > 0 {
				if params.Timeout > 300 {
					params.Timeout = 300
				}
				timeout = time.Duration(params.Timeout) * time.Second
			}

			// Determine working directory
			execWorkingDir := workingDir
			if params.WorkingDir != "" {
				execWorkingDir = params.WorkingDir
			}

			// Check if all commands are safe/read-only
			allSafe := true
			for _, cmd := range params.Commands {
				if !isSafeReadOnlyCommand(cmd.Command) {
					allSafe = false
					break
				}
			}

			// Request permission if any command is not safe
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for executing shell commands")
			}

			if !allSafe {
				// Build description for permission request
				var cmdList strings.Builder
				for i, cmd := range params.Commands {
					cmdList.WriteString(fmt.Sprintf("%d. %s\n", i+1, cmd.Command))
				}

				p, err := permissions.Request(ctx,
					permission.CreatePermissionRequest{
						SessionID:   sessionID,
						Path:        execWorkingDir,
						ToolCallID:  call.ID,
						ToolName:    ParallelBashToolName,
						Action:      "execute_parallel",
						Description: fmt.Sprintf("Execute %d commands in parallel:\n%s", len(params.Commands), cmdList.String()),
						Params:      params,
					},
				)
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				if !p {
					return NewPermissionDeniedResponse(), nil
				}
			}

			// Execute commands in parallel
			startTime := time.Now()
			results := make([]ParallelBashResult, len(params.Commands))
			var wg sync.WaitGroup

			// Create context with timeout for all commands
			execCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			bgManager := shell.GetBackgroundShellManager()
			bgManager.Cleanup()

			concurrency := params.MaxConcurrency
			if concurrency <= 0 {
				concurrency = DefaultParallelConcurrency
			}
			if concurrency > len(params.Commands) {
				concurrency = len(params.Commands)
			}
			if concurrency > maxCommands {
				concurrency = maxCommands
			}
			sem := make(chan struct{}, concurrency)

			for i, cmd := range params.Commands {
				wg.Add(1)
				go func(idx int, command ParallelBashCommand) {
					defer wg.Done()

					select {
					case sem <- struct{}{}:
					case <-execCtx.Done():
						results[idx] = ParallelBashResult{
							Index:       idx,
							Command:     command.Command,
							Description: command.Description,
							Output:      "canceled before execution",
							ExitCode:    130,
							Duration:    0,
							Error:       "canceled",
						}
						return
					}
					defer func() { <-sem }()

					cmdStart := time.Now()
					result := ParallelBashResult{
						Index:       idx,
						Command:     command.Command,
						Description: command.Description,
					}

					// Start background shell
					bgShell, err := bgManager.Start(execCtx, execWorkingDir, blockFuncs(), command.Command, command.Description)
					if err != nil {
						result.Error = fmt.Sprintf("Failed to start: %v", err)
						result.ExitCode = 1
						result.Duration = time.Since(cmdStart).Milliseconds()
						results[idx] = result
						if params.FailFast {
							cancel()
						}
						return
					}

					// Wait for completion or context cancellation
					bgShell.Wait()

					stdout, stderr, _, execErr := bgShell.GetOutput()
					bgManager.Remove(bgShell.ID)

					result.Duration = time.Since(cmdStart).Milliseconds()
					result.ExitCode = shell.ExitCode(execErr)

					// Combine and truncate output
					output := stdout
					if stderr != "" {
						if output != "" {
							output += "\n"
						}
						output += stderr
					}

					// Add exit code info if non-zero
					if execErr != nil && !shell.IsInterrupt(execErr) {
						if result.ExitCode != 0 {
							output += fmt.Sprintf("\nExit code: %d", result.ExitCode)
						}
					}

					// Truncate if needed
					if len(output) > ParallelMaxOutputPerJob {
						half := ParallelMaxOutputPerJob / 2
						output = output[:half] + "\n... [truncated] ...\n" + output[len(output)-half:]
					}

					if output == "" {
						output = BashNoOutput
					}

					result.Output = output
					results[idx] = result
					if params.FailFast && (result.ExitCode != 0 || result.Error != "") {
						cancel()
					}
				}(i, cmd)
			}

			// Wait for all commands
			wg.Wait()
			totalDuration := time.Since(startTime).Milliseconds()

			// Sort by index to preserve order
			sort.Slice(results, func(i, j int) bool {
				return results[i].Index < results[j].Index
			})

			// Build summary
			successCount := 0
			failCount := 0
			for _, r := range results {
				if r.ExitCode == 0 && r.Error == "" {
					successCount++
				} else {
					failCount++
				}
			}

			summary := fmt.Sprintf("Completed %d/%d commands successfully in %dms (concurrency=%d, fail_fast=%t)",
				successCount, len(results), totalDuration, concurrency, params.FailFast)

			// Build formatted output
			var output strings.Builder
			output.WriteString(fmt.Sprintf("## Parallel Execution Results\n\n"))
			output.WriteString(fmt.Sprintf("**Summary:** %s\n\n", summary))
			output.WriteString("---\n\n")

			for _, r := range results {
				statusIcon := "✓"
				if r.ExitCode != 0 || r.Error != "" {
					statusIcon = "✗"
				}

				output.WriteString(fmt.Sprintf("### [%s] Command %d: %s\n", statusIcon, r.Index+1, r.Description))
				output.WriteString(fmt.Sprintf("```\n%s\n```\n", r.Command))
				output.WriteString(fmt.Sprintf("**Duration:** %dms | **Exit Code:** %d\n\n", r.Duration, r.ExitCode))

				if r.Error != "" {
					output.WriteString(fmt.Sprintf("**Error:** %s\n\n", r.Error))
				}

				output.WriteString("**Output:**\n```\n")
				output.WriteString(r.Output)
				output.WriteString("\n```\n\n---\n\n")
			}

			metadata := ParallelBashResponseMetadata{
				TotalDuration: totalDuration,
				WorkingDir:    execWorkingDir,
				Results:       results,
				Summary:       summary,
			}

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(output.String()), metadata), nil
		})
}

// isSafeReadOnlyCommand checks if a command is safe/read-only
func isSafeReadOnlyCommand(command string) bool {
	cmdLower := strings.ToLower(strings.TrimSpace(command))
	for _, safe := range safeCommands {
		if strings.HasPrefix(cmdLower, safe) {
			if len(cmdLower) == len(safe) || cmdLower[len(safe)] == ' ' || cmdLower[len(safe)] == '-' {
				return true
			}
		}
	}
	return false
}

func maxParallelCommandsForLane() int {
	max := DefaultMaxParallelCommands
	bin := strings.ToLower(strings.TrimSpace(filepathBase(os.Args[0])))
	if strings.Contains(bin, "superfloyd") {
		max = SuperFloydMaxParallel
	}
	if v := strings.TrimSpace(os.Getenv("SUPERFLOYD_MAX_PARALLEL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > SuperFloydMaxParallelCeiling {
				n = SuperFloydMaxParallelCeiling
			}
			max = n
		}
	}
	if max < 1 {
		max = 1
	}
	return max
}

func filepathBase(path string) string {
	if path == "" {
		return ""
	}
	i := strings.LastIndex(path, "/")
	if i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}
