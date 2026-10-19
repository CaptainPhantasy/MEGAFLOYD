package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/spf13/cobra"
)

var scoreboardCmd = &cobra.Command{
	Use:   "scoreboard",
	Short: "Show SuperFloyd benchmark scoreboard",
	Long:  "Print a compact benchmark scoreboard from local Floyd session data.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		ctx := cmd.Context()

		if dataDir == "" {
			cfgStore, err := config.Load("", "", false)
			if err != nil {
				return err
			}
			cfg := cfgStore.Config()
			if err != nil {
				return fmt.Errorf("failed to initialize config: %w", err)
			}
			dataDir = cfg.Options.DataDirectory
		}

		conn, err := db.Connect(ctx, dataDir)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer conn.Close()

		stats, err := gatherStats(ctx, conn)
		if err != nil {
			return fmt.Errorf("failed to gather stats: %w", err)
		}
		if stats.Total.TotalSessions == 0 {
			return fmt.Errorf("no data available: no sessions found in database")
		}

		topTools := make([]ToolUsage, 0, len(stats.ToolUsage))
		topTools = append(topTools, stats.ToolUsage...)
		sort.Slice(topTools, func(i, j int) bool { return topTools[i].CallCount > topTools[j].CallCount })
		if len(topTools) > 5 {
			topTools = topTools[:5]
		}

		avgLatencySec := stats.AvgResponseTimeMs / 1000.0
		fmt.Println("╔══════════════════════════════════════════════════════╗")
		fmt.Println("║               SUPERFLOYD SCOREBOARD                 ║")
		fmt.Println("╠══════════════════════════════════════════════════════╣")
		fmt.Printf("║ Sessions                : %-27d ║\n", stats.Total.TotalSessions)
		fmt.Printf("║ Messages                : %-27d ║\n", stats.Total.TotalMessages)
		fmt.Printf("║ Avg tokens / session    : %-27.1f ║\n", stats.Total.AvgTokensPerSession)
		fmt.Printf("║ Avg messages / session  : %-27.2f ║\n", stats.Total.AvgMessagesPerSession)
		fmt.Printf("║ Avg response latency    : %-27.2fs ║\n", avgLatencySec)
		fmt.Printf("║ Total token volume      : %-27d ║\n", stats.Total.TotalTokens)
		fmt.Printf("║ Estimated cost          : $%-26.4f ║\n", stats.Total.TotalCost)
		fmt.Printf("║ Retry signals           : %-27s ║\n", "n/a (schema v1)")
		fmt.Printf("║ Regression signals      : %-27s ║\n", "n/a (schema v1)")
		fmt.Println("╠══════════════════════════════════════════════════════╣")
		fmt.Println("║ Top Tools                                            ║")
		if len(topTools) == 0 {
			fmt.Printf("║ %-52s ║\n", "(no tool calls recorded)")
		} else {
			for _, t := range topTools {
				label := fmt.Sprintf("%s (%d)", t.ToolName, t.CallCount)
				if len(label) > 52 {
					label = label[:52]
				}
				fmt.Printf("║ %-52s ║\n", label)
			}
		}
		fmt.Println("╚══════════════════════════════════════════════════════╝")

		if len(topTools) > 0 {
			names := make([]string, 0, len(topTools))
			for _, t := range topTools {
				names = append(names, t.ToolName)
			}
			fmt.Printf("Top tool set: %s\n", strings.Join(names, ", "))
		}

		return nil
	},
}
