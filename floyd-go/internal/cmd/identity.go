package cmd

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const (
	floydBinaryName     = "floyd"
	megafloydBinaryName = "megafloyd"
)

// binaryNameOverride holds the MEGAFLOYD_BINARY_NAME value. main.go reads
// the environment so this package stays free of MEGAFLOYD_* lookups; see
// docs/megafloyd-namespace.md.
var binaryNameOverride string

// SetBinaryNameOverride records the explicit binary-name override. A
// non-empty value selects the megafloyd identity even when the executable
// itself is named floyd (a renamed copy, a shim, a test harness).
func SetBinaryNameOverride(name string) {
	binaryNameOverride = name
}

// resolveCLIName returns the identity the CLI presents in usage and version
// output. An explicit override wins; otherwise the executable base name
// selects megafloyd, and every other name keeps the floyd identity.
func resolveCLIName(exePath, override string) string {
	if strings.TrimSpace(override) != "" {
		return megafloydBinaryName
	}
	base := filepath.Base(exePath)
	base = strings.TrimSuffix(base, ".exe")
	if strings.EqualFold(base, megafloydBinaryName) {
		return megafloydBinaryName
	}
	return floydBinaryName
}

// exampleCommandLine matches a floyd command at the start of an example
// exampleCommandLine matches a floyd command at the start of an example
// line or after a pipe, so piped examples keep their prompt. It
// deliberately leaves paths such as /path/to/custom/.floyd alone.
var exampleCommandLine = regexp.MustCompile(`(?m)(^|[|;][ \t]+)floyd([ \t]|$)`)

// applyCLIName rewrites the presentation of cmd for name. The floyd
// identity is left untouched, so behavior under that name is unchanged.
func applyCLIName(cmd *cobra.Command, name string) {
	if cmd == nil || name == "" || name == floydBinaryName {
		return
	}
	if cmd.Name() == floydBinaryName {
		cmd.Use = strings.Replace(cmd.Use, floydBinaryName, name, 1)
	}
	cmd.Example = exampleCommandLine.ReplaceAllStringFunc(cmd.Example, func(line string) string {
		return strings.Replace(line, floydBinaryName, name, 1)
	})
	cmd.SetVersionTemplate(defaultVersionTemplate)
	renameFloydFlagUsage(cmd, name)
	for _, child := range cmd.Commands() {
		applyCLIName(child, name)
	}
}

// renameFloydFlagUsage updates the two persistent flag descriptions that
// name the product. Other flags are left alone.
func renameFloydFlagUsage(cmd *cobra.Command, name string) {
	updates := map[string][2]string{
		"data-dir": {
			"Custom floyd data directory",
			"Custom MegaFloyd data directory",
		},
		"host": {
			"Connect to a specific floyd server host (for advanced users)",
			"Connect to a specific MegaFloyd server host (for advanced users)",
		},
	}
	for flag, texts := range updates {
		if f := cmd.PersistentFlags().Lookup(flag); f != nil && f.Usage == texts[0] {
			f.Usage = texts[1]
		}
	}
}
