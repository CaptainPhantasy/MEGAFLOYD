package proto

type RuntimeQueueEdit struct {
	Prompt string `json:"prompt"`
}
type RuntimeQueueSteer struct {
	IDs []string `json:"ids"`
}
type RuntimePlanRequest struct {
	Action  string `json:"action"`
	Content string `json:"content,omitempty"`
}
type RuntimePlan struct {
	Active  bool   `json:"active"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content"`
}
type RuntimeSwarmRequest struct {
	Active      *bool `json:"active,omitempty"`
	MaxParallel *int  `json:"max_parallel,omitempty"`
}
type RuntimeSubagentRequest struct {
	Prompt      string `json:"prompt"`
	Profile     string `json:"profile,omitempty"`
	Description string `json:"description,omitempty"`
}
