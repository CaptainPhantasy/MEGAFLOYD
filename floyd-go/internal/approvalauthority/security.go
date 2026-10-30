package approvalauthority

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// Package initialization precedes configuration and model tools. On Linux this
// disables same-user debugger access before any untrusted command can run.
var clientIdentityError = prepareClientIdentity()

type identity struct {
	uid uint32
	pid int
}

func (s *server) authenticate(conn *net.UnixConn) error {
	peer, err := peerIdentity(conn)
	if err != nil {
		return err
	}
	if peer.uid == uint32(os.Getuid()) || !slices.Contains(s.config.AllowedUIDs, peer.uid) {
		return errors.New("unauthorized approval authority UID")
	}
	executable, err := peerExecutable(peer.pid)
	if err != nil {
		return err
	}
	for _, allowed := range s.config.AllowedExecutables {
		if executable != allowed.Path {
			continue
		}
		if err := immutablePath(allowed.Path, false, 0); err != nil {
			return err
		}
		if err := verifyPeerRuntime(peer.pid, allowed.Path); err != nil {
			return err
		}
		digest, err := peerExecutableHash(peer.pid, allowed.Path)
		if err != nil {
			return err
		}
		if digest != allowed.SHA256 {
			return errors.New("approval authority executable digest mismatch")
		}
		// Only pinned images may reach account-policy or effective-access helper
		// subprocesses. Untrusted shell clients are rejected without spawning them.
		if err := validateEngineAccount(peer.uid); err != nil {
			return err
		}
		if err := validateEffectiveAccess(s.config, peer.uid); err != nil {
			return err
		}
		// Recheck peer credentials after image inspection; an exited/replaced peer
		// or an image transition is not accepted based on stale observations.
		again, err := peerIdentity(conn)
		if err != nil || again != peer {
			return errors.New("approval authority peer changed")
		}
		image, err := peerExecutable(peer.pid)
		if err != nil || image != executable {
			return errors.New("approval authority executable changed")
		}
		return nil
	}
	return errors.New("untrusted approval authority executable")
}

