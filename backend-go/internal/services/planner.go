package services

import "zhilv-yuntu-go/internal/domain"

// Planner 定义“行程生成器”的能力。
// 这是一种策略模式：TripService 不关心具体是 LLM 生成、规则生成还是测试假对象生成，
// 只需要调用 GenerateDraft。
type Planner interface {
	GenerateDraft(request domain.TripRequest, contexts []string, dayCount int) (PlannerDraft, bool, error)
	EditDay(request domain.TripEditRequest, targetDay domain.DayPlan) (DayEditDraft, bool, error)
}

type PlannerDraft struct {
	Summary string            `json:"summary"`
	Tips    []string          `json:"tips"`
	Days    []PlannerDayDraft `json:"days"`
}

type PlannerDayDraft struct {
	DayIndex        int    `json:"day_index"`
	Theme           string `json:"theme"`
	SpotName        string `json:"spot_name"`
	SpotDescription string `json:"spot_description"`
	MealName        string `json:"meal_name"`
	MealNotes       string `json:"meal_notes"`
	DailyNote       string `json:"daily_note"`
}

type DayEditDraft struct {
	Theme           string `json:"theme"`
	SpotName        string `json:"spot_name"`
	SpotDescription string `json:"spot_description"`
	MealName        string `json:"meal_name"`
	MealNotes       string `json:"meal_notes"`
	DailyNote       string `json:"daily_note"`
}

// RulePlanner 是纯规则版本的 Planner。
// 它不调用大模型，适合作为初学 Go 的起点，也适合没有 API Key 时本地运行。
type RulePlanner struct{}

func NewRulePlanner() *RulePlanner {
	return &RulePlanner{}
}

func (p *RulePlanner) GenerateDraft(request domain.TripRequest, contexts []string, dayCount int) (PlannerDraft, bool, error) {
	days := make([]PlannerDayDraft, 0, dayCount)
	spots := pickDemoSpots(request.Destination, contexts, dayCount)
	for i := 0; i < dayCount; i++ {
		dayIndex := i + 1
		spot := spots[i]
		days = append(days, PlannerDayDraft{
			DayIndex:        dayIndex,
			Theme:           request.Destination + " 第 " + itoa(dayIndex) + " 天轻松游",
			SpotName:        spot,
			SpotDescription: "根据本地攻略和旅行偏好安排，适合用半天时间慢慢游览。",
			MealName:        request.Destination + " 特色餐饮 " + itoa(dayIndex),
			MealNotes:       "根据用户偏好和本地攻略预留的一条餐饮建议。",
			DailyNote:       "今天以轻松游览为主，建议根据体力和天气灵活调整停留时间。",
		})
	}

	preferenceText := joinOrDefault(request.Preferences, "常规旅行体验")
	return PlannerDraft{
		Summary: "这是一份为 " + request.Destination + " 生成的 " + itoa(dayCount) + " 日行程，偏好重点为：" + preferenceText + "。",
		Tips: []string{
			"建议根据" + request.Destination + "当天实时天气准备雨具或薄外套。",
			"古镇、生态廊道和石板路更适合慢慢走，鞋子尽量选择舒适防滑的款式。",
		},
		Days: days,
	}, true, nil
}

func (p *RulePlanner) EditDay(request domain.TripEditRequest, targetDay domain.DayPlan) (DayEditDraft, bool, error) {
	return DayEditDraft{}, false, nil
}
