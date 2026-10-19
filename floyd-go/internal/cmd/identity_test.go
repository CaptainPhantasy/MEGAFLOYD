package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveCLIName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exePath  string
		override string
		want     string
	}{
		{"megafloyd absolute", "/usr/local/bin/megafloyd", "", megafloydBinaryName},
		{"megafloyd bare", "megafloyd", "", megafloydBinaryName},
		{"megafloyd mixed case", "/opt/MegaFloyd", "", megafloydBinaryName},
		{"megafloyd windows suffix", "/opt/megafloyd.exe", "", megafloydBinaryName},
		{"floyd absolute", "/usr/local/bin/floyd", "", floydBinaryName},
		{"floyd bare", "floyd", "", floydBinaryName},
		{"custom path", "/opt/tools/ff", "", floydBinaryName},
		{"empty path", "", "", floydBinaryName},
		{"override on floyd", "/usr/local/bin/floyd", "megafloyd", megafloydBinaryName},
		{"override arbitrary value", "/usr/local/bin/floyd", "1", megafloydBinaryName},
		{"whitespace override ignored", "/usr/local/bin/floyd", "  ", floydBinaryName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveCLIName(tt.exePath, tt.override); got != tt.want {
				t.Fatalf("resolveCLIName(%q, %q) = %q, want %q", tt.exePath, tt.override, got, tt.want)
			}
		})
	}
}

func TestApplyCLIName(t *testing.T) {
	t.Parallel()

	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{
			Use: "floyd [command]",
			Example: `
floyd

floyd run "hi"

cat README.md | floyd run "hi"

floyd --data-dir /path/to/custom/.floyd
`,
		}
		cmd.PersistentFlags().StringP("data-dir", "D", "", "Custom floyd data directory")
		cmd.PersistentFlags().StringP("host", "H", "", "Connect to a specific floyd server host (for advanced users)")
		return cmd
	}

	t.Run("megafloyd", func(t *testing.T) {
		t.Parallel()
		cmd := newCommand()
		applyCLIName(cmd, megafloydBinaryName)

		if cmd.Use != "megafloyd [command]" {
			t.Fatalf("Use = %q, want %q", cmd.Use, "megafloyd [command]")
		}
		if got := cmd.PersistentFlags().Lookup("data-dir").Usage; got != "Custom megafloyd data directory" {
			t.Fatalf("data-dir usage = %q", got)
		}
		if got := cmd.PersistentFlags().Lookup("host").Usage; got != "Connect to a specific megafloyd server host (for advanced users)" {
			t.Fatalf("host usage = %q", got)
		}
		for _, want := range []string{
			"\nmegafloyd\n",
			"\nmegafloyd run \"hi\"\n",
			"| megafloyd run \"hi\"\n",
			"\nmegafloyd --data-dir /path/to/custom/.floyd\n",
		} {
			if !strings.Contains(cmd.Example, want) {
				t.Fatalf("example does not contain %q:\n%s", want, cmd.Example)
			}
		}
		if strings.Contains(cmd.Example, "\nfloyd") || strings.Contains(cmd.Example, "| floyd") {
			t.Fatalf("example still names floyd:\n%s", cmd.Example)
		}
	})

	t.Run("floyd unchanged", func(t *testing.T) {
		t.Parallel()
		cmd := newCommand()
		beforeUse := cmd.Use
		beforeExample := cmd.Example
		beforeDataDir := cmd.PersistentFlags().Lookup("data-dir").Usage

		applyCLIName(cmd, floydBinaryName)

		if cmd.Use != beforeUse || cmd.Example != beforeExample {
			t.Fatalf("floyd identity changed: Use=%q Example=%q", cmd.Use, cmd.Example)
		}
		if got := cmd.PersistentFlags().Lookup("data-dir").Usage; got != beforeDataDir {
			t.Fatalf("floyd data-dir usage changed: %q", got)
		}
	})
}

func TestVersionLineUsesCLIName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cliName string
		want    string
	}{
		{"megafloyd", megafloydBinaryName, "megafloyd version 1.2.3\n"},
		{"floyd", floydBinaryName, "floyd version 1.2.3\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{
				Use:     "floyd",
				Version: "1.2.3",
				RunE:    func(*cobra.Command, []string) error { return nil },
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--version"})
			applyCLIName(cmd, tt.cliName)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("version output = %q, want %q", got, tt.want)
			}
		})
	}
}
