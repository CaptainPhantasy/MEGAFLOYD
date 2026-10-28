package prompt

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

// captureLogger returns a logger writing JSON records to a buffer, plus a
// drain function that decodes and clears the records collected so far.
func captureLogger(t *testing.T, level slog.Level) (*slog.Logger, func() []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	drain := func() []map[string]any {
		var records []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &record), "log line: %s", line)
			records = append(records, record)
		}
		buf.Reset()
		return records
	}
	return logger, drain
}

func recordWithMessage(records []map[string]any, message string) map[string]any {
	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}
	return nil
}

func TestStartupPolicyVerification(t *testing.T) {
	t.Parallel()
	bundleDir := t.TempDir()
	bundlePath := filepath.Join(bundleDir, "MegaFloyd.md")
	clean := []byte(StandardPolicy())
	require.NoError(t, os.WriteFile(bundlePath, clean, 0o600))

	logger, drain := captureLogger(t, slog.LevelDebug)
	loaded := StandardPolicySHA256()

	check := CheckStandard(loaded, []string{bundlePath})
	ReportStandard(logger, check)
	require.True(t, check.BundlePresent)
	require.True(t, check.Match)

	records := drain()
	loadedRecord := recordWithMessage(records, "Loaded behavioral standard")
	require.NotNil(t, loadedRecord, "startup must name the loaded standard")
	require.Equal(t, "INFO", loadedRecord["level"])
	require.Equal(t, loaded, loadedRecord["sha256"])

	matchRecord := recordWithMessage(records, "Bundled behavioral standard matches the loaded policy")
	require.NotNil(t, matchRecord)
	require.Equal(t, loaded, matchRecord["sha256"])
	require.Equal(t, bundlePath, matchRecord["path"])
	require.Nil(t, recordWithMessage(records, "Bundled behavioral standard differs from the loaded policy"))

	// Tamper one byte of the bundle standard.
	tampered := bytes.Clone(clean)
	index := bytes.Index(tampered, []byte("# "))
	require.NotEqual(t, -1, index, "fixture standard must contain a heading")
	tampered[index+2] ^= 0x01
	require.NotEqual(t, clean, tampered)
	require.NoError(t, os.WriteFile(bundlePath, tampered, 0o600))

	check = CheckStandard(loaded, []string{bundlePath})
	ReportStandard(logger, check)
	require.True(t, check.BundlePresent)
	require.False(t, check.Match)

	records = drain()
	warnRecord := recordWithMessage(records, "Bundled behavioral standard differs from the loaded policy")
	require.NotNil(t, warnRecord, "a tampered bundle must warn")
	require.Equal(t, "WARN", warnRecord["level"])
	require.Equal(t, loaded, warnRecord["loaded_sha256"])
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tampered)), warnRecord["bundle_sha256"])
	require.NotEqual(t, warnRecord["loaded_sha256"], warnRecord["bundle_sha256"])
	require.Equal(t, bundlePath, warnRecord["path"])
	require.Nil(t, recordWithMessage(records, "Bundled behavioral standard matches the loaded policy"))

	// Restoring the original bytes brings the clean record back.
	require.NoError(t, os.WriteFile(bundlePath, clean, 0o600))
	check = CheckStandard(loaded, []string{bundlePath})
	ReportStandard(logger, check)
	require.True(t, check.Match)

	records = drain()
	restoredRecord := recordWithMessage(records, "Bundled behavioral standard matches the loaded policy")
	require.NotNil(t, restoredRecord)
	require.Equal(t, loaded, restoredRecord["sha256"])
	require.Nil(t, recordWithMessage(records, "Bundled behavioral standard differs from the loaded policy"))
}

func TestStartupPolicyAbsentBundleIsQuiet(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "MegaFloyd.md")

	logger, drain := captureLogger(t, slog.LevelDebug)
	loaded := StandardPolicySHA256()

	check := CheckStandard(loaded, []string{missing})
	ReportStandard(logger, check)
	require.False(t, check.BundlePresent)
	require.False(t, check.Match)
	require.Empty(t, check.BundlePath)

	records := drain()
	loadedRecord := recordWithMessage(records, "Loaded behavioral standard")
	require.NotNil(t, loadedRecord)
	require.Equal(t, loaded, loadedRecord["sha256"])
	for _, record := range records {
		require.NotEqual(t, "WARN", record["level"], "absence is normal and must not warn")
		require.NotEqual(t, "ERROR", record["level"])
	}
}

func TestStandardCandidatesPreferBundleThenGlobalPolicy(t *testing.T) {
	// Not parallel: config.GlobalConfig reads process environment.
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_CONFIG", "")
	paths := StandardCandidates()
	require.NotEmpty(t, paths)
	for _, path := range paths {
		require.Equal(t, filepath.Clean(path), path)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		require.False(t, seen[path], "duplicate candidate %s", path)
		seen[path] = true
	}
	global := config.GlobalConfig()
	require.NotEmpty(t, global)
	require.Equal(t, filepath.Join(filepath.Dir(global), "MegaFloyd.md"), paths[len(paths)-1],
		"the global policy file must be a candidate")
}
