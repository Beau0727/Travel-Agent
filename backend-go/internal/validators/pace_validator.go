package validators

import "zhilv-yuntu-go/internal/domain"

type PaceValidator struct{}

func (v PaceValidator) Name() string {
	return "pace_validator"
}

func (v PaceValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	if request.Pace != "轻松" {
		return nil
	}
	issues := []Issue{}
	for _, day := range itinerary.Days {
		itemCount := len(day.Spots) + len(day.Meals) + len(day.Transport)
		if itemCount > 6 {
			issues = append(issues, Issue{
				Code:     CodePaceTooPacked,
				Level:    "warning",
				Message:  "用户选择轻松节奏，但当天安排偏多。",
				DayIndex: day.DayIndex,
			})
		}
	}
	return issues
}
