package fileops

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/legacy-ai/floyd/internal/diff"
	ferrors "github.com/legacy-ai/floyd/internal/errors"
	"github.com/legacy-ai/floyd/internal/filepathext"
	"github.com/legacy-ai/floyd/internal/validation"
)

type Config struct {
	WorkspacePath   string
	MaxReadSizeByte int64
}

type Result struct {
	Success bool
	Error   error
	Path    string
	Content string
	Created bool
	Files   []string
	Info    fs.FileInfo
}

type Manager struct {
	config        Config
	workspacePath string
}

func NewManager(cfg Config) *Manager {
	workspace := cfg.WorkspacePath
	if workspace == "" {
		if wd, err := os.Getwd(); err == nil {
			workspace = wd
		}
	}
	if cfg.MaxReadSizeByte == 0 {
		cfg.MaxReadSizeByte = 10 * 1024 * 1024
	}
	return &Manager{
		config:        cfg,
		workspacePath: workspace,
	}
}

func (m *Manager) Initialize() error {
	info, err := os.Stat(m.workspacePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ferrors.CreateUserError(
				fmt.Sprintf("Workspace directory does not exist: %s", m.workspacePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Provide a valid workspace path",
					},
				},
			)
		}
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to access workspace directory: %s", m.workspacePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)
	}
	if !info.IsDir() {
		return ferrors.CreateUserError(
			fmt.Sprintf("Workspace path is not a directory: %s", m.workspacePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
			},
		)
	}
	return nil
}

func (m *Manager) GetAbsolutePath(relPath string) string {
	cleaned := filepath.Clean(relPath)
	for cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		cleaned = strings.TrimPrefix(cleaned, ".."+string(filepath.Separator))
		cleaned = strings.TrimPrefix(cleaned, "..")
		cleaned = strings.TrimPrefix(cleaned, string(filepath.Separator))
	}
	if filepathext.SmartIsAbs(cleaned) {
		return filepath.Clean(cleaned)
	}
	return filepath.Join(m.workspacePath, cleaned)
}

func (m *Manager) GetRelativePath(absPath string) string {
	rel, err := filepath.Rel(m.workspacePath, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

func (m *Manager) ReadFile(filePath string) Result {
	absolutePath := m.GetAbsolutePath(filePath)

	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("File not found: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check that the file exists and the path is correct",
					},
				},
			)}
		}
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied reading file: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check file permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to read file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	if info.IsDir() {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Not a file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
			},
		)}
	}

	if m.config.MaxReadSizeByte > 0 && info.Size() > m.config.MaxReadSizeByte {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("File too large to read: %s (%d bytes)", filePath, info.Size()),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Resolution: []string{
					"Try reading a smaller file or use a text editor to open this file",
					"Adjust file_ops.max_read_size_bytes in config if needed",
				},
			},
		)}
	}

	content, err := os.ReadFile(absolutePath)
	if err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to read file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: filePath, Content: string(content)}
}

func (m *Manager) Stat(path string) Result {
	absolutePath := m.GetAbsolutePath(path)
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Path not found: %s", path),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileNotFound,
					Resolution: []string{
						"Check the path and try again.",
					},
				},
			)}
		}
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied accessing path: %s", path),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check file permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to access path: %s", path),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: path, Info: info}
}

func (m *Manager) WriteFile(filePath, content string, createDirs bool) Result {
	absolutePath := m.GetAbsolutePath(filePath)
	if createDirs {
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Failed to create directory: %s", filepath.Dir(filePath)),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Cause:    err,
				},
			)}
		}
	}

	created := false
	if _, err := os.Stat(absolutePath); err != nil {
		if os.IsNotExist(err) {
			created = true
		}
	}

	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied writing file: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check file permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to write file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: filePath, Created: created}
}

func (m *Manager) AppendFile(filePath, content string, createDirs bool) Result {
	absolutePath := m.GetAbsolutePath(filePath)
	if createDirs {
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Failed to create directory: %s", filepath.Dir(filePath)),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Cause:    err,
				},
			)}
		}
	}

	file, err := os.OpenFile(absolutePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to append to file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions or try running with elevated privileges",
				},
			},
		)}
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to append to file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: filePath}
}

func (m *Manager) CreateTempFile(dir, prefix, suffix, content string) Result {
	baseDir := m.workspacePath
	if dir != "" {
		baseDir = m.GetAbsolutePath(dir)
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to create directory: %s", baseDir),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	if prefix == "" {
		prefix = "floyd-"
	}
	pattern := prefix + "*" + suffix
	tmp, err := os.CreateTemp(baseDir, pattern)
	if err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			"Failed to create temp file",
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}
	defer tmp.Close()

	if content != "" {
		if _, err := tmp.WriteString(content); err != nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				"Failed to write temp file content",
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileWrite,
					Cause:    err,
				},
			)}
		}
	}

	return Result{Success: true, Path: m.GetRelativePath(tmp.Name())}
}

