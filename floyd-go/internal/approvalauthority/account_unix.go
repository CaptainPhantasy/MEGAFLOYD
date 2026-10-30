//go:build darwin || linux

package approvalauthority

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// validateLocalEngineAccount rejects known administrative identity locally.
// An unknown self-query only defers to the genuine authority's strict root
// query; it cannot establish approval or authorize a record by itself.
func validateLocalEngineAccount() (localPolicyStatus, error) {
	uid := uint32(os.Getuid())
	if uid == 0 {
		return localPolicyDeferred, errors.New("approval authority refuses a root engine account")
	}
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return localPolicyDeferred, errors.New("approval authority local engine account is unknown")
	}
	if err := validateAccountGroups(account); err != nil {
		return localPolicyDeferred, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "-l")
	command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "USER=" + account.Username, "LOGNAME=" + account.Username, "HOME=" + account.HomeDir}
	output, err := command.CombinedOutput()
	return classifyLocalSudoPolicy(account.Username, output, err)
}

func classifyLocalSudoPolicy(account string, output []byte, queryErr error) (localPolicyStatus, error) {
	if strings.Contains(string(output), "User "+account+" may run the following commands on ") {
		return localPolicyDeferred, errors.New("approval authority refuses a local engine account with visible sudo grants")
	}
	finished := queryErr == nil
	if !finished {
		var exit *exec.ExitError
		finished = errors.As(queryErr, &exit) && exit.ExitCode() == 1
	}
	if finished && sudoDeniedOutput(account, output) {
		return localPolicyAbsent, nil
	}
	return localPolicyDeferred, nil
}

func validateAccountGroups(account *user.User) error {
	groups, err := account.GroupIds()
	if err != nil {
		return errors.New("approval authority engine groups cannot be verified")
	}
	for _, id := range groups {
		group, err := user.LookupGroupId(id)
		if err != nil {
			return errors.New("approval authority engine group cannot be identified")
		}
		if privilegedGroup(group.Name) {
			return errors.New("approval authority refuses an administrative engine account")
		}
	}
	return nil
}

// validateEngineAccount refuses accounts that can administer the authority.
// Root queries sudo's policy for the peer rather than trusting peer-provided
// privilege claims or assuming distinct numeric UIDs imply isolation.
func validateEngineAccount(uid uint32) error {
	if uid == 0 {
		return errors.New("approval authority refuses a root engine account")
	}
	if os.Geteuid() != 0 {
		return errors.New("approval authority requires root to inspect engine account privilege")
	}
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return errors.New("approval authority engine account is unknown")
	}
	if err := validateAccountGroups(account); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "-l", "-U", account.Username)
	command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	output, err := command.CombinedOutput()
	// Sudo versions return either 0 or 1 for an account with no grants.
	// Only its complete, account-specific denial line establishes absence.
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return errors.New("approval authority cannot inspect engine sudo policy")
		}
	}
	if !sudoDeniedOutput(account.Username, output) {
		return errors.New("approval authority refuses an engine account whose absence of sudo privileges is unproven")
	}
	return nil
}

func privilegedGroup(name string) bool {
	switch name {
	case "admin", "sudo", "wheel", "docker", "lxd", "incus", "disk":
		return true
	default:
		return false
	}
}

func sudoDeniedOutput(account string, output []byte) bool {
	pattern := "^User " + regexp.QuoteMeta(account) + " is not allowed to run sudo on [[:alnum:]_.-]+\\.\n?$"
	return regexp.MustCompile(pattern).Match(output)
}
