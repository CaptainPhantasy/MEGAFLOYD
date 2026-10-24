package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/goal"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

type goalDispatchResult struct {
	prompt string
	err    error
}

type goalDispatchCoordinator struct {
	*blockingCoordinator
	sessions session.Service
	accepted atomic.Int32
	canceled atomic.Int32
	results  chan goalDispatchResult
}

func (c *goalDispatchCoordinator) BeginAccepted(string) *agent.AcceptedRun {
	c.accepted.Add(1)
	return nil
}

func (c *goalDispatchCoordinator) Cancel(string) { c.canceled.Add(1) }

func (c *goalDispatchCoordinator) RunAccepted(ctx context.Context, _ *agent.AcceptedRun, sessionID, prompt string, _ ...message.Attachment) (*fantasy.AgentResult, error) {
	// Exercise the real goal accounting at the accepted execution boundary,
	// without constructing a provider or making a model call.
	run, err := goal.Begin(ctx, c.sessions, sessionID)
	if err == nil && run != nil {
		err = run.Finish(ctx, nil)
	}
	c.results <- goalDispatchResult{prompt: prompt, err: err}
	return nil, context.Canceled
}

func TestCanonicalGoalConfigureDefersDispatch(t *testing.T) {
	for _, deferred := range []bool{true, false} {
		t.Run(fmt.Sprintf("defer_start=%t", deferred), func(t *testing.T) {
			dir := t.TempDir()
			conn, err := db.Connect(t.Context(), dir)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
			sessions := session.NewService(db.New(conn), conn)
			sess, err := sessions.Create(t.Context(), "goal dispatch")
			require.NoError(t, err)
			coord := &goalDispatchCoordinator{
				blockingCoordinator: newBlockingCoordinator(),
				sessions:            sessions,
				results:             make(chan goalDispatchResult, 4),
			}
			b, _ := newTestBackend(t)
			ws := insertAgentWorkspace(t, b, coord)
			ws.Sessions = sessions
			t.Cleanup(func() { ws.runWG.Wait(); ws.cancel() })

			// Decode the actual optional wire field. Omitting it must preserve
			// the existing standalone start/resume behavior.
			flag := ""
			if deferred {
				flag = `,"defer_start":true`
			}
			for step, payload := range []string{
				`{"objective":"Use the configured tools","budget":{"turn_budget":4}` + flag + `}`,
				`{"control":"resume"` + flag + `}`,
			} {
				var input goal.Input
				require.NoError(t, json.Unmarshal([]byte(payload), &input))
				before := coord.accepted.Load()
				state, err := b.RuntimeGoal(t.Context(), ws.ID, sess.ID, &input)
				require.NoError(t, err)
				require.Equal(t, "active", state.Status)
				require.EqualValues(t, 4, *state.Budget.TurnBudget)
				wantPrompt := "Work toward the active goal: Use the configured tools"
				if deferred {
					require.Equal(t, before, coord.accepted.Load(), "configuration must not dispatch a synthetic turn")
					persisted, err := goal.Get(t.Context(), sessions, sess.ID)
					require.NoError(t, err)
					require.EqualValues(t, step, persisted.TurnsUsed)
					require.Nil(t, persisted.StartedAt)
					wantPrompt = fmt.Sprintf("Actual user prompt %d", step+1)
					require.NoError(t, b.SendMessage(ws.ID, proto.AgentMessage{SessionID: sess.ID, Prompt: wantPrompt}))
				}
				require.Equal(t, before+1, coord.accepted.Load())
				ws.runWG.Wait()
				result := <-coord.results
				require.NoError(t, result.err)
				require.Equal(t, wantPrompt, result.prompt)
				persisted, err := goal.Get(t.Context(), sessions, sess.ID)
				require.NoError(t, err)
				require.EqualValues(t, step+1, persisted.TurnsUsed, "only the one accepted prompt consumes a turn")

				control := "pause"
				if step == 1 {
					control = "cancel"
				}
				input = goal.Input{}
				require.NoError(t, json.Unmarshal([]byte(`{"control":"`+control+`","defer_start":true}`), &input))
				state, err = b.RuntimeGoal(t.Context(), ws.ID, sess.ID, &input)
				require.NoError(t, err)
				require.Equal(t, "paused", state.Status)
				require.EqualValues(t, step+1, coord.canceled.Load(), "pause/cancel must remain immediate")
			}
		})
	}
}
