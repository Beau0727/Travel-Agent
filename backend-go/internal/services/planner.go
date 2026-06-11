package services

import (
	"strings"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/geo"
)

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
	Spots           []PlannerSpotDraft `json:"spots,omitempty"`
	Meals           []PlannerMealDraft `json:"meals,omitempty"`
}

type PlannerSpotDraft struct {
	Name        string `json:"name"`
	StartTime   string `json:"start_time,omitempty"`
	EndTime     string `json:"end_time,omitempty"`
	Description string `json:"description,omitempty"`
}

type PlannerMealDraft struct {
	Name     string `json:"name"`
	MealType string `json:"meal_type,omitempty"`
	Time     string `json:"time,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

type DayEditDraft struct {
	Theme           string   `json:"theme"`
	SpotName        string   `json:"spot_name"`
	SpotDescription string   `json:"spot_description"`
	MealName        string   `json:"meal_name"`
	MealNotes       string   `json:"meal_notes"`
	DailyNote       string   `json:"daily_note"`
	ChangeSummary   []string `json:"change_summary,omitempty"`
}

// RulePlanner 是纯规则版本的 Planner。
// 它不调用大模型，适合作为初学 Go 的起点，也适合没有 API Key 时本地运行。
type RulePlanner struct{}

func NewRulePlanner() *RulePlanner {
	return &RulePlanner{}
}

func (p *RulePlanner) GenerateDraft(request domain.TripRequest, contexts []string, dayCount int) (PlannerDraft, bool, error) {
	days := make([]PlannerDayDraft, 0, dayCount)
	spotTarget := dailySpotTarget(request.Pace)
	mealTarget := dailyMealTarget(request.Pace)
	spots := pickDemoSpots(request.Destination, contexts, dayCount*spotTarget)
	meals := pickDemoMeals(request.Destination, contexts, dayCount*mealTarget)
	for i := 0; i < dayCount; i++ {
		dayIndex := i + 1
		daySpots := plannerSpotDrafts(spots[i*spotTarget:(i+1)*spotTarget], daySpotTimeSlots(spotTarget), "根据本地攻略和旅行偏好安排，适合用半天时间慢慢游览。")
		dayMeals := plannerMealDrafts(meals[i*mealTarget:(i+1)*mealTarget], mealTarget, "根据用户偏好和本地攻略预留的一条餐饮建议。")
		days = append(days, PlannerDayDraft{
			DayIndex:        dayIndex,
			Theme:           request.Destination + " 第 " + itoa(dayIndex) + " 天轻松游",
			SpotName:        daySpots[0].Name,
			SpotDescription: daySpots[0].Description,
			MealName:        dayMeals[0].Name,
			MealNotes:       dayMeals[0].Notes,
			DailyNote:       "今天以轻松游览为主，建议根据体力和天气灵活调整停留时间。",
			Spots:           daySpots,
			Meals:           dayMeals,
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

func SanitizePlannerDraft(request domain.TripRequest, contexts []string, draft PlannerDraft, dayCount int) PlannerDraft {
	return SanitizePlannerDraftWithCandidates(request, contexts, draft, dayCount, PlaceCandidateBundle{})
}

func SanitizePlannerDraftWithCandidates(request domain.TripRequest, contexts []string, draft PlannerDraft, dayCount int, candidates PlaceCandidateBundle) PlannerDraft {
	if dayCount <= 0 {
		dayCount = len(draft.Days)
	}
	if dayCount <= 0 {
		dayCount = 1
	}

	spotTarget := dailySpotTarget(request.Pace)
	mealTarget := dailyMealTarget(request.Pace)
	spots := candidateNamesOrFallback(CandidateNames(candidates.Attractions), pickDemoSpots(request.Destination, contexts, dayCount*spotTarget), dayCount*spotTarget)
	meals := candidateNamesOrFallback(CandidateNames(candidates.Meals), pickDemoMeals(request.Destination, contexts, dayCount*mealTarget), dayCount*mealTarget)
	for i := 0; i < dayCount; i++ {
		if i >= len(draft.Days) {
			draft.Days = append(draft.Days, PlannerDayDraft{DayIndex: i + 1})
		}
		if draft.Days[i].DayIndex <= 0 {
			draft.Days[i].DayIndex = i + 1
		}
		daySpots := spots[i*spotTarget : (i+1)*spotTarget]
		dayMeals := selectMealNamesForDay(meals, draft.Days[i].Spots, daySpots, i, mealTarget)
		normalizePlannerDayPlaces(&draft.Days[i], request.Destination, daySpots, dayMeals, spotTarget, mealTarget)
		enforcePlannerDayCandidatePools(&draft.Days[i], request.Destination, daySpots, dayMeals, len(candidates.Attractions) > 0, len(candidates.Meals) > 0)
		if draft.Days[i].Theme == "" {
			draft.Days[i].Theme = request.Destination + " 第 " + itoa(i+1) + " 天行程"
		}
		if draft.Days[i].SpotDescription == "" {
			draft.Days[i].SpotDescription = "结合在线资料、本地攻略和旅行偏好安排，出发前建议再核对开放状态。"
		}
		if draft.Days[i].MealNotes == "" {
			draft.Days[i].MealNotes = "可按当天路线和排队情况灵活选择同类本地餐饮。"
		}
		if draft.Days[i].DailyNote == "" {
			draft.Days[i].DailyNote = "当天节奏可根据天气、体力和交通情况微调。"
		}
		draft.Days[i].SpotName = draft.Days[i].Spots[0].Name
		draft.Days[i].SpotDescription = defaultString(draft.Days[i].Spots[0].Description, draft.Days[i].SpotDescription)
		draft.Days[i].MealName = draft.Days[i].Meals[0].Name
		draft.Days[i].MealNotes = defaultString(draft.Days[i].Meals[0].Notes, draft.Days[i].MealNotes)
	}
	if len(draft.Days) > dayCount {
		draft.Days = draft.Days[:dayCount]
	}
	if draft.Summary == "" {
		draft.Summary = "这是一份为 " + request.Destination + " 生成的 " + itoa(dayCount) + " 日行程。"
	}
	draft.Tips = cleanStringList(draft.Tips)
	if len(draft.Tips) == 0 {
		draft.Tips = []string{"出发前建议复核景点开放状态、票务预约和地图实时交通。"}
	}
	return draft
}

func candidateNamesOrFallback(candidateNames []string, fallback []string, target int) []string {
	names := appendUniqueStrings(candidateNames, fallback...)
	if target <= 0 {
		return names
	}
	for len(names) < target && len(fallback) > 0 {
		names = appendUniqueStrings(names, fallback...)
		if len(names) < target {
			names = append(names, fallback[len(names)%len(fallback)])
		}
	}
	if len(names) == 0 {
		return fallback
	}
	for len(names) < target {
		names = append(names, names[len(names)%len(names)])
	}
	return names[:target]
}

func enforcePlannerDayCandidatePools(day *PlannerDayDraft, destination string, spotPool []string, mealPool []string, enforceSpots bool, enforceMeals bool) {
	for i := range day.Spots {
		if enforceSpots && !contains(spotPool, day.Spots[i].Name) {
			day.Spots[i].Name = spotPool[i%len(spotPool)]
		}
		if isPlaceholderName(day.Spots[i].Name, destination) || geo.HasConflictingCityMention(destination, day.Spots[i].Name) {
			day.Spots[i].Name = spotPool[i%len(spotPool)]
		}
	}
	for i := range day.Meals {
		if enforceMeals && !contains(mealPool, day.Meals[i].Name) {
			day.Meals[i].Name = mealPool[i%len(mealPool)]
		}
		if isPlaceholderName(day.Meals[i].Name, destination) || geo.HasConflictingCityMention(destination, day.Meals[i].Name) {
			day.Meals[i].Name = mealPool[i%len(mealPool)]
		}
	}
}

func selectMealNamesForDay(meals []string, existingSpots []PlannerSpotDraft, fallbackSpots []string, dayIndex int, mealTarget int) []string {
	if len(meals) == 0 {
		return nil
	}
	if mealTarget <= 0 {
		mealTarget = 2
	}
	spotNames := make([]string, 0, len(existingSpots)+len(fallbackSpots))
	for _, spot := range existingSpots {
		if strings.TrimSpace(spot.Name) != "" {
			spotNames = append(spotNames, spot.Name)
		}
	}
	spotNames = append(spotNames, fallbackSpots...)

	scored := make([]mealScore, 0, len(meals))
	for index, meal := range meals {
		score := 0
		for _, spot := range spotNames {
			score += placeNameAffinity(spot, meal)
		}
		score -= cyclicDistance(index, dayIndex*mealTarget)
		scored = append(scored, mealScore{name: meal, score: score, index: index})
	}
	sortScoredMeals(scored)

	selected := make([]string, 0, mealTarget)
	for _, meal := range scored {
		if !contains(selected, meal.name) {
			selected = append(selected, meal.name)
		}
		if len(selected) >= mealTarget {
			return selected
		}
	}
	for len(selected) < mealTarget {
		selected = append(selected, meals[(dayIndex*mealTarget+len(selected))%len(meals)])
	}
	return selected
}

type mealScore struct {
	name  string
	score int
	index int
}

func placeNameAffinity(spotName, mealName string) int {
	spotTokens := meaningfulPlaceTokens(spotName)
	mealTokens := meaningfulPlaceTokens(mealName)
	score := 0
	for _, spotToken := range spotTokens {
		for _, mealToken := range mealTokens {
			if spotToken == mealToken {
				score += 6
				continue
			}
			if strings.Contains(spotToken, mealToken) || strings.Contains(mealToken, spotToken) {
				score += 3
			}
		}
		if strings.Contains(mealName, spotToken) {
			score += 5
		}
	}
	return score
}

func meaningfulPlaceTokens(name string) []string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer(
		"风味", " ",
		"餐厅", " ",
		"饭店", " ",
		"小吃店", " ",
		"小吃", " ",
		"菜馆", " ",
		"美食街", " ",
		"海鲜", " ",
		"古城", " 古城 ",
		"古镇", " 古镇 ",
		"公园", " 公园 ",
		"广场", " 广场 ",
	)
	name = replacer.Replace(name)
	raw := strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '·' || r == '-' || r == '_' || r == '（' || r == '）' || r == '(' || r == ')'
	})
	tokens := []string{}
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if len([]rune(token)) >= 2 && !contains(tokens, token) {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func cyclicDistance(index, target int) int {
	if index >= target {
		return index - target
	}
	return target - index
}

func sortScoredMeals(values []mealScore) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j].score > values[i].score || (values[j].score == values[i].score && values[j].index < values[i].index) {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func normalizePlannerDayPlaces(day *PlannerDayDraft, destination string, spotFallbacks []string, mealFallbacks []string, spotTarget int, mealTarget int) {
	if len(day.Spots) == 0 && strings.TrimSpace(day.SpotName) != "" {
		day.Spots = []PlannerSpotDraft{{
			Name:        day.SpotName,
			Description: day.SpotDescription,
		}}
	}
	if len(day.Meals) == 0 && strings.TrimSpace(day.MealName) != "" {
		day.Meals = []PlannerMealDraft{{
			Name:     day.MealName,
			MealType: "午餐",
			Notes:    day.MealNotes,
		}}
	}
	timeSlots := daySpotTimeSlots(spotTarget)
	for len(day.Spots) < spotTarget {
		index := len(day.Spots)
		name := spotFallbacks[index%len(spotFallbacks)]
		day.Spots = append(day.Spots, PlannerSpotDraft{
			Name:        name,
			Description: "结合在线资料、本地攻略和旅行偏好安排，出发前建议再核对开放状态。",
		})
	}
	for i := range day.Spots {
		if isPlaceholderName(day.Spots[i].Name, destination) {
			day.Spots[i].Name = spotFallbacks[i%len(spotFallbacks)]
		}
		if day.Spots[i].StartTime == "" {
			day.Spots[i].StartTime = timeSlots[i%len(timeSlots)][0]
		}
		if day.Spots[i].EndTime == "" {
			day.Spots[i].EndTime = timeSlots[i%len(timeSlots)][1]
		}
		if day.Spots[i].Description == "" {
			day.Spots[i].Description = "结合在线资料、本地攻略和旅行偏好安排，出发前建议再核对开放状态。"
		}
	}
	for len(day.Meals) < mealTarget {
		index := len(day.Meals)
		name := mealFallbacks[index%len(mealFallbacks)]
		day.Meals = append(day.Meals, PlannerMealDraft{
			Name:  name,
			Notes: "可按当天路线和排队情况灵活选择同类本地餐饮。",
		})
	}
	mealTypes := defaultMealTypes(mealTarget)
	mealTimes := defaultMealTimes(mealTarget)
	for i := range day.Meals {
		if isPlaceholderName(day.Meals[i].Name, destination) {
			day.Meals[i].Name = mealFallbacks[i%len(mealFallbacks)]
		}
		if day.Meals[i].MealType == "" {
			day.Meals[i].MealType = mealTypes[i%len(mealTypes)]
		}
		if day.Meals[i].Time == "" {
			day.Meals[i].Time = mealTimes[i%len(mealTimes)]
		}
		if day.Meals[i].Notes == "" {
			day.Meals[i].Notes = "可按当天路线和排队情况灵活选择同类本地餐饮。"
		}
	}
	if day.SpotName == "" || isPlaceholderName(day.SpotName, destination) {
		day.SpotName = day.Spots[0].Name
	}
	if day.SpotDescription == "" {
		day.SpotDescription = day.Spots[0].Description
	}
	if day.MealName == "" || isPlaceholderName(day.MealName, destination) {
		day.MealName = day.Meals[0].Name
	}
	if day.MealNotes == "" {
		day.MealNotes = day.Meals[0].Notes
	}
}

func plannerSpotDrafts(names []string, slots [][2]string, description string) []PlannerSpotDraft {
	spots := make([]PlannerSpotDraft, 0, len(names))
	for i, name := range names {
		slot := slots[i%len(slots)]
		spots = append(spots, PlannerSpotDraft{
			Name:        name,
			StartTime:   slot[0],
			EndTime:     slot[1],
			Description: description,
		})
	}
	return spots
}

func plannerMealDrafts(names []string, count int, notes string) []PlannerMealDraft {
	types := defaultMealTypes(count)
	times := defaultMealTimes(count)
	meals := make([]PlannerMealDraft, 0, len(names))
	for i, name := range names {
		meals = append(meals, PlannerMealDraft{
			Name:     name,
			MealType: types[i%len(types)],
			Time:     times[i%len(times)],
			Notes:    notes,
		})
	}
	return meals
}

func dailySpotTarget(pace string) int {
	pace = strings.ToLower(strings.TrimSpace(pace))
	switch {
	case strings.Contains(pace, "紧凑"), strings.Contains(pace, "compact"), strings.Contains(pace, "fast"):
		return 4
	case strings.Contains(pace, "轻松"), strings.Contains(pace, "relaxed"), strings.Contains(pace, "slow"):
		return 2
	default:
		return 3
	}
}

func dailyMealTarget(pace string) int {
	return 2
}

func daySpotTimeSlots(count int) [][2]string {
	if count <= 2 {
		return [][2]string{{"10:00", "12:00"}, {"14:30", "16:30"}}
	}
	if count >= 4 {
		return [][2]string{{"09:30", "10:45"}, {"11:00", "12:15"}, {"14:30", "16:00"}, {"16:20", "17:40"}}
	}
	return [][2]string{{"09:30", "11:00"}, {"14:00", "15:30"}, {"16:00", "17:30"}}
}

func defaultMealTypes(count int) []string {
	if count <= 1 {
		return []string{"午餐"}
	}
	return []string{"午餐", "晚餐"}
}

func defaultMealTimes(count int) []string {
	if count <= 1 {
		return []string{"12:30"}
	}
	return []string{"12:30", "18:30"}
}
