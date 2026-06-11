package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/services"
	"travel-agent-go/internal/tools"
	"travel-agent-go/internal/validators"
)

// MCPToolClient is the in-process MCP-style boundary between agents and tools.
// It uses tools/list and tools/call semantics without forcing a separate MCP
// server process into the current backend.
type MCPToolClient interface {
	ListTools() []MCPToolDescriptor
	CallTool(ctx context.Context, state *State, name ToolName, arguments any) ToolExecutionResult
}

type MCPToolDescriptor struct {
	Name        ToolName `json:"name"`
	Description string   `json:"description"`
	ReadOnly    bool     `json:"read_only"`
}

type MCPToolObservation struct {
	CallID    string          `json:"call_id"`
	Name      ToolName        `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt string          `json:"created_at"`
}

type LocalMCPToolClient struct {
	ragTool         *tools.RAGTool
	webResearchTool *tools.WebResearchTool
	plannerTool     *tools.PlannerTool
	mapTool         *tools.MapTool
	routeTool       *tools.RouteTool
	weatherService  *services.WeatherService
	assembler       *services.ItineraryAssembler
	validatorSet    *validators.Set
}

func NewLocalMCPToolClient(
	ragTool *tools.RAGTool,
	webResearchTool *tools.WebResearchTool,
	plannerTool *tools.PlannerTool,
	mapTool *tools.MapTool,
	routeTool *tools.RouteTool,
	weatherService *services.WeatherService,
	assembler *services.ItineraryAssembler,
	validatorSet *validators.Set,
) *LocalMCPToolClient {
	return &LocalMCPToolClient{
		ragTool:         ragTool,
		webResearchTool: webResearchTool,
		plannerTool:     plannerTool,
		mapTool:         mapTool,
		routeTool:       routeTool,
		weatherService:  weatherService,
		assembler:       assembler,
		validatorSet:    validatorSet,
	}
}

func (c *LocalMCPToolClient) ListTools() []MCPToolDescriptor {
	return []MCPToolDescriptor{
		{Name: ToolWebResearch, Description: "Search online travel evidence for the requested destination.", ReadOnly: true},
		{Name: ToolRAGSearch, Description: "Search local travel-guide contexts.", ReadOnly: true},
		{Name: ToolWeatherForecast, Description: "Load destination weather context.", ReadOnly: true},
		{Name: ToolCollectAttractions, Description: "Collect destination-scoped attraction candidates.", ReadOnly: true},
		{Name: ToolCollectMeals, Description: "Collect destination-scoped local restaurant candidates.", ReadOnly: true},
		{Name: ToolGenerateItineraryDraft, Description: "Generate and assemble itinerary draft from context and candidates.", ReadOnly: false},
		{Name: ToolEnrichMap, Description: "Enrich itinerary POIs with map data.", ReadOnly: false},
		{Name: ToolEnrichRoutes, Description: "Enrich itinerary routes.", ReadOnly: false},
		{Name: ToolValidateItinerary, Description: "Validate itinerary quality.", ReadOnly: true},
		{Name: ToolRepairItinerary, Description: "Repair known validation issues.", ReadOnly: false},
		{Name: ToolFinishItinerary, Description: "Finalize itinerary output.", ReadOnly: false},
	}
}

func (c *LocalMCPToolClient) CallTool(ctx context.Context, state *State, name ToolName, arguments any) ToolExecutionResult {
	callID := fmt.Sprintf("mcp_%d", time.Now().UnixNano())
	if state != nil {
		state.AddTrace("mcp_tool_call", "calling "+string(name))
	}

	result := c.dispatch(ctx, state, name, arguments)
	if state != nil {
		state.AddMCPToolObservation(MCPToolObservation{
			CallID:    callID,
			Name:      name,
			Arguments: rawMCPArguments(arguments),
			OK:        result.OK,
			Result:    result.ResultRawMessage(),
			Error:     result.Error,
		})
		if result.OK {
			state.AddTrace("mcp_tool_call", "completed "+string(name))
		} else {
			state.AddTrace("mcp_tool_call", "failed "+string(name)+": "+result.Error)
		}
	}
	return result
}

func (c *LocalMCPToolClient) dispatch(ctx context.Context, state *State, name ToolName, arguments any) ToolExecutionResult {
	if c == nil {
		return NewToolError(name, errors.New("mcp tool client is not configured"))
	}
	if err := ctx.Err(); err != nil {
		return NewToolError(name, err)
	}

	switch name {
	case ToolWebResearch:
		return c.callWebResearch(ctx, state, arguments)
	case ToolRAGSearch:
		return c.callRAGSearch(ctx, state, arguments)
	case ToolWeatherForecast:
		return c.callWeatherForecast(ctx, state, arguments)
	case ToolCollectAttractions:
		return c.callPlaceCandidates(state, arguments, services.PlaceKindAttraction)
	case ToolCollectMeals:
		return c.callPlaceCandidates(state, arguments, services.PlaceKindMeal)
	case ToolGenerateItineraryDraft:
		return c.callGenerateItineraryDraft(ctx, state, arguments)
	case ToolEnrichMap:
		return c.callEnrichMap(ctx, state)
	case ToolEnrichRoutes:
		return c.callEnrichRoutes(ctx, state)
	case ToolValidateItinerary:
		return c.callValidateItinerary(state)
	case ToolRepairItinerary:
		return c.callRepairItinerary(ctx, state)
	case ToolFinishItinerary:
		return c.callFinishItinerary(ctx, state)
	default:
		return NewToolError(name, fmt.Errorf("unknown MCP tool %q", name))
	}
}

func (c *LocalMCPToolClient) callWebResearch(ctx context.Context, state *State, raw any) ToolExecutionResult {
	if c.webResearchTool == nil {
		return NewToolError(ToolWebResearch, errors.New("web research tool is not configured"))
	}
	args, err := typedMCPArgs[WebResearchToolArgs](raw)
	if err != nil {
		return NewToolError(ToolWebResearch, err)
	}
	request := tripRequestFromState(state)
	args.Normalize(request)
	result, err := c.webResearchTool.Research(ctx, tools.WebResearchInput{
		Destination: args.Destination,
		Query:       args.Query,
		TopK:        args.TopK,
	})
	if err != nil {
		return NewToolError(ToolWebResearch, err)
	}
	if state != nil {
		state.EvidenceReport = &result.Evidence
		if evidenceContext := services.FormatEvidenceContext(result.Evidence); evidenceContext != "" {
			state.RAGContexts = appendUniqueStrings(state.RAGContexts, evidenceContext)
		}
	}

	sources := make([]WebResearchSourceDigest, 0, len(result.Sources))
	for _, source := range result.Sources {
		sources = append(sources, WebResearchSourceDigest{
			Title:   source.Title,
			URL:     source.URL,
			Snippet: source.Snippet,
		})
	}
	return NewToolOK(ToolWebResearch, WebResearchToolResult{
		Query:    result.Query,
		Count:    len(result.Sources),
		Sources:  sources,
		Evidence: result.Evidence,
		Disabled: len(result.Sources) == 0 && len(result.Evidence.Claims) == 0,
	})
}

func (c *LocalMCPToolClient) callRAGSearch(ctx context.Context, state *State, raw any) ToolExecutionResult {
	if c.ragTool == nil {
		return NewToolError(ToolRAGSearch, errors.New("rag tool is not configured"))
	}
	args, err := typedMCPArgs[RAGSearchToolArgs](raw)
	if err != nil {
		return NewToolError(ToolRAGSearch, err)
	}
	request := tripRequestFromState(state)
	args.Normalize(request)
	contexts, err := c.ragTool.Search(ctx, tools.RAGSearchInput{
		Destination:  args.Destination,
		Preferences:  args.Preferences,
		Pace:         args.Pace,
		SpecialNotes: args.SpecialNotes,
		TopK:         args.TopK,
	})
	if err != nil {
		return NewToolError(ToolRAGSearch, err)
	}
	if state != nil {
		state.RAGContexts = appendUniqueStrings(state.RAGContexts, contexts...)
	}
	return NewToolOK(ToolRAGSearch, RAGSearchToolResult{Count: len(contexts), Contexts: contexts})
}

func (c *LocalMCPToolClient) callWeatherForecast(ctx context.Context, state *State, raw any) ToolExecutionResult {
	if c.weatherService == nil {
		return NewToolError(ToolWeatherForecast, errors.New("weather service is not configured"))
	}
	args, err := typedMCPArgs[WeatherForecastToolArgs](raw)
	if err != nil {
		return NewToolError(ToolWeatherForecast, err)
	}
	request := tripRequestFromState(state)
	args.Normalize(request)
	forecast := c.weatherService.Forecast(ctx, args.City)
	contextAdded := false
	if state != nil {
		state.WeatherForecast = &forecast
		if contextText := formatWeatherContext(forecast); contextText != "" {
			state.RAGContexts = appendUniqueStrings(state.RAGContexts, contextText)
			contextAdded = true
		}
	}
	return NewToolOK(ToolWeatherForecast, WeatherForecastToolResult{Forecast: forecast, ContextAdded: contextAdded})
}

func (c *LocalMCPToolClient) callPlaceCandidates(state *State, raw any, kind string) ToolExecutionResult {
	args, err := typedMCPArgs[PlaceCandidateToolArgs](raw)
	if err != nil {
		if kind == services.PlaceKindAttraction {
			return NewToolError(ToolCollectAttractions, err)
		}
		return NewToolError(ToolCollectMeals, err)
	}
	request := tripRequestFromState(state)
	args.Normalize(request)
	contexts := []string{}
	if state != nil {
		contexts = state.RAGContexts
	}
	var candidates []services.PlaceCandidate
	toolName := ToolCollectAttractions
	if kind == services.PlaceKindMeal {
		toolName = ToolCollectMeals
		candidates = services.BuildMealCandidates(args.Destination, contexts, args.Limit)
	} else {
		candidates = services.BuildAttractionCandidates(args.Destination, contexts, args.Limit)
	}
	return NewToolOK(toolName, PlaceCandidateToolResult{
		Kind:       kind,
		Count:      len(candidates),
		Candidates: candidates,
	})
}

func (c *LocalMCPToolClient) callGenerateItineraryDraft(ctx context.Context, state *State, raw any) ToolExecutionResult {
	if state == nil {
		return NewToolError(ToolGenerateItineraryDraft, errors.New("agent state is required"))
	}
	if c.plannerTool == nil {
		return NewToolError(ToolGenerateItineraryDraft, errors.New("planner tool is not configured"))
	}
	if c.assembler == nil {
		return NewToolError(ToolGenerateItineraryDraft, errors.New("itinerary assembler is not configured"))
	}
	args, err := typedMCPArgs[GenerateItineraryDraftToolArgs](raw)
	if err != nil {
		return NewToolError(ToolGenerateItineraryDraft, err)
	}
	dayCount := args.EffectiveDayCount(state.DayCount)
	if dayCount <= 0 {
		dayCount = calcDayCount(state.Request.StartDate, state.Request.EndDate)
	}
	draft, err := c.plannerTool.Generate(ctx, tools.PlannerInput{
		Request:         state.Request,
		Contexts:        state.RAGContexts,
		DayCount:        dayCount,
		CandidateBundle: state.CandidateBundle,
	})
	if err != nil {
		return NewToolError(ToolGenerateItineraryDraft, err)
	}
	state.DayCount = dayCount
	state.PlannerDraft = draft
	itinerary := c.assembler.AssembleWithEvidence(state.Request, draft, state.RAGContexts, dayCount, state.EvidenceReport)
	state.DraftItinerary = itinerary
	state.FinalItinerary = itinerary
	return NewToolOK(ToolGenerateItineraryDraft, GenerateItineraryDraftToolResult{
		TripID:          itinerary.TripID,
		Summary:         itinerary.Summary,
		Days:            len(itinerary.Days),
		EstimatedBudget: itinerary.EstimatedBudget,
	})
}

func (c *LocalMCPToolClient) callEnrichMap(ctx context.Context, state *State) ToolExecutionResult {
	if state == nil || !hasUsableItinerary(state.FinalItinerary) {
		return NewToolError(ToolEnrichMap, errors.New("no itinerary to enrich"))
	}
	if c.mapTool == nil {
		return NewToolError(ToolEnrichMap, errors.New("map tool is not configured"))
	}
	if err := c.mapTool.EnrichItinerary(ctx, &state.FinalItinerary); err != nil {
		return NewToolError(ToolEnrichMap, err)
	}
	return NewToolOK(ToolEnrichMap, EnrichMapToolResult{Days: len(state.FinalItinerary.Days)})
}

func (c *LocalMCPToolClient) callEnrichRoutes(ctx context.Context, state *State) ToolExecutionResult {
	if state == nil || !hasUsableItinerary(state.FinalItinerary) {
		return NewToolError(ToolEnrichRoutes, errors.New("no itinerary to enrich"))
	}
	if c.routeTool == nil {
		return NewToolError(ToolEnrichRoutes, errors.New("route tool is not configured"))
	}
	if err := c.routeTool.Enrich(ctx, &state.FinalItinerary, state.WeatherForecast); err != nil {
		return NewToolError(ToolEnrichRoutes, err)
	}
	return NewToolOK(ToolEnrichRoutes, EnrichRoutesToolResult{
		Days:          len(state.FinalItinerary.Days),
		TransportLegs: countTransportLegs(state.FinalItinerary),
	})
}

func (c *LocalMCPToolClient) callValidateItinerary(state *State) ToolExecutionResult {
	if state == nil || !hasUsableItinerary(state.FinalItinerary) {
		return NewToolError(ToolValidateItinerary, errors.New("no itinerary to validate"))
	}
	if c.validatorSet == nil {
		return NewToolOK(ToolValidateItinerary, ValidateItineraryToolResult{})
	}
	issues := c.validatorSet.Validate(state.Request, state.FinalItinerary)
	state.ValidationIssues = make([]ValidationIssue, 0, len(issues))
	for _, issue := range issues {
		state.ValidationIssues = append(state.ValidationIssues, ValidationIssue{
			Code:     issue.Code,
			Level:    issue.Level,
			Message:  issue.Message,
			DayIndex: issue.DayIndex,
		})
	}
	return NewToolOK(ToolValidateItinerary, ValidateItineraryToolResult{
		Count:  len(state.ValidationIssues),
		Issues: state.ValidationIssues,
	})
}

func (c *LocalMCPToolClient) callRepairItinerary(ctx context.Context, state *State) ToolExecutionResult {
	if state == nil || !hasUsableItinerary(state.FinalItinerary) {
		return NewToolError(ToolRepairItinerary, errors.New("no itinerary to repair"))
	}
	if err := repairItineraryStep(ctx, state); err != nil {
		return NewToolError(ToolRepairItinerary, err)
	}
	return NewToolOK(ToolRepairItinerary, RepairItineraryToolResult{
		RemainingKnownIssues: len(state.ValidationIssues),
	})
}

func (c *LocalMCPToolClient) callFinishItinerary(ctx context.Context, state *State) ToolExecutionResult {
	if state == nil || !hasUsableItinerary(state.FinalItinerary) {
		return NewToolError(ToolFinishItinerary, errors.New("no itinerary to finish"))
	}
	if err := finalizeStep(ctx, state); err != nil {
		return NewToolError(ToolFinishItinerary, err)
	}
	return NewToolOK(ToolFinishItinerary, FinishItineraryToolResult{
		TripID:          state.FinalItinerary.TripID,
		Days:            len(state.FinalItinerary.Days),
		EstimatedBudget: state.FinalItinerary.EstimatedBudget,
	})
}

func typedMCPArgs[T any](raw any) (T, error) {
	var out T
	if raw == nil {
		return out, nil
	}
	if value, ok := raw.(T); ok {
		return value, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return out, err
	}
	if len(data) == 0 || string(data) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func rawMCPArguments(arguments any) json.RawMessage {
	if arguments == nil {
		return nil
	}
	data, err := json.Marshal(arguments)
	if err != nil {
		data, _ = json.Marshal(fmt.Sprintf("%v", arguments))
	}
	return json.RawMessage(data)
}

func tripRequestFromState(state *State) domain.TripRequest {
	if state == nil {
		return domain.TripRequest{}
	}
	return state.Request
}
