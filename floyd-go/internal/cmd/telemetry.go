package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/legacy-ai/floyd/internal/enginebridge"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/usage"
)

var (
	telemetryOutDir     string
	telemetryJSON       bool
	telemetryEngineHome string
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Collect local usage data and report on it",
	Long: "Turn what sessions actually did into one local event log: prompts, replies, " +
		"tool calls, tool results, error classes, retries and repeated asks. " +
		"Nothing leaves this machine.",
}

var telemetryCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect events from CLI and desktop sessions",
	RunE:  runTelemetryCollect,
}

var telemetryReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Summarize the collected event log",
	RunE:  runTelemetryReport,
}

var telemetryShipCmd = &cobra.Command{
	Use:   "ship",
	Short: "Send the event log to the local Jaeger instance",
	RunE:  runTelemetryShip,
}

var (
	telemetryEndpoint    string
	telemetryAllowRemote bool
)

func init() {
	telemetryCollectCmd.Flags().StringVar(&telemetryOutDir, "out", "", "directory for events.jsonl (default: <megafloyd-home>/telemetry)")
	telemetryCollectCmd.Flags().StringVar(&telemetryEngineHome, "engine-home", "", "engine home to scan (default: discovered)")
	telemetryCollectCmd.Flags().BoolVar(&telemetryJSON, "json", false, "output in JSON format")
	telemetryReportCmd.Flags().StringVar(&telemetryOutDir, "out", "", "directory holding events.jsonl (default: <megafloyd-home>/telemetry)")
	telemetryReportCmd.Flags().BoolVar(&telemetryJSON, "json", false, "output in JSON format")
	telemetryShipCmd.Flags().StringVar(&telemetryOutDir, "out", "", "directory holding events.jsonl (default: <megafloyd-home>/telemetry)")
	telemetryShipCmd.Flags().StringVar(&telemetryEndpoint, "endpoint", "", "OTLP/HTTP endpoint (default: http://127.0.0.1:4318/v1/traces)")
	telemetryShipCmd.Flags().BoolVar(&telemetryAllowRemote, "allow-remote", false, "allow shipping to a non-local endpoint")
	telemetryCmd.AddCommand(telemetryCollectCmd, telemetryReportCmd, telemetryShipCmd)
}

func telemetryDir() (string, error) {
	if telemetryOutDir != "" {
		return telemetryOutDir, nil
	}
	home, err := megafloyd.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "telemetry"), nil
}

func runTelemetryCollect(cmd *cobra.Command, _ []string) error {
	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	dir, err := telemetryDir()
	if err != nil {
		return err
	}

	sessions, err := svc.sessions.List(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	events := make([]usage.Event, 0, len(sessions)*32)
	for _, item := range sessions {
		messages, err := svc.messages.List(ctx, item.ID)
		if err != nil {
			continue
		}
		events = append(events, usage.FromCLI(item.ID, messages)...)
	}

	engineHome := telemetryEngineHome
	if engineHome == "" {
		engineHome = enginebridge.Home()
	}
	engineEvents, err := usage.FromEngine(engineHome)
	if err != nil {
		return err
	}
	events = append(events, engineEvents...)

	eventsPath := filepath.Join(dir, "events.jsonl")
	written, err := usage.Write(eventsPath, events)
	if err != nil {
		return err
	}
	all, err := usage.Load(eventsPath)
	if err != nil {
		return err
	}
	summary := usage.Summarize(all)
	if err := usage.WriteSummary(filepath.Join(dir, "summary.json"), summary); err != nil {
		return err
	}

	if telemetryJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]any{
			"dir":       dir,
			"collected": len(events),
			"written":   written,
			"summary":   summary,
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Collected %d events (%d new) into %s\n\n%s", len(events), written, eventsPath, summary.Text())
	return nil
}

func runTelemetryReport(cmd *cobra.Command, _ []string) error {
	dir, err := telemetryDir()
	if err != nil {
		return err
	}
	events, err := usage.Load(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return fmt.Errorf("read events (run `megafloyd telemetry collect` first): %w", err)
	}
	summary := usage.Summarize(events)
	if telemetryJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		return enc.Encode(summary)
	}
	fmt.Fprint(cmd.OutOrStdout(), summary.Text())
	return nil
}

func runTelemetryShip(cmd *cobra.Command, _ []string) error {
	dir, err := telemetryDir()
	if err != nil {
		return err
	}
	events, err := usage.Load(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return fmt.Errorf("read events (run `megafloyd telemetry collect` first): %w", err)
	}
	sessions, err := usage.Ship(telemetryEndpoint, events, telemetryAllowRemote)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Shipped %d events in %d session traces\n", len(events), sessions)
	return nil
}
