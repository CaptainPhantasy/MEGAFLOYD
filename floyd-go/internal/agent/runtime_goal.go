package agent

import (
	"context"
	"github.com/legacy-ai/floyd/internal/goal"
)

func (a *sessionAgent) beginRuntimeGoal(ctx context.Context, call SessionAgentCall) (*goal.Run, error) {
	if a.isSubAgent {
		return nil, nil
	}
	return goal.Begin(ctx, a.sessions, call.SessionID)
}

func (a *sessionAgent) goalContinuation(ctx context.Context, run *goal.Run, call SessionAgentCall) *SessionAgentCall {
	prompt := run.Continuation(ctx)
	if prompt == "" {
		return nil
	}
	return &SessionAgentCall{SessionID: call.SessionID, Prompt: prompt, RunSettings: call.RunSettings.clone(), MaxOutputTokens: call.MaxOutputTokens, ProviderOptions: call.ProviderOptions, Temperature: call.Temperature, TopP: call.TopP, TopK: call.TopK, FrequencyPenalty: call.FrequencyPenalty, PresencePenalty: call.PresencePenalty, NonInteractive: call.NonInteractive, OnAuthRefresh: call.OnAuthRefresh}
}
