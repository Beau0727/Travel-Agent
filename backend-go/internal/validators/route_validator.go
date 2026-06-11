package validators

import "travel-agent-go/internal/domain"

const (
	CodeRouteTooLong     = "route_too_long"
	CodeWalkingTooLong   = "walking_too_long"
	CodeRouteUnavailable = "route_unavailable"
)

type RouteValidator struct{}

func (v RouteValidator) Name() string {
	return "route_validator"
}

func (v RouteValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	issues := []Issue{}
	for _, day := range itinerary.Days {
		totalSeconds := 0
		walkingMeters := 0
		reportedUnavailable := false

		for _, item := range day.Transport {
			totalSeconds += item.DurationSeconds
			if item.RouteMode == "walking" {
				walkingMeters += item.DistanceMeters
			}
			if item.RouteStatus == "failed" && !reportedUnavailable {
				reportedUnavailable = true
				issues = append(issues, Issue{
					Code:     CodeRouteUnavailable,
					Level:    "warning",
					Message:  "部分路线未能完成真实路径规划，需要出行前再次确认交通。",
					DayIndex: day.DayIndex,
				})
			}
		}

		if totalSeconds > 4*60*60 {
			issues = append(issues, Issue{
				Code:     CodeRouteTooLong,
				Level:    "warning",
				Message:  "当天真实交通耗时较长，建议减少点位或调整顺序。",
				DayIndex: day.DayIndex,
			})
		}
		if request.Pace == "轻松" && walkingMeters > 5000 {
			issues = append(issues, Issue{
				Code:     CodeWalkingTooLong,
				Level:    "warning",
				Message:  "用户选择轻松节奏，但当天步行距离偏长。",
				DayIndex: day.DayIndex,
			})
		}
	}
	return issues
}
