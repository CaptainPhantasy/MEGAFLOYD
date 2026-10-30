//go:build linux

package approvalauthority

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// DefaultSocketPath uses the canonical system runtime directory.
const DefaultSocketPath = "/run/megafloyd-approval-authority.sock"

func fileUID(info os.FileInfo) (uint32, error) { return statUID(info) }
func peerIdentity(conn *net.UnixConn) (identity, error) {
	var peer identity
	var peerErr error
	raw, err := conn.SyscallConn()
	if err != nil {
		return peer, err
	}
	err = raw.Control(func(fd uintptr) {
		credentials, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			peerErr = e
			return
		}
		peer = identity{uid: credentials.Uid, pid: int(credentials.Pid)}
	})
	if err != nil {
		return peer, err
	}
	if peer.pid <= 0 && peerErr == nil {
		peerErr = errors.New("approval authority peer PID unavailable")
	}
	return peer, peerErr
}

func peerExecutable(pid int) (string, error) { return os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)) }

func peerExecutableHash(pid int, _ string) (string, error) {
	return fileHash(fmt.Sprintf("/proc/%d/exe", pid))
}

func verifyPeerRuntime(pid int, path string) error {
	if err := validateExecutable(path); err != nil {
		return err
	}
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "TracerPid:") {
			tracer, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "TracerPid:")))
			if err != nil || tracer != 0 {
				return errors.New("approval authority rejects traced engine")
			}
			return nil
		}
	}
	return errors.New("approval authority tracer status unavailable")
}
func prepareClientIdentity() error { return unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) }

// credentialReader checks the actual sender of every stream chunk. Connection
// credentials alone would let a forked child write through an inherited FD.
type credentialReader struct {
	conn     *net.UnixConn
	expected identity
}

func (r credentialReader) Read(data []byte) (int, error) {
	control := make([]byte, unix.CmsgSpace(unix.SizeofUcred))
	n, controlN, flags, _, err := r.conn.ReadMsgUnix(data, control)
	if err != nil {
		return 0, err
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return 0, errors.New("truncated approval authority credentials")
	}
	messages, err := unix.ParseSocketControlMessage(control[:controlN])
	if err != nil {
		return 0, err
	}
	verified := false
	for _, message := range messages {
		if message.Header.Level != unix.SOL_SOCKET || message.Header.Type != unix.SCM_CREDENTIALS {
			return 0, errors.New("unexpected approval authority control message")
		}
		credentials, err := unix.ParseUnixCredentials(&message)
		if err != nil || credentials.Uid != r.expected.uid || int(credentials.Pid) != r.expected.pid {
			return 0, errors.New("approval authority message sender mismatch")
		}
		verified = true
	}
	if !verified {
		return 0, errors.New("approval authority message credentials unavailable")
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func authenticatedReader(conn *net.UnixConn, expected identity) (io.Reader, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var optionErr error
	if err := raw.Control(func(fd uintptr) { optionErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PASSCRED, 1) }); err != nil {
		return nil, err
	}
	if optionErr != nil {
		return nil, optionErr
	}
	return credentialReader{conn: conn, expected: expected}, nil
}

func validateExecutable(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, program := range f.Progs {
		if program.Type == elf.PT_INTERP {
			return errors.New("approval authority requires a static engine to prevent loader injection")
		}
	}
	return nil
}

func authorityTransportSupported() error { return nil }
