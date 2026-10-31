package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/spf13/cobra"
)

type appWorkspace interface{ App() *app.App }

var (
	swarmCmd    = &cobra.Command{Use: "swarm", Short: "Dispatch tasks through named agent profiles", Long: "Run configured agent profiles in independent sessions, with their selected models and tool restrictions. Progress and cancellation work in both local and server mode."}
	swarmRunCmd = &cobra.Command{Use: "run <prompt>", Short: "Run a prompt across named agents and print their results", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(permission.WithNonInteractive(cmd.Context()), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if timeout > 0 {
			var stop context.CancelFunc
			ctx, stop = context.WithTimeout(ctx, timeout)
			defer stop()
		}
		cmd.SetContext(ctx)
		ws, cleanup, err := setupWorkspace(cmd)
		if err != nil {
			return err
		}
		defer cleanup()
		planMode, err := applyLaunchOptions(cmd, ws)
		if err != nil {
			return err
		}
		if err := ws.UpdateAgentModel(ctx); err != nil {
			return err
		}
		parent, err := ws.CreateSession(ctx, "Swarm: "+strings.Join(args, " "))
		if err != nil {
			return err
		}
		if planMode {
			if err := enterLaunchPlan(ctx, ws, parent.ID); err != nil {
				return err
			}
		}
		names, _ := cmd.Flags().GetStringSlice("agent")
		if len(names) == 0 {
			return fmt.Errorf("at least one agent is required")
		}
		active, parallel := true, len(names)
		if _, err := ws.SetSwarm(ctx, parent.ID, &active, &parallel); err != nil {
			return err
		}
		tasks := map[string]agent.RuntimeTask{}
		order := []string{}
		labels := map[string]string{}
		defer func() {
			for id, t := range tasks {
				if t.Status == "running" || t.Status == "queued" {
					stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = ws.Task(stopCtx, id, "cancel")
					stop()
				}
			}
		}()
		for _, name := range names {
			task, err := ws.StartAgentTask(ctx, parent.ID, strings.Join(args, " "), name)
			if err != nil {
				return err
			}
			tasks[task.ID] = task
			order = append(order, task.ID)
			labels[task.ID] = name
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s (%s)\n", name, task.Status, task.ID)
		}
		timer := time.NewTicker(150 * time.Millisecond)
		defer timer.Stop()
		for {
			pending := false
			for _, id := range order {
				prior := tasks[id]
				if prior.Status != "running" && prior.Status != "queued" {
					continue
				}
				next, err := ws.Task(ctx, id, "")
				if err != nil {
					return err
				}
				if next.Status != prior.Status {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", labels[id], next.Status)
				}
				tasks[id] = next
				if next.Status == "running" || next.Status == "queued" {
					pending = true
				}
			}
			if !pending {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
		var failures []string
		for _, id := range order {
			task := tasks[id]
			output := task.OutputPreview
			if task.Status == "completed" {
				messages, err := ws.ListMessages(ctx, id)
				if err != nil {
					return err
				}
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == message.Assistant {
						output = messages[i].Content().Text
						break
					}
				}
			} else {
				failures = append(failures, fmt.Sprintf("%s: %s", labels[id], task.Status))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "== %s (session %s; model %s; %s) ==\n%s\n", labels[id], id, task.Model, task.Status, strings.TrimRight(output, "\n"))
		}
		if len(failures) > 0 {
			return fmt.Errorf("swarm tasks failed: %s", strings.Join(failures, "; "))
		}
		return nil
	}}
)

func init() {
	swarmRunCmd.Flags().StringSlice("agent", []string{"coder"}, "Agent profile to dispatch to (repeatable)")
	swarmRunCmd.Flags().Duration("timeout", 0, "Cancel remaining tasks after this duration (for example 5m)")
	addRuntimeFlags(swarmRunCmd)
	swarmCmd.AddCommand(swarmRunCmd)
	rootCmd.AddCommand(swarmCmd)
}
