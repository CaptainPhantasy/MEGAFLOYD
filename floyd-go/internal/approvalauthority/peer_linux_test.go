//go:build linux

package approvalauthority

import (
	"debug/elf"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorityRejectsDynamicExecutableRuntime(t *testing.T) {
	for _, path := range []string{"/bin/sh", "/usr/bin/env"} {
		file, err := elf.Open(path)
		if err != nil {
			continue
		}
		dynamic := false
		for _, program := range file.Progs {
			dynamic = dynamic || program.Type == elf.PT_INTERP
		}
		file.Close()
		if dynamic {
			require.ErrorContains(t, verifyPeerRuntime(os.Getpid(), path), "static engine")
			return
		}
	}
	t.Skip("no dynamic ELF system executable available")
}
