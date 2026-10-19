package cmd

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/fileops"
	"github.com/legacy-ai/floyd/internal/fsops"
	"github.com/spf13/cobra"
)

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "File operations",
}

var fileReadCmd = &cobra.Command{
	Use:   "read <path>",
	Short: "Read a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		result := mgr.ReadFile(args[0])
		if !result.Success {
			return result.Error
		}
		cmd.Print(result.Content)
		return nil
	},
}

var fileInfoCmd = &cobra.Command{
	Use:   "info <path>",
	Short: "Show file or directory info",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		result := mgr.Stat(args[0])
		if !result.Success {
			return result.Error
		}
		info := result.Info
		mode := info.Mode().String()
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		}
		cmd.Printf("Path: %s\n", args[0])
		cmd.Printf("Type: %s\n", kind)
		cmd.Printf("Size: %d\n", info.Size())
		cmd.Printf("Mode: %s\n", mode)
		cmd.Printf("Modified: %s\n", info.ModTime().Format(time.RFC3339))
		return nil
	},
}

var fileListCmd = &cobra.Command{
	Use:   "ls <dir>",
	Short: "List a directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		result := mgr.ListDirectory(args[0])
		if !result.Success {
			return result.Error
		}
		for _, name := range result.Files {
			cmd.Println(name)
		}
		return nil
	},
}

var fileMkdirCmd = &cobra.Command{
	Use:   "mkdir <dir>",
	Short: "Create a directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		recursive, _ := cmd.Flags().GetBool("parents")
		result := mgr.CreateDirectory(args[0], recursive)
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileWriteCmd = &cobra.Command{
	Use:   "write <path>",
	Short: "Write a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		mkdirs, _ := cmd.Flags().GetBool("parents")
		content, _ := cmd.Flags().GetString("content")
		if content == "" {
			stdinContent, readErr := readStdinIfAny()
			if readErr != nil {
				return readErr
			}
			content = stdinContent
		}
		if content == "" {
			return errors.New("No content provided")
		}
		if !force && mgr.FileExists(args[0]) {
			return errors.New("File exists (use --force to overwrite)")
		}
		result := mgr.WriteFile(args[0], content, mkdirs)
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileApplyCmd = &cobra.Command{
	Use:   "apply <path>",
	Short: "Apply content from stdin or --content",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		content, _ := cmd.Flags().GetString("content")
		if content == "" {
			stdinContent, readErr := readStdinIfAny()
			if readErr != nil {
				return readErr
			}
			content = stdinContent
		}
		if content == "" {
			return errors.New("No content provided (use --content or pipe data)")
		}
		result := mgr.ApplyPatch(args[0], content)
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileAppendCmd = &cobra.Command{
	Use:   "append <path>",
	Short: "Append to a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		mkdirs, _ := cmd.Flags().GetBool("parents")
		content, _ := cmd.Flags().GetString("content")
		if content == "" {
			stdinContent, readErr := readStdinIfAny()
			if readErr != nil {
				return readErr
			}
			content = stdinContent
		}
		if content == "" {
			return errors.New("No content provided")
		}
		result := mgr.AppendFile(args[0], content, mkdirs)
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileCopyCmd = &cobra.Command{
	Use:   "cp <source> <dest>",
	Short: "Copy a file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		mkdirs, _ := cmd.Flags().GetBool("parents")
		result := mgr.CopyFile(args[0], args[1], force, mkdirs)
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileTempCmd = &cobra.Command{
	Use:   "temp",
	Short: "Create a temporary file",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		dir, _ := cmd.Flags().GetString("dir")
		prefix, _ := cmd.Flags().GetString("prefix")
		suffix, _ := cmd.Flags().GetString("suffix")
		content, _ := cmd.Flags().GetString("content")
		if content == "" {
			stdinContent, readErr := readStdinIfAny()
			if readErr != nil {
				return readErr
			}
			content = stdinContent
		}
		result := mgr.CreateTempFile(dir, prefix, suffix, content)
		if !result.Success {
			return result.Error
		}
		cmd.Println(result.Path)
		return nil
	},
}

var fileRenameCmd = &cobra.Command{
	Use:   "mv <source> <dest>",
	Short: "Rename or move a file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		result := mgr.Rename(args[0], args[1])
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

var fileFindCmd = &cobra.Command{
	Use:   "find <dir>",
	Short: "Find files matching a pattern",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern, _ := cmd.Flags().GetString("pattern")
		recursive, _ := cmd.Flags().GetBool("recursive")
		includeDirs, _ := cmd.Flags().GetBool("dirs")

		var re *regexp.Regexp
		if pattern != "" {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return err
			}
			re = compiled
		}

		results, err := fsops.FindFiles(args[0], fsops.FindOptions{
			Pattern:            re,
			Recursive:          recursive,
			IncludeDirectories: includeDirs,
		})
		if err != nil {
			return err
		}
		for _, item := range results {
			cmd.Println(item)
		}
		return nil
	},
}

var fileDiffCmd = &cobra.Command{
	Use:   "diff <path>",
	Short: "Generate a unified diff against stdin or --content",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		content, _ := cmd.Flags().GetString("content")
		if content == "" {
			stdinContent, readErr := readStdinIfAny()
			if readErr != nil {
				return readErr
			}
			content = stdinContent
		}
		if content == "" {
			return errors.New("No content provided (use --content or pipe data)")
		}
		diffText, result := mgr.DiffFileContents(args[0], content)
		if !result.Success {
			return result.Error
		}
		cmd.Print(diffText)
		return nil
	},
}

var fileRemoveCmd = &cobra.Command{
	Use:   "rm <path>",
	Short: "Delete a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := fileOpsManager(cmd)
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			return errors.New("Refusing to delete without --force")
		}
		result := mgr.DeleteFile(args[0])
		if !result.Success {
			return result.Error
		}
		return nil
	},
}

