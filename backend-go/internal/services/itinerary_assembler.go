package services

import (
	"strings"
	"time"

	"travel-agent-go/internal/domain"
)

// ItineraryAssembler 负责把 PlannerDraft 组装成完整 Itinerary。
// 它不是 Agent，也不是工具，而是纯业务组装器：预算、日期、住宿、餐饮、交通都在这里落地。
type ItineraryAssembler struct{}

func NewItineraryAssembler() *ItineraryAssembler {
	return &ItineraryAssembler{}
}

func (a *ItineraryAssembler) Assemble(
	request domain.TripRequest,
	draft PlannerDraft,
	contexts []string,
	dayCount int,
) domain.Itinerary {
	return a.AssembleWithEvidence(request, draft, contexts, dayCount, nil)
}

func (a *ItineraryAssembler) AssembleWithEvidence(
	request domain.TripRequest,
	draft PlannerDraft,
	contexts []string,
	dayCount int,
	evidence *domain.EvidenceReport,
) domain.Itinerary {
	days := a.buildDays(request, draft, dayCount)
	tips := cleanUserTips(draft.Tips, request.Destination)
	tips = appendUniqueText(tips,
		"默认全程安排同一家酒店，减少每天换住宿的时间成本；如果你想分区域住宿，可以在生成后提出要求，我会再按需求联网匹配。",
		"每日路线按“酒店出发 → 当天点位 → 返回酒店”组织，交通方式会结合距离建议步行、公共交通或打车。",
	)
	itinerary := domain.Itinerary{
		TripID:          "trip_" + request.Destination + "_" + request.StartDate,
		Destination:     request.Destination,
		Summary:         draft.Summary,
		Days:            days,
		EstimatedBudget: 0,
		BudgetBreakdown: domain.BudgetBreakdown{},
		Tips:            tips,
		SourceNotes: append([]string{
			"Itinerary is assembled by Go TravelPlanningAgent.",
		}, firstN(contexts, 2)...),
		Evidence: evidence,
	}
	if evidence != nil {
		itinerary.SourceNotes = append(itinerary.SourceNotes, evidenceSourceNotes(*evidence)...)
		itinerary.Tips = append(itinerary.Tips, evidenceRiskTips(*evidence)...)
	}
	return refreshBudget(itinerary, request.Budget)
}

func (a *ItineraryAssembler) buildDays(request domain.TripRequest, draft PlannerDraft, dayCount int) []domain.DayPlan {
	start, _ := time.Parse("2006-01-02", request.StartDate)
	hotelLevel := request.HotelLevel
	if hotelLevel == "" {
		hotelLevel = "舒适型"
	}

	hotelCosts, mealCosts, transportCosts := allocateDailyCosts(request, dayCount)
	hotelName := defaultTripHotelName(request.Destination, hotelLevel)
	hotelLocation := request.Destination + " 市区"
	days := make([]domain.DayPlan, 0, dayCount)
	for i := 0; i < dayCount; i++ {
		dayIndex := i + 1
		dayDraft := findDayDraft(draft.Days, dayIndex)
		date := ""
		if !start.IsZero() {
			date = start.AddDate(0, 0, i).Format("2006-01-02")
		}

		hotel := domain.HotelItem{
			Name:          hotelName,
			Level:         hotelLevel,
			EstimatedCost: hotelCosts[i],
			Location:      hotelLocation,
			City:          request.Destination,
		}
		spots := buildSpotItems(request, dayDraft)
		meals := buildMealItems(request, dayDraft, mealCosts[i])
		transport := buildTimelineTransport(hotel.Name, spots, meals, transportCosts[i])
		days = append(days, domain.DayPlan{
			DayIndex:  dayIndex,
			Date:      date,
			Theme:     dayDraft.Theme,
			Spots:     spots,
			Meals:     meals,
			Hotel:     &hotel,
			Transport: transport,
			Notes: []string{
				"当前旅行节奏：" + defaultString(request.Pace, "适中"),
				dayDraft.DailyNote,
			},
		})
	}
	return days
}

func defaultTripHotelName(destination, hotelLevel string) string {
	destination = strings.TrimSpace(destination)
	hotelLevel = strings.TrimSpace(hotelLevel)
	if hotelLevel == "" {
		hotelLevel = "舒适型"
	}
	if destination == "" {
		return hotelLevel + "酒店（全程默认同住）"
	}
	return destination + hotelLevel + "酒店（全程默认同住）"
}

func buildSpotItems(request domain.TripRequest, dayDraft PlannerDayDraft) []domain.SpotItem {
	spots := make([]domain.SpotItem, 0, len(dayDraft.Spots))
	for _, spot := range dayDraft.Spots {
		name := strings.TrimSpace(spot.Name)
		if name == "" {
			continue
		}
		description := defaultString(spot.Description, dayDraft.SpotDescription)
		spots = append(spots, domain.SpotItem{
			Name:          name,
			StartTime:     defaultString(spot.StartTime, "10:00"),
			EndTime:       defaultString(spot.EndTime, "12:00"),
			Description:   description,
			EstimatedCost: estimateTicketCost(name, description),
			Location:      request.Destination,
			City:          request.Destination,
		})
	}
	if len(spots) == 0 && strings.TrimSpace(dayDraft.SpotName) != "" {
		description := defaultString(dayDraft.SpotDescription, "结合旅行偏好安排的目的地内景点。")
		spots = append(spots, domain.SpotItem{
			Name:          dayDraft.SpotName,
			StartTime:     "10:00",
			EndTime:       "12:00",
			Description:   description,
			EstimatedCost: estimateTicketCost(dayDraft.SpotName, description),
			Location:      request.Destination,
			City:          request.Destination,
		})
	}
	return spots
}

