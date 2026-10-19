package cmd

import (
	"os"
	"sort"
	"strings"

	"github.com/legacy-ai/floyd/internal/codebase"
	"github.com/spf13/cobra"
)

var codebaseCmd = &cobra.Command{
	Use:   "codebase",
	Short: "Analyze a codebase for structure and dependencies",
}

var codebaseAnalyzeCmd = &cobra.Command{
	Use:   "analyze [dir]",
	Short: "Analyze a codebase and print summary",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := getArgOrCwd(args)
		maxFiles, _ := cmd.Flags().GetInt("max-files")
		maxSize, _ := cmd.Flags().GetInt64("max-size")
		ignore, _ := cmd.Flags().GetStringSlice("ignore")

		result, err := codebase.AnalyzeCodebase(dir, codebase.AnalyzeOptions{
			IgnorePatterns: ignore,
			MaxFiles:       maxFiles,
			MaxSizePerFile: maxSize,
		})
		if err != nil {
			return err
		}

		cmd.Printf("Root: %s\n", result.Root)
		cmd.Printf("Total files: %d\n", result.TotalFiles)
		cmd.Printf("Total lines: %d\n", result.TotalLinesOfCode)
		cmd.Printf("Languages: %d\n", len(result.FilesByLanguage))
		cmd.Printf("Directories: %d\n", len(result.Directories))

		languages := make([]string, 0, len(result.FilesByLanguage))
		for lang := range result.FilesByLanguage {
			languages = append(languages, lang)
		}
		sort.Strings(languages)
		for _, lang := range languages {
			cmd.Printf("  %s: %d\n", lang, result.FilesByLanguage[lang])
		}

		if len(result.Directories) > 0 {
			type dirSummary struct {
				Path  string
				Count int
			}
			summaries := make([]dirSummary, 0, len(result.Directories))
			for dir, files := range result.Directories {
				summaries = append(summaries, dirSummary{Path: dir, Count: len(files)})
			}
			sort.Slice(summaries, func(i, j int) bool {
				if summaries[i].Count == summaries[j].Count {
					return summaries[i].Path < summaries[j].Path
				}
				return summaries[i].Count > summaries[j].Count
			})
			cmd.Println("Top directories:")
			limit := 5
			if len(summaries) < limit {
				limit = len(summaries)
			}
			for i := 0; i < limit; i++ {
				cmd.Printf("  %s: %d\n", summaries[i].Path, summaries[i].Count)
			}
		}

		uniqueDeps := map[string]struct{}{}
		externalDeps := 0
		depsByType := map[string]int{}
		for _, dep := range result.Dependencies {
			key := dep.Type + ":" + dep.Name
			uniqueDeps[key] = struct{}{}
			if dep.IsExternal {
				externalDeps++
			}
			if dep.Type != "" {
				depsByType[dep.Type]++
			} else {
				depsByType["unknown"]++
			}
		}
		cmd.Printf("Dependencies found: %d (unique: %d, external: %d)\n", len(result.Dependencies), len(uniqueDeps), externalDeps)
		if len(depsByType) > 0 {
			keys := make([]string, 0, len(depsByType))
			for key := range depsByType {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				cmd.Printf("  %s: %d\n", key, depsByType[key])
			}
		}
		return nil
	},
}

var codebaseDepsCmd = &cobra.Command{
	Use:   "deps [dir]",
	Short: "Analyze project dependency manifests",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := getArgOrCwd(args)
		deps, err := codebase.AnalyzeProjectDependencies(dir)
		if err != nil {
			return err
		}
		if len(deps) == 0 {
			cmd.Println("No dependencies found.")
			return nil
		}
		keys := make([]string, 0, len(deps))
		for k := range deps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			cmd.Printf("%s: %s\n", k, deps[k])
		}
		return nil
	},
}

var codebaseSearchCmd = &cobra.Command{
	Use:   "search <term> [dir]",
	Short: "Search for content within a codebase",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		term := args[0]
		dir := getArgOrCwd(args[1:])

		caseSensitive, _ := cmd.Flags().GetBool("case-sensitive")
		extensions, _ := cmd.Flags().GetStringSlice("ext")
		maxResults, _ := cmd.Flags().GetInt("max-results")
		ignore, _ := cmd.Flags().GetStringSlice("ignore")

		matches, err := codebase.FindFilesByContent(dir, term, codebase.SearchOptions{
			CaseSensitive:  caseSensitive,
			FileExtensions: normalizeExtensions(extensions),
			MaxResults:     maxResults,
			IgnorePatterns: ignore,
		})
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			cmd.Println("No matches found.")
			return nil
		}
		for _, match := range matches {
			cmd.Printf("%s:%d %s\n", match.Path, match.Line, match.Content)
		}
		return nil
	},
}

func init() {
	codebaseAnalyzeCmd.Flags().Int("max-files", 1000, "Maximum number of files to analyze")
	codebaseAnalyzeCmd.Flags().Int64("max-size", 1024*1024, "Maximum file size in bytes to analyze")
	codebaseAnalyzeCmd.Flags().StringSlice("ignore", nil, "Ignore pattern (repeatable)")

	codebaseSearchCmd.Flags().Bool("case-sensitive", false, "Use case-sensitive search")
	codebaseSearchCmd.Flags().StringSlice("ext", nil, "File extension to include (repeatable)")
	codebaseSearchCmd.Flags().Int("max-results", 100, "Maximum search results")
	codebaseSearchCmd.Flags().StringSlice("ignore", nil, "Ignore pattern (repeatable)")

	codebaseCmd.AddCommand(codebaseAnalyzeCmd, codebaseDepsCmd, codebaseSearchCmd)
	rootCmd.AddCommand(codebaseCmd)
}

func getArgOrCwd(args []string) string {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return args[0]
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func normalizeExtensions(exts []string) []string {
	if len(exts) == 0 {
		return nil
	}
	out := make([]string, 0, len(exts))
	for _, ext := range exts {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		ext = strings.TrimPrefix(ext, ".")
		out = append(out, ext)
	}
	return out
}
