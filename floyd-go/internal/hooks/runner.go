package hooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/shell"
)

const abandonGrace = time.Second

var runShell = shell.Run

type compiledHook struct {
	cfg     config.HookConfig
	index   int
	matcher *regexp.Regexp
	invalid string
}
type Runner struct {
	hooks      []compiledHook
	cwd        string
	projectDir string
	mu         sync.Mutex
	recorder   func(Execution)
	async      map[string]*asyncState
}

func NewRunner(hs []config.HookConfig, cwd, projectDir string) *Runner {
	r := &Runner{cwd: cwd, projectDir: projectDir}
	for i, h := range hs {
		ch := compiledHook{cfg: h, index: i}
		if h.Matcher != "" {
			var err error
			ch.matcher, err = regexp.Compile(h.Matcher)
			if err != nil {
				ch.invalid = "invalid matcher"
				slog.Warn("Hook matcher failed to compile; skipping hook", "config_index", i, "command_hash", commandHash(h.Command))
			}
		}
		r.hooks = append(r.hooks, ch)
	}
	return r
}

func (r *Runner) Hooks() []config.HookConfig {
	out := []config.HookConfig{}
	for _, h := range r.hooks {
		if h.invalid == "" {
			out = append(out, h.cfg)
		}
	}
	return out
}
func commandHash(command string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(command))) }
func safeName(h compiledHook) string {
	if h.cfg.Name != "" {
		return h.cfg.Name
	}
	return fmt.Sprintf("hook[%d]", h.index)
}

type PlanEntry struct {
	CanApprove    bool   `json:"can_approve"`
	ConfigIndex   int    `json:"config_index"`
	Order         int    `json:"order"`
	Name          string `json:"name"`
	CommandHash   string `json:"command_hash"`
	Matcher       string `json:"matcher,omitempty"`
	Matched       bool   `json:"matched"`
	DuplicateOf   *int   `json:"duplicate_of,omitempty"`
	Mode          string `json:"mode"`
	Required      bool   `json:"required"`
	CanBlock      bool   `json:"can_block"`
	CanRewrite    bool   `json:"can_rewrite"`
	Observational bool   `json:"observational"`
	Error         string `json:"error,omitempty"`
}

func invalidMode(event string, h config.HookConfig) string {
	s, known := EventSemantics(event)
	if strings.TrimSpace(h.Command) == "" {
		return "command is required"
	}
	if h.Async && (h.Required || h.Sequential) {
		return "async observers cannot be required or sequential"
	}
	if h.Required && !s.CanRequire {
		return "event does not support required checks"
	}
	if h.Async && !s.CanAsync {
		return "event does not support async observers"
	}
	if h.Sequential && !known {
		return "unknown event does not support ordered checks"
	}
	return ""
}

// Preview uses the execution planner without executing commands or loading files.
// Duplicate commands with different authority or timeout are separate checks.
func (r *Runner) Preview(event, tool string) []PlanEntry {
	s, _ := EventSemantics(event)
	out := make([]PlanEntry, len(r.hooks))
	seen := map[string]int{}
	ordered := false
	order := 0
	for i, h := range r.hooks {
		row := PlanEntry{ConfigIndex: h.index, Name: safeName(h), CommandHash: commandHash(h.cfg.Command), Matcher: h.cfg.Matcher, Required: h.cfg.Required, Mode: "parallel", CanBlock: s.CanBlock, CanApprove: s.CanApprove, CanRewrite: s.CanRewrite, Observational: s.Observational}
		if h.cfg.Async {
			row.Mode = "async"
			row.CanBlock = false
			row.CanApprove = false
			row.CanRewrite = false
			row.Observational = true
		}
		row.Error = h.invalid
		if row.Error == "" {
			row.Error = invalidMode(event, h.cfg)
		}
		row.Matched = row.Error == "" && (h.matcher == nil || h.matcher.MatchString(tool))
		if row.Matched {
			key := fmt.Sprintf("%s\x00%d\x00%t\x00%t\x00%t", h.cfg.Command, h.cfg.Timeout, h.cfg.Required, h.cfg.Sequential, h.cfg.Async)
			if prev, ok := seen[key]; ok {
				v := prev
				row.DuplicateOf = &v
			} else {
				seen[key] = h.index
				order++
				row.Order = order
				if !h.cfg.Async && (h.cfg.Required || h.cfg.Sequential) {
					ordered = true
				}
			}
		}
		out[i] = row
	}
	if ordered {
		for i := range out {
			if out[i].Mode != "async" {
				out[i].Mode = "ordered"
			}
		}
	}
	return out
}

