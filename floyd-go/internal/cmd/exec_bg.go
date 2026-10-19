package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/execution"
	"github.com/spf13/cobra"
)

type bgJobRecord struct {
	PID        int    `json:"pid"`
	Command    string `json:"command"`
	StartedAt  string `json:"started_at"`
	StdoutPath string `json:"stdout_path"`
	StderrPath string `json:"stderr_path"`
	Cwd        string `json:"cwd"`
	Shell      string `json:"shell"`
}

var execBgCmd = &cobra.Command{
	Use:   "bg",
	Short: "Background execution helpers",
}

var execBgStartCmd = &cobra.Command{
	Use:   "start <command>",
	Short: "Start a background command",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args, " ")
		debug, _ := cmd.Flags().GetBool("debug")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		cfgStore, err := config.Load(cwd, dataDir, debug)
		if err != nil {
			return err
		}
		cfg := cfgStore.Config()
		if err != nil {
			return err
		}

		// TODO(port-wave4): the chassis config has no options.execution
		// surface yet; flags are the only source for now.
		execInputs, err := resolveExecutionConfig(cmd, nil)
		if err != nil {
			return err
		}

		env := execution.NewEnvironment(execution.Config{
			Shell:           execInputs.Shell,
			DefaultTimeout:  execInputs.Timeout,
			MaxBufferSize:   execInputs.MaxBufferBytes,
			AllowedPrefixes: execInputs.AllowedPrefix,
			AllowedPatterns: execInputs.AllowedRegex,
			DeniedPrefixes:  execInputs.DeniedPrefix,
			DeniedPatterns:  execInputs.DeniedRegex,
		})
		if err := env.Initialize(context.Background()); err != nil {
			return err
		}
		if err := env.ValidateCommand(command); err != nil {
			return err
		}

		shell := execInputs.Shell
		if shell == "" {
			shell = runtimeDefaultShell()
		}

		bgDir := execBgDir(cfg.Options.DataDirectory)
		if err := os.MkdirAll(bgDir, 0o755); err != nil {
			return fmt.Errorf("failed to create background directory: %w", err)
		}

		stdoutFile, err := os.CreateTemp(bgDir, "stdout-*.log")
		if err != nil {
			return fmt.Errorf("failed to create stdout log: %w", err)
		}
		defer stdoutFile.Close()

		stderrFile, err := os.CreateTemp(bgDir, "stderr-*.log")
		if err != nil {
			return fmt.Errorf("failed to create stderr log: %w", err)
		}
		defer stderrFile.Close()

		cmdExec := exec.Command(shell, "-c", command) // #nosec G204
		cmdExec.Dir = cwd
		cmdExec.Stdout = stdoutFile
		cmdExec.Stderr = stderrFile
		cmdExec.Env = mergeEnv(os.Environ(), execInputs.Env)

		if err := cmdExec.Start(); err != nil {
			return err
		}

		record := bgJobRecord{
			PID:        cmdExec.Process.Pid,
			Command:    command,
			StartedAt:  time.Now().UTC().Format(time.RFC3339),
			StdoutPath: stdoutFile.Name(),
			StderrPath: stderrFile.Name(),
			Cwd:        cwd,
			Shell:      shell,
		}

		if err := saveBgJob(bgDir, record); err != nil {
			return err
		}

		cmd.Printf("Started job %d\n", record.PID)
		return nil
	},
}

var execBgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List background jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Flags().GetBool("debug")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		cfgStore, err := config.Load(cwd, dataDir, debug)
		if err != nil {
			return err
		}
		cfg := cfgStore.Config()
		if err != nil {
			return err
		}

		bgDir := execBgDir(cfg.Options.DataDirectory)
		jobs, err := loadBgJobs(bgDir)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			cmd.Println("No background jobs.")
			return nil
		}
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].StartedAt < jobs[j].StartedAt
		})
		for _, job := range jobs {
			status := "running"
			if !isProcessRunning(job.PID) {
				status = "finished"
			}
			cmd.Printf("%d\t%s\t%s\n", job.PID, status, job.Command)
		}
		return nil
	},
}

var execBgLogsCmd = &cobra.Command{
	Use:   "logs <pid>",
	Short: "Show background job logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Flags().GetBool("debug")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		cfgStore, err := config.Load(cwd, dataDir, debug)
		if err != nil {
			return err
		}
		cfg := cfgStore.Config()
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid pid: %s", args[0])
		}

		bgDir := execBgDir(cfg.Options.DataDirectory)
		job, ok, err := findBgJob(bgDir, pid)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("background job not found: %d", pid)
		}

		showStdout, _ := cmd.Flags().GetBool("stdout")
		showStderr, _ := cmd.Flags().GetBool("stderr")
		if !showStdout && !showStderr {
			showStdout = true
			showStderr = true
		}

		if showStdout {
			cmd.Println("--- stdout ---")
			data, _ := os.ReadFile(job.StdoutPath)
			cmd.Print(string(data))
			if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
				cmd.Println()
			}
		}
		if showStderr {
			cmd.Println("--- stderr ---")
			data, _ := os.ReadFile(job.StderrPath)
			cmd.Print(string(data))
			if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
				cmd.Println()
			}
		}
		return nil
	},
}

var execBgKillCmd = &cobra.Command{
	Use:   "kill <pid>",
	Short: "Terminate a background job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Flags().GetBool("debug")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		cfgStore, err := config.Load(cwd, dataDir, debug)
		if err != nil {
			return err
		}
		cfg := cfgStore.Config()
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid pid: %s", args[0])
		}

		bgDir := execBgDir(cfg.Options.DataDirectory)
		if err := killProcess(pid); err != nil {
			return err
		}
		return removeBgJob(bgDir, pid)
	},
}

func init() {
	execBgLogsCmd.Flags().Bool("stdout", true, "Show stdout")
	execBgLogsCmd.Flags().Bool("stderr", true, "Show stderr")

	execBgCmd.AddCommand(execBgStartCmd, execBgListCmd, execBgLogsCmd, execBgKillCmd)
	execCmd.AddCommand(execBgCmd)
}

func execBgDir(dataDir string) string {
	return filepath.Join(dataDir, "exec-bg")
}

func bgJobPath(dir string, pid int) string {
	return filepath.Join(dir, fmt.Sprintf("%d.json", pid))
}

func saveBgJob(dir string, job bgJobRecord) error {
	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bgJobPath(dir, job.PID), payload, 0o644)
}

func loadBgJobs(dir string) ([]bgJobRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	jobs := []bgJobRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job bgJobRecord
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func findBgJob(dir string, pid int) (bgJobRecord, bool, error) {
	path := bgJobPath(dir, pid)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bgJobRecord{}, false, nil
		}
		return bgJobRecord{}, false, err
	}
	var job bgJobRecord
	if err := json.Unmarshal(data, &job); err != nil {
		return bgJobRecord{}, false, err
	}
	return job, true, nil
}

func removeBgJob(dir string, pid int) error {
	path := bgJobPath(dir, pid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		err = process.Signal(syscall.Signal(0))
		return err == nil
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func killProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func mergeEnv(base []string, overrides map[string]string) []string {
	result := make(map[string]string, len(base)+len(overrides))
	for _, pair := range base {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			result[key] = value
		}
	}
	for key, value := range overrides {
		result[key] = value
	}
	out := make([]string, 0, len(result))
	for key, value := range result {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func runtimeDefaultShell() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "bash"
}
