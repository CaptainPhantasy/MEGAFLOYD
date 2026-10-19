package cmd

import (
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/spf13/cobra"
)

// appWorkspace is the local-mode workspace accessor that exposes the
// in-process app (and through it, the agent coordinator).
type appWorkspace interface {
	App() *app.App
}

var swarmCmd = &cobra.Command{
	Use:   "swarm",
	Short: "Dispatch tasks across agents through the swarm coordinator",
	Long: `Submits prompts to the swarm coordinator, which routes each task to a
named agent, runs it in its own session, and collects the results.

The chassis dispatches every configured agent through a single
coordinator today, so distinct agent names fan out to concurrent
sessions on that coordinator.`,
}

var swarmRunCmd = &cobra.Command{
	Use:   "run <prompt>",
	Short: "Submit a prompt to one or more agents and print results",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if useClientServer() {
			return fmt.Errorf("swarm dispatch runs in local mode only; unset FLOYD_CLIENT_SERVER")
		}

		ws, cleanup, err := setupLocalWorkspace(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		aw, ok := ws.(appWorkspace)
		if !ok {
			return fmt.Errorf("swarm dispatch requires a local workspace")
		}
		application := aw.App()
		if application.AgentCoordinator == nil {
			return fmt.Errorf("agent not initialized - configure a provider first")
		}
		_ = application.AgentCoordinator.UpdateModels(cmd.Context())

		swarm := agent.NewSwarmCoordinator(application.AgentCoordinator)
		for name := range application.Config().Agents {
			swarm.RegisterAgent(name, application.AgentCoordinator)
		}

		agentNames, _ := cmd.Flags().GetStringSlice("agent")
		prompt := strings.Join(args, " ")

		type swarmTask struct {
			id   string
			name string
		}
		tasks := make([]swarmTask, 0, len(agentNames))
		for _, name := range agentNames {
			tasks = append(tasks, swarmTask{id: swarm.Submit(prompt, name), name: name})
		}

		var failures []string
		for _, t := range tasks {
			res, err := swarm.Await(t.id)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", t.name, err))
				continue
			}
			fmt.Printf("== %s (session %s) ==\n%s\n", res.AgentName, res.SessionID, strings.TrimRight(res.Output, "\n"))
		}
		if len(failures) > 0 {
			return fmt.Errorf("swarm tasks failed: %s", strings.Join(failures, "; "))
		}
		return nil
	},
}

func init() {
	swarmRunCmd.Flags().StringSlice("agent", []string{"coder"}, "Agent name to dispatch to (repeatable)")
	swarmCmd.AddCommand(swarmRunCmd)
	rootCmd.AddCommand(swarmCmd)
}
