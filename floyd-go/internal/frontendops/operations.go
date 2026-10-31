// Package frontendops runs user-invoked maintenance operations where the
// workspace lives, with the same implementations for local and remote clients.
package frontendops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/codebase"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/enginebridge"
	"github.com/legacy-ai/floyd/internal/engineexport"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	memory "github.com/legacy-ai/floyd/internal/megafloyd/memory"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/usage"
	"github.com/legacy-ai/floyd/internal/version"
)

type Environment struct {
	Config   *config.ConfigStore
	Sessions session.Service
	Messages message.Service
	Hooks    *hookruntime.Runtime
}

func Run(ctx context.Context, e Environment, in proto.FrontendRequest) (proto.FrontendResult, error) {
	if err := ctx.Err(); err != nil {
		return proto.FrontendResult{}, err
	}
	if e.Config == nil || e.Config.Config() == nil {
		return proto.FrontendResult{}, errors.New("workspace configuration is unavailable")
	}
	cfg := e.Config.Config()
	cwd := e.Config.WorkingDir()
	result := proto.FrontendResult{Title: in.Kind}
	var data any
	var err error
	switch in.Kind {
	case "hooks":
		switch in.Action {
		case "explain":
			data, err = tools.ExplainHooks(cfg, in.Argument, in.ToolName, cwd, "effective_workspace")
		case "trace":
			if in.SessionID != "" {
				if _, err = e.Sessions.Get(ctx, in.SessionID); err != nil {
					return result, err
				}
			}
			var query hookstore.Query
			query, err = (tools.HookTraceParams{SessionID: in.SessionID, ToolCallID: in.ToolCallID, EventID: in.EventID, JobID: in.JobID, After: in.After, Limit: in.Limit}).Query(in.SessionID)
			if err != nil {
				return result, err
			}
			if e.Hooks != nil && e.Hooks.Store() != nil {
				data, err = e.Hooks.Store().List(ctx, query)
			} else {
				reader, closer, openErr := hookstore.OpenReader(cfg.Options.DataDirectory)
				if openErr != nil {
					return result, openErr
				}
				if closer != nil {
					defer closer.Close()
				}
				data, err = reader.List(ctx, query)
			}
		default:
			err = errors.New("usage: /hooks explain <event> [tool] | trace")
		}
	case "doctor":
		result.Title = "Doctor"
		result.Body = doctor(e.Config)
		return result, nil
	case "codebase":
		switch in.Action {
		case "", "analyze":
			data, err = codebase.AnalyzeCodebase(cwd, codebase.AnalyzeOptions{})
		case "deps":
			data, err = codebase.AnalyzeProjectDependencies(cwd)
		case "search":
			if in.Argument == "" {
				return result, errors.New("usage: /codebase search <text>")
			}
			data, err = codebase.FindFilesByContent(cwd, in.Argument, codebase.SearchOptions{})
		default:
			err = errors.New("usage: /codebase analyze | deps | search <text>")
		}
	case "memory":
		dir, err := megafloyd.PersonalDataDir(cwd, cfg.Options.DataDirectory)
		if err != nil {
			return result, err
		}
		result.Path = filepath.Join(dir, memory.FileName)
		raw, err := os.ReadFile(result.Path)
		if errors.Is(err, os.ErrNotExist) {
			result.Body = "No memory checkpoint exists yet.\n" + result.Path
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if len(raw) > memory.MaxSize {
			return result, errors.New("memory checkpoint exceeds its supported size")
		}
		var checkpoint memory.Checkpoint
		if err = json.Unmarshal(raw, &checkpoint); err != nil {
			return result, fmt.Errorf("read memory checkpoint: %w", err)
		}
		data = checkpoint
	case "session":
		return sessionTransfer(ctx, e, in)
	case "telemetry":
		return telemetry(ctx, e, in)
	default:
		return result, fmt.Errorf("unknown frontend operation %q", in.Kind)
	}
	if err != nil {
		return result, err
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return result, err
	}
	result.Body = string(encoded)
	return result, nil
}

func doctor(store *config.ConfigStore) string {
	cfg := store.Config()
	var b strings.Builder
	exe, _ := os.Executable()
	fmt.Fprintf(&b, "Binary: %s\nVersion: %s (build %s)\nPlatform: %s/%s\nWorkspace: %s\nData: %s\n", exe, version.Version, version.BuildID, runtime.GOOS, runtime.GOARCH, store.WorkingDir(), cfg.Options.DataDirectory)
	if _, paths, err := config.Inspect(context.Background(), store.WorkingDir(), cfg.Options.DataDirectory); err != nil {
		fmt.Fprintf(&b, "Configuration: INVALID: %v\n", err)
	} else {
		fmt.Fprintf(&b, "Configuration: %d files checked\n", len(paths))
	}
	for _, name := range []string{"git", "rg", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			fmt.Fprintf(&b, "%s: missing from PATH\n", name)
		} else {
			fmt.Fprintf(&b, "%s: %s\n", name, path)
		}
	}
	keys := []string{}
	for slot := range cfg.Models {
		keys = append(keys, string(slot))
	}
	sort.Strings(keys)
	for _, key := range keys {
		selected := cfg.Models[config.SelectedModelType(key)]
		state := "configured"
		if !cfg.IsModelAvailable(selected.Provider, selected.Model) {
			state = "missing"
		}
		fmt.Fprintf(&b, "Model %s: %s/%s (%s)\n", key, selected.Provider, selected.Model, state)
	}
	check := prompt.CheckStandard(prompt.StandardPolicySHA256(), prompt.StandardCandidates())
	fmt.Fprintf(&b, "Loaded instruction SHA-256: %s\n", check.LoadedSHA256)
	if check.BundlePresent && !check.Match {
		b.WriteString("Instruction bundle: MISMATCH; rebuild with the approved standard.\n")
	}
	b.WriteString("Provider credentials and live connectivity were not probed.\n")
	return b.String()
}

