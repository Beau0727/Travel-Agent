package agent

import (
	"context"
	"strings"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/geo"
	"travel-agent-go/internal/services"
	"travel-agent-go/internal/tools"
	"travel-agent-go/internal/validators"
)

// NewDefaultTravelPlanningAgent 组装默认工作流。
// 这一步把“固定服务编排”变成“显式 Agent 工作流”：
// 需求理解 -> RAG 工具 -> Planner 工具 -> Itinerary 组装 -> Validator -> 修正 -> 输出。
func NewDefaultTravelPlanningAgent(
	ragTool *tools.RAGTool,
	webResearchTool *tools.WebResearchTool,
	plannerTool *tools.PlannerTool,
	mapTool *tools.MapTool,
	routeTool *tools.RouteTool,
	weatherService *services.WeatherService,
	assembler *services.ItineraryAssembler,
	validatorSet *validators.Set,
) *TravelPlanningAgent {
	return NewTravelPlanningAgent(
		NewStepFunc("understand_request", understandRequestStep),
		NewStepFunc("research_online_evidence", researchOnlineEvidenceStep(webResearchTool)),
		NewStepFunc("retrieve_context", retrieveContextStep(ragTool)),
		NewStepFunc("load_weather", loadWeatherStep(weatherService)),
		NewStepFunc("generate_draft", generateDraftStep(plannerTool)),
		NewStepFunc("assemble_itinerary", assembleItineraryStep(assembler)),
		NewStepFunc("enrich_map", enrichMapStep(mapTool)),
		NewStepFunc("enrich_routes", enrichRoutesStep(routeTool)),
		NewStepFunc("validate_itinerary", validateItineraryStep(validatorSet)),
		NewStepFunc("repair_itinerary", repairItineraryStep),
		NewStepFunc("finalize", finalizeStep),
	)
}

func understandRequestStep(ctx context.Context, state *State) error {
	state.DayCount = calcDayCount(state.Request.StartDate, state.Request.EndDate)
	state.DestinationAliases = geo.Aliases(state.Request.Destination)
	state.AddTrace("understand_request", "识别目的地："+state.Request.Destination)
	return nil
}

func researchOnlineEvidenceStep(webResearchTool *tools.WebResearchTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		if webResearchTool == nil || state.Request.Destination == "" {
			return nil
		}
		result, err := webResearchTool.Research(ctx, tools.WebResearchInput{
			Destination: state.Request.Destination,
			Query:       buildOnlineResearchQuery(state.Request),
			TopK:        6,
		})
		if err != nil {
			state.AddTrace("research_online_evidence", "联网资料收集失败，降级使用本地资料")
			return nil
		}
		if len(result.Evidence.Sources) == 0 && len(result.Evidence.Claims) == 0 {
			state.AddTrace("research_online_evidence", "未获得可用联网证据")
			return nil
		}
		state.EvidenceReport = &result.Evidence
		if evidenceContext := services.FormatEvidenceContext(result.Evidence); evidenceContext != "" {
			state.RAGContexts = appendUniqueStrings(state.RAGContexts, evidenceContext)
		}
		state.AddTrace("research_online_evidence", "在线证据来源："+itoa(len(result.Evidence.Sources))+"，候选事实："+itoa(len(result.Evidence.Claims)))
		return nil
	}
}

func retrieveContextStep(ragTool *tools.RAGTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		contexts, err := ragTool.Search(ctx, tools.RAGSearchInput{
			Destination:  state.Request.Destination,
			Preferences:  state.Request.Preferences,
			Pace:         state.Request.Pace,
			SpecialNotes: state.Request.SpecialNotes,
			TopK:         5,
		})
		if err != nil {
			state.AddTrace("retrieve_context", "RAG 工具失败，使用空上下文兜底")
			return nil
		}
		state.RAGContexts = appendUniqueStrings(state.RAGContexts, contexts...)
		state.AddTrace("retrieve_context", "检索到攻略片段："+itoa(len(contexts)))
		return nil
	}
}

func loadWeatherStep(weatherService *services.WeatherService) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		if weatherService == nil || state.Request.Destination == "" {
			return nil
		}
		forecast := weatherService.Forecast(ctx, state.Request.Destination)
		state.WeatherForecast = &forecast
		contextText := formatWeatherContext(forecast)
		if contextText != "" {
			state.RAGContexts = appendUniqueStrings(state.RAGContexts, contextText)
		}
		state.AddTrace("load_weather", "已加载目的地天气："+state.Request.Destination)
		return nil
	}
}

func generateDraftStep(plannerTool *tools.PlannerTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		draft, err := plannerTool.Generate(ctx, tools.PlannerInput{
			Request:         state.Request,
			Contexts:        state.RAGContexts,
			DayCount:        state.DayCount,
			CandidateBundle: state.CandidateBundle,
		})
		if err != nil {
			return err
		}
		state.PlannerDraft = draft
		state.AddTrace("generate_draft", "生成草稿天数："+itoa(len(draft.Days)))
		return nil
	}
}

func assembleItineraryStep(assembler *services.ItineraryAssembler) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		itinerary := assembler.AssembleWithEvidence(state.Request, state.PlannerDraft, state.RAGContexts, state.DayCount, state.EvidenceReport)
		state.DraftItinerary = itinerary
		state.FinalItinerary = itinerary
		return nil
	}
}