func init() {
	fileMkdirCmd.Flags().Bool("parents", true, "Create parent directories as needed")
	fileWriteCmd.Flags().Bool("parents", true, "Create parent directories as needed")
	fileWriteCmd.Flags().String("content", "", "Content to write; if empty, reads from stdin")
	fileWriteCmd.Flags().Bool("force", false, "Overwrite existing file")
	fileAppendCmd.Flags().Bool("parents", true, "Create parent directories as needed")
	fileAppendCmd.Flags().String("content", "", "Content to append; if empty, reads from stdin")
	fileCopyCmd.Flags().Bool("parents", true, "Create parent directories as needed")
	fileCopyCmd.Flags().Bool("force", false, "Overwrite existing file")
	fileDiffCmd.Flags().String("content", "", "Content to compare against; if empty, reads from stdin")
	fileApplyCmd.Flags().String("content", "", "Content to apply; if empty, reads from stdin")
	fileRemoveCmd.Flags().Bool("force", false, "Confirm deletion")
	fileTempCmd.Flags().String("dir", "", "Directory to create the temp file in")
	fileTempCmd.Flags().String("prefix", "floyd-", "Temp file prefix")
	fileTempCmd.Flags().String("suffix", "", "Temp file suffix")
	fileTempCmd.Flags().String("content", "", "Content to write; if empty, reads from stdin")
	fileFindCmd.Flags().String("pattern", "", "Regex pattern to match")
	fileFindCmd.Flags().Bool("recursive", true, "Search directories recursively")
	fileFindCmd.Flags().Bool("dirs", false, "Include directories in results")

	fileCmd.AddCommand(fileReadCmd, fileInfoCmd, fileListCmd, fileMkdirCmd, fileWriteCmd, fileAppendCmd, fileApplyCmd, fileCopyCmd, fileRenameCmd, fileTempCmd, fileFindCmd, fileDiffCmd, fileRemoveCmd)
	rootCmd.AddCommand(fileCmd)
}

func fileOpsManager(cmd *cobra.Command) (*fileops.Manager, error) {
	debug, _ := cmd.Flags().GetBool("debug")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, err
	}
	cfgStore, err := config.Load(cwd, dataDir, debug)
	if err != nil {
		return nil, err
	}
	cfg := cfgStore.Config()

	// TODO(port-wave4): the chassis config has no options.file_ops
	// surface yet; the manager default applies until then.
	maxRead := int64(0)
	_ = cfg

	mgr := fileops.NewManager(fileops.Config{
		WorkspacePath:   cwd,
		MaxReadSizeByte: maxRead,
	})
	if err := mgr.Initialize(); err != nil {
		return nil, err
	}
	return mgr, nil
}

func readStdinIfAny() (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeNamedPipe == 0 && !info.Mode().IsRegular() {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}