func (r *Runner) matchingHooks(tool string) []config.HookConfig {
	var out []config.HookConfig
	for _, h := range r.hooks {
		if h.invalid == "" && (h.matcher == nil || h.matcher.MatchString(tool)) {
			out = append(out, h.cfg)
		}
	}
	return out
}

func (r *Runner) Run(ctx context.Context, event, sid, tool, input string) (AggregateResult, error) {
	if canonical, err := config.NormalizeHookEvent(event); err == nil {
		event = canonical
	}
	ctx = ensureEventDetails(ctx)
	plan := r.Preview(event, tool)
	s, _ := EventSemantics(event)
	for i, row := range plan {
		if row.Error != "" && r.hooks[i].cfg.Required {
			a := AggregateResult{RequiredFailure: true, Reason: "Required hook configuration is invalid: " + row.Error}
			if s.CanBlock {
				a.Decision = DecisionDeny
			}
			return a, nil
		}
	}
	type item struct {
		hook   compiledHook
		plan   PlanEntry
		result HookResult
		ran    bool
	}
	var syncItems []*item
	agg := AggregateResult{Decision: DecisionNone}
	for i, row := range plan {
		if !row.Matched || row.DuplicateOf != nil {
			continue
		}
		h := r.hooks[i]
		if h.cfg.Async {
			agg.AsyncJobs = append(agg.AsyncJobs, r.startAsync(ctx, event, sid, tool, input, h, row.Mode))
			continue
		}
		syncItems = append(syncItems, &item{hook: h, plan: row})
	}
	ordered := len(syncItems) > 0 && syncItems[0].plan.Mode == "ordered"
	currentInput := input
	if ordered {
		for _, it := range syncItems {
			if ctx.Err() != nil {
				agg.Cancelled = true
				break
			}
			it.ran = true
			it.result = r.execute(ctx, event, sid, tool, currentInput, it.hook, it.plan.Mode, "")
			if s.CanRewrite && it.result.UpdatedInput != "" {
				if next, err := shallowMerge(currentInput, it.result.UpdatedInput); err == nil {
					currentInput = next
				}
			}
			if it.hook.cfg.Required && failed(it.result) {
				agg.RequiredFailure = true
				break
			}
			if it.result.Cancelled {
				agg.Cancelled = true
				break
			}
			if s.CanBlock && (it.result.Decision == DecisionDeny || it.result.Halt) {
				break
			}
		}
	} else {
		var wg sync.WaitGroup
		for _, it := range syncItems {
			it.ran = true
			wg.Add(1)
			go func() {
				defer wg.Done()
				it.result = r.execute(ctx, event, sid, tool, input, it.hook, it.plan.Mode, "")
			}()
		}
		wg.Wait()
	}
	var results []HookResult
	for _, it := range syncItems {
		if !it.ran {
			continue
		}
		result := it.result
		if result.Cancelled {
			agg.Cancelled = true
		}
		if it.hook.cfg.Required && failed(result) {
			agg.RequiredFailure = true
		}
		if !s.CanBlock {
			result.Decision = DecisionNone
			result.Halt = false
		}
		if !s.CanRewrite {
			result.UpdatedInput = ""
		}
		results = append(results, result)
		agg.Hooks = append(agg.Hooks, HookInfo{Name: safeName(it.hook), Matcher: it.hook.cfg.Matcher, Decision: it.result.Decision.String(), Halt: it.result.Halt, Reason: it.result.Reason, InputRewrite: s.CanRewrite && it.result.UpdatedInput != ""})
	}
	if agg.Cancelled {
		for _, it := range syncItems {
			if it.hook.cfg.Required && !it.ran {
				agg.RequiredFailure = true
			}
		}
	}
	merged := aggregate(results, input)
	agg.Decision = merged.Decision
	agg.Halt = merged.Halt
	agg.Reason = merged.Reason
	agg.Context = merged.Context
	agg.UpdatedInput = merged.UpdatedInput
	agg.HookCount = len(results) + len(agg.AsyncJobs)
	if agg.RequiredFailure && agg.Reason == "" {
		agg.Reason = "Required hook check did not succeed"
	}
	if s.CanBlock && (agg.RequiredFailure || agg.Cancelled) {
		agg.Decision = DecisionDeny
	}
	return agg, nil
}

