// megafloyd-approval-authority is an administrator-run local approval service.
// It offers no command-line client or generic approval-signing operation.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/legacy-ai/floyd/internal/approvalauthority"
)

type stringsFlag []string

func (f *stringsFlag) String() string         { return strings.Join(*f, ",") }
func (f *stringsFlag) Set(value string) error { *f = append(*f, value); return nil }

func main() {
	socket := flag.String("socket", approvalauthority.DefaultSocketPath, "Administrator-owned Unix socket path")
	state := flag.String("state-dir", "", "Existing service-owned private storage directory (0700)")
	var executables, users stringsFlag
	flag.Var(&executables, "allow-executable", "Exact root-owned executable path=sha256 (repeatable)")
	flag.Var(&users, "allow-uid", "Allowed engine OS UID, distinct from service UID (repeatable)")
	flag.Parse()
	config := approvalauthority.Config{SocketPath: *socket, StateDir: *state}
	for _, value := range executables {
		path, digest, ok := strings.Cut(value, "=")
		if !ok {
			fatal(fmt.Errorf("invalid executable pin"))
		}
		config.AllowedExecutables = append(config.AllowedExecutables, approvalauthority.Executable{Path: path, SHA256: digest})
	}
	for _, value := range users {
		uid, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			fatal(err)
		}
		config.AllowedUIDs = append(config.AllowedUIDs, uint32(uid))
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := approvalauthority.Serve(ctx, config); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
