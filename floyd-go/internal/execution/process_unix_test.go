//go:build !windows

package execution

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackgroundCancellationStopsShellAndChild(t *testing.T) {
	for _, mode := range []string{"kill", "kill-all", "cancel-context"} {
		t.Run(mode, func(t *testing.T) {
			e := NewEnvironment(Config{Shell: "/bin/sh"})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan int, 1)
			exited := make(chan int, 1)
			job, err := e.StartBackground(ctx, `sleep 10 & child=$!; printf '%s\n' "$child"; wait "$child"`, BackgroundOptions{
				Options: Options{Cwd: t.TempDir()},
				OnOutput: func(output string) {
					if pid, err := strconv.Atoi(strings.TrimSpace(output)); err == nil {
						ready <- pid
					}
				},
				OnExit: func(code int) { exited <- code },
			})
			require.NoError(t, err)
			var childPID int
			select {
			case childPID = <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("child readiness was not received")
			}
			child, err := os.FindProcess(childPID)
			require.NoError(t, err)
			// On a failing baseline, stop only this fixture's known child as well.
			defer child.Kill()
			require.NoError(t, syscall.Kill(childPID, 0), "child should exist before cancellation")
			switch mode {
			case "kill":
				require.NoError(t, job.Kill())
			case "kill-all":
				e.KillAll()
			case "cancel-context":
				cancel()
			}
			select {
			case code := <-exited:
				require.NotZero(t, code)
			case <-time.After(time.Second):
				t.Fatal("shell cancellation left child alive holding output pipes")
			}
			require.False(t, job.IsRunning())
			require.Eventually(t, func() bool { return syscall.Kill(childPID, 0) == syscall.ESRCH }, 2*time.Second, 10*time.Millisecond, "background child must terminate after cancellation")
		})
	}
}

func TestForegroundTimeoutStopsShellChildren(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEnvironment(Config{Shell: "/bin/sh"})
	started := time.Now()
	result, err := e.Execute(t.Context(), `sleep 10 & child=$!; printf '%s\n' "$child"; wait "$child"`, Options{Cwd: root, Timeout: 200 * time.Millisecond})
	require.NoError(t, err)
	require.Error(t, result.Error)
	require.NotZero(t, result.ExitCode)
	require.Less(t, time.Since(started), 2*time.Second, "timeout must not wait for an orphaned child")
	childPID, err := strconv.Atoi(strings.TrimSpace(result.Output))
	require.NoError(t, err, "child readiness must be captured before the timeout")
	require.Eventually(t, func() bool { return syscall.Kill(childPID, 0) == syscall.ESRCH }, 2*time.Second, 10*time.Millisecond, "timed-out child must terminate")
}
