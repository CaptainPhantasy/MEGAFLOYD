// Package goal persists bounded objectives in the owning Go session. It never
// launches another execution engine or uses a model to judge completion.
package goal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/session"
)

type Budget struct {
	TokenBudget            *int64 `json:"tokenBudget"`
	TurnBudget             *int64 `json:"turnBudget"`
	WallClockBudgetMs      *int64 `json:"wallClockBudgetMs"`
	RemainingTokens        *int64 `json:"remainingTokens"`
	RemainingTurns         *int64 `json:"remainingTurns"`
	RemainingWallClockMs   *int64 `json:"remainingWallClockMs"`
	TokenBudgetReached     bool   `json:"tokenBudgetReached"`
	TurnBudgetReached      bool   `json:"turnBudgetReached"`
	WallClockBudgetReached bool   `json:"wallClockBudgetReached"`
	OverBudget             bool   `json:"overBudget"`
}
type State struct {
	GoalID              string     `json:"goalId"`
	Objective           string     `json:"objective"`
	CompletionCriterion string     `json:"completionCriterion,omitempty"`
	Status              string     `json:"status"`
	TurnsUsed           int64      `json:"turnsUsed"`
	TokensUsed          int64      `json:"tokensUsed"`
	WallClockMs         int64      `json:"wallClockMs"`
	TerminalReason      string     `json:"terminalReason,omitempty"`
	Budget              Budget     `json:"budget"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
}
type BudgetInput struct {
	TokenBudget       *int64 `json:"token_budget,omitempty"`
	TurnBudget        *int64 `json:"turn_budget,omitempty"`
	WallClockBudgetMs *int64 `json:"wall_clock_budget_ms,omitempty"`
}
type Input struct {
	Objective           string       `json:"objective,omitempty"`
	Control             string       `json:"control,omitempty"`
	CompletionCriterion string       `json:"completion_criterion,omitempty"`
	Reason              string       `json:"reason,omitempty"`
	Budget              *BudgetInput `json:"budget,omitempty"`
}

func read(metadata map[string]any) (*State, error) {
	v, ok := metadata["goal"]
	if !ok || v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var s State
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode goal: %w", err)
	}
	if s.GoalID == "" {
		return nil, errors.New("stored goal has no identity")
	}
	return &s, nil
}
func refresh(s *State, now time.Time) {
	elapsed := s.WallClockMs
	if s.StartedAt != nil && now.After(*s.StartedAt) {
		elapsed += now.Sub(*s.StartedAt).Milliseconds()
	}
	remaining := func(limit *int64, used int64) (*int64, bool) {
		if limit == nil {
			return nil, false
		}
		n := max(int64(0), *limit-used)
		return &n, used >= *limit
	}
	s.Budget.RemainingTokens, s.Budget.TokenBudgetReached = remaining(s.Budget.TokenBudget, s.TokensUsed)
	s.Budget.RemainingTurns, s.Budget.TurnBudgetReached = remaining(s.Budget.TurnBudget, s.TurnsUsed)
	s.Budget.RemainingWallClockMs, s.Budget.WallClockBudgetReached = remaining(s.Budget.WallClockBudgetMs, elapsed)
	s.Budget.OverBudget = s.Budget.TokenBudgetReached || s.Budget.TurnBudgetReached || s.Budget.WallClockBudgetReached
}
func store(m map[string]any, s *State) error {
	refresh(s, time.Now())
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var value map[string]any
	if err = json.Unmarshal(b, &value); err != nil {
		return err
	}
	m["goal"] = value
	return nil
}
func mutate(ctx context.Context, svc session.Service, id string, fn func(*State) (*State, error)) (*State, error) {
	m, ok := svc.(session.MetadataMutator)
	if !ok {
		return nil, errors.New("session goal storage is unavailable")
	}
	var out *State
	_, err := m.MutateMetadata(ctx, id, func(meta map[string]any) error {
		s, e := read(meta)
		if e != nil {
			return e
		}
		s, e = fn(s)
		if e != nil {
			return e
		}
		if s == nil {
			return nil
		}
		out = s
		return store(meta, s)
	})
	return out, err
}
func Get(ctx context.Context, svc session.Service, id string) (*State, error) {
	v, err := svc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s, err := read(v.Metadata)
	if s != nil {
		refresh(s, time.Now())
		if s.StartedAt != nil {
			s.WallClockMs += max(int64(0), time.Since(*s.StartedAt).Milliseconds())
		}
	}
	return s, err
}

func Configure(ctx context.Context, svc session.Service, id string, in Input) (*State, error) {
	return mutate(ctx, svc, id, func(s *State) (*State, error) {
		if strings.TrimSpace(in.Objective) != "" {
			if s != nil && s.Status == "active" {
				return nil, errors.New("pause or cancel the existing goal before replacing it")
			}
			turns := int64(3)
			s = &State{GoalID: uuid.NewString(), Objective: strings.TrimSpace(in.Objective), CompletionCriterion: in.CompletionCriterion, Status: "active", Budget: Budget{TurnBudget: &turns}}
		} else if s == nil {
			return nil, errors.New("no goal exists for this session")
		}
		if in.Budget != nil {
			for _, n := range []*int64{in.Budget.TokenBudget, in.Budget.TurnBudget, in.Budget.WallClockBudgetMs} {
				if n != nil && *n <= 0 {
					return nil, errors.New("goal budgets must be positive")
				}
			}
			if in.Budget.TokenBudget != nil {
				s.Budget.TokenBudget = in.Budget.TokenBudget
			}
			if in.Budget.TurnBudget != nil {
				s.Budget.TurnBudget = in.Budget.TurnBudget
			}
			if in.Budget.WallClockBudgetMs != nil {
				s.Budget.WallClockBudgetMs = in.Budget.WallClockBudgetMs
			}
		}
		switch in.Control {
		case "":
		case "pause":
			s.Status = "paused"
			s.TerminalReason = "paused by user"
		case "cancel":
			s.Status = "paused"
			s.TerminalReason = "cancelled by user"
		case "resume":
			refresh(s, time.Now())
			if s.Budget.OverBudget {
				return nil, errors.New("goal budget is exhausted; increase the budget before resuming")
			}
			if s.Status == "complete" {
				return nil, errors.New("completed goal cannot be resumed")
			}
			s.Status = "active"
			s.TerminalReason = ""
		case "complete", "blocked":
			s.Status = in.Control
			s.TerminalReason = strings.TrimSpace(in.Reason)
		default:
			return nil, errors.New("unknown goal control")
		}
		return s, nil
	})
}

type Run struct {
	svc               session.Service
	sessionID, goalID string
	started           time.Time
	mu                sync.Mutex
	finished          bool
}

func Begin(ctx context.Context, svc session.Service, id string) (*Run, error) {
	if _, ok := svc.(session.MetadataMutator); !ok {
		return nil, nil
	}
	now := time.Now().UTC()
	var active bool
	s, err := mutate(ctx, svc, id, func(s *State) (*State, error) {
		if s == nil || s.Status != "active" {
			return s, nil
		}
		refresh(s, now)
		if s.Budget.OverBudget {
			s.Status = "blocked"
			s.TerminalReason = "goal budget reached"
			return s, nil
		}
		if s.StartedAt != nil {
			s.WallClockMs += max(int64(0), now.Sub(*s.StartedAt).Milliseconds())
		}
		s.TurnsUsed++
		s.StartedAt = &now
		active = true
		return s, nil
	})
	if err != nil || !active {
		return nil, err
	}
	return &Run{svc: svc, sessionID: id, goalID: s.GoalID, started: now}, nil
}
func (r *Run) Prompt() string {
	if r == nil {
		return ""
	}
	s, err := Get(context.Background(), r.svc, r.sessionID)
	if err != nil || s == nil {
		return ""
	}
	return fmt.Sprintf("\n<active_goal id=%q>\nObjective: %s\nCompletion criterion: %s\nWork toward this objective within the user's instructions. Use the goal tool to mark complete only when achieved, or blocked when user input is required. Remaining turns including this turn: %d.\n</active_goal>", s.GoalID, s.Objective, s.CompletionCriterion, remainingTurns(s))
}
func remainingTurns(s *State) int64 {
	if s.Budget.RemainingTurns == nil {
		return 1
	}
	return *s.Budget.RemainingTurns + 1
}
func (r *Run) AddUsage(ctx context.Context, tokens int64) error {
	if r == nil || tokens <= 0 {
		return nil
	}
	_, err := mutate(ctx, r.svc, r.sessionID, func(s *State) (*State, error) {
		if s != nil && s.GoalID == r.goalID {
			s.TokensUsed += tokens
		}
		return s, nil
	})
	return err
}
func (r *Run) ShouldStop() bool {
	if r == nil {
		return false
	}
	s, err := Get(context.Background(), r.svc, r.sessionID)
	return err != nil || s == nil || s.GoalID != r.goalID || s.Status != "active" || s.Budget.TokenBudgetReached || s.Budget.WallClockBudgetReached
}
func (r *Run) Finish(ctx context.Context, runErr error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return nil
	}
	r.finished = true
	_, err := mutate(ctx, r.svc, r.sessionID, func(s *State) (*State, error) {
		if s == nil || s.GoalID != r.goalID {
			return s, nil
		}
		s.WallClockMs += max(int64(0), time.Since(r.started).Milliseconds())
		s.StartedAt = nil
		refresh(s, time.Now())
		if s.Status == "active" {
			if runErr != nil {
				s.Status = "blocked"
				s.TerminalReason = "run failed"
				if errors.Is(runErr, context.Canceled) {
					s.Status = "paused"
					s.TerminalReason = "run cancelled"
				}
			} else if s.Budget.OverBudget {
				s.Status = "blocked"
				s.TerminalReason = "goal budget reached"
			}
		}
		return s, nil
	})
	return err
}
func (r *Run) Continuation(ctx context.Context) string {
	if r == nil {
		return ""
	}
	s, err := Get(ctx, r.svc, r.sessionID)
	if err != nil || s == nil || s.GoalID != r.goalID || s.Status != "active" || s.Budget.OverBudget {
		return ""
	}
	return "Continue working on the active goal: " + s.Objective + ". Use the goal tool when it is complete or blocked."
}
