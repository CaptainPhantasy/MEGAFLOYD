//go:build darwin || linux

package approvalauthority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"slices"
	"strconv"
	"syscall"
	"time"
)

// The script is fixed code. Every mode and path is a separate positional
// argument; none is evaluated as shell source. Kernel access checks include
// ACL grants that ordinary owner and permission-bit checks cannot establish.
const effectiveAccessScript = `
while [ "$#" -gt 0 ]; do
  [ "$#" -ge 2 ] || exit 2
  mode=$1
  path=$2
  shift 2
  case "$mode" in
    public)
      if [ -w "$path" ]; then printf 'WRITABLE\n'; exit 1; fi
      ;;
    private)
      if [ -r "$path" ] || [ -w "$path" ] || [ -x "$path" ]; then
        printf 'PRIVATE_ACCESS\n'; exit 1
      fi
      ;;
    *) exit 2 ;;
  esac
done
printf 'PASS\n'
`

func validateEffectiveAccess(c Config, uid uint32) error {
	if os.Geteuid() != 0 || uid == 0 {
		return errors.New("approval authority requires root to verify unprivileged path access")
	}
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return errors.New("approval authority effective-access account is unknown")
	}
	groups, err := account.GroupIds()
	if err != nil {
		return errors.New("approval authority effective-access groups are unknown")
	}
	credentials, err := accessCredentials(account, groups)
	if err != nil {
		return err
	}
	checks, err := protectedPathAccess(c)
	if err != nil {
		return err
	}
	if err := validateNativeACL(checks); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runEffectiveAccessProbe(ctx, credentials, checks); err != nil {
		return fmt.Errorf("approval authority effective access rejected engine UID %d: %w", uid, err)
	}
	return nil
}

func accessCredentials(account *user.User, groupIDs []string) (*syscall.Credential, error) {
	if account == nil {
		return nil, errors.New("approval authority effective-access account is missing")
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return nil, errors.New("approval authority effective-access UID is invalid")
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil {
		return nil, errors.New("approval authority effective-access primary GID is invalid")
	}
	groups := []uint32{uint32(gid)}
	for _, id := range groupIDs {
		value, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			return nil, errors.New("approval authority effective-access supplementary GID is invalid")
		}
		if !slices.Contains(groups, uint32(value)) {
			groups = append(groups, uint32(value))
		}
	}
	slices.Sort(groups)
	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: groups}, nil
}

func runEffectiveAccessProbe(ctx context.Context, credentials *syscall.Credential, checks []pathAccess) error {
	if len(checks) == 0 {
		return errors.New("approval authority effective-access paths are missing")
	}
	arguments := []string{"-c", effectiveAccessScript, "approval-path-check"}
	for _, check := range checks {
		mode := "public"
		if check.private {
			mode = "private"
		}
		arguments = append(arguments, mode, check.path)
	}
	command := exec.CommandContext(ctx, "/bin/sh", arguments...)
	command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	command.Dir = "/"
	command.SysProcAttr = &syscall.SysProcAttr{Credential: credentials}
	output, err := command.CombinedOutput()
	if err == nil && string(output) == "PASS\n" {
		return nil
	}
	if string(output) == "WRITABLE\n" {
		return errors.New("a protected executable or ancestor is effectively writable")
	}
	if string(output) == "PRIVATE_ACCESS\n" {
		return errors.New("private approval storage is effectively accessible")
	}
	if err != nil {
		return fmt.Errorf("cannot establish protected path access: %w", err)
	}
	return errors.New("protected path access returned incomplete evidence")
}
