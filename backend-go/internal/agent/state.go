package agent

import (
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/services"
)

// State 是 Agent 的“工作记忆”。
// 传统 service 往往把中间变量藏在一个函数里；Agent 项目会把这些中间结果显式放进状态，
// 这样每一步都可以观察、修改和校验状态。
type State struct {
	Request          domain.TripRequest
	DayCount         int
	RAGContexts      []string
	EvidenceReport   *domain.EvidenceReport
	PlannerDraft     services.PlannerDraft
	DraftItinerary   domain.Itinerary
	FinalItinerary   domain.Itinerary
	WeatherForecast  *domain.WeatherForecastResponse
	ValidationIssues []ValidationIssue
	ToolObservations []ToolObservation
	Trace            []TraceEvent
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
