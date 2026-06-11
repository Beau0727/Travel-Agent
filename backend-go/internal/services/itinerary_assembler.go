package services

import (
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
	itinerary := domain.Itinerary{
		TripID:          "trip_" + request.Destination + "_" + request.StartDate,
		Destination:     request.Destination,
		Summary:         draft.Summary,
		Days:            days,
		EstimatedBudget: 0,
		BudgetBreakdown: domain.BudgetBreakdown{},
		Tips:            cleanUserTips(draft.Tips, request.Destination),
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
	days := make([]domain.DayPlan, 0, dayCount)
	for i := 0; i < dayCount; i++ {
		dayIndex := i + 1
		dayDraft := findDayDraft(draft.Days, dayIndex)
		date := ""
		if !start.IsZero() {
			date = start.AddDate(0, 0, i).Format("2006-01-02")
		}

		ticketCost := estimateTicketCost(dayDraft.SpotName, dayDraft.SpotDescription)
		hotel := domain.HotelItem{
			Name:          request.Destination + " " + hotelLevel + "住宿 " + itoa(dayIndex),
			Level:         hotelLevel,
			EstimatedCost: hotelCosts[i],
			Location:      request.Destination + " 市区",
		}
		days = append(days, domain.DayPlan{
			DayIndex: dayIndex,
			Date:     date,
			Theme:    dayDraft.Theme,
			Spots: []domain.SpotItem{{
				Name:          dayDraft.SpotName,
				StartTime:     "10:00",
				EndTime:       "12:00",
				Description:   dayDraft.SpotDescription,
				EstimatedCost: ticketCost,
				Location:      request.Destination,
			}},
			Meals: []domain.MealItem{{
				Name:          dayDraft.MealName,
				MealType:      "午餐",
				EstimatedCost: mealCosts[i],
				Notes:         dayDraft.MealNotes,
				Location:      request.Destination,
			}},
			Hotel: &hotel,
			Transport: []domain.TransportItem{{
				Mode:          "打车",
				FromPlace:     request.Destination + " 出发点",
				ToPlace:       dayDraft.SpotName,
				EstimatedCost: transportCosts[i],
				Duration:      "30 分钟",
			}},
			Notes: []string{
				"当前旅行节奏：" + defaultString(request.Pace, "适中"),
				dayDraft.DailyNote,
			},
		})
	}
	return days
}
