package validators

import (
	"strings"

	"travel-agent-go/internal/domain"
)

type PreferenceValidator struct{}

func (v PreferenceValidator) Name() string {
	return "preference_validator"
}

func (v PreferenceValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	if !strings.Contains(request.SpecialNotes, "不想太早") && !strings.Contains(request.SpecialNotes, "自然醒") {
		return nil
	}

	issues := []Issue{}
	for _, day := range itinerary.Days {
		for _, spot := range day.Spots {
			if spot.StartTime != "" && spot.StartTime < "10:00" {
				issues = append(issues, Issue{
					Code:     CodeEarlyStartConflict,
					Level:    "warning",
					Message:  "用户不想太早起床，但行程开始时间早于 10:00。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
	}
	return issues
}