func fileHash(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validateConfig(c Config) error {
	if len(c.AllowedUIDs) == 0 || len(c.AllowedExecutables) == 0 {
		return errors.New("approval authority requires explicit engine UID and executable pins")
	}
	for _, uid := range c.AllowedUIDs {
		if err := validateEngineAccount(uid); err != nil {
			return err
		}
		if uid == uint32(os.Getuid()) {
			return errors.New("approval authority and engine must use separate OS identities")
		}
	}
	if !filepath.IsAbs(c.SocketPath) || !filepath.IsAbs(c.StateDir) {
		return errors.New("approval authority paths must be absolute")
	}
	if err := immutablePath(filepath.Dir(c.SocketPath), true, uint32(os.Getuid())); err != nil {
		return err
	}
	if err := immutablePath(c.StateDir, true, uint32(os.Getuid())); err != nil {
		return err
	}
	info, err := os.Stat(c.StateDir)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		return errors.New("approval authority storage must have mode 0700")
	}
	if info, err := os.Lstat(c.SocketPath); err == nil {
		uid, ownerErr := fileUID(info)
		if ownerErr != nil || uid != uint32(os.Getuid()) || info.Mode()&os.ModeSocket == 0 {
			return errors.New("approval authority existing socket has wrong type or owner")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for _, allowed := range c.AllowedExecutables {
		if len(allowed.SHA256) != 64 {
			return errors.New("approval authority requires a SHA256 executable pin")
		}
		decoded, err := hex.DecodeString(allowed.SHA256)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(allowed.SHA256) != allowed.SHA256 {
			return errors.New("invalid approval authority executable pin")
		}
		if err := immutablePath(allowed.Path, false, 0); err != nil {
			return err
		}
		if err := validateExecutable(allowed.Path); err != nil {
			return err
		}
		digest, err := fileHash(allowed.Path)
		if err != nil {
			return err
		}
		if digest != allowed.SHA256 {
			return errors.New("approval authority executable pin does not match")
		}
	}
	for _, uid := range c.AllowedUIDs {
		if err := validateEffectiveAccess(c, uid); err != nil {
			return err
		}
	}
	return nil
}

// immutablePath requires root-owned pinned code and service-owned state. A
// root-owned sticky ancestor such as /private/tmp protects its owned children;
// the protected target itself must never be writable by other identities.
func immutablePath(target string, directory bool, owner uint32) error {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return errors.New("approval authority requires canonical absolute paths")
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	if resolved != target {
		return errors.New("approval authority protected paths cannot use symlinks")
	}
	current := target
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		uid, err := fileUID(info)
		if err != nil {
			return err
		}
		expectedOwner := uint32(0)
		if current == target {
			expectedOwner = owner
		}
		if uid != expectedOwner {
			return fmt.Errorf("approval authority protected path has unexpected owner: %s", current)
		}
		if current == target && info.IsDir() != directory {
			return errors.New("approval authority protected path has wrong type")
		}
		if !directory && current == target && !info.Mode().IsRegular() {
			return errors.New("approval authority executable is not a regular file")
		}
		writable := info.Mode().Perm()&0o022 != 0
		stickyAncestor := current != target && info.IsDir() && info.Mode()&os.ModeSticky != 0 && uid == 0
		if writable && !stickyAncestor {
			return fmt.Errorf("approval authority protected path is writable by other identities: %s", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func validateSocket(socket string, owner uint32) error {
	if !filepath.IsAbs(socket) {
		return errors.New("approval authority socket path must be absolute")
	}
	if err := immutablePath(filepath.Dir(socket), true, owner); err != nil {
		return err
	}
	info, err := os.Lstat(socket)
	if err != nil {
		return err
	}
	uid, err := fileUID(info)
	if err != nil {
		return err
	}
	if uid != owner || info.Mode()&os.ModeSocket == 0 {
		return errors.New("approval authority socket identity mismatch")
	}
	return nil
}

// quarantineStaleSocket preserves a confirmed stale service-owned socket rather
// than unlinking it. A live endpoint or changed inode is never replaced.
func quarantineStaleSocket(socket string) error {
	before, err := os.Lstat(socket)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	uid, err := fileUID(before)
	if err != nil {
		return err
	}
	if uid != uint32(os.Getuid()) || before.Mode()&os.ModeSocket == 0 {
		return errors.New("approval authority cannot replace foreign socket object")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	conn, dialErr := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if dialErr == nil {
		conn.Close()
		return errors.New("approval authority socket is already active")
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		return fmt.Errorf("approval authority socket liveness is uncertain: %w", dialErr)
	}
	after, err := os.Lstat(socket)
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return errors.New("approval authority socket changed during recovery")
	}
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	destination := socket + ".stale-" + hex.EncodeToString(suffix[:])
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return errors.New("approval authority socket quarantine destination is occupied")
	}
	return os.Rename(socket, destination)
}

// pathAccess distinguishes public immutable code/socket ancestors from private
// authority storage, which the engine must not read, write, or search.
type pathAccess struct {
	path    string
	private bool
}

func protectedPathAccess(c Config) ([]pathAccess, error) {
	targets := []pathAccess{{path: c.StateDir, private: true}, {path: filepath.Dir(c.SocketPath)}}
	for _, executable := range c.AllowedExecutables {
		targets = append(targets, pathAccess{path: executable.Path})
	}
	modes := make(map[string]bool)
	for _, target := range targets {
		if !filepath.IsAbs(target.path) || filepath.Clean(target.path) != target.path {
			return nil, errors.New("approval authority effective-access path is not canonical")
		}
		for current := target.path; ; current = filepath.Dir(current) {
			info, err := os.Lstat(current)
			if err != nil {
				return nil, err
			}
			owner, err := fileUID(info)
			if err != nil {
				return nil, err
			}
			stickyAncestor := current != target.path && info.IsDir() && info.Mode()&os.ModeSticky != 0 && owner == 0
			if !stickyAncestor {
				modes[current] = modes[current] || (current == target.path && target.private)
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	paths := make([]string, 0, len(modes))
	for path := range modes {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	checks := make([]pathAccess, 0, len(paths))
	for _, path := range paths {
		checks = append(checks, pathAccess{path: path, private: modes[path]})
	}
	return checks, nil
}
