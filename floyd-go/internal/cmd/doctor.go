package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show SuperFloyd runtime health and resilience state",
	Long:  "Displays quality gates, degradation controls, consistency lock, auto-stabilize status, and recent failure/circuit state.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cfgStore, err := config.Load(cwd, dataDir, false)
		if err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		cfg := cfgStore.Config()

		isSF := isSuperFloydBinary()
		st, _ := loadRuntimeHealth(cfg.Options.DataDirectory)
		stabilizeNow := shouldStabilizeFromBenchmarks(cmd.Context(), cfgStore)
		consistencyErr := enforceConsistencyLock(cfgStore)

		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║                     SUPERFLOYD DOCTOR                       ║")
		fmt.Println("╠══════════════════════════════════════════════════════════════╣")
		fmt.Printf("║ Binary lane                : %-31s ║\n", yesNo(isSF))
		fmt.Printf("║ Active mode                : %-31s ║\n", truncate(strings.ToUpper(cmd.Root().Use), 31))
		fmt.Printf("║ Max parallelism            : %-31s ║\n", getMaxParallelism())
		fmt.Printf("║ Quality gates enabled      : %-31s ║\n", yesNo(qualityGatesEnabled()))
		fmt.Printf("║ Degradation controls       : %-31s ║\n", yesNo(degradationControlsEnabled()))
		fmt.Printf("║ Consistency lock enabled   : %-31s ║\n", yesNo(consistencyLockEnabled()))
		fmt.Printf("║ Auto-stabilize enabled     : %-31s ║\n", yesNo(autoStabilizeEnabled()))
		fmt.Printf("║ Auto-stabilize active now  : %-31s ║\n", yesNo(stabilizeNow))
		fmt.Printf("║ Runtime data dir           : %-31s ║\n", truncate(cfg.Options.DataDirectory, 31))
		fmt.Printf("║ Failure records (1h)       : %-31d ║\n", len(st.Failures))
		fmt.Println("╠══════════════════════════════════════════════════════════════╣")
		if consistencyErr == nil {
			fmt.Printf("║ Consistency check          : %-31s ║\n", "PASS")
		} else {
			fmt.Printf("║ Consistency check          : %-31s ║\n", "FAIL")
			fmt.Printf("║ Consistency detail         : %-31s ║\n", truncate(consistencyErr.Error(), 31))
		}

		if len(st.Failures) > 0 {
			last := st.Failures[len(st.Failures)-1]
			fmt.Printf("║ Last failure hash          : %-31s ║\n", truncate(last.Hash, 31))
			fmt.Printf("║ Last failure class         : %-31s ║\n", truncate(last.ExitClass, 31))
			fmt.Printf("║ Last failure message       : %-31s ║\n", truncate(last.Message, 31))
		}
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")

		if !isSF {
			fmt.Println("Note: running outside superfloyd lane; safeguards may be inactive by design.")
		}
		fmt.Println("Env toggles: SUPERFLOYD_QUALITY_GATES, SUPERFLOYD_DEGRADATION_CONTROLS, SUPERFLOYD_CONSISTENCY_LOCK, SUPERFLOYD_AUTOSTABILIZE")
		return nil
	},
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func getMaxParallelism() string {
	v := os.Getenv("SUPERFLOYD_MAX_PARALLEL")
	if v == "" {
		return "default (1)"
	}
	return v
}
