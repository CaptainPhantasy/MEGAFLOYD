package agent

import (
	"context"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/stretchr/testify/require"
)

func TestDeletionConfirmerRejectsRemoteOnlyInputPromptly(t *testing.T) {
	f := newDeletionFixture(t)
	service := question.NewRemoteService()
	f.guard.SetConfirmer(questionConfirmer{questions: service})
	events := service.Subscribe(t.Context())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	decision := f.guard.Check(ctx, "remote-deletion", "call", deletionguard.Classify("rm item", f.workingDir))
	require.False(t, decision.Allowed)
	require.Contains(t, decision.Reason, question.ErrLocalInputRequired.Error())
	require.Empty(t, service.Pending())
	require.Empty(t, f.record("remote-deletion").Confirmations)
	select {
	case <-events:
		t.Fatal("unsupported remote confirmation was published")
	default:
	}
}

func TestDeletionConfirmerRequiresTwoDirectLocalAnswers(t *testing.T) {
	f := newDeletionFixture(t)
	service := question.NewService()
	f.guard.SetConfirmer(questionConfirmer{questions: service})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	events := service.Subscribe(ctx)
	result := make(chan deletionguard.Decision, 1)
	go func() {
		result <- f.guard.Check(ctx, "local-deletion", "call", deletionguard.Classify("rm item", f.workingDir))
	}()
	for stage := 1; stage <= 2; stage++ {
		select {
		case event := <-events:
			require.True(t, event.Payload.RequireLocalInput, "deletion confirmer must mark the engine-owned trust requirement")
			require.Equal(t, 1, len(event.Payload.Questions))
			yes := true
			require.True(t, service.Answer([]question.Answer{{QuestionID: event.Payload.Questions[0].ID, Yes: &yes}}))
		case <-ctx.Done():
			t.Fatal("local deletion confirmation did not arrive")
		}
	}
	select {
	case decision := <-result:
		require.True(t, decision.Allowed, decision.Reason)
	case <-ctx.Done():
		t.Fatal("local answers did not release the guard")
	}
	require.Len(t, f.record("local-deletion").Confirmations, 2)
}
