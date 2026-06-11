package agent

import (
	"fmt"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/services"
)

// State 是 Agent 的“工作记忆”。
// 传统 service 往往把中间变量藏在一个函数里；Agent 项目会把这些中间结果显式放进状态，
// 这样每一步都可以观察、修改和校验状态。
type State struct {
	Request             domain.TripRequest
	DayCount            int
	DestinationAliases  []string
	RAGContexts         []string
	ResearchBundle      ResearchBundle
	CandidateBundle     services.PlaceCandidateBundle
	EvidenceReport      *domain.EvidenceReport
	PlannerDraft        services.PlannerDraft
	DraftItinerary      domain.Itinerary
	FinalItinerary      domain.Itinerary
	WeatherForecast     *domain.WeatherForecastResponse
	ValidationIssues    []ValidationIssue
	ToolObservations    []ToolObservation
	MCPToolObservations []MCPToolObservation
	A2AAgentCards       []A2AAgentCard
	A2AMessages         []A2AMessage
	Trace               []TraceEvent
}

type CandidatePlace struct {
	Name               string
	Kind               string
	City               string
	Adcode             string
	Address            string
	Latitude           *float64
	Longitude          *float64
	Rating             float64
	ReviewCount        int
	Tags               []string
	SourceIDs          []string
	SourceTypes        []string
	Verified           bool
	VerificationStatus string
	Confidence         float64
}

type ResearchBundle struct {
	Attractions []CandidatePlace
	Meals       []CandidatePlace
}

// ValidationIssue 是校验器发现的问题。
// 目前先用规则校验，后续可以把这些 issue 交给 LLM 做二次修正。
type ValidationIssue struct {
	Code     string `json:"code"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	DayIndex int    `json:"day_index,omitempty"`
}

// TraceEvent 记录 Agent 每一步做了什么。
// 这是 Agent 工程里很重要的可观测性设计：当结果不好时，你能回头看它在哪一步偏了。
type TraceEvent struct {
	Step      string `json:"step"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func (s *State) AddTrace(step, message string) {
	s.Trace = append(s.Trace, TraceEvent{
		Step:      step,
		Message:   message,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}

func (s *State) AddToolObservation(observation ToolObservation) {
	observation.CreatedAt = time.Now().Format(time.RFC3339)
	s.ToolObservations = append(s.ToolObservations, observation)
}

func (s *State) AddMCPToolObservation(observation MCPToolObservation) {
	observation.CreatedAt = time.Now().Format(time.RFC3339)
	s.MCPToolObservations = append(s.MCPToolObservations, observation)
}

func (s *State) RegisterA2AAgent(worker RoleAgent) {
	for _, card := range s.A2AAgentCards {
		if card.Name == worker.Name() {
			return
		}
	}
	s.A2AAgentCards = append(s.A2AAgentCards, A2AAgentCard{
		Name: worker.Name(),
		Role: worker.Role(),
		Goal: worker.Goal(),
	})
}

func (s *State) AddA2AMessage(from, to, messageType, task string, payload map[string]any) {
	s.A2AMessages = append(s.A2AMessages, A2AMessage{
		ID:        fmt.Sprintf("a2a_%06d", len(s.A2AMessages)+1),
		From:      from,
		To:        to,
		Type:      messageType,
		Task:      task,
		Payload:   payload,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}
