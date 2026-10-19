package fsops

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	ferrors "github.com/legacy-ai/floyd/internal/errors"
	"github.com/legacy-ai/floyd/internal/validation"
)

func FileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func DirectoryExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func EnsureDirectory(dirPath string) error {
	if DirectoryExists(dirPath) {
		return nil
	}
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to create directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}
	slog.Debug("Created directory", "path", dirPath)
	return nil
}

func ReadTextFile(filePath string) (string, error) {
	if !validation.IsValidFilePath(filePath) {
		return "", ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid file path.",
				},
			},
		)
	}
	if !FileExists(filePath) {
		return "", ferrors.CreateUserError(
			fmt.Sprintf("File not found: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the file path and try again.",
				},
			},
		)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ferrors.CreateUserError(
				fmt.Sprintf("File not found: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileNotFound,
					Cause:    err,
					Resolution: []string{
						"Check the file path and try again.",
					},
				},
			)
		}
		return "", ferrors.CreateUserError(
			fmt.Sprintf("Failed to read file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileRead,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}
	return string(content), nil
}

func ReadFileLines(filePath string, start, end int) ([]string, error) {
	content, err := ReadTextFile(filePath)
	if err != nil {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Failed to read lines %d-%d from file: %s", start, end, filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileRead,
				Cause:    err,
				Resolution: []string{
					"Check the file path and line range, then try again.",
				},
			},
		)
	}

	lines := splitLines(content)
	startIndex := start - 1
	if startIndex < 0 {
		startIndex = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if startIndex >= len(lines) || startIndex >= end {
		return []string{}, nil
	}

	return lines[startIndex:end], nil
}

func WriteTextFile(filePath, content string, createDir, overwrite bool) error {
	if !validation.IsValidFilePath(filePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid file path.",
				},
			},
		)
	}

	if createDir {
		if err := EnsureDirectory(filepath.Dir(filePath)); err != nil {
			return err
		}
	}

	if !overwrite && FileExists(filePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("File already exists: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Resolution: []string{
					"Use overwrite option to replace existing file.",
				},
			},
		)
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		if _, ok := err.(*ferrors.UserError); ok {
			return err
		}
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to write file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}

	slog.Debug("Wrote file", "path", filePath, "bytes", len(content))
	return nil
}

func AppendTextFile(filePath, content string, createDir bool) error {
	if !validation.IsValidFilePath(filePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid file path.",
				},
			},
		)
	}

	if createDir {
		if err := EnsureDirectory(filepath.Dir(filePath)); err != nil {
			return err
		}
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to append to file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}
	defer file.Close()

	if _, err := file.Write([]byte(content)); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to append to file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}

	slog.Debug("Appended to file", "path", filePath, "bytes", len(content))
	return nil
}

func DeleteFile(filePath string) error {
	if !validation.IsValidFilePath(filePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid file path.",
				},
			},
		)
	}

	if !FileExists(filePath) {
		slog.Debug("File does not exist, nothing to delete", "path", filePath)
		return nil
	}

	if err := os.Remove(filePath); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to delete file: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and try again.",
				},
			},
		)
	}
	return nil
}

func Rename(oldPath, newPath string) error {
	if !validation.IsValidPath(oldPath) || !validation.IsValidPath(newPath) {
		badPath := oldPath
		if !validation.IsValidPath(newPath) {
			badPath = newPath
		}
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid path: %s", badPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide valid file paths.",
				},
			},
		)
	}

	if !FileExists(oldPath) && !DirectoryExists(oldPath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Path not found: %s", oldPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the source path and try again.",
				},
			},
		)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to rename: %s -> %s", oldPath, newPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and ensure destination path is valid.",
				},
			},
		)
	}

	return nil
}

func CopyFile(sourcePath, destPath string, overwrite, createDir bool) error {
	if !validation.IsValidFilePath(sourcePath) || !validation.IsValidFilePath(destPath) {
		badPath := sourcePath
		if !validation.IsValidFilePath(destPath) {
			badPath = destPath
		}
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", badPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide valid file paths.",
				},
			},
		)
	}

	if !FileExists(sourcePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Source file not found: %s", sourcePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the source path and try again.",
				},
			},
		)
	}

	if createDir {
		if err := EnsureDirectory(filepath.Dir(destPath)); err != nil {
			return err
		}
	}

	if !overwrite && FileExists(destPath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Destination file already exists: %s", destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Resolution: []string{
					"Use overwrite option to replace existing file.",
				},
			},
		)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to open source file: %s", sourcePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileRead,
				Cause:    err,
			},
		)
	}
	defer source.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !overwrite {
		flags = flags | os.O_EXCL
	}
	destination, err := os.OpenFile(destPath, flags, 0o644)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to open destination file: %s", destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
			},
		)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to copy file: %s -> %s", sourcePath, destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and paths, then try again.",
				},
			},
		)
	}

	return nil
}

