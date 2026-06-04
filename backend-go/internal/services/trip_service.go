package services

import (
	"context"
	"math"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// ItineraryGenerator 是 TripService 依赖的最小抽象。
// Agent 包会实现这个接口，但 services 包不需要 import agent 包，
// 这样可以避免 Go 中不允许的循环依赖。
type ItineraryGenerator interface {
	Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error)
}

// TripService 是项目的业务编排中心，对应 Python 版 trip_service.py。
// 它体现了 Go 中常见的“服务对象”设计：
// 1. 结构体保存依赖。
// 2. 构造函数 NewTripService 注入依赖。
// 3. 方法负责具体业务用例。
type TripService struct {
	planner Planner
	agent   ItineraryGenerator
}

func NewTripService(planner Planner, travelAgent ItineraryGenerator) *TripService {
	return &TripService{planner: planner, agent: travelAgent}
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

	targetIndex := 0
	if strings.HasPrefix(request.EditScope, "day_") {
		var n int
		if _, err := fmtSscanf(request.EditScope, "day_%d", &n); err == nil && n > 0 {
			for i, day := range itinerary.Days {
				if day.DayIndex == n {
					targetIndex = i
					break
				}
			}
		}
	}

	targetDay := itinerary.Days[targetIndex]
	logging.Info(ctx, "trip service edit target selected",
		"trip_id", request.TripID,
		"edit_scope", request.EditScope,
		"target_day", targetDay.DayIndex,
	)
	if draft, ok, _ := s.planner.EditDay(request, targetDay); ok {
		logging.Info(ctx, "trip service applying planner edit draft",
			"trip_id", request.TripID,
			"target_day", targetDay.DayIndex,
		)
		applyDayEditDraft(&targetDay, draft)
	} else {
		logging.Info(ctx, "trip service applying rule edit",
			"trip_id", request.TripID,
			"target_day", targetDay.DayIndex,
		)
		applyRuleEdit(&targetDay, request.UserInstruction)
	}
	itinerary.Days[targetIndex] = targetDay
	itinerary.SourceNotes = append(itinerary.SourceNotes, "已根据用户编辑指令更新行程："+request.UserInstruction)
	itinerary.Tips = cleanUserTips(itinerary.Tips, itinerary.Destination)
	itinerary.Tips = append(itinerary.Tips, "已根据你的修改要求更新目标日期，出发前建议再确认当天交通、天气和景点开放情况。")

	updated := refreshBudget(itinerary, itinerary.EstimatedBudget)
	logging.Info(ctx, "trip service edit completed",
		"trip_id", request.TripID,
		"target_day", targetDay.DayIndex,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return updated, nil
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
