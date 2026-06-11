package validators

import (
	"strings"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/geo"
)

type DestinationConsistencyValidator struct{}

func (v DestinationConsistencyValidator) Name() string {
	return "destination_consistency_validator"
}

func (v DestinationConsistencyValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	destination := strings.TrimSpace(request.Destination)
	if destination == "" {
		destination = strings.TrimSpace(itinerary.Destination)
	}
	if destination == "" {
		return nil
	}

	issues := []Issue{}
	for _, day := range itinerary.Days {
		if day.Hotel != nil && placeConflictsDestination(destination, day.Hotel.Name, day.Hotel.Location, day.Hotel.City, day.Hotel.Address) {
			issues = append(issues, Issue{
				Code:     CodeDestinationMismatch,
				Level:    "error",
				Message:  "住宿点位疑似不在目的地城市内，需要重新匹配当前城市酒店。",
				DayIndex: day.DayIndex,
			})
		}
		for _, spot := range day.Spots {
			if placeConflictsDestination(destination, spot.Name, spot.Location, spot.City, spot.Address) {
				issues = append(issues, Issue{
					Code:     CodeDestinationMismatch,
					Level:    "error",
					Message:  "行程包含疑似其他城市的景点，需要替换为目的地城市内点位。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
		for _, meal := range day.Meals {
			if placeConflictsDestination(destination, meal.Name, meal.Location, meal.City, meal.Address) {
				issues = append(issues, Issue{
					Code:     CodeDestinationMismatch,
					Level:    "error",
					Message:  "行程包含疑似其他城市的餐饮，需要替换为目的地城市内餐饮。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
	}
	return issues
}

func placeConflictsDestination(destination string, values ...string) bool {
	if len(values) == 0 {
		return false
	}
	name := strings.TrimSpace(values[0])
	if name != "" && geo.HasConflictingCityMention(destination, name) {
		return true
	}
	for _, value := range values[1:] {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if geo.HasConflictingCityMention(destination, value) && !geo.CityMatchesDestination(value, destination) {
			return true
		}
	}
	return false
}
