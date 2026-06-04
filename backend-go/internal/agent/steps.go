package agent

import (
	"context"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/services"
	"zhilv-yuntu-go/internal/tools"
	"zhilv-yuntu-go/internal/validators"
)

// NewDefaultTravelPlanningAgent 组装默认工作流。
// 这一步把“固定服务编排”变成“显式 Agent 工作流”：
// 需求理解 -> RAG 工具 -> Planner 工具 -> Itinerary 组装 -> Validator -> 修正 -> 输出。
func NewDefaultTravelPlanningAgent(
	ragTool *tools.RAGTool,
	plannerTool *tools.PlannerTool,
	mapTool *tools.MapTool,
	assembler *services.ItineraryAssembler,
	validatorSet *validators.Set,
) *TravelPlanningAgent {
	return NewTravelPlanningAgent(
		NewStepFunc("understand_request", understandRequestStep),
		NewStepFunc("retrieve_context", retrieveContextStep(ragTool)),
		NewStepFunc("generate_draft", generateDraftStep(plannerTool)),
		NewStepFunc("assemble_itinerary", assembleItineraryStep(assembler)),
		NewStepFunc("enrich_map", enrichMapStep(mapTool)),
		NewStepFunc("validate_itinerary", validateItineraryStep(validatorSet)),
		NewStepFunc("repair_itinerary", repairItineraryStep),
		NewStepFunc("finalize", finalizeStep),
	)
}

func understandRequestStep(ctx context.Context, state *State) error {
	state.DayCount = calcDayCount(state.Request.StartDate, state.Request.EndDate)
	state.AddTrace("understand_request", "识别目的地："+state.Request.Destination)
	return nil
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
			state.RAGContexts = []string{}
			return nil
		}
		state.RAGContexts = contexts
		state.AddTrace("retrieve_context", "检索到攻略片段："+itoa(len(contexts)))
		return nil
	}
}

func generateDraftStep(plannerTool *tools.PlannerTool) func(ctx context.Context, state *State) error {
	return func(ctx context.Context, state *State) error {
		draft, err := plannerTool.Generate(ctx, tools.PlannerInput{
			Request:  state.Request,
			Contexts: state.RAGContexts,
			DayCount: state.DayCount,
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
		itinerary := assembler.Assemble(state.Request, state.PlannerDraft, state.RAGContexts, state.DayCount)
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
		}
	}
	state.FinalItinerary.SourceNotes = append(state.FinalItinerary.SourceNotes, "Agent 已根据校验结果进行规则级修正。")
	return nil
}

func finalizeStep(ctx context.Context, state *State) error {
	state.FinalItinerary.SourceNotes = append(
		state.FinalItinerary.SourceNotes,
		"Agent trace steps: "+itoa(len(state.Trace)),
	)
	return nil
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
