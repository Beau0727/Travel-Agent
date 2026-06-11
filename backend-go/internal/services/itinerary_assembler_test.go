package services

import (
	"strings"
	"testing"

	"travel-agent-go/internal/domain"
)

func TestAssemblerUsesSameHotelAndRoundTripTransport(t *testing.T) {
	t.Parallel()

	assembler := NewItineraryAssembler()
	itinerary := assembler.Assemble(domain.TripRequest{
		Destination: "大理",
		StartDate:   "2026-06-10",
		EndDate:     "2026-06-11",
		Budget:      3000,
		Pace:        "适中",
		HotelLevel:  "舒适型",
	}, PlannerDraft{
		Summary: "大理两日游",
		Days: []PlannerDayDraft{
			{
				DayIndex: 1,
				Theme:    "古城与洱海",
				Spots: []PlannerSpotDraft{
					{Name: "大理古城", StartTime: "09:30", EndTime: "11:00"},
					{Name: "洱海生态廊道", StartTime: "14:00", EndTime: "16:00"},
				},
				Meals: []PlannerMealDraft{
					{Name: "大理白族风味餐厅", MealType: "午餐"},
					{Name: "洱海砂锅鱼餐厅", MealType: "晚餐"},
				},
				DailyNote: "轻松游览。",
			},
			{
				DayIndex: 2,
				Theme:    "喜洲慢行",
				Spots: []PlannerSpotDraft{
					{Name: "喜洲古镇", StartTime: "10:00", EndTime: "12:00"},
					{Name: "双廊古镇", StartTime: "14:30", EndTime: "16:30"},
				},
				Meals: []PlannerMealDraft{
					{Name: "喜洲粑粑小吃店", MealType: "午餐"},
					{Name: "大理乳扇小吃", MealType: "晚餐"},
				},
				DailyNote: "错峰出行。",
			},
		},
	}, nil, 2)

	if len(itinerary.Days) != 2 {
		t.Fatalf("expected 2 days, got %d", len(itinerary.Days))
	}
	firstHotel := itinerary.Days[0].Hotel
	secondHotel := itinerary.Days[1].Hotel
	if firstHotel == nil || secondHotel == nil {
		t.Fatalf("expected hotel on every day: %#v", itinerary.Days)
	}
	if firstHotel.Name == "" || firstHotel.Name != secondHotel.Name {
		t.Fatalf("expected same hotel across days, got %q and %q", firstHotel.Name, secondHotel.Name)
	}
	for _, day := range itinerary.Days {
		if len(day.Transport) == 0 {
			t.Fatalf("expected transport legs for day %d", day.DayIndex)
		}
		if day.Transport[0].FromPlace != day.Hotel.Name {
			t.Fatalf("expected day %d to start from hotel, got %#v", day.DayIndex, day.Transport[0])
		}
		last := day.Transport[len(day.Transport)-1]
		if last.ToPlace != day.Hotel.Name {
			t.Fatalf("expected day %d to return to hotel, got %#v", day.DayIndex, last)
		}
		onlyTaxi := true
		for _, leg := range day.Transport {
			if !strings.Contains(leg.Mode, "打车") {
				onlyTaxi = false
				break
			}
		}
		if onlyTaxi {
			t.Fatalf("expected varied transport modes, got %#v", day.Transport)
		}
	}
}
