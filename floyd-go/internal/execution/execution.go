package execution

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ferrors "github.com/legacy-ai/floyd/internal/errors"
)

type Result struct {
	Output   string
	ExitCode int
	Error    error
	Command  string
	Duration time.Duration
}

type Options struct {
	Cwd            string
	Env            map[string]string
	Timeout        time.Duration
	Shell          string
	MaxBufferSize  int
	CaptureStderr  bool
	SkipValidation bool
}

type BackgroundOptions struct {
	Options
	OnOutput func(output string)
	OnError  func(output string)
	OnExit   func(code int)
}

type BackgroundProcess struct {
	PID       int
	Kill      func() error
	IsRunning func() bool
}

type BackgroundJob struct {
	PID        int
	Command    string
	StartedAt  time.Time
	FinishedAt *time.Time
	ExitCode   *int
	Stdout     bytes.Buffer
	Stderr     bytes.Buffer
	Kill       func() error
	IsRunning  func() bool
	mu         sync.Mutex
}

type BackgroundJobInfo struct {
	PID        int
	Command    string
	Running    bool
	StartedAt  time.Time
	FinishedAt *time.Time
	ExitCode   *int
}

type Config struct {
	Shell           string
	DefaultTimeout  time.Duration
	MaxBufferSize   int
	AllowedPrefixes []string
	AllowedPatterns []*regexp.Regexp
	DeniedPrefixes  []string
	DeniedPatterns  []*regexp.Regexp
}

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*rm\s+(-rf?|--recursive)\s+[\/~]`),
	regexp.MustCompile(`^\s*dd\s+.*of=\/dev\/(disk|hd|sd)`),
	regexp.MustCompile(`^\s*mkfs`),
	regexp.MustCompile(`^\s*:\(\)\{\s*:\|:\s*&\s*\}\s*;`),
	regexp.MustCompile(`^\s*>(\/dev\/sd|\/dev\/hd)`),
	regexp.MustCompile(`^\s*sudo\s+.*(rm|mkfs|dd|chmod|chown)`),
}

type Environment struct {
	config               Config
	workingDirectory     string
	environmentVariables map[string]string
	executionCount       int
	mu                   sync.Mutex
	background           map[int]*exec.Cmd
	backgroundJobs       map[int]*BackgroundJob
}

func NewEnvironment(cfg Config) *Environment {
	wd, _ := os.Getwd()
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 30 * time.Second
	}
	if cfg.MaxBufferSize == 0 {
		cfg.MaxBufferSize = 5 * 1024 * 1024
	}

	env := map[string]string{}
	for _, pair := range os.Environ() {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	return &Environment{
		config:               cfg,
		workingDirectory:     wd,
		environmentVariables: env,
		background:           map[int]*exec.Cmd{},
		backgroundJobs:       map[int]*BackgroundJob{},
	}
}

func (e *Environment) Initialize(ctx context.Context) error {
	shell := e.config.Shell
	if shell == "" {
		shell = defaultShell()
	}

	_, err := e.Execute(ctx, fmt.Sprintf("%s -c \"echo Shell is available\"", shell), Options{Timeout: 5 * time.Second, SkipValidation: true})
	if err != nil {
		return ferrors.CreateUserError(
			"Failed to initialize command execution environment",
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryCommandExecution,
				Cause:    err,
				Resolution: []string{
					"Check that your shell is properly configured",
				},
			},
		)
	}
	return nil
}

func (e *Environment) Execute(ctx context.Context, command string, options Options) (Result, error) {
	e.mu.Lock()
	e.executionCount++
	e.mu.Unlock()

	if !options.SkipValidation {
		if err := e.validateCommand(command); err != nil {
			return Result{}, err
		}
	}

	cwd := options.Cwd
	if cwd == "" {
		cwd = e.workingDirectory
	}
	shell := options.Shell
	if shell == "" {
		shell = e.config.Shell
	}
	if shell == "" {
		shell = defaultShell()
	}
	if err := validateShell(shell); err != nil {
		return Result{}, err
	}
	maxBuffer := options.MaxBufferSize
	if maxBuffer == 0 {
		maxBuffer = e.config.MaxBufferSize
	}
	captureStderr := options.CaptureStderr
	if !options.CaptureStderr {
		captureStderr = false
	} else {
		captureStderr = true
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = e.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env := e.mergeEnv(options.Env)

	slog.Debug("Executing command", "command", command, "cwd", cwd, "shell", shell)
	start := time.Now()

	cmd := exec.CommandContext(ctx, shell, "-c", command) // #nosec G204
	configureProcess(cmd)
	cmd.Dir = cwd
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	if captureStderr {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	if captureStderr {
		output += stderr.String()
	}
	if maxBuffer > 0 && len(output) > maxBuffer {
		output = output[:maxBuffer]
	}

	exitCode := 0
	if err != nil {
		exitCode = exitCodeFromError(err)
		slog.Warn("Command execution failed", "command", command, "exit_code", exitCode, "duration", duration)
		return Result{Output: output, ExitCode: exitCode, Error: err, Command: command, Duration: duration}, nil
	}

	return Result{Output: output, ExitCode: 0, Command: command, Duration: duration}, nil
}

func (e *Environment) ExecuteBackground(ctx context.Context, command string, options BackgroundOptions) (*BackgroundProcess, error) {
	if err := e.validateCommand(command); err != nil {
		return nil, err
	}

	cwd := options.Cwd
	if cwd == "" {
		cwd = e.workingDirectory
	}
	shell := options.Shell
	if shell == "" {
		shell = e.config.Shell
	}
	if shell == "" {
		shell = defaultShell()
	}
	if err := validateShell(shell); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, shell, "-c", command) // #nosec G204
	configureProcess(cmd)
	cmd.Dir = cwd
	cmd.Env = e.mergeEnv(options.Env)

	// Let os/exec own the copy goroutines so Wait does not finish until both
	// streams have been delivered to their callbacks.
	cmd.Stdout = outputCallback(options.OnOutput)
	cmd.Stderr = outputCallback(options.OnError)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	pid := cmd.Process.Pid
	var isRunning atomic.Bool
	isRunning.Store(true)
	e.mu.Lock()
	e.background[pid] = cmd
	e.mu.Unlock()

	go func() {
		err := cmd.Wait()
		isRunning.Store(false)
		code := exitCodeFromError(err)
		e.mu.Lock()
		delete(e.background, pid)
		e.mu.Unlock()
		if options.OnExit != nil {
			options.OnExit(code)
		}
	}()

	return &BackgroundProcess{
		PID: pid,
		Kill: func() error {
			e.mu.Lock()
			defer e.mu.Unlock()
			if !isRunning.Load() {
				return os.ErrProcessDone
			}
			return cmd.Cancel()
		},
		IsRunning: isRunning.Load,
	}, nil
}

func (e *Environment) StartBackground(ctx context.Context, command string, options BackgroundOptions) (*BackgroundJob, error) {
	job := &BackgroundJob{
		Command:   command,
		StartedAt: time.Now(),
	}

	process, err := e.ExecuteBackground(ctx, command, BackgroundOptions{
		Options: options.Options,
		OnOutput: func(output string) {
			job.mu.Lock()
			job.Stdout.WriteString(output)
			job.mu.Unlock()
			if options.OnOutput != nil {
				options.OnOutput(output)
			}
		},
		OnError: func(output string) {
			job.mu.Lock()
			job.Stderr.WriteString(output)
			job.mu.Unlock()
			if options.OnError != nil {
				options.OnError(output)
			}
		},
		OnExit: func(code int) {
			job.mu.Lock()
			now := time.Now()
			job.FinishedAt = &now
			job.ExitCode = &code
			job.mu.Unlock()
			if options.OnExit != nil {
				options.OnExit(code)
			}
		},
	})
	if err != nil {
		return nil, err
	}

	job.PID = process.PID
	job.Kill = process.Kill
	job.IsRunning = process.IsRunning

	e.mu.Lock()
	e.backgroundJobs[job.PID] = job
	e.mu.Unlock()

	return job, nil
}

func (e *Environment) ListBackground() []BackgroundJobInfo {
	e.mu.Lock()
	defer e.mu.Unlock()

	results := make([]BackgroundJobInfo, 0, len(e.backgroundJobs))
	for _, job := range e.backgroundJobs {
		job.mu.Lock()
		info := BackgroundJobInfo{
			PID:        job.PID,
			Command:    job.Command,
			Running:    job.IsRunning != nil && job.IsRunning(),
			StartedAt:  job.StartedAt,
			FinishedAt: job.FinishedAt,
			ExitCode:   job.ExitCode,
		}
		job.mu.Unlock()
		results = append(results, info)
	}
	return results
}

func (e *Environment) BackgroundOutput(pid int) (string, string, bool) {
	e.mu.Lock()
	job, ok := e.backgroundJobs[pid]
	e.mu.Unlock()
	if !ok {
		return "", "", false
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.Stdout.String(), job.Stderr.String(), true
}

func (e *Environment) KillBackground(pid int) error {
	e.mu.Lock()
	job, ok := e.backgroundJobs[pid]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("background job not found: %d", pid)
	}
	if job.Kill == nil {
		return fmt.Errorf("background job cannot be killed: %d", pid)
	}
	err := job.Kill()
	e.mu.Lock()
	delete(e.backgroundJobs, pid)
	e.mu.Unlock()
	return err
}

func (e *Environment) KillAll() {
	e.mu.Lock()
	commands := make([]*exec.Cmd, 0, len(e.background))
	for pid, cmd := range e.background {
		commands = append(commands, cmd)
		delete(e.background, pid)
	}
	clear(e.backgroundJobs)
	e.mu.Unlock()

	// A job's Kill callback also acquires e.mu. Stop the captured commands
	// directly, outside that lock, once per process.
	for _, cmd := range commands {
		if cmd.Process != nil {
			_ = cmd.Cancel()
		}
	}
}

type outputCallback func(string)

func (write outputCallback) Write(data []byte) (int, error) {
	if write != nil {
		write(string(data))
	}
	return len(data), nil
}

func (e *Environment) SetWorkingDirectory(directory string) {
	e.workingDirectory = directory
	slog.Debug("Working directory set", "dir", directory)
}

func (e *Environment) WorkingDirectory() string {
	return e.workingDirectory
}

func (e *Environment) SetEnvironmentVariable(name, value string) {
	e.environmentVariables[name] = value
	slog.Debug("Environment variable set", "name", name)
}

func (e *Environment) GetEnvironmentVariable(name string) (string, bool) {
	value, ok := e.environmentVariables[name]
	return value, ok
}

func (e *Environment) mergeEnv(overrides map[string]string) []string {
	merged := make(map[string]string, len(e.environmentVariables)+len(overrides))
	for k, v := range e.environmentVariables {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}

	result := make([]string, 0, len(merged))
	for k, v := range merged {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func (e *Environment) validateCommand(command string) error {
	for _, prefix := range e.config.DeniedPrefixes {
		if strings.HasPrefix(command, prefix) {
			return ferrors.CreateUserError(
				fmt.Sprintf("Command execution blocked: '%s' matches denied prefix", command),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryCommandExecution,
					Resolution: []string{
						"This command is blocked by your configuration.",
					},
				},
			)
		}
	}
	for _, pattern := range e.config.DeniedPatterns {
		if pattern.MatchString(command) {
			return ferrors.CreateUserError(
				fmt.Sprintf("Command execution blocked: '%s' matches denied pattern", command),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryCommandExecution,
					Resolution: []string{
						"This command is blocked by your configuration.",
					},
				},
			)
		}
	}

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(command) {
			return ferrors.CreateUserError(
				fmt.Sprintf("Command execution blocked: '%s' matches dangerous pattern", command),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryCommandExecution,
					Resolution: []string{
						"This command is blocked for safety reasons. Please use a different command.",
					},
				},
			)
		}
	}

	if len(e.config.AllowedPrefixes) == 0 && len(e.config.AllowedPatterns) == 0 {
		return nil
	}

	for _, prefix := range e.config.AllowedPrefixes {
		if strings.HasPrefix(command, prefix) {
			return nil
		}
	}
	for _, pattern := range e.config.AllowedPatterns {
		if pattern.MatchString(command) {
			return nil
		}
	}

	return ferrors.CreateUserError(
		fmt.Sprintf("Command execution blocked: '%s' is not in the allowed list", command),
		ferrors.UserErrorOptions{
			Category: ferrors.ErrorCategoryCommandExecution,
			Resolution: []string{
				"This command is not allowed by your configuration.",
			},
		},
	)
}

func (e *Environment) ValidateCommand(command string) error {
	return e.validateCommand(command)
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "bash"
}

// validateShell checks if the shell binary exists and is executable
func validateShell(shell string) error {
	// On Windows, "cmd" is always available
	if runtime.GOOS == "windows" && shell == "cmd" {
		return nil
	}
	_, err := exec.LookPath(shell)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Shell '%s' not found in PATH", shell),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryCommandExecution,
				Resolution: []string{
					"Ensure the shell is installed and in your PATH.",
					"Set a valid shell via the SHELL environment variable.",
				},
			},
		)
	}
	return nil
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}
