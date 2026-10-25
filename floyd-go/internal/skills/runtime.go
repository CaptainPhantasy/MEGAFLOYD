// Runtime execution support for Agent Skills.
// See https://agentskills.io for the specification.
package skills

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/legacy-ai/floyd/internal/extensibility"
)

// Runtime specifies how a skill is executed at runtime.
type Runtime = extensibility.Runtime

// ExecutableSkill is a skill definition carrying a runtime configuration.
type ExecutableSkill = extensibility.YAMLSkill

// ExecutorResult holds the output of a skill execution.
type ExecutorResult struct {
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration"`
	SkillName string        `json:"skill_name"`
}

// SkillExecutor defines the interface for executing skills at runtime.
type SkillExecutor interface {
	// CanExecute returns true if this executor can handle the given runtime type.
	CanExecute(runtime *Runtime) bool
	// Execute runs the skill with the given runtime configuration.
	Execute(ctx context.Context, skill *ExecutableSkill) (*ExecutorResult, error)
	// Type returns the executor type identifier (e.g., "bash", "builtin").
	Type() string
}

// BashExecutor executes skills using bash/shell.
type BashExecutor struct {
	shell      string // Defaults to "/bin/bash".
	workingDir string
}

// NewBashExecutor creates a BashExecutor with the default shell.
func NewBashExecutor() *BashExecutor {
	return &BashExecutor{shell: "/bin/bash"}
}

func (e *BashExecutor) CanExecute(runtime *Runtime) bool {
	if runtime == nil {
		return false
	}
	return runtime.Type == "bash" || runtime.Type == "sh" || runtime.Type == "shell"
}

func (e *BashExecutor) Type() string { return "bash" }

func (e *BashExecutor) Execute(ctx context.Context, skill *ExecutableSkill) (*ExecutorResult, error) {
	if skill.Runtime == nil {
		return nil, fmt.Errorf("skill %q has no runtime configuration", skill.Name)
	}

	command := skill.Runtime.Command
	if command == "" {
		return nil, fmt.Errorf("skill %q has no command in runtime config", skill.Name)
	}

	timeout := time.Duration(skill.Runtime.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.shell, "-c", command)
	cmd.Dir = e.workingDir
	cmd.Env = os.Environ()
	cmd.WaitDelay = time.Second

	// Inject environment variables from runtime config
	if len(skill.Runtime.Env) > 0 {
		cmd.Env = append(cmd.Env, envMapToSlice(skill.Runtime.Env)...)
	}

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecutorResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Duration:  duration,
		SkillName: skill.Name,
	}

	if err != nil {
		if ctx.Err() != nil {
			result.ExitCode = -1
			result.Stderr = fmt.Sprintf("skill %q timed out after %s\n%s", skill.Name, timeout, stderr.String())
		} else {
			result.ExitCode = exitCode(err)
		}
	}

	return result, nil
}

// BuiltinExecutor executes skills that are implemented as Go functions.
// These are registered at init time by packages that provide built-in skills.
type BuiltinExecutor struct {
	handlers map[string]BuiltinHandler
}

// BuiltinHandler is the function signature for a built-in skill handler.
type BuiltinHandler func(ctx context.Context, skill *ExecutableSkill) (*ExecutorResult, error)

// NewBuiltinExecutor creates a BuiltinExecutor with an empty handler registry.
func NewBuiltinExecutor() *BuiltinExecutor {
	return &BuiltinExecutor{
		handlers: make(map[string]BuiltinHandler),
	}
}

func (e *BuiltinExecutor) CanExecute(runtime *Runtime) bool {
	if runtime == nil {
		return false
	}
	return runtime.Type == "builtin"
}

func (e *BuiltinExecutor) Type() string { return "builtin" }

// Register adds a built-in skill handler.
func (e *BuiltinExecutor) Register(name string, handler BuiltinHandler) {
	e.handlers[name] = handler
}

// Has returns true if a built-in handler is registered for the given name.
func (e *BuiltinExecutor) Has(name string) bool {
	_, ok := e.handlers[name]
	return ok
}

func (e *BuiltinExecutor) Execute(ctx context.Context, skill *ExecutableSkill) (*ExecutorResult, error) {
	handler, ok := e.handlers[skill.Name]
	if !ok {
		return nil, fmt.Errorf("no builtin handler registered for skill %q", skill.Name)
	}

	start := time.Now()
	result, err := handler(ctx, skill)
	if result != nil {
		result.Duration = time.Since(start)
		result.SkillName = skill.Name
	}

	return result, err
}

// RuntimeDispatcher selects the appropriate executor for a skill's runtime type.
type RuntimeDispatcher struct {
	executors []SkillExecutor
}

// NewRuntimeDispatcher creates a dispatcher with the default executor chain:
// builtin → bash.
func NewRuntimeDispatcher() *RuntimeDispatcher {
	return NewRuntimeDispatcherForDirectory("")
}

// NewRuntimeDispatcherForDirectory runs executable skills in their workspace.
func NewRuntimeDispatcherForDirectory(workingDir string) *RuntimeDispatcher {
	bash := NewBashExecutor()
	bash.workingDir = workingDir
	return &RuntimeDispatcher{
		executors: []SkillExecutor{
			NewBuiltinExecutor(),
			bash,
		},
	}
}

// Dispatch finds the first executor that can handle the skill's runtime type
// and executes the skill.
func (d *RuntimeDispatcher) Dispatch(ctx context.Context, skill *ExecutableSkill) (*ExecutorResult, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is nil")
	}

	if skill.Runtime == nil {
		return nil, fmt.Errorf("skill %q has no runtime configuration — it is a documentation-only skill", skill.Name)
	}

	for _, executor := range d.executors {
		if executor.CanExecute(skill.Runtime) {
			return executor.Execute(ctx, skill)
		}
	}

	return nil, fmt.Errorf("no executor found for skill %q with runtime type %q", skill.Name, skill.Runtime.Type)
}

// --- helpers ---

func envMapToSlice(env map[string]string) []string {
	slice := make([]string, 0, len(env))
	for k, v := range env {
		slice = append(slice, k+"="+v)
	}
	return slice
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	type exitError interface {
		ExitCode() int
	}
	if ee, ok := err.(exitError); ok {
		return ee.ExitCode()
	}
	return 1
}

// limitedBuffer bounds captured output while continuing to drain subprocess pipes.
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 256*1024 - b.Len()
	if remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(remaining, n)])
	}
	return n, nil
}
