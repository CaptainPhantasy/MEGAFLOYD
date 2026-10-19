// Package main is the entry point for the Floyd CLI.
//
//	@title			Floyd API
//	@version		1.0
//	@description	Floyd is a terminal-based AI coding assistant. This API is served over a Unix socket (or Windows named pipe) and provides programmatic access to workspaces, sessions, agents, LSP, MCP, and more.
//	@contact.name	Charm
//	@contact.url	https://charm.sh
//	@license.name	MIT
//	@license.url	https://github.com/legacy-ai/floyd/blob/main/LICENSE
//	@BasePath		/v1
package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/legacy-ai/floyd/internal/cmd"
	_ "github.com/legacy-ai/floyd/internal/dns"
	"github.com/legacy-ai/floyd/internal/megafloyd"
)

func main() {
	// When the binary is named megafloyd, default its home to ~/.megafloyd by
	// exporting the FLOYD_* variables the chassis reads. Other names keep the
	// chassis defaults.
	megafloyd.ApplyFromExecutable()

	// MEGAFLOYD_BINARY_NAME selects the megafloyd identity for executables
	// that cannot carry that name themselves (a renamed copy or a shim).
	cmd.SetBinaryNameOverride(os.Getenv("MEGAFLOYD_BINARY_NAME"))

	if os.Getenv("FLOYD_PROFILE") != "" {
		go func() {
			slog.Info("Serving pprof at localhost:6060")
			if httpErr := http.ListenAndServe("localhost:6060", nil); httpErr != nil {
				slog.Error("Failed to pprof listen", "error", httpErr)
			}
		}()
	}

	cmd.Execute()
}
