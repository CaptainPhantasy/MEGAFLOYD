package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// RuntimePrompt is the actual pending call, with a stable identity independent
// of its position in the queue. A caller RunID remains intact when steering.
type RuntimePrompt struct {
	ID string `json:"id"`
	RunID string `json:"run_id,omitempty"`
	UserMessageID string `json:"user_message_id,omitempty"`
	Prompt string `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
	Steered bool `json:"steered,omitempty"`
}

type runtimePromptKey struct { agent *sessionAgent; session string }
type runtimePromptState struct { active RuntimePrompt; consumed []SessionAgentCall }
var runtimePrompts = struct { sync.Mutex; items map[runtimePromptKey]*runtimePromptState }{items: make(map[runtimePromptKey]*runtimePromptState)}

func identifyQueuedCall(call SessionAgentCall) SessionAgentCall {
	if call.queueID == "" { call.queueID = uuid.NewString() }
	if call.queuedAt.IsZero() { call.queuedAt = time.Now().UTC() }
	return call
}

func runtimePrompt(call SessionAgentCall) RuntimePrompt {
	return RuntimePrompt{ID: call.queueID, RunID: call.RunID, Prompt: call.Prompt, CreatedAt: call.queuedAt, Steered: call.steer}
}

func (a *sessionAgent) beginRuntimePrompt(call SessionAgentCall) {
	call = identifyQueuedCall(call)
	runtimePrompts.Lock()
	runtimePrompts.items[runtimePromptKey{a, call.SessionID}] = &runtimePromptState{active: runtimePrompt(call)}
	runtimePrompts.Unlock()
}

func (a *sessionAgent) recordRuntimeSteer(call SessionAgentCall) {
	runtimePrompts.Lock()
	defer runtimePrompts.Unlock()
	if state := runtimePrompts.items[runtimePromptKey{a, call.SessionID}]; state != nil {
		state.consumed = append(state.consumed, call)
	}
}

func (a *sessionAgent) completeRuntimeSteers(ctx context.Context, sessionID string, complete notify.RunComplete) {
	key := runtimePromptKey{a, sessionID}
	runtimePrompts.Lock()
	state := runtimePrompts.items[key]
	if state == nil || state.active.RunID != complete.RunID {
		runtimePrompts.Unlock()
		return
	}
	delete(runtimePrompts.items, key)
	drops := append([]SessionAgentCall(nil), state.consumed...)
	runtimePrompts.Unlock()
	for _, call := range drops {
		if call.RunID == "" || call.RunID == complete.RunID || a.runComplete == nil { continue }
		steeredComplete := complete
		steeredComplete.RunID = call.RunID
		a.runComplete.PublishMustDeliver(ctx, pubsub.UpdatedEvent, steeredComplete)
	}
}

func (a *sessionAgent) runtimeQueue(sessionID string) ([]RuntimePrompt, *RuntimePrompt) {
	mu := a.sessionMu(sessionID)
	mu.Lock()
	calls, _ := a.messageQueue.Get(sessionID)
	calls=append([]SessionAgentCall(nil),calls...)
	items := make([]RuntimePrompt, 0, len(calls))
	for i := range calls { calls[i] = identifyQueuedCall(calls[i]); items = append(items, runtimePrompt(calls[i])) }
	if len(calls) > 0 { a.messageQueue.Set(sessionID, calls) }
	mu.Unlock()
	runtimePrompts.Lock()
	defer runtimePrompts.Unlock()
	if state := runtimePrompts.items[runtimePromptKey{a,sessionID}]; state != nil { active := state.active; return items, &active }
	return items, nil
}

func (a *sessionAgent) runtimeMutateQueue(sessionID, id, prompt string, remove bool) error {
	mu := a.sessionMu(sessionID)
	mu.Lock()
	calls, _ := a.messageQueue.Get(sessionID)
	calls=append([]SessionAgentCall(nil),calls...)
	for i, call := range calls {
		if call.queueID != id { continue }
		if remove { calls = append(calls[:i:i], calls[i+1:]...) } else {
			if strings.TrimSpace(prompt) == "" { mu.Unlock(); return errors.New("prompt must not be empty") }
			calls[i].Prompt = prompt
		}
		a.messageQueue.Set(sessionID, calls)
		mu.Unlock()
		if remove { a.publishCanceledQueueDrops([]SessionAgentCall{call}) }
		return nil
	}
	mu.Unlock()
	return fmt.Errorf("queued prompt %q not found", id)
}

func (a *sessionAgent) runtimeSteerQueue(sessionID string, ids []string) error {
	if len(ids) == 0 { return errors.New("at least one queued prompt is required") }
	mu := a.sessionMu(sessionID)
	mu.Lock()
	defer mu.Unlock()
	if !a.IsSessionBusy(sessionID) { return errors.New("session has no active run to steer") }
	calls, _ := a.messageQueue.Get(sessionID)
	calls=append([]SessionAgentCall(nil),calls...)
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids { wanted[id] = false }
	for _, call := range calls { if _, ok := wanted[call.queueID]; ok { wanted[call.queueID] = true } }
	for id, found := range wanted { if !found { return fmt.Errorf("queued prompt %q not found", id) } }
	for i := range calls { if wanted[calls[i].queueID] { calls[i].steer = true } }
	a.messageQueue.Set(sessionID, calls)
	return nil
}

// These optional methods expose the same queue used by terminal and API runs.
func (c *coordinator) RuntimeQueue(sessionID string) ([]RuntimePrompt, *RuntimePrompt, error) {
	selected:=c.currentAgent;if child,found:=c.RuntimeChildAgent(sessionID);found{selected=child}
	a, ok := selected.(*sessionAgent)
	if !ok { return nil,nil,errors.New("agent queue is unavailable") }
	items,active := a.runtimeQueue(sessionID)
	return items,active,nil
}
func (c *coordinator) RuntimeEditQueue(sessionID,id,prompt string,remove bool) error {
	selected:=c.currentAgent;if child,found:=c.RuntimeChildAgent(sessionID);found{selected=child}
	a,ok:=selected.(*sessionAgent); if !ok { return errors.New("agent queue is unavailable") }
	return a.runtimeMutateQueue(sessionID,id,prompt,remove)
}
func (c *coordinator) RuntimeSteerQueue(sessionID string, ids []string) error {
	selected:=c.currentAgent;if child,found:=c.RuntimeChildAgent(sessionID);found{selected=child}
	a,ok:=selected.(*sessionAgent); if !ok { return errors.New("agent queue is unavailable") }
	return a.runtimeSteerQueue(sessionID,ids)
}

func (a *sessionAgent) runtimePromptUserMessage(sessionID,runID,messageID string){
 runtimePrompts.Lock();defer runtimePrompts.Unlock()
 if state:=runtimePrompts.items[runtimePromptKey{a,sessionID}];state!=nil&&state.active.RunID==runID{state.active.UserMessageID=messageID}
}
func (c *coordinator) RuntimeClearQueue(sessionID string)error{
 selected:=c.currentAgent;if child,found:=c.RuntimeChildAgent(sessionID);found{selected=child}
 a,ok:=selected.(*sessionAgent);if !ok{return errors.New("agent queue is unavailable")}
 mu:=a.sessionMu(sessionID);mu.Lock();calls,_:=a.messageQueue.Take(sessionID);mu.Unlock()
 a.publishCanceledQueueDrops(calls);return nil
}
