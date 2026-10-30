//go:build darwin

package approvalauthority

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Darwin W_OK omits some fine-grained directory ACL grants: add_file can allow
// creation even when access(W_OK) is false. Conservatively reject every allow
// ACE on the protected chain. Deny-only ACLs cannot confer additional access.
func validateNativeACL(checks []pathAccess) error {
	paths := make(map[string]bool)
	for _, check := range checks {
		for path := check.path; ; path = filepath.Dir(path) {
			paths[path] = true
			if filepath.Dir(path) == path {
				break
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	slices.Sort(ordered)
	if len(ordered) == 0 {
		return errors.New("approval authority ACL paths are missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/ls", append([]string{"-ldeb"}, ordered...)...)
	command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.New("approval authority native ACL inspection failed")
	}
	return inspectDarwinACLListing(string(output), len(ordered))
}

var (
	aclMetadataLine = regexp.MustCompile(`^[bcdlps-][rwxStTs-]{9}[@+]?\s`)
	aclEntryLine    = regexp.MustCompile(`^\s+[0-9]+:\s+.+\s(allow|deny)\s+.+$`)
)

func inspectDarwinACLListing(output string, expectedPaths int) error {
	metadata := 0
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if aclMetadataLine.MatchString(line) {
			metadata++
			continue
		}
		entry := aclEntryLine.FindStringSubmatch(line)
		if len(entry) == 0 {
			return errors.New("approval authority ACL inspection returned unrecognized evidence")
		}
		if entry[1] == "allow" {
			return errors.New("approval authority protected paths contain an allow ACL")
		}
	}
	if metadata != expectedPaths {
		return errors.New("approval authority ACL inspection returned incomplete evidence")
	}
	return nil
}
