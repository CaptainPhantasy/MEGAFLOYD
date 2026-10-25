package cmd

import (
	"bytes"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestCanonicalNestedHelpUsesOwnCommand(t *testing.T) {
	root := &cobra.Command{Use: "floyd", Short: rootCmd.Short, Version: "2026.10.24"}
	child := &cobra.Command{Use: "run", Short: runCmd.Short, Example: runCmd.Example}
	nested := &cobra.Command{Use: "status", Example: "floyd auth status"}
	root.AddCommand(child)
	child.AddCommand(nested)
	applyCLIName(root, megafloydBinaryName)
	var out bytes.Buffer
	child.SetOut(&out)
	if err := child.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "floyd run") && !strings.Contains(out.String(), "megafloyd run") {
		t.Fatalf("legacy executable in help: %s", out.String())
	}
	for _, cmd := range []*cobra.Command{child, nested} {
		if strings.Contains(cmd.Example, "\nfloyd ") || strings.HasPrefix(cmd.Example, "floyd ") {
			t.Fatalf("legacy executable in child: %s", cmd.Example)
		}
	}
	if !strings.Contains(root.Short, "MegaFloyd") {
		t.Fatal("help omits product name")
	}
}