func sessionTransfer(ctx context.Context, e Environment, in proto.FrontendRequest) (proto.FrontendResult, error) {
	out := proto.FrontendResult{Title: "Session " + in.Action}
	switch in.Action {
	case "export":
		id := in.Argument
		if id == "" {
			id = in.SessionID
		}
		stored, err := e.Sessions.Get(ctx, id)
		if err != nil {
			return out, err
		}
		if err = e.Messages.FlushAll(ctx); err != nil {
			return out, err
		}
		messages, err := message.PublicList(ctx, e.Messages, id)
		if err != nil {
			return out, err
		}
		converted := make([]engineexport.Message, 0, len(messages))
		for _, m := range messages {
			at := time.Unix(m.CreatedAt, 0)
			if m.CreatedAt > 1000000000000 {
				at = time.UnixMilli(m.CreatedAt)
			}
			converted = append(converted, engineexport.Message{Role: string(m.Role), Parts: m.Parts, CreatedAt: at})
		}
		exported, err := engineexport.Export(enginebridge.Home(), id, stored.Title, e.Config.WorkingDir(), converted)
		if err != nil {
			return out, err
		}
		out.Path = exported.Dir
		out.SessionID = exported.EngineID
		out.Body = fmt.Sprintf("Exported %d messages.\nEngine session: %s\n%s", exported.Messages, exported.EngineID, exported.Dir)
		return out, nil
	case "import":
		if in.Argument == "" {
			return out, errors.New("usage: /session import <engine-session-id>")
		}
		found, err := enginebridge.Find(enginebridge.Home(), in.Argument)
		if err != nil {
			return out, err
		}
		transcript, err := enginebridge.ReadTranscript(found.Dir)
		if err != nil {
			return out, err
		}
		imported := make([]session.ImportMessage, 0, len(transcript))
		for _, m := range transcript {
			parts, err := message.MarshalParts(m.Parts)
			if err != nil {
				return out, err
			}
			imported = append(imported, session.ImportMessage{Role: m.Role, Parts: parts, CreatedAt: m.CreatedAt})
		}
		result, err := session.ImportEngine(ctx, e.Sessions, found.ID, found.Title, imported)
		if err != nil {
			return out, err
		}
		out.SessionID = result.Session.ID
		out.Body = fmt.Sprintf("Session: %s\nCopied %d messages. Already imported: %t", result.Session.ID, result.Copied, result.Existing)
		return out, nil
	default:
		return out, errors.New("usage: /session export [session-id] | import <engine-session-id>")
	}
}

func telemetry(ctx context.Context, e Environment, in proto.FrontendRequest) (proto.FrontendResult, error) {
	home, err := megafloyd.HomeDir()
	if err != nil {
		return proto.FrontendResult{}, err
	}
	dir := filepath.Join(home, "telemetry")
	path := filepath.Join(dir, "events.jsonl")
	out := proto.FrontendResult{Title: "Local telemetry", Path: path}
	switch in.Action {
	case "collect":
		sessions, err := e.Sessions.List(ctx)
		if err != nil {
			return out, err
		}
		events := []usage.Event{}
		for _, s := range sessions {
			messages, err := message.PublicList(ctx, e.Messages, s.ID)
			if err != nil {
				return out, err
			}
			events = append(events, usage.FromCLI(s.ID, messages)...)
		}
		engine, err := usage.FromEngine(enginebridge.Home())
		if err != nil {
			return out, err
		}
		events = append(events, engine...)
		written, err := usage.Write(path, events)
		if err != nil {
			return out, err
		}
		out.Body = fmt.Sprintf("Collected %d events (%d new) at %s\n\n", len(events), written, path)
	case "", "report":
	case "ship":
		events, err := usage.Load(path)
		if err != nil {
			return out, err
		}
		count, err := usage.Ship(in.Argument, events, false)
		if err != nil {
			return out, err
		}
		out.Body = fmt.Sprintf("Shipped %d events in %d traces to local Jaeger.", len(events), count)
		return out, nil
	default:
		return out, errors.New("usage: /telemetry collect | report | ship")
	}
	events, err := usage.Load(path)
	if err != nil {
		return out, fmt.Errorf("read telemetry; run /telemetry collect first: %w", err)
	}
	summary := usage.Summarize(events)
	if in.Action == "collect" {
		if err = usage.WriteSummary(filepath.Join(dir, "summary.json"), summary); err != nil {
			return out, err
		}
	}
	out.Body += summary.Text()
	return out, nil
}
