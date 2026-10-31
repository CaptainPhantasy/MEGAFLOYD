package proto

// RalphGoalRequest controls the existing persistent Ralph loop. It does not
// replace the separate bounded session-goal API.
type RalphGoalRequest struct {
	Action    string `json:"action"`
	Objective string `json:"objective,omitempty"`
}

type CompactRequest struct {
	Instruction string `json:"instruction,omitempty"`
}

type FrontendRequest struct {
	Kind       string `json:"kind"`
	Action     string `json:"action,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Argument   string `json:"argument,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	EventID    string `json:"event_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	After      int64  `json:"after,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type FrontendResult struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Path      string `json:"path,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}
