package agent

// WithSessionIdle serializes transcript changes with both direct runs and
// accepted work that has not yet reached the active-run transition.
func (c *coordinator) WithSessionIdle(sessionID string, operation func() error) error {
	selected := c.runtimeSessionAgent(sessionID)
	agent, ok := selected.(*sessionAgent)
	if !ok {
		if selected.IsSessionBusy(sessionID) || selected.QueuedPrompts(sessionID) > 0 {
			return ErrSessionBusy
		}
		return operation()
	}
	mu := agent.sessionMu(sessionID)
	mu.Lock()
	defer mu.Unlock()
	agent.acceptedMu.Lock()
	defer agent.acceptedMu.Unlock()
	accepted, _ := agent.acceptedRuns.Get(sessionID)
	if accepted > 0 || agent.IsSessionBusy(sessionID) || agent.QueuedPrompts(sessionID) > 0 {
		return ErrSessionBusy
	}
	return operation()
}
