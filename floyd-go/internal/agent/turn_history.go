package agent

import (
	"context"
	"time"

	"github.com/legacy-ai/floyd/internal/history"
)

type runFileHistory struct {
	recorder   history.TurnRecorder
	workingDir string
	current    context.Context
	finished   bool
}

func (a *sessionAgent) fileHistoryRun() *runFileHistory {
	run := &runFileHistory{workingDir: a.workingDir}
	if !a.isSubAgent {
		run.recorder, _ = a.history.(history.TurnRecorder)
	}
	return run
}

func (r *runFileHistory) Begin(ctx context.Context, sessionID, messageID string) (context.Context, error) {
	if r.recorder == nil {
		return ctx, nil
	}
	if err := r.Finish(); err != nil {
		return nil, err
	}
	recorded, err := r.recorder.BeginTurn(ctx, sessionID, messageID, r.workingDir)
	if err != nil {
		return nil, err
	}
	r.current, r.finished = recorded, false
	return recorded, nil
}

func (r *runFileHistory) Context(ctx context.Context) context.Context {
	return history.CarryTurn(ctx, r.current)
}

func (r *runFileHistory) Finish() error {
	if r.recorder == nil || r.current == nil || r.finished {
		return nil
	}
	r.finished = true
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.current), 5*time.Second)
	defer cancel()
	return r.recorder.CompleteTurn(ctx)
}
