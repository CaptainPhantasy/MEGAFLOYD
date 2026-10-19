package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommandRegistersPortedSubcommands(t *testing.T) {
	t.Parallel()

	required := []string{
		"ai",
		"codebase",
		"doctor",
		"exec",
		"file",
		"prompt",
		"scoreboard",
		"resilience",
	}

	registered := map[string]*cobra.Command{}
	seen := map[string]bool{}
	for _, cmd := range rootCmd.Commands() {
		name := cmd.Name()
		if seen[name] {
			t.Errorf("command %q is registered more than once", name)
		}
		seen[name] = true
		registered[name] = cmd
	}

	for _, name := range required {
		if _, ok := registered[name]; !ok {
			t.Errorf("root command is missing %q", name)
		}
	}

	subcommands := []struct {
		path []string
	}{
		{[]string{"ai", "dry-run"}},
		{[]string{"codebase", "analyze"}},
		{[]string{"codebase", "deps"}},
		{[]string{"codebase", "search"}},
		{[]string{"exec", "bg"}},
		{[]string{"exec", "bg", "start"}},
		{[]string{"exec", "bg", "list"}},
		{[]string{"exec", "bg", "logs"}},
		{[]string{"exec", "bg", "kill"}},
		{[]string{"file", "read"}},
		{[]string{"file", "diff"}},
		{[]string{"prompt", "list"}},
		{[]string{"prompt", "render"}},
	}

	for _, tt := range subcommands {
		cmd, _, err := rootCmd.Find(tt.path)
		if err != nil {
			t.Errorf("Find(%v) error = %v", tt.path, err)
			continue
		}
		want := tt.path[len(tt.path)-1]
		if cmd.Name() != want {
			t.Errorf("Find(%v) = %q, want %q", tt.path, cmd.Name(), want)
		}
	}
}