func buildMealItems(request domain.TripRequest, dayDraft PlannerDayDraft, dailyMealBudget float64) []domain.MealItem {
	meals := make([]domain.MealItem, 0, len(dayDraft.Meals))
	costs := prorate(dailyMealBudget, maxInt(1, len(dayDraft.Meals)))
	for i, meal := range dayDraft.Meals {
		name := strings.TrimSpace(meal.Name)
		if name == "" {
			continue
		}
		cost := dailyMealBudget
		if i < len(costs) {
			cost = costs[i]
		}
		meals = append(meals, domain.MealItem{
			Name:          name,
			MealType:      defaultString(meal.MealType, "午餐"),
			Time:          meal.Time,
			EstimatedCost: cost,
			Notes:         defaultString(meal.Notes, dayDraft.MealNotes),
			Location:      request.Destination,
			City:          request.Destination,
		})
	}
	if len(meals) == 0 && strings.TrimSpace(dayDraft.MealName) != "" {
		meals = append(meals, domain.MealItem{
			Name:          dayDraft.MealName,
			MealType:      "午餐",
			Time:          "12:30",
			EstimatedCost: dailyMealBudget,
			Notes:         dayDraft.MealNotes,
			Location:      request.Destination,
			City:          request.Destination,
		})
	}
	return meals
}

func buildTimelineTransport(hotelName string, spots []domain.SpotItem, meals []domain.MealItem, dailyTransportBudget float64) []domain.TransportItem {
	points := timelinePlaceNames(spots, meals)
	if len(points) == 0 {
		return nil
	}
	hotelName = strings.TrimSpace(hotelName)
	if hotelName == "" {
		hotelName = "酒店"
	}
	routePoints := make([]string, 0, len(points)+2)
	routePoints = append(routePoints, hotelName)
	routePoints = append(routePoints, points...)
	routePoints = append(routePoints, hotelName)

	legCount := len(routePoints) - 1
	costs := prorate(dailyTransportBudget, legCount)
	transports := make([]domain.TransportItem, 0, legCount)
	for i := range points {
		cost := dailyTransportBudget
		if i < len(costs) {
			cost = costs[i]
		}
		from := routePoints[i]
		to := routePoints[i+1]
		transports = append(transports, domain.TransportItem{
			Mode:          placeholderTransportMode(i, legCount),
			FromPlace:     from,
			ToPlace:       to,
			EstimatedCost: cost,
			Duration:      placeholderTransportDuration(i),
		})
	}
	// Add the final leg back to the same hotel.
	lastIndex := legCount - 1
	cost := dailyTransportBudget
	if lastIndex < len(costs) {
		cost = costs[lastIndex]
	}
	transports = append(transports, domain.TransportItem{
		Mode:          placeholderTransportMode(lastIndex, legCount),
		FromPlace:     routePoints[lastIndex],
		ToPlace:       routePoints[lastIndex+1],
		EstimatedCost: cost,
		Duration:      placeholderTransportDuration(lastIndex),
	})
	return transports
}

func timelinePlaceNames(spots []domain.SpotItem, meals []domain.MealItem) []string {
	points := []string{}
	if len(spots) > 0 {
		points = append(points, spots[0].Name)
	}
	lunch, dinner, others := splitMealNames(meals)
	if lunch != "" {
		points = append(points, lunch)
	}
	for i := 1; i < len(spots); i++ {
		points = append(points, spots[i].Name)
	}
	if dinner != "" {
		points = append(points, dinner)
	}
	points = append(points, others...)
	return points
}

func placeholderTransportMode(index, total int) string {
	if total <= 0 {
		return "交通待确认"
	}
	if index == 0 || index == total-1 {
		return "公交/地铁优先，打车备选"
	}
	if index%3 == 0 {
		return "步行"
	}
	if index%3 == 1 {
		return "公交/地铁"
	}
	return "打车/网约车"
}

func placeholderTransportDuration(index int) string {
	switch index % 3 {
	case 0:
		return "20-35 分钟"
	case 1:
		return "15-30 分钟"
	default:
		return "10-25 分钟"
	}
}

func splitMealNames(meals []domain.MealItem) (string, string, []string) {
	others := []string{}
	lunch := ""
	dinner := ""
	for _, meal := range meals {
		switch {
		case lunch == "" && strings.Contains(meal.MealType, "午"):
			lunch = meal.Name
		case dinner == "" && strings.Contains(meal.MealType, "晚"):
			dinner = meal.Name
		default:
			others = append(others, meal.Name)
		}
	}
	return lunch, dinner, others
}