func (m *Manager) CopyFile(sourcePath, destPath string, overwrite bool, createDirs bool) Result {
	source := m.GetAbsolutePath(sourcePath)
	dest := m.GetAbsolutePath(destPath)

	if createDirs {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Failed to create directory: %s", filepath.Dir(destPath)),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Cause:    err,
				},
			)}
		}
	}

	if !overwrite {
		if _, err := os.Stat(dest); err == nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Destination file already exists: %s", destPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Use overwrite option to replace existing file.",
					},
				},
			)}
		}
	}

	input, err := os.ReadFile(source)
	if err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to read source file: %s", sourcePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileRead,
				Cause:    err,
			},
		)}
	}

	if err := os.WriteFile(dest, input, 0o644); err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to write destination file: %s", destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: destPath}
}

func (m *Manager) Rename(oldPath, newPath string) Result {
	source := m.GetAbsolutePath(oldPath)
	dest := m.GetAbsolutePath(newPath)

	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Path not found: %s", oldPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileNotFound,
					Resolution: []string{
						"Check the source path and try again.",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to access source path: %s", oldPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to create directory: %s", filepath.Dir(newPath)),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	if err := os.Rename(source, dest); err != nil {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to rename: %s -> %s", oldPath, newPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and ensure destination path is valid.",
				},
			},
		)}
	}

	return Result{Success: true, Path: newPath}
}

func (m *Manager) DeleteFile(filePath string) Result {
	absolutePath := m.GetAbsolutePath(filePath)
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("File not found: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to delete file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}
	if info.IsDir() {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Not a file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
			},
		)}
	}

	if err := os.Remove(absolutePath); err != nil {
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied deleting file: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check file permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to delete file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	return Result{Success: true, Path: filePath}
}

func (m *Manager) FileExists(filePath string) bool {
	absolutePath := m.GetAbsolutePath(filePath)
	info, err := os.Stat(absolutePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func (m *Manager) CreateDirectory(dirPath string, recursive bool) Result {
	absolutePath := m.GetAbsolutePath(dirPath)

	if err := os.MkdirAll(absolutePath, 0o755); err != nil {
		if os.IsExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Directory already exists: %s", dirPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
				},
			)}
		}
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied creating directory: %s", dirPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check file permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to create directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	if !recursive {
		if _, err := os.Stat(absolutePath); err != nil {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Failed to create directory: %s", dirPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Cause:    err,
				},
			)}
		}
	}

	return Result{Success: true, Path: dirPath}
}

func (m *Manager) ListDirectory(dirPath string) Result {
	absolutePath := m.GetAbsolutePath(dirPath)
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Directory not found: %s", dirPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to list directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}
	if !info.IsDir() {
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Not a directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
			},
		)}
	}

	entries, err := os.ReadDir(absolutePath)
	if err != nil {
		if os.IsPermission(err) {
			return Result{Success: false, Error: ferrors.CreateUserError(
				fmt.Sprintf("Permission denied listing directory: %s", dirPath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileSystem,
					Resolution: []string{
						"Check directory permissions or try running with elevated privileges",
					},
				},
			)}
		}
		return Result{Success: false, Error: ferrors.CreateUserError(
			fmt.Sprintf("Failed to list directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)}
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.Name())
	}

	return Result{Success: true, Path: dirPath, Files: files}
}

func (m *Manager) GenerateDiff(original, modified string) string {
	linesA := strings.Split(original, "\n")
	linesB := strings.Split(modified, "\n")

	var diff []string
	max := len(linesA)
	if len(linesB) > max {
		max = len(linesB)
	}

	for i := 0; i < max; i++ {
		same := i < len(linesA) && i < len(linesB) && linesA[i] == linesB[i]
		switch {
		case same:
			diff = append(diff, "  "+linesA[i])
		case i >= len(linesA):
			diff = append(diff, "+ "+linesB[i])
		case i >= len(linesB):
			diff = append(diff, "- "+linesA[i])
		default:
			diff = append(diff, "- "+linesA[i])
			diff = append(diff, "+ "+linesB[i])
		}
	}

	return strings.Join(diff, "\n")
}

func (m *Manager) ApplyPatch(filePath, patch string) Result {
	return m.WriteFile(filePath, patch, true)
}

func (m *Manager) ValidatePath(path string) bool {
	return validation.IsValidPath(path)
}

func (m *Manager) DiffFileContents(filePath, modified string) (string, Result) {
	readResult := m.ReadFile(filePath)
	if !readResult.Success {
		return "", readResult
	}

	unified, _, _ := diff.GenerateDiff(readResult.Content, modified, filePath)
	return unified, Result{Success: true, Path: filePath}
}

func IsNotFound(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func IsPermission(err error) bool {
	return err != nil && os.IsPermission(err)
}

func IsDir(info fs.FileInfo) bool {
	return info != nil && info.IsDir()
}

func LogPath(operation, path string) {
	slog.Debug("File operation", "operation", operation, "path", path)
}
