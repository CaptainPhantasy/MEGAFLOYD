package approvalauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthorityRevisionAndRestart(t *testing.T) {
	s := &server{config: Config{StateDir: t.TempDir()}}
	got := s.apply(1001, request{Operation: "read", Namespace: "workspace", Session: "session"})
	require.Empty(t, got.Error)
	require.Zero(t, got.Revision)
	require.Nil(t, got.Data)
	original := []byte(`{"confirmations":["human-one","human-two"]}`)
	got = s.apply(1001, request{Operation: "write", Namespace: "workspace", Session: "session", Data: original})
	require.Empty(t, got.Error)
	require.Equal(t, uint64(1), got.Revision)
	// A workspace file or a stale writer cannot overwrite the protected state.
	rejected := s.apply(1001, request{Operation: "write", Namespace: "workspace", Session: "session", Data: []byte("forged")})
	require.Contains(t, rejected.Error, "revision conflict")
	s = &server{config: s.config}
	got = s.apply(1001, request{Operation: "read", Namespace: "workspace", Session: "session"})
	require.Empty(t, got.Error)
	require.Equal(t, uint64(1), got.Revision)
	require.Equal(t, original, got.Data)
	got = s.apply(1001, request{Operation: "write", Namespace: "workspace", Session: "session", Revision: 1, Data: []byte("reconciled")})
	require.Empty(t, got.Error)
	require.Equal(t, uint64(2), got.Revision)
	require.Contains(t, s.apply(1001, request{Operation: "write", Namespace: "workspace", Session: "session", Revision: 1, Data: original}).Error, "revision conflict")
}

func TestAuthoritySeparatesIdentityAndRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s := &server{config: Config{StateDir: dir}}
	got := s.apply(1001, request{Operation: "write", Namespace: "../../outside", Session: "../../../record.json", Data: []byte("approved")})
	require.Empty(t, got.Error)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Regexp(t, "^[0-9a-f]{64}$", entries[0].Name())
	for _, req := range []request{{Operation: "read", Namespace: "different", Session: "../../../record.json"}, {Operation: "read", Namespace: "../../outside", Session: "different"}} {
		got = s.apply(1001, req)
		require.Empty(t, got.Error)
		require.Nil(t, got.Data)
	}
	for _, req := range []request{{Operation: "sign", Namespace: "n", Session: "s"}, {Operation: "write", Namespace: "n", Session: "s"}, {Operation: "read", Namespace: "", Session: "s"}, {Operation: "read", Namespace: "n", Session: strings.Repeat("s", 4097)}} {
		require.NotEmpty(t, s.apply(1001, req).Error)
	}
}

func TestAuthorityCompareAndSwapSerializesConcurrentWriters(t *testing.T) {
	s := &server{config: Config{StateDir: t.TempDir()}}
	var passed atomic.Int32
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Go(func() {
			got := s.apply(1001, request{Operation: "write", Namespace: "n", Session: "s", Data: []byte(strconv.Itoa(i))})
			if got.Error == "" {
				passed.Add(1)
			}
		})
	}
	workers.Wait()
	require.Equal(t, int32(1), passed.Load())
}

func TestAuthorityRejectsMalformedProtectedState(t *testing.T) {
	dir := t.TempDir()
	ns := sha256.Sum256([]byte("1001\x00n"))
	sid := sha256.Sum256([]byte("s"))
	parent := filepath.Join(dir, hex.EncodeToString(ns[:]))
	require.NoError(t, os.Mkdir(parent, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(parent, hex.EncodeToString(sid[:])+".json"), []byte(`{"revision":0,"data":""}`), 0o600))
	s := &server{config: Config{StateDir: dir}}
	require.NotEmpty(t, s.apply(1001, request{Operation: "read", Namespace: "n", Session: "s"}).Error)
}

func TestAuthorityRejectsUnpinnedSameUserPeer(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		s := &server{config: Config{AllowedUIDs: []uint32{uint32(os.Getuid())}}}
		done <- s.authenticate(conn)
	}()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer conn.Close()
	require.ErrorContains(t, <-done, "UID")
	require.ErrorContains(t, NewClient(socket, uint32(os.Getuid())).CompareAndSwap(t.Context(), "n", "s", 0, []byte("forged")), "separate OS identity")
}

func TestAuthorityProtocolAuthenticatesBeforeReading(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		s := &server{authorize: func(*net.UnixConn) error { return os.ErrPermission }}
		s.handle(conn)
	}()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	var result response
	require.NoError(t, json.NewDecoder(conn).Decode(&result))
	require.Contains(t, result.Error, "rejected peer")
	<-done
}

func TestAuthorityRejectsMutableOrSymlinkPaths(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "engine")
	require.NoError(t, os.WriteFile(target, []byte("image"), 0o777))
	require.Error(t, immutablePath(target, false, 0), "user-controlled code cannot be pinned")
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))
	require.ErrorContains(t, immutablePath(link, false, uint32(os.Getuid())), "symlinks")
	require.Error(t, immutablePath(filepath.Join(dir, "..", filepath.Base(dir), "engine"), false, 0))
	require.Error(t, validateSocket(target, uint32(os.Getuid())))
	require.Error(t, validateConfig(Config{}))
	require.Error(t, validateConfig(Config{AllowedUIDs: []uint32{uint32(os.Getuid())}, AllowedExecutables: []Executable{{Path: target, SHA256: strings.Repeat("0", 64)}}}))
}

func TestAuthorityStorageLockPreventsSecondBroker(t *testing.T) {
	dir := t.TempDir()
	unlock, err := lockStorage(dir)
	require.NoError(t, err)
	defer unlock()
	_, err = lockStorage(dir)
	require.ErrorContains(t, err, "already in use")
}

