package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/logging"
	"travel-agent-go/internal/validators"
)

// ItineraryGenerator 是 TripService 依赖的最小抽象。
// Agent 包会实现这个接口，但 services 包不需要 import agent 包，
// 这样可以避免 Go 中不允许的循环依赖。
type ItineraryGenerator interface {
	Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error)
}

type WeatherForecaster interface {
	Forecast(ctx context.Context, city string) domain.WeatherForecastResponse
}

type MapEnricher interface {
	EnrichItinerary(ctx context.Context, itinerary *domain.Itinerary) error
}

type RouteEnricher interface {
	Enrich(ctx context.Context, itinerary *domain.Itinerary, weather *domain.WeatherForecastResponse) error
}

type EditValidator interface {
	Validate(request domain.TripRequest, itinerary domain.Itinerary) []validators.Issue
}

// TripService 是项目的业务编排中心，对应 Python 版 trip_service.py。
// 它体现了 Go 中常见的“服务对象”设计：
// 1. 结构体保存依赖。
// 2. 构造函数 NewTripService 注入依赖。
// 3. 方法负责具体业务用例。
type TripService struct {
	planner        Planner
	agent          ItineraryGenerator
	weatherService WeatherForecaster
	mapEnricher    MapEnricher
	routeEnricher  RouteEnricher
	validator      EditValidator
}

func NewTripService(
	planner Planner,
	travelAgent ItineraryGenerator,
	weatherService WeatherForecaster,
	mapEnricher MapEnricher,
	routeEnricher RouteEnricher,
	validator EditValidator,
) *TripService {
	return &TripService{
		planner:        planner,
		agent:          travelAgent,
		weatherService: weatherService,
		mapEnricher:    mapEnricher,
		routeEnricher:  routeEnricher,
		validator:      validator,
	}
}

func (s *TripService) Generate(request domain.TripRequest) (domain.Itinerary, error) {
	return s.agent.Generate(context.Background(), request)
}

