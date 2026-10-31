//go:build darwin || linux

package approvalauthority

import (
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorityEffectiveAccessDropsAllCredentials(t *testing.T) {
	credentials, err := accessCredentials(&user.User{Uid: "4294967294", Gid: "20"}, []string{"99", "20", "12", "99"})
	require.NoError(t, err)
	require.Equal(t, &syscall.Credential{Uid: 4294967294, Gid: 20, Groups: []uint32{12, 20, 99}}, credentials)
	require.False(t, credentials.NoSetGroups, "the helper must replace, not inherit, root supplementary groups")
	for _, account := range []*user.User{nil, {Uid: "invalid", Gid: "20"}, {Uid: "1", Gid: "invalid"}} {
		_, err := accessCredentials(account, nil)
		require.Error(t, err)
	}
	_, err = accessCredentials(&user.User{Uid: "1", Gid: "20"}, []string{"invalid"})
	require.Error(t, err)
}

func TestAuthorityEffectiveAccessDistinguishesPublicAndPrivate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("current-credential probe requires an unprivileged test runner")
	}
	path := filepath.Join(t.TempDir(), "readable-file")
	require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o400))
	require.NoError(t, runEffectiveAccessProbe(t.Context(), nil, []pathAccess{{path: path}}))
	require.ErrorContains(t, runEffectiveAccessProbe(t.Context(), nil, []pathAccess{{path: path, private: true}}), "private approval storage")
	require.NoError(t, os.Chmod(path, 0o600))
	require.ErrorContains(t, runEffectiveAccessProbe(t.Context(), nil, []pathAccess{{path: path}}), "effectively writable")
	require.NoError(t, os.Chmod(path, 0))
	require.NoError(t, runEffectiveAccessProbe(t.Context(), nil, []pathAccess{{path: path, private: true}}))
}

func TestAuthorityEffectiveAccessNeverEvaluatesPathArguments(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("current-credential probe requires an unprivileged test runner")
	}
	path := filepath.Join(t.TempDir(), "literal; printf injected")
	require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o400))
	require.NoError(t, runEffectiveAccessProbe(t.Context(), nil, []pathAccess{{path: path}}))
	require.Error(t, runEffectiveAccessProbe(t.Context(), nil, nil))
}

func TestAuthorityEffectiveAccessIncludesStoreSocketCodeAndAncestors(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	state := filepath.Join(root, "private-store")
	sockets := filepath.Join(root, "sockets")
	binary := filepath.Join(root, "engine")
	require.NoError(t, os.Mkdir(state, 0o700))
	require.NoError(t, os.Mkdir(sockets, 0o755))
	require.NoError(t, os.WriteFile(binary, []byte("fixture"), 0o555))
	checks, err := protectedPathAccess(Config{StateDir: state, SocketPath: filepath.Join(sockets, "authority.sock"), AllowedExecutables: []Executable{{Path: binary}}})
	require.NoError(t, err)
	modes := make(map[string]bool)
	for _, check := range checks {
		modes[check.path] = check.private
	}
	require.True(t, modes[state])
	require.Contains(t, modes, sockets)
	require.False(t, modes[sockets])
	require.Contains(t, modes, binary)
	require.Contains(t, modes, root)
	require.Contains(t, modes, string(filepath.Separator))
	for ancestor := filepath.Dir(root); ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Stat(ancestor)
		require.NoError(t, err)
		uid, err := fileUID(info)
		require.NoError(t, err)
		if info.Mode()&os.ModeSticky != 0 && uid == 0 {
			require.NotContains(t, modes, ancestor, "a root-owned sticky ancestor protects its owned child without forbidding unrelated temporary-file writes")
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
}
