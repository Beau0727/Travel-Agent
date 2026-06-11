package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/llm"
)

type ToolName string

const (
	ToolRAGSearch              ToolName = "rag_search"
	ToolWebResearch            ToolName = "web_research"
	ToolWeatherForecast        ToolName = "weather_forecast"
	ToolGenerateItineraryDraft ToolName = "generate_itinerary_draft"
	ToolEnrichMap              ToolName = "enrich_map"
	ToolEnrichRoutes           ToolName = "enrich_routes"
	ToolValidateItinerary      ToolName = "validate_itinerary"
	ToolRepairItinerary        ToolName = "repair_itinerary"
	ToolFinishItinerary        ToolName = "finish_itinerary"
)

type RAGSearchToolArgs struct {
	Destination  string   `json:"destination"`
	Preferences  []string `json:"preferences"`
	Pace         string   `json:"pace"`
	SpecialNotes string   `json:"special_notes"`
	TopK         int      `json:"top_k"`
}

type WebResearchToolArgs struct {
	Destination string `json:"destination"`
	Query       string `json:"query"`
	TopK        int    `json:"top_k"`
}

func (a *WebResearchToolArgs) Normalize(request domain.TripRequest) {
	if a.Destination == "" {
		a.Destination = request.Destination
	}
	if strings.TrimSpace(a.Query) == "" && a.Destination != "" {
		a.Query = a.Destination + " 旅游攻略 官网 官方 地图 门票 开放时间 预约 景点 美食 行程"
	}
	if a.TopK <= 0 {
		a.TopK = 6
	}
}

func (a *RAGSearchToolArgs) Normalize(request domain.TripRequest) {
	if a.Destination == "" {
		a.Destination = request.Destination
	}
	if len(a.Preferences) == 0 {
		a.Preferences = request.Preferences
	}
	if a.Pace == "" {
		a.Pace = request.Pace
	}
	if a.SpecialNotes == "" {
		a.SpecialNotes = request.SpecialNotes
	}
	if a.TopK <= 0 {
		a.TopK = 5
	}
}

type WeatherForecastToolArgs struct {
	City string `json:"city"`
}

func (a *WeatherForecastToolArgs) Normalize(request domain.TripRequest) {
	if a.City == "" {
		a.City = request.Destination
	}
}

type GenerateItineraryDraftToolArgs struct {
	DayCount int `json:"day_count"`
}

func (a GenerateItineraryDraftToolArgs) EffectiveDayCount(defaultDayCount int) int {
	if a.DayCount > 0 {
		return a.DayCount
	}
	return defaultDayCount
}

type RAGSearchToolResult struct {
	Count    int      `json:"count"`
	Contexts []string `json:"contexts"`
}

type WebResearchToolResult struct {
	Query    string                    `json:"query"`
	Count    int                       `json:"count"`
	Sources  []WebResearchSourceDigest `json:"sources"`
	Evidence domain.EvidenceReport     `json:"evidence,omitempty"`
	Disabled bool                      `json:"disabled,omitempty"`
}

type WebResearchSourceDigest struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WeatherForecastToolResult struct {
	Forecast     domain.WeatherForecastResponse `json:"forecast"`
	ContextAdded bool                           `json:"context_added"`
}

type GenerateItineraryDraftToolResult struct {
	TripID          string  `json:"trip_id"`
	Summary         string  `json:"summary"`
	Days            int     `json:"days"`
	EstimatedBudget float64 `json:"estimated_budget"`
}

type EnrichMapToolResult struct {
	Days int `json:"days"`
}

type EnrichRoutesToolResult struct {
	Days          int `json:"days"`
	TransportLegs int `json:"transport_legs"`
}

type ValidateItineraryToolResult struct {
	Count  int               `json:"count"`
	Issues []ValidationIssue `json:"issues"`
}

type RepairItineraryToolResult struct {
	RemainingKnownIssues int `json:"remaining_known_issues"`
}

type FinishItineraryToolResult struct {
	TripID          string  `json:"trip_id"`
	Days            int     `json:"days"`
	EstimatedBudget float64 `json:"estimated_budget"`
}

type ToolExecutionResult struct {
	Tool   ToolName `json:"tool"`
	OK     bool     `json:"ok"`
	Result any      `json:"result,omitempty"`
	Error  string   `json:"error,omitempty"`
}

func NewToolOK(name ToolName, result any) ToolExecutionResult {
	return ToolExecutionResult{Tool: name, OK: true, Result: result}
}

func NewToolError(name ToolName, err error) ToolExecutionResult {
	return ToolExecutionResult{Tool: name, OK: false, Error: err.Error()}
}

func (r ToolExecutionResult) JSON() string {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"tool":%q,"ok":false,"error":%q}`, r.Tool, err.Error())
	}
	return string(data)
}

func (r ToolExecutionResult) ResultRawMessage() json.RawMessage {
	if r.Result == nil {
		return nil
	}
	data, err := json.Marshal(r.Result)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"marshal_error":%q}`, err.Error()))
	}
	return data
}

type ToolObservation struct {
	CallID    string          `json:"call_id"`
	Name      ToolName        `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt string          `json:"created_at"`
}

func NewToolObservation(call toolCall, result ToolExecutionResult) ToolObservation {
	return ToolObservation{
		CallID:    call.ID,
		Name:      ToolName(call.Function.Name),
		Arguments: normalizedRawMessage(call.Function.Arguments),
		OK:        result.OK,
		Result:    result.ResultRawMessage(),
		Error:     result.Error,
	}
}

func normalizedRawMessage(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !json.Valid([]byte(value)) {
		data, _ := json.Marshal(value)
		return data
	}
	return json.RawMessage(value)
}

type toolChatRequest struct {
	Model       string            `json:"model"`
	Messages    []toolChatMessage `json:"messages"`
	Tools       []toolDefinition  `json:"tools"`
	ToolChoice  string            `json:"tool_choice"`
	Temperature float64           `json:"temperature"`
}

type toolChatResponse struct {
	Choices []toolChatChoice `json:"choices"`
}

type toolChatChoice struct {
	Message toolChatMessage `json:"message"`
}

type toolChatMessage = llm.ChatMessage
type toolCall = llm.ToolCall
type toolCallFunction = llm.ToolCallFunction
type toolDefinition = llm.ToolDefinition
type toolFunctionDefinition = llm.ToolFunctionDefinition