func enrichMapStep(mapTool *tools.MapTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		if mapTool == nil {
			return nil
		}
		if err := mapTool.EnrichItinerary(ctx, &state.FinalItinerary); err != nil {
			state.AddTrace("enrich_map", "地图工具失败，保留无坐标 itinerary")
			return nil
		}
		state.AddTrace("enrich_map", "已尝试补充高德 POI 坐标")
		return nil
	}
}

func enrichRoutesStep(routeTool *tools.RouteTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		if routeTool == nil {
			return nil
		}
		if err := routeTool.Enrich(ctx, &state.FinalItinerary, state.WeatherForecast); err != nil {
			state.AddTrace("enrich_routes", "路线规划失败，保留原交通信息")
			return nil
		}
		state.AddTrace("enrich_routes", "已尝试补充高德真实路线距离和耗时")
		return nil
	}
}

func validateItineraryStep(validatorSet *validators.Set) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		issues := validatorSet.Validate(state.Request, state.FinalItinerary)
		state.ValidationIssues = make([]ValidationIssue, 0, len(issues))
		for _, issue := range issues {
			state.ValidationIssues = append(state.ValidationIssues, ValidationIssue{
				Code:     issue.Code,
				Level:    issue.Level,
				Message:  issue.Message,
				DayIndex: issue.DayIndex,
			})
		}
		state.AddTrace("validate_itinerary", "发现问题数量："+itoa(len(issues)))
		return nil
	}
}

func repairItineraryStep(ctx context.Context, state *State) error {
	if len(state.ValidationIssues) == 0 {
		return nil
	}
	for _, issue := range state.ValidationIssues {
		switch issue.Code {
		case validators.CodeBudgetExceeded:
			state.FinalItinerary.Tips = append(state.FinalItinerary.Tips, "当前预算略紧，建议优先保留核心景点，餐饮和交通可按当天情况灵活调整。")
		case validators.CodeEarlyStartConflict:
			relaxEarlyStarts(&state.FinalItinerary, issue.DayIndex)
		case validators.CodePaceTooPacked:
			addPaceRepairNote(&state.FinalItinerary, issue.DayIndex)
		case validators.CodePlaceholderContent:
			if services.SanitizeItineraryContent(state.Request, state.RAGContexts, &state.FinalItinerary) {
				state.AddTrace("repair_itinerary", "已替换占位景点或餐饮名称")
			}
		case validators.CodeDestinationMismatch:
			if services.SanitizeItineraryContent(state.Request, state.RAGContexts, &state.FinalItinerary) {
				state.AddTrace("repair_itinerary", "已替换疑似跨城景点或餐饮，并重建当天交通线")
			}
		}
	}
	state.FinalItinerary.SourceNotes = append(state.FinalItinerary.SourceNotes, "Agent 已根据校验结果进行规则级修正。")
	return nil
}

func finalizeStep(ctx context.Context, state *State) error {
	if services.SanitizeItineraryContent(state.Request, state.RAGContexts, &state.FinalItinerary) {
		state.AddTrace("finalize", "最终输出前已替换占位景点或餐饮名称")
	}
	state.FinalItinerary.SourceNotes = append(
		state.FinalItinerary.SourceNotes,
		"Agent trace steps: "+itoa(len(state.Trace)),
	)
	if state.EvidenceReport != nil && state.FinalItinerary.Evidence == nil {
		state.FinalItinerary.Evidence = state.EvidenceReport
	}
	return nil
}

func buildOnlineResearchQuery(request domain.TripRequest) string {
	parts := []string{
		request.Destination,
		"旅游攻略",
		"官网",
		"官方",
		"地图",
		"门票",
		"开放时间",
		"预约",
		"景点",
		"美食",
		"交通",
		"避坑",
	}
	if len(request.Preferences) > 0 {
		parts = append(parts, request.Preferences...)
	}
	if request.Pace != "" {
		parts = append(parts, request.Pace)
	}
	if request.SpecialNotes != "" {
		parts = append(parts, request.SpecialNotes)
	}
	return strings.Join(parts, " ")
}

func relaxEarlyStarts(itinerary *domain.Itinerary, dayIndex int) {
	for i := range itinerary.Days {
		if itinerary.Days[i].DayIndex != dayIndex {
			continue
		}
		for j := range itinerary.Days[i].Spots {
			if itinerary.Days[i].Spots[j].StartTime < "10:00" {
				itinerary.Days[i].Spots[j].StartTime = "10:00"
				itinerary.Days[i].Spots[j].EndTime = "12:00"
			}
		}
		itinerary.Days[i].Notes = append(itinerary.Days[i].Notes, "已根据“不想太早起床”的偏好，把出发时间调整得更从容。")
	}
}

func addPaceRepairNote(itinerary *domain.Itinerary, dayIndex int) {
	for i := range itinerary.Days {
		if itinerary.Days[i].DayIndex == dayIndex {
			itinerary.Days[i].Notes = append(itinerary.Days[i].Notes, "当天安排较满，建议把部分点位作为备选，不必全部打卡。")
		}
	}
}

func calcDayCount(startDate, endDate string) int {
	start, err1 := time.Parse("2006-01-02", startDate)
	end, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return 1
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	n := value
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