func failed(result HookResult) bool {
	return result.ExecutionError != "" || result.Cancelled || result.TimedOut || result.Decision == DecisionDeny || result.Halt
}

func (r *Runner) execute(ctx context.Context, event, sid, tool, input string, h compiledHook, mode, jobID string) HookResult {
	d, _ := ctx.Value(eventDetailsKey{}).(EventDetails)
	e := Execution{Outcome: d.Outcome, ID: uuid.NewString(), EventID: d.EventID, Event: event, SessionID: sid, ToolName: tool, ToolCallID: d.ToolCallID, RunID: d.RunID, ParentToolCallID: d.ParentToolCallID, ConfigIndex: h.index, HookName: safeName(h), CommandHash: commandHash(h.cfg.Command), Mode: mode, Required: h.cfg.Required, State: "running", StartedAt: time.Now().UnixNano(), Decision: "none", AsyncJobID: jobID}
	r.record(e)
	result := r.runOne(ctx, h.cfg, BuildEnv(event, tool, sid, r.cwd, r.projectDir, input), BuildPayloadContext(ctx, event, sid, r.cwd, tool, input))
	if h.cfg.Required && result.UpdatedInput != "" {
		if _, err := shallowMerge(input, result.UpdatedInput); err != nil {
			result.ExecutionError = "invalid_rewrite"
			result.Decision = DecisionDeny
			result.UpdatedInput = ""
		}
	}
	e.State = resultState(result)
	e.FinishedAt = time.Now().UnixNano()
	e.Decision = result.Decision.String()
	e.ExitCode = result.ExitCode
	semantics, _ := EventSemantics(event)
	e.Rewritten = semantics.CanRewrite && result.UpdatedInput != ""
	e.ErrorKind = result.ExecutionError
	r.record(e)
	return result
}

// Hooks execute trusted user-authored shell configuration. They are not a sandbox.
// Cancellation never reads buffers until the interpreter yields; an uncooperative
// runner is abandoned after the same historical one-second grace period.
func (r *Runner) runOne(parent context.Context, h config.HookConfig, env []string, payload []byte) HookResult {
	ctx, cancel := context.WithTimeout(parent, h.TimeoutDuration())
	defer cancel()
	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runShell(ctx, shell.RunOptions{Command: h.Command, Cwd: r.cwd, Env: env, Stdin: bytes.NewReader(payload), Stdout: &stdout, Stderr: &stderr})
	}()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		select {
		case err = <-done:
		case <-time.After(abandonGrace):
			return interrupted(parent)
		}
	}
	if ctx.Err() != nil || shell.IsInterrupt(err) {
		return interrupted(parent)
	}
	code := shell.ExitCode(err)
	if err != nil {
		result := HookResult{ExitCode: &code}
		switch code {
		case 2, HaltExitCode:
			result.Decision = DecisionDeny
			result.Halt = code == HaltExitCode
			result.Reason = strings.TrimSpace(stderr.String())
			if result.Reason == "" {
				result.Reason = "blocked by hook"
			}
		default:
			result.ExecutionError = "nonzero_exit"
			if h.Required {
				result.Decision = DecisionDeny
				result.Reason = "Required hook command failed"
			}
			slog.Warn("Hook command failed", "command_hash", commandHash(h.Command), "exit_code", code)
		}
		return result
	}
	if h.Required {
		if err := validateRequiredOutput(stdout.String()); err != nil {
			return HookResult{Decision: DecisionDeny, Reason: "Required hook returned invalid output", ExecutionError: "invalid_output", ExitCode: &code}
		}
	}
	result := parseStdout(stdout.String())
	result.ExitCode = &code
	return result
}

func interrupted(parent context.Context) HookResult {
	if parent.Err() != nil {
		return HookResult{Cancelled: true, ExecutionError: "cancelled"}
	}
	return HookResult{TimedOut: true, ExecutionError: "timeout"}
}
