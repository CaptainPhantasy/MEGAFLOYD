package permission

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	ModeManual = "manual"
	ModeYolo   = "yolo"
	ModeAuto   = "auto"
)

type ModeService interface {
	SetMode(string) error
	Mode() string
}

func (s *permissionService) SetMode(mode string) error {
	switch mode {
	case ModeManual:
		s.policyMode.Store(0)
		s.skip.Store(false)
	case ModeYolo:
		s.policyMode.Store(1)
		s.skip.Store(false)
	case ModeAuto:
		s.policyMode.Store(0)
		s.skip.Store(true)
	default:
		return fmt.Errorf("unknown permission mode %q", mode)
	}
	return nil
}

func (s *permissionService) Mode() string {
	if s.skip.Load() {
		return ModeAuto
	}
	if s.policyMode.Load() == 1 {
		return ModeYolo
	}
	return ModeManual
}

func sensitivePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		name := strings.ToLower(part)
		if name == ".git" || name == ".ssh" || name == ".aws" || name == ".gnupg" || name == ".env" || strings.HasPrefix(name, ".env.") || strings.Contains(name, "credential") || name == "id_rsa" || name == "id_ed25519" {
			return true
		}
	}
	return false
}

func (s *permissionService) routineRequest(req CreatePermissionRequest) bool {
	if req.Path != "" && (sensitivePath(req.Path) || s.ValidatePath(req.Path) != nil) {
		return false
	}
	switch req.ToolName {
	case "view", "read", "read_file", "glob", "grep", "ls", "lsp", "edit", "write", "multi_edit":
		return true
	case "bash":
		data, err := json.Marshal(req.Params)
		if err != nil {
			return false
		}
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return false
		}
		return routineCommand(input.Command)
	}
	for _, suffix := range []string{"_health_check", "_list_servers", "_search_tools", "_describe_tool", "_list_resources", "_read_resource"} {
		if strings.HasPrefix(req.ToolName, "mcp_mcp-gateway_") && strings.HasSuffix(req.ToolName, suffix) {
			return true
		}
	}
	return false
}

func literalWord(w *syntax.Word) (string, bool) {
	var value strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			value.WriteString(p.Value)
		case *syntax.SglQuoted:
			value.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				value.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return value.String(), true
}

func routineCommand(command string) bool {
	f, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil || len(f.Stmts) != 1 {
		return false
	}
	stmt := f.Stmts[0]
	if stmt.Background || stmt.Coprocess || len(stmt.Redirs) != 0 {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return false
	}
	args := make([]string, len(call.Args))
	for i, word := range call.Args {
		var valid bool
		args[i], valid = literalWord(word)
		if !valid || sensitivePath(args[i]) {
			return false
		}
	}
	name := filepath.Base(args[0])
	if strings.Contains(args[0], "/") && args[0] != "/bin/"+name && args[0] != "/usr/bin/"+name && args[0] != "/opt/homebrew/bin/"+name {
		return false
	}
	switch name {
	case "pwd", "ls", "cat", "head", "tail", "wc", "stat", "file", "echo", "printf", "true", "false", "whoami", "hostname", "uname":
		return true
	case "rg", "grep":
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "--pre") || strings.HasPrefix(arg, "--hostname-bin") {
				return false
			}
		}
		return true
	case "git":
		if len(args) < 2 {
			return false
		}
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "--output") || strings.HasPrefix(arg, "--ext-diff") || strings.HasPrefix(arg, "--textconv") {
				return false
			}
		}
		switch args[1] {
		case "status", "diff", "log", "show", "rev-parse", "ls-files", "ls-tree", "version":
			return true
		case "branch":
			return len(args) == 3 && args[2] == "--show-current"
		}
	}
	return false
}