func (s *TripService) Edit(ctx context.Context, request domain.TripEditRequest) (domain.Itinerary, error) {
	start := time.Now()
	itinerary := request.CurrentItinerary
	if len(itinerary.Days) == 0 {
		logging.Warn(ctx, "trip service edit skipped empty itinerary",
			"trip_id", request.TripID,
			"edit_scope", request.EditScope,
		)
		return itinerary, nil
	}

	now := time.Now().Format(time.RFC3339)
	conversationID := firstNonEmptyString(
		request.ConversationID,
		itinerary.EditConversationID,
		"edit_"+request.TripID,
	)
	itinerary.EditConversationID = conversationID
	itinerary.EditMessages = seedEditMessages(itinerary.EditMessages, request.Messages)
	itinerary.EditMessages = append(itinerary.EditMessages, domain.TripEditMessage{
		Role:      "user",
		Content:   strings.TrimSpace(request.UserInstruction),
		CreatedAt: now,
	})

	targetIndices := selectEditTargetIndices(itinerary, request.EditScope)
	logging.Info(ctx, "trip service edit target selected",
		"trip_id", request.TripID,
		"edit_scope", request.EditScope,
		"targets", len(targetIndices),
	)

	changeSummary := []string{}
	for _, targetIndex := range targetIndices {
		beforeDay := itinerary.Days[targetIndex]
		targetDay := itinerary.Days[targetIndex]
		var draft DayEditDraft
		if plannerDraft, ok, _ := s.planner.EditDay(request, targetDay); ok {
			logging.Info(ctx, "trip service applying planner edit draft",
				"trip_id", request.TripID,
				"target_day", targetDay.DayIndex,
			)
			draft = plannerDraft
			applyDayEditDraft(&targetDay, draft)
		} else {
			logging.Info(ctx, "trip service applying rule edit",
				"trip_id", request.TripID,
				"target_day", targetDay.DayIndex,
			)
			applyRuleEdit(&targetDay, request.UserInstruction)
		}
		resetEditedDayTransport(&targetDay, itinerary.Destination)
		itinerary.Days[targetIndex] = targetDay
		changeSummary = append(changeSummary, summarizeDayEdit(beforeDay, targetDay, draft, request.UserInstruction)...)
	}

	changeSummary = appendUniqueText(changeSummary, "已重新计算预算，并尝试刷新地图点位、真实路线和校验结果。")
	itinerary.SourceNotes = append(itinerary.SourceNotes, "已根据用户编辑指令更新行程："+request.UserInstruction)
	itinerary.Tips = cleanUserTips(itinerary.Tips, itinerary.Destination)
	itinerary.Tips = appendUniqueText(itinerary.Tips, "已根据你的修改要求更新行程，出发前建议再确认当天交通、天气和景点开放情况。")

	forecast := s.refreshEditEnrichment(ctx, &itinerary)
	updated := refreshBudget(itinerary, itinerary.EstimatedBudget)
	validationRequest := buildEditValidationRequest(updated, request.UserInstruction)
	issues := s.validateEditedItinerary(validationRequest, updated)
	if applyEditValidationRepairs(&updated, issues) {
		issues = s.validateEditedItinerary(validationRequest, updated)
	}
	updated.EditIssues = toTripEditIssues(issues)
	if forecast != nil && len(forecast.Advice) > 0 {
		updated.Tips = appendUniqueText(updated.Tips, forecast.Advice...)
	}
	updated.LastChangeSummary = firstNonEmptySlice(changeSummary, []string{"已根据你的指令更新行程。"})
	assistantText := strings.Join(updated.LastChangeSummary, "\n")
	if len(updated.EditIssues) > 0 {
		assistantText += "\n仍需留意：" + updated.EditIssues[0].Message
	}
	updated.EditMessages = append(updated.EditMessages, domain.TripEditMessage{
		Role:      "assistant",
		Content:   assistantText,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	updated.EditMessages = trimEditMessages(updated.EditMessages, 20)
	updated.EditRevisions = append(updated.EditRevisions, domain.TripEditRevision{
		RevisionID:    fmt.Sprintf("rev_%d", time.Now().UnixNano()),
		TripID:        request.TripID,
		Instruction:   request.UserInstruction,
		EditScope:     request.EditScope,
		ChangeSummary: updated.LastChangeSummary,
		CreatedAt:     time.Now().Format(time.RFC3339),
	})

	logging.Info(ctx, "trip service edit completed",
		"trip_id", request.TripID,
		"targets", len(targetIndices),
		"issues", len(updated.EditIssues),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return updated, nil
}

func (s *TripService) refreshEditEnrichment(ctx context.Context, itinerary *domain.Itinerary) *domain.WeatherForecastResponse {
	var forecast *domain.WeatherForecastResponse
	if s.weatherService != nil && itinerary.Destination != "" {
		value := s.weatherService.Forecast(ctx, itinerary.Destination)
		forecast = &value
	}
	if s.mapEnricher != nil {
		if err := s.mapEnricher.EnrichItinerary(ctx, itinerary); err != nil {
			logging.Warn(ctx, "trip service edit map enrichment failed",
				"trip_id", itinerary.TripID,
				"error", err,
			)
		}
	}
	if s.routeEnricher != nil {
		if err := s.routeEnricher.Enrich(ctx, itinerary, forecast); err != nil {
			logging.Warn(ctx, "trip service edit route enrichment failed",
				"trip_id", itinerary.TripID,
				"error", err,
			)
		}
	}
	return forecast
}

func (s *TripService) validateEditedItinerary(request domain.TripRequest, itinerary domain.Itinerary) []validators.Issue {
	if s.validator == nil {
		return nil
	}
	return s.validator.Validate(request, itinerary)
}

func selectEditTargetIndices(itinerary domain.Itinerary, scope string) []int {
	if strings.TrimSpace(scope) == "" || scope == "all" {
		indices := make([]int, 0, len(itinerary.Days))
		for i := range itinerary.Days {
			indices = append(indices, i)
		}
		return indices
	}
	if strings.HasPrefix(scope, "day_") {
		var n int
		if _, err := fmtSscanf(scope, "day_%d", &n); err == nil && n > 0 {
			for i, day := range itinerary.Days {
				if day.DayIndex == n {
					return []int{i}
				}
			}
		}
	}
	return []int{0}
}

func seedEditMessages(existing, incoming []domain.TripEditMessage) []domain.TripEditMessage {
	if len(incoming) > len(existing) {
		return trimEditMessages(incoming, 20)
	}
	return trimEditMessages(existing, 20)
}

func trimEditMessages(messages []domain.TripEditMessage, max int) []domain.TripEditMessage {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return messages[len(messages)-max:]
}

func resetEditedDayTransport(day *domain.DayPlan, destination string) {
	if len(day.Spots) == 0 && len(day.Meals) == 0 {
		day.Transport = nil
		return
	}
	from := destination + " 出发点"
	if day.Hotel != nil && strings.TrimSpace(day.Hotel.Name) != "" {
		from = day.Hotel.Name
	}
	day.Transport = buildTimelineTransport(from, day.Spots, day.Meals, 0)
	for i := range day.Transport {
		day.Transport[i].RouteStatus = "pending"
	}
}

func summarizeDayEdit(before, after domain.DayPlan, draft DayEditDraft, instruction string) []string {
	summary := []string{}
	if len(draft.ChangeSummary) > 0 {
		summary = append(summary, draft.ChangeSummary...)
	}
	if before.Theme != after.Theme {
		summary = append(summary, fmt.Sprintf("第 %d 天主题已调整为「%s」。", after.DayIndex, after.Theme))
	}
	if len(before.Spots) > 0 && len(after.Spots) > 0 && before.Spots[0].Name != after.Spots[0].Name {
		summary = append(summary, fmt.Sprintf("第 %d 天景点由「%s」调整为「%s」。", after.DayIndex, before.Spots[0].Name, after.Spots[0].Name))
	}
	if len(before.Meals) > 0 && len(after.Meals) > 0 && before.Meals[0].Name != after.Meals[0].Name {
		summary = append(summary, fmt.Sprintf("第 %d 天餐饮由「%s」调整为「%s」。", after.DayIndex, before.Meals[0].Name, after.Meals[0].Name))
	}
	if len(summary) == 0 {
		summary = append(summary, fmt.Sprintf("第 %d 天已按「%s」进行轻量调整。", after.DayIndex, instruction))
	}
	return appendUniqueText(summary)
}

func buildEditValidationRequest(itinerary domain.Itinerary, instruction string) domain.TripRequest {
	startDate := ""
	endDate := ""
	if len(itinerary.Days) > 0 {
		startDate = itinerary.Days[0].Date
		endDate = itinerary.Days[len(itinerary.Days)-1].Date
	}
	pace := "适中"
	text := instruction + " " + itinerary.Summary
	for _, tip := range itinerary.Tips {
		text += " " + tip
	}
	if strings.Contains(text, "轻松") || strings.Contains(text, "不想太累") || strings.Contains(text, "太累") {
		pace = "轻松"
	} else if strings.Contains(text, "紧凑") {
		pace = "紧凑"
	}
	budget := itinerary.EstimatedBudget
	if itinerary.BudgetBreakdown.Total > 0 {
		budget = itinerary.BudgetBreakdown.Total
	}
	return domain.TripRequest{
		Destination: itinerary.Destination,
		StartDate:   startDate,
		EndDate:     endDate,
		Budget:      budget,
		Pace:        pace,
	}
}

func applyEditValidationRepairs(itinerary *domain.Itinerary, issues []validators.Issue) bool {
	repaired := false
	for _, issue := range issues {
		switch issue.Code {
		case validators.CodeBudgetExceeded:
			itinerary.Tips = appendUniqueText(itinerary.Tips, "当前预算略紧，建议优先保留核心体验，餐饮和交通可按当天情况灵活调整。")
			repaired = true
		case validators.CodeEarlyStartConflict:
			for i := range itinerary.Days {
				if issue.DayIndex != 0 && itinerary.Days[i].DayIndex != issue.DayIndex {
					continue
				}
				for j := range itinerary.Days[i].Spots {
					if itinerary.Days[i].Spots[j].StartTime < "10:00" {
						itinerary.Days[i].Spots[j].StartTime = "10:00"
						itinerary.Days[i].Spots[j].EndTime = "12:00"
						repaired = true
					}
				}
			}
		case validators.CodePaceTooPacked, validators.CodeRouteTooLong, validators.CodeWalkingTooLong:
			for i := range itinerary.Days {
				if issue.DayIndex != 0 && itinerary.Days[i].DayIndex != issue.DayIndex {
					continue
				}
				itinerary.Days[i].Notes = appendUniqueText(itinerary.Days[i].Notes, "已根据校验结果提示：当天节奏可能偏满，建议把部分点位作为备选。")
				repaired = true
			}
		case validators.CodeDestinationMismatch, validators.CodePlaceholderContent:
			if SanitizeItineraryContent(domain.TripRequest{Destination: itinerary.Destination, Budget: itinerary.EstimatedBudget}, nil, itinerary) {
				repaired = true
			}
		}
	}
	return repaired
}

func toTripEditIssues(issues []validators.Issue) []domain.TripEditIssue {
	result := make([]domain.TripEditIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, domain.TripEditIssue{
			Code:     issue.Code,
			Level:    issue.Level,
			Message:  issue.Message,
			DayIndex: issue.DayIndex,
		})
	}
	return result
}

func appendUniqueText(values []string, additions ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range append(values, additions...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func firstNonEmptySlice(value []string, fallback []string) []string {
	if len(value) > 0 {
		return value
	}
	return fallback
}

func findDayDraft(days []PlannerDayDraft, dayIndex int) PlannerDayDraft {
	for _, day := range days {
		if day.DayIndex == dayIndex {
			return day
		}
	}
	return PlannerDayDraft{
		DayIndex:        dayIndex,
		Theme:           "第 " + itoa(dayIndex) + " 天轻松游",
		SpotName:        "推荐景点 " + itoa(dayIndex),
		SpotDescription: "根据旅行偏好生成的兜底景点。",
		MealName:        "本地特色餐饮",
		MealNotes:       "按用户饮食偏好灵活选择。",
		DailyNote:       "保持轻松节奏，预留弹性时间。",
	}
}

func allocateDailyCosts(request domain.TripRequest, dayCount int) ([]float64, []float64, []float64) {
	targetTotal := request.Budget * 0.85
	if request.Pace == "轻松" {
		targetTotal = request.Budget * 0.78
	} else if request.Pace == "紧凑" {
		targetTotal = request.Budget * 0.92
	}

	hotelRatio := 0.50
	if strings.Contains(request.HotelLevel, "高档") || strings.Contains(request.HotelLevel, "高端") {
		hotelRatio = 0.56
	} else if strings.Contains(request.HotelLevel, "经济") {
		hotelRatio = 0.40
	}
	mealRatio := 0.22
	if contains(request.Preferences, "美食") {
		mealRatio = 0.28
	}
	transportRatio := math.Max(0.12, 1-hotelRatio-mealRatio)
	ratioSum := hotelRatio + mealRatio + transportRatio

	hotelTotal := targetTotal * hotelRatio / ratioSum
	mealTotal := targetTotal * mealRatio / ratioSum
	transportTotal := targetTotal * transportRatio / ratioSum

	return prorate(hotelTotal, dayCount), prorate(mealTotal, dayCount), prorate(transportTotal, dayCount)
}
