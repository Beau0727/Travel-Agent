package agent

// A2AAgentCard is the local agent-card shape used by the orchestrator.
// It mirrors the A2A idea that agents advertise role and capability before
// receiving tasks.
type A2AAgentCard struct {
	Name string `json:"name"`
	Role string `json:"role"`
	Goal string `json:"goal"`
}

// A2AMessage records task handoff between the orchestrator and role agents.
// The current implementation is in-process; the structure is intentionally
// transport-neutral so it can be exposed through an A2A-compatible adapter later.
type A2AMessage struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Type      string         `json:"type"`
	Task      string         `json:"task"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"created_at"`
}
