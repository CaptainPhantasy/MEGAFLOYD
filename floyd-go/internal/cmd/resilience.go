package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/spf13/cobra"
)

const (
	retryBudgetWindow   = time.Hour
	retryBudgetFailures = 6
	circuitWindow       = 10 * time.Minute
	circuitHits         = 2
)

type resilienceReport struct {
	Profile             string `json:"profile"`
	Binary              string `json:"binary"`
	DataDir             string `json:"data_dir"`
	QualityGates        bool   `json:"quality_gates"`
	DegradationControls bool   `json:"degradation_controls"`
	ConsistencyLock     bool   `json:"consistency_lock"`
	AutoStabilize       bool   `json:"auto_stabilize"`
	MaxParallelism      string `json:"max_parallelism"`
	RetryBudget         string `json:"retry_budget"`
	CircuitBreaker      string `json:"circuit_breaker"`
	ConsistencyCheck    string `json:"consistency_check"`
	ConsistencyDetail   string `json:"consistency_detail,omitempty"`
	FailuresLastHour    int    `json:"failures_last_hour"`
	LastFailureHash     string `json:"last_failure_hash,omitempty"`
	LastFailureClass    string `json:"last_failure_class,omitempty"`
	LastFailureMessage  string `json:"last_failure_message,omitempty"`
}

var resilienceCmd = &cobra.Command{
	Use:   "resilience",
	Short: "Show SuperFloyd resilience state",
	Long:  "Print the runtime lane, safety toggles, retry budget, circuit breaker, consistency lock, and recent failure state. Read-only.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")

		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cfgStore, err := config.Load(cwd, dataDir, false)
		if err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}

		report := collectResilienceReport(cfgStore, filepath.Base(os.Args[0]))
		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}
		printResilienceReport(cmd, report)
		return nil
	},
}

func init() {
	resilienceCmd.Flags().Bool("json", false, "Print the report as JSON")
}

func collectResilienceReport(cfgStore *config.ConfigStore, binary string) resilienceReport {
	cfg := cfgStore.Config()
	now := time.Now()

	report := resilienceReport{
		Profile:             string(resolveRuntimeProfile(binary)),
		Binary:              binary,
		DataDir:             cfg.Options.DataDirectory,
		QualityGates:        qualityGatesEnabled(),
		DegradationControls: degradationControlsEnabled(),
		ConsistencyLock:     consistencyLockEnabled(),
		AutoStabilize:       autoStabilizeEnabled(),
		MaxParallelism:      getMaxParallelism(),
	}

	health, _ := loadRuntimeHealth(cfg.Options.DataDirectory)
	recent := keepRecentFailures(health.Failures, now.Add(-retryBudgetWindow).Unix())
	report.FailuresLastHour = len(recent)
	if len(recent) >= retryBudgetFailures {
		report.RetryBudget = "exhausted"
	} else {
		report.RetryBudget = "available"
	}

	circuit := keepRecentFailures(health.Failures, now.Add(-circuitWindow).Unix())
	counts := map[string]int{}
	for _, failure := range circuit {
		counts[failure.Hash]++
	}
	report.CircuitBreaker = "clear"
	for _, hits := range counts {
		if hits >= circuitHits {
			report.CircuitBreaker = "engaged"
			break
		}
	}

	if len(health.Failures) > 0 {
		last := health.Failures[len(health.Failures)-1]
		report.LastFailureHash = last.Hash
		report.LastFailureClass = last.ExitClass
		report.LastFailureMessage = last.Message
	}

	if err := enforceConsistencyLock(cfgStore); err != nil {
		report.ConsistencyCheck = "FAIL"
		report.ConsistencyDetail = err.Error()
	} else {
		report.ConsistencyCheck = "PASS"
	}

	return report
}

func printResilienceReport(cmd *cobra.Command, report resilienceReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "╔══════════════════════════════════════════════════════╗")
	fmt.Fprintln(out, "║               SUPERFLOYD RESILIENCE                 ║")
	fmt.Fprintln(out, "╠══════════════════════════════════════════════════════╣")
	fmt.Fprintf(out, "║ Profile                 : %-27s ║\n", report.Profile)
	fmt.Fprintf(out, "║ Binary                  : %-27s ║\n", truncate(report.Binary, 27))
	fmt.Fprintf(out, "║ Runtime data dir        : %-27s ║\n", truncate(report.DataDir, 27))
	fmt.Fprintf(out, "║ Quality gates           : %-27s ║\n", yesNo(report.QualityGates))
	fmt.Fprintf(out, "║ Degradation controls    : %-27s ║\n", yesNo(report.DegradationControls))
	fmt.Fprintf(out, "║ Consistency lock        : %-27s ║\n", yesNo(report.ConsistencyLock))
	fmt.Fprintf(out, "║ Auto-stabilize          : %-27s ║\n", yesNo(report.AutoStabilize))
	fmt.Fprintf(out, "║ Max parallelism         : %-27s ║\n", truncate(report.MaxParallelism, 27))
	fmt.Fprintf(out, "║ Retry budget            : %-27s ║\n", report.RetryBudget)
	fmt.Fprintf(out, "║ Circuit breaker         : %-27s ║\n", report.CircuitBreaker)
	fmt.Fprintf(out, "║ Failures (1h)           : %-27d ║\n", report.FailuresLastHour)
	fmt.Fprintf(out, "║ Consistency check       : %-27s ║\n", report.ConsistencyCheck)
	if report.ConsistencyDetail != "" {
		fmt.Fprintf(out, "║ Consistency detail      : %-27s ║\n", truncate(report.ConsistencyDetail, 27))
	}
	if report.LastFailureHash != "" {
		fmt.Fprintf(out, "║ Last failure hash       : %-27s ║\n", truncate(report.LastFailureHash, 27))
		fmt.Fprintf(out, "║ Last failure class      : %-27s ║\n", truncate(report.LastFailureClass, 27))
		fmt.Fprintf(out, "║ Last failure message    : %-27s ║\n", truncate(report.LastFailureMessage, 27))
	}
	fmt.Fprintln(out, "╚══════════════════════════════════════════════════════╝")
}
