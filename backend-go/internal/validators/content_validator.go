package validators

import (
	"strings"

	"travel-agent-go/internal/domain"
)

type ContentValidator struct{}

func (v ContentValidator) Name() string {
	return "content_validator"
}

func (v ContentValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	issues := []Issue{}
	for _, day := range itinerary.Days {
		for _, spot := range day.Spots {
			if isPlaceholderContent(spot.Name, request.Destination) {
				issues = append(issues, Issue{
					Code:     CodePlaceholderContent,
					Level:    "error",
					Message:  "行程包含占位景点名称，需要替换为真实点位。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
		for _, meal := range day.Meals {
			if isPlaceholderContent(meal.Name, request.Destination) {
				issues = append(issues, Issue{
					Code:     CodePlaceholderContent,
					Level:    "error",
					Message:  "行程包含占位餐饮名称，需要替换为真实餐饮或本地风味候选。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
		for _, transport := range day.Transport {
			if isPlaceholderContent(transport.FromPlace, request.Destination) || isPlaceholderContent(transport.ToPlace, request.Destination) {
				issues = append(issues, Issue{
					Code:     CodePlaceholderContent,
					Level:    "warning",
					Message:  "交通段包含占位出发点或目的地，需要等待真实点位补全后重算。",
					DayIndex: day.DayIndex,
				})
				break
			}
		}
	}
	return issues
}

func isPlaceholderContent(value, destination string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	placeholders := []string{
		"推荐景点", "特色餐饮", "出发点", "待确认", "待定",
		"景点 1", "景点1", "餐饮 1", "餐饮1",
	}
	for _, placeholder := range placeholders {
		if strings.Contains(value, placeholder) {
			return true
		}
	}
	return destination != "" && (value == destination || value == destination+"市")
}
