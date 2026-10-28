package prompt

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/legacy-ai/floyd/internal/config"
)

// StandardCheck records the startup comparison between the standard compiled
// into this binary and the standard file shipped beside it.
type StandardCheck struct {
	// LoadedSHA256 identifies the standard bytes this binary loads.
	LoadedSHA256 string
	// BundlePath is the standard file that was found, empty when none was.
	BundlePath string
	// BundleSHA256 identifies the bytes at BundlePath, empty when none was.
	BundleSHA256 string
	// BundlePresent reports whether a bundle standard was found at all.
	BundlePresent bool
	// Match reports whether a found bundle standard matches the loaded one.
	Match bool
	// CheckedPaths lists the locations inspected, in order.
	CheckedPaths []string
}

// VerifyLoadedStandard logs the hash of the loaded standard and compares it
// with the standard shipped beside the executable or installed at the global
// policy path. A mismatch is reported, never fatal: hash binding is a build
// operation and project-level policy files are not hash-gated.
func VerifyLoadedStandard(logger *slog.Logger) StandardCheck {
	check := CheckStandard(StandardPolicySHA256(), StandardCandidates())
	ReportStandard(logger, check)
	return check
}

// CheckStandard hashes the first readable candidate and compares it with the
// loaded hash. A missing candidate is normal and leaves BundlePresent false.
func CheckStandard(loadedSHA256 string, candidates []string) StandardCheck {
	check := StandardCheck{LoadedSHA256: loadedSHA256, CheckedPaths: candidates}
	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		check.BundlePath = path
		check.BundleSHA256 = fmt.Sprintf("%x", sha256.Sum256(content))
		check.BundlePresent = true
		check.Match = check.BundleSHA256 == loadedSHA256
		break
	}
	return check
}

// ReportStandard emits the startup records for a completed check.
func ReportStandard(logger *slog.Logger, check StandardCheck) {
	logger.Info("Loaded behavioral standard", "sha256", check.LoadedSHA256)
	switch {
	case !check.BundlePresent:
		logger.Debug(
			"No bundled behavioral standard beside this binary",
			"checked", check.CheckedPaths,
		)
	case check.Match:
		logger.Info(
			"Bundled behavioral standard matches the loaded policy",
			"sha256", check.BundleSHA256,
			"path", check.BundlePath,
		)
	default:
		logger.Warn(
			"Bundled behavioral standard differs from the loaded policy",
			"loaded_sha256", check.LoadedSHA256,
			"bundle_sha256", check.BundleSHA256,
			"path", check.BundlePath,
		)
	}
}

// StandardCandidates lists the standard files shipped beside this executable,
// then the global policy file for the active home.
func StandardCandidates() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if dir, err := filepath.Abs(filepath.Dir(exe)); err == nil {
			// Installed bundle: Contents/Resources/megafloyd/bin/megafloyd
			// keeps its standard one directory up in the same folder.
			paths = append(paths, filepath.Join(dir, "..", "MegaFloyd.md"))
			paths = append(paths, filepath.Join(dir, "MegaFloyd.md"))
		}
	}
	if global := config.GlobalConfig(); global != "" {
		paths = append(paths, filepath.Join(filepath.Dir(global), "MegaFloyd.md"))
	}
	return dedupePaths(paths)
}

func dedupePaths(paths []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, path := range paths {
		clean := filepath.Clean(path)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		unique = append(unique, clean)
	}
	return unique
}
