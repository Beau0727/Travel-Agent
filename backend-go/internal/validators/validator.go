package validators

import "zhilv-yuntu-go/internal/domain"

const (
	CodeBudgetExceeded     = "budget_exceeded"
	CodeEarlyStartConflict = "early_start_conflict"
	CodePaceTooPacked      = "pace_too_packed"
)

// Issue 是校验器发现的问题。
type Issue struct {
	Code     string
	Level    string
	Message  string
	DayIndex int
}

// Validator 是 Agent 的“自我检查”节点。
// 不同校验器只关心一类质量问题，组合起来就是结果评审层。
type Validator interface {
	Name() string
	Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue
}

type Set struct {
	validators []Validator
}

func NewSet(validators ...Validator) *Set {
	return &Set{validators: validators}
}

func NewDefaultSet() *Set {
	return NewSet(
		BudgetValidator{},
		PaceValidator{},
		PreferenceValidator{},
	)
}

func (s *Set) Validate(request domain.TripRequest, itinerary domain.Itinerary) []Issue {
	issues := []Issue{}
	for _, validator := range s.validators {
		issues = append(issues, validator.Validate(request, itinerary)...)
	}
	return issues
}
