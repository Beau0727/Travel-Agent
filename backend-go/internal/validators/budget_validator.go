package validators

import "zhilv-yuntu-go/internal/domain"

type BudgetValidator struct{}

func (v BudgetValidator) Name() string {
	return "budget_validator"
}

func (v BudgetValidator) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	if request.Budget <= 0 {
		return nil
	}
	if itinerary.EstimatedBudget <= request.Budget*1.05 {
		return nil
	}
	return []Issue{{
		Code:    CodeBudgetExceeded,
		Level:   "warning",
		Message: "预估费用超过用户预算 5% 以上，需要提示用户或压缩安排。",
	}}
}
