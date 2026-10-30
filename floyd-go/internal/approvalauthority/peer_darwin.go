//go:build darwin

package approvalauthority

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DefaultSocketPath uses the canonical system runtime directory.
const DefaultSocketPath = "/private/var/run/megafloyd-approval-authority.sock"

func fileUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*unix.Stat_t)
	if ok {
		return stat.Uid, nil
	}
	return statUID(info)
}

func peerIdentity(conn *net.UnixConn) (identity, error) {
	var peer identity
	var peerErr error
	raw, err := conn.SyscallConn()
	if err != nil {
		return peer, err
	}
	err = raw.Control(func(fd uintptr) {
		credentials, e := unix.GetsockoptXucred(int(fd), 0, unix.LOCAL_PEERCRED)
		if e != nil {
			peerErr = e
			return
		}
		pid, e := unix.GetsockoptInt(int(fd), 0, unix.LOCAL_PEERPID)
		if e != nil {
			peerErr = e
			return
		}
		peer = identity{uid: credentials.Uid, pid: pid}
	})
	if err != nil {
		return peer, err
	}
	if peer.pid <= 0 && peerErr == nil {
		peerErr = errors.New("approval authority peer PID unavailable")
	}
	return peer, peerErr
}

func peerExecutable(pid int) (string, error) {
	// proc_pidpath is proc_info(PROC_INFO_CALL_PIDINFO, pid,
	// PROC_PIDPATHINFO, 0, buffer, PROC_PIDPATHINFO_MAXSIZE). The kernel
	// returns zero on success; libproc obtains the length from the buffer.
	buffer := make([]byte, 4096)
	//nolint:staticcheck // CGO-disabled engines use the validated Darwin proc_info kernel API.
	_, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, 2, uintptr(pid), 11, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if errno != 0 {
		return "", errno
	}
	end := bytes.IndexByte(buffer, 0)
	if end <= 0 {
		return "", errors.New("invalid approval authority peer executable")
	}
	return string(buffer[:end]), nil
}

func verifyPeerRuntime(pid int, path string) error {
	// Hardened runtime prevents DYLD injection and unapproved debugger access.
	const csOpsStatus = 0
	const csRuntime = 0x00010000
	const csGetTaskAllow = 0x00000004
	const csDebugged = 0x10000000
	var flags uint32
	//nolint:staticcheck // CGO-disabled engines query kernel code-signing flags directly.
	_, _, errno := unix.Syscall6(unix.SYS_CSOPS, uintptr(pid), csOpsStatus, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), 0, 0)
	if errno != 0 {
		return errno
	}
	if flags&csRuntime == 0 || flags&(csGetTaskAllow|csDebugged) != 0 {
		return errors.New("approval authority requires an undebugged hardened engine")
	}
	return validateExecutable(path)
}

func peerExecutableHash(_ int, path string) (string, error) { return fileHash(path) }
func prepareClientIdentity() error                          { return nil }

func authenticatedReader(conn *net.UnixConn, _ identity) (io.Reader, error) { return conn, nil }

// validateExecutable excludes hardened-runtime exceptions that would permit
// loader injection or debugger control despite the runtime flag itself.
func validateExecutable(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	verify := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", path)
	verify.Env = []string{"PATH=/usr/bin:/bin"}
	if err := verify.Run(); err != nil {
		return errors.New("approval authority engine code signature is invalid")
	}
	command := exec.CommandContext(ctx, "/usr/bin/codesign", "--display", "--entitlements", ":-", path)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	output, err := command.Output()
	if err != nil {
		return errors.New("approval authority engine entitlements unavailable")
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(output))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("invalid approval authority engine entitlements")
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "key" {
			continue
		}
		var key string
		if decoder.DecodeElement(&key, &element) != nil {
			return errors.New("invalid approval authority engine entitlement key")
		}
		if strings.HasPrefix(key, "com.apple.security.cs.") || key == "com.apple.security.get-task-allow" {
			return errors.New("approval authority rejects engine runtime exception entitlements")
		}
	}
}
