//go:build !darwin && !linux

package approvalauthority

import (
	"errors"
	"io"
	"net"
	"os"
)

func unsupported() error {
	return errors.New("protected approval authority is unsupported on this platform")
}
func fileUID(os.FileInfo) (uint32, error)            { return 0, unsupported() }
func peerIdentity(*net.UnixConn) (identity, error)   { return identity{}, unsupported() }
func peerExecutable(int) (string, error)             { return "", unsupported() }
func peerExecutableHash(int, string) (string, error) { return "", unsupported() }
func verifyPeerRuntime(int, string) error            { return unsupported() }
func prepareClientIdentity() error                   { return unsupported() }
func lockStorage(string) (func(), error)             { return nil, unsupported() }

func authenticatedReader(*net.UnixConn, identity) (io.Reader, error) { return nil, unsupported() }

func validateExecutable(string) error { return unsupported() }

func validateEngineAccount(uint32) error { return unsupported() }

const DefaultSocketPath = "/var/run/megafloyd-approval-authority.sock"

func validateLocalEngineAccount() (localPolicyStatus, error) {
	return localPolicyDeferred, unsupported()
}

func validateEffectiveAccess(Config, uint32) error { return unsupported() }

func validateNativeACL([]pathAccess) error { return unsupported() }