func TestAuthorityServiceRejectsExistingSocket(t *testing.T) {
	// Configuration is checked before the server can replace an existing object.
	socket := filepath.Join(t.TempDir(), "socket")
	require.NoError(t, os.WriteFile(socket, []byte("preserve"), 0o600))
	err := Serve(context.Background(), Config{SocketPath: socket})
	require.Error(t, err)
	content, readErr := os.ReadFile(socket)
	require.NoError(t, readErr)
	require.True(t, bytes.Equal([]byte("preserve"), content))
}

func TestAuthorityNamespaceIsBoundToKernelUID(t *testing.T) {
	s := &server{config: Config{StateDir: t.TempDir()}}
	require.Empty(t, s.apply(1001, request{Operation: "write", Namespace: "shared-workspace", Session: "shared-session", Data: []byte("first-user")}).Error)
	other := s.apply(1002, request{Operation: "read", Namespace: "shared-workspace", Session: "shared-session"})
	require.Empty(t, other.Error)
	require.Zero(t, other.Revision)
	require.Nil(t, other.Data)
	require.Empty(t, s.apply(1002, request{Operation: "write", Namespace: "shared-workspace", Session: "shared-session", Data: []byte("second-user")}).Error)
	first := s.apply(1001, request{Operation: "read", Namespace: "shared-workspace", Session: "shared-session"})
	require.Equal(t, []byte("first-user"), first.Data)
}

func TestAuthorityPreservesStaleSocketAndRestarts(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	socket := filepath.Join(t.TempDir(), "socket")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	listener.SetUnlinkOnClose(false)
	require.ErrorContains(t, quarantineStaleSocket(socket), "already active")
	require.NoError(t, listener.Close())
	require.NoError(t, quarantineStaleSocket(socket))
	preserved, err := filepath.Glob(socket + ".stale-*")
	require.NoError(t, err)
	require.Len(t, preserved, 1)
	info, err := os.Lstat(preserved[0])
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
	restarted, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer restarted.Close()
}

func TestAuthorityChallengeRejectsPrequeuedOrWrongNonce(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	for _, prequeue := range []bool{false, true} {
		t.Run(strconv.FormatBool(prequeue), func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "s")
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
			require.NoError(t, err)
			defer listener.Close()
			s := &server{config: Config{StateDir: t.TempDir()}, authorize: func(*net.UnixConn) error { return nil }}
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.AcceptUnix()
				if err == nil {
					defer conn.Close()
					s.handle(conn)
				}
			}()
			conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
			require.NoError(t, err)
			defer conn.Close()
			wrong := request{Nonce: strings.Repeat("0", 64), Operation: "write", Namespace: "n", Session: "s", Data: []byte("forged")}
			if prequeue {
				require.NoError(t, json.NewEncoder(conn).Encode(wrong))
			}
			decoder := json.NewDecoder(conn)
			var greeting challenge
			require.NoError(t, decoder.Decode(&greeting))
			require.Len(t, greeting.Nonce, 64)
			if !prequeue {
				require.NoError(t, json.NewEncoder(conn).Encode(wrong))
			}
			var result response
			require.NoError(t, decoder.Decode(&result))
			if prequeue {
				// Linux may reject bytes queued before SO_PASSCRED was enabled
				// at the credential reader, before the nonce can be inspected.
				require.Contains(t, []string{"invalid approval authority request", "approval authority rejected request sender"}, result.Error)
			} else {
				require.Equal(t, "approval authority rejected request sender", result.Error)
			}
			require.Zero(t, result.Revision)
			require.Empty(t, result.Data)
			<-done
			require.Nil(t, s.apply(uint32(os.Getuid()), request{Operation: "read", Namespace: "n", Session: "s"}).Data)
		})
	}
}

func TestAuthorityInheritedDescriptorHelper(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	payload := os.Getenv("MEGAFLOYD_AUTHORITY_TEST_PAYLOAD")
	if payload == "" {
		return
	}
	file := os.NewFile(3, "inherited-authority-socket")
	if _, err := file.Write([]byte(payload)); err != nil {
		os.Exit(2)
	}
	file.Close()
	os.Exit(0)
}

func TestAuthorityRejectsInheritedDescriptorSender(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer listener.Close()
	s := &server{config: Config{StateDir: t.TempDir()}, authorize: func(*net.UnixConn) error { return nil }}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.AcceptUnix()
		if err == nil {
			defer conn.Close()
			s.handle(conn)
		}
	}()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	var greeting challenge
	require.NoError(t, decoder.Decode(&greeting))
	inherited, err := conn.File()
	require.NoError(t, err)
	defer inherited.Close()
	payload, err := json.Marshal(request{Nonce: greeting.Nonce, Operation: "write", Namespace: "n", Session: "s", Data: []byte("forged-child-write")})
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)
	child := exec.CommandContext(t.Context(), executable, "-test.run=^TestAuthorityInheritedDescriptorHelper$")
	child.ExtraFiles = []*os.File{inherited}
	child.Env = append(os.Environ(), "MEGAFLOYD_AUTHORITY_TEST_PAYLOAD="+string(payload)+"\n")
	require.NoError(t, child.Run())
	var result response
	require.NoError(t, decoder.Decode(&result))
	require.NotEmpty(t, result.Error)
	<-done
	require.Nil(t, s.apply(uint32(os.Getuid()), request{Operation: "read", Namespace: "n", Session: "s"}).Data)
}

func requireSupportedAuthorityTransport(t *testing.T) {
	t.Helper()
	if err := authorityTransportSupported(); err != nil {
		t.Skip("supported approval transport required: " + err.Error())
	}
}
