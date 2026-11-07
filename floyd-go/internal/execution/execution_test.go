package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecuteUsesWorkspaceEnvironmentAndExitStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEnvironment(Config{Shell: "/bin/sh"})
	e.SetWorkingDirectory(root)
	e.SetEnvironmentVariable("MEGAFLOYD_EXECUTION_TEST", "base")
	result, err := e.Execute(t.Context(), `printf '%s' "$MEGAFLOYD_EXECUTION_TEST"; printf 'diagnostic' >&2; exit 7`, Options{Env: map[string]string{"MEGAFLOYD_EXECUTION_TEST": "override"}, CaptureStderr: true})
	require.NoError(t, err)
	require.Equal(t, "overridediagnostic", result.Output)
	require.Equal(t, 7, result.ExitCode)
	require.Error(t, result.Error)
	result, err = e.Execute(t.Context(), `printf 'fixture' > fixture.txt`, Options{})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)
	content, err := os.ReadFile(filepath.Join(root, "fixture.txt"))
	require.NoError(t, err)
	require.Equal(t, "fixture", string(content))
	limited, err := e.Execute(t.Context(), `printf abcdef`, Options{MaxBufferSize: 3})
	require.NoError(t, err)
	require.Equal(t, "abc", limited.Output)
}

func TestExecuteRejectsDeniedCommandBeforeSideEffects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEnvironment(Config{Shell: "/bin/sh", DeniedPrefixes: []string{"printf"}})
	_, err := e.Execute(t.Context(), `printf forbidden > marker`, Options{Cwd: root})
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(root, "marker"))
}

func TestBackgroundCompletionIncludesAllOutput(t *testing.T) {
	t.Parallel()
	e := NewEnvironment(Config{Shell: "/bin/sh"})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	exited := make(chan int, 1)
	job, err := e.StartBackground(ctx, `printf stdout; printf stderr >&2; exit 9`, BackgroundOptions{Options: Options{Cwd: t.TempDir()}, OnExit: func(code int) { exited <- code }})
	require.NoError(t, err)
	select {
	case code := <-exited:
		require.Equal(t, 9, code)
	case <-ctx.Done():
		t.Fatal("background command did not complete")
	}
	require.False(t, job.IsRunning())
	stdout, stderr, found := e.BackgroundOutput(job.PID)
	require.True(t, found)
	require.Equal(t, "stdout", stdout)
	require.Equal(t, "stderr", stderr)
	jobs := e.ListBackground()
	require.Len(t, jobs, 1)
	require.NotNil(t, jobs[0].FinishedAt)
	require.Equal(t, 9, *jobs[0].ExitCode)
}

func TestKillAllReturnsAndStopsBackgroundJobs(t *testing.T) {
	t.Parallel()
	e := NewEnvironment(Config{Shell: "/bin/sh"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	exited := make(chan int, 1)
	job, err := e.StartBackground(ctx, `exec sleep 10`, BackgroundOptions{Options: Options{Cwd: t.TempDir()}, OnExit: func(code int) { exited <- code }})
	require.NoError(t, err)
	require.True(t, job.IsRunning())
	done := make(chan struct{})
	go func() { e.KillAll(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("KillAll deadlocked while stopping a tracked background job")
	}
	select {
	case code := <-exited:
		require.NotZero(t, code)
	case <-time.After(time.Second):
		t.Fatal("background process did not exit")
	}
	require.False(t, job.IsRunning())
	require.Empty(t, e.ListBackground())
}

func TestBackgroundExitWaitsForOutputDelivery(t *testing.T) {
	t.Parallel()
	e := NewEnvironment(Config{Shell: "/bin/sh"})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	outputStarted := make(chan struct{})
	releaseOutput := make(chan struct{})
	exited := make(chan int, 1)
	_, err := e.ExecuteBackground(ctx, `printf complete-output`, BackgroundOptions{
		Options: Options{Cwd: t.TempDir()},
		OnOutput: func(output string) {
			if strings.Contains(output, "complete-output") {
				close(outputStarted)
			}
			<-releaseOutput
		},
		OnExit: func(code int) { exited <- code },
	})
	require.NoError(t, err)
	select {
	case <-outputStarted:
	case <-ctx.Done():
		t.Fatal("background output did not arrive")
	}
	select {
	case <-exited:
		close(releaseOutput)
		t.Fatal("exit was reported before output delivery completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseOutput)
	select {
	case code := <-exited:
		require.Zero(t, code)
	case <-ctx.Done():
		t.Fatal("background completion did not arrive")
	}
}