func ListDirectory(dirPath string) ([]string, error) {
	if !validation.IsValidDirectoryPath(dirPath) {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Invalid directory path: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid directory path.",
				},
			},
		)
	}

	if !DirectoryExists(dirPath) {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Directory not found: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the directory path and try again.",
				},
			},
		)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Failed to list directory: %s", dirPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check directory permissions and try again.",
				},
			},
		)
	}

	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result, nil
}

func GetFileInfo(filePath string) (fs.FileInfo, error) {
	if !validation.IsValidPath(filePath) {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Invalid path: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid file or directory path.",
				},
			},
		)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ferrors.CreateUserError(
				fmt.Sprintf("Path not found: %s", filePath),
				ferrors.UserErrorOptions{
					Category: ferrors.ErrorCategoryFileNotFound,
					Cause:    err,
					Resolution: []string{
						"Check the path and try again.",
					},
				},
			)
		}
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Failed to get file info: %s", filePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check permissions and try again.",
				},
			},
		)
	}

	return info, nil
}

type FindOptions struct {
	Pattern            *regexp.Regexp
	Recursive          bool
	IncludeDirectories bool
}

func FindFiles(directory string, options FindOptions) ([]string, error) {
	if !validation.IsValidDirectoryPath(directory) {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Invalid directory path: %s", directory),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide a valid directory path.",
				},
			},
		)
	}

	if !DirectoryExists(directory) {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Directory not found: %s", directory),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the directory path and try again.",
				},
			},
		)
	}

	results := []string{}
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != directory && !options.Recursive {
				return filepath.SkipDir
			}
			if options.IncludeDirectories {
				if options.Pattern == nil || options.Pattern.MatchString(d.Name()) {
					results = append(results, path)
				}
			}
			return nil
		}
		if options.Pattern == nil || options.Pattern.MatchString(d.Name()) {
			results = append(results, path)
		}
		return nil
	}

	if err := filepath.WalkDir(directory, walkFn); err != nil {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Failed to find files in directory: %s", directory),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check directory permissions and try again.",
				},
			},
		)
	}

	return results, nil
}

func StreamFile(sourcePath, destPath string, overwrite, createDir bool) error {
	if !validation.IsValidFilePath(sourcePath) || !validation.IsValidFilePath(destPath) {
		badPath := sourcePath
		if !validation.IsValidFilePath(destPath) {
			badPath = destPath
		}
		return ferrors.CreateUserError(
			fmt.Sprintf("Invalid file path: %s", badPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryValidation,
				Resolution: []string{
					"Provide valid file paths.",
				},
			},
		)
	}

	if !FileExists(sourcePath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Source file not found: %s", sourcePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Check the source path and try again.",
				},
			},
		)
	}

	if !overwrite && FileExists(destPath) {
		return ferrors.CreateUserError(
			fmt.Sprintf("Destination file already exists: %s", destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Resolution: []string{
					"Use overwrite option to replace existing file.",
				},
			},
		)
	}

	if createDir {
		if err := EnsureDirectory(filepath.Dir(destPath)); err != nil {
			return err
		}
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to open source file: %s", sourcePath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileRead,
				Cause:    err,
			},
		)
	}
	defer source.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !overwrite {
		flags = flags | os.O_EXCL
	}
	destination, err := os.OpenFile(destPath, flags, 0o644)
	if err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to open destination file: %s", destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileWrite,
				Cause:    err,
			},
		)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return ferrors.CreateUserError(
			fmt.Sprintf("Failed to stream file: %s -> %s", sourcePath, destPath),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check file permissions and paths, then try again.",
				},
			},
		)
	}

	return nil
}

func CreateTempFile(prefix, suffix, content string) (string, error) {
	if prefix == "" {
		prefix = "tmp-"
	}
	if suffix == "" {
		suffix = ""
	}

	tmpDir := os.TempDir()
	workDir, err := os.MkdirTemp(tmpDir, prefix)
	if err != nil {
		return "", ferrors.CreateUserError(
			"Failed to create temporary file",
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check temporary directory permissions and try again.",
				},
			},
		)
	}

	filename := fmt.Sprintf("%s%d%s", prefix, time.Now().UnixNano(), suffix)
	path := filepath.Join(workDir, filename)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", ferrors.CreateUserError(
			"Failed to create temporary file",
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
				Resolution: []string{
					"Check temporary directory permissions and try again.",
				},
			},
		)
	}

	slog.Debug("Created temporary file", "path", path)
	return path, nil
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	return regexp.MustCompile("\\r\\n|\\n|\\r").Split(content, -1)
}
