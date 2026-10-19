package tools

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// goFilesBuildCheck runs `go build ./...` when a .go file is modified and
// appends any compiler errors to the tool result so the model can self-correct
// without a separate tool call. Returns empty string if the file is not Go or
// compilation succeeds.
func goFilesBuildCheck(filePath, workingDir string) string {
	if !strings.HasSuffix(filePath, ".go") {
		return ""
	}

	if os.Getenv("FLOYD_DISABLE_BUILD_CHECK") != "" {
		return ""
	}

	// Determine the Go module root by walking up from the file.
	moduleRoot := findGoModRoot(filePath, workingDir)
	if moduleRoot == "" {
		return ""
	}

	// Build only the package containing the modified file for speed.
	pkgDir := filepath.Dir(filePath)
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "./..."
	} else {
		relPkg = "./" + relPkg
	}

	ctx, cancel := newBuildContext()
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", relPkg)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	buildErr := cmd.Run()
	if buildErr == nil {
		slog.Debug("Selfheal: go build succeeded", "pkg", relPkg, "root", moduleRoot)
		return "\n<build_check>\ngo build: OK\n</build_check>\n"
	}

	errOutput := stderr.String()
	if len(errOutput) > 2000 {
		errOutput = errOutput[:2000] + "\n... (truncated)"
	}

	slog.Warn("Selfheal: go build failed", "pkg", relPkg, "errors", errOutput)
	return fmt.Sprintf("\n<build_check>\ngo build FAILED — you MUST fix these errors before proceeding:\n%s\n</build_check>\n", errOutput)
}

// findGoModRoot walks up from filePath looking for go.mod, bounded by workingDir.
func findGoModRoot(filePath, workingDir string) string {
	dir := filepath.Dir(filePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		// Don't walk above workingDir
		if len(dir) < len(workingDir) {
			break
		}
		dir = parent
	}
	return ""
}

func newBuildContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}
