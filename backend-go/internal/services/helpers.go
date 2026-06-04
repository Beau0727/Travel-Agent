package services

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/domain"
)

func calcDayCount(startDate, endDate string) int {
	start, err1 := time.Parse("2006-01-02", startDate)
	end, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return 1
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}

func estimateTicketCost(name, description string) float64 {
	text := name + " " + description
	bucket := stableBucket(text, 4)
	if strings.Contains(text, "古城") || strings.Contains(text, "古镇") || strings.Contains(text, "公园") || strings.Contains(text, "廊道") {
		return []float64{0, 20, 30, 40}[bucket]
	}
	if strings.Contains(text, "寺") || strings.Contains(text, "三塔") || strings.Contains(text, "博物馆") {
		return 60 + float64(bucket)*18
	}
	if strings.Contains(text, "索道") || strings.Contains(text, "缆车") || strings.Contains(text, "游船") {
		return 120 + float64(bucket)*28
	}
	return 35 + float64(bucket)*12
}

func refreshBudget(itinerary domain.Itinerary, requestBudget float64) domain.Itinerary {
	var transport, hotel, meals, tickets float64
	for _, day := range itinerary.Days {
		for _, item := range day.Transport {
			transport += item.EstimatedCost
		}
		if day.Hotel != nil {
			hotel += day.Hotel.EstimatedCost
		}
		for _, item := range day.Meals {
			meals += item.EstimatedCost
		}
		for _, item := range day.Spots {
			tickets += item.EstimatedCost
		}
	}

	subtotal := round2(transport + hotel + meals + tickets)
	other := round2(math.Max(subtotal*0.06, 0))
	if requestBudget > 0 {
		other = round2(math.Max(0, math.Min(requestBudget*0.12, requestBudget-subtotal)))
	}
	total := round2(subtotal + other)

	itinerary.BudgetBreakdown = domain.BudgetBreakdown{
		Transport: round2(transport),
		Hotel:     round2(hotel),
		Meals:     round2(meals),
		Tickets:   round2(tickets),
		Other:     other,
		Total:     total,
	}
	itinerary.EstimatedBudget = total
	return itinerary
}

func applyDayEditDraft(day *domain.DayPlan, draft DayEditDraft) {
	day.Theme = draft.Theme
	if len(day.Spots) > 0 {
		day.Spots[0].Name = draft.SpotName
		day.Spots[0].Description = draft.SpotDescription
		day.Spots[0].EstimatedCost = estimateTicketCost(draft.SpotName, draft.SpotDescription)
	}
	if len(day.Meals) > 0 {
		day.Meals[0].Name = draft.MealName
		day.Meals[0].Notes = draft.MealNotes
	}
	if len(day.Notes) > 0 {
		day.Notes[len(day.Notes)-1] = draft.DailyNote
	} else {
		day.Notes = append(day.Notes, draft.DailyNote)
	}
}

func applyRuleEdit(day *domain.DayPlan, instruction string) {
	if strings.Contains(instruction, "轻松") {
		day.Theme = day.Theme + "（已调整为更轻松）"
		day.Notes = append(day.Notes, "已根据用户要求把节奏调整得更轻松。")
	}
	if strings.Contains(instruction, "不要安排") && len(day.Spots) > 0 {
		day.Spots[0].Name = "自由活动 / 弹性安排"
		day.Spots[0].Description = "根据用户要求，减少固定景点安排，保留更多自由活动时间。"
		day.Spots[0].EstimatedCost = 0
	}
}

func pickDemoSpots(destination string, contexts []string, dayCount int) []string {
	joined := strings.Join(contexts, "\n")
	candidates := []string{}
	for _, name := range []string{"大理古城", "喜洲古镇", "崇圣寺三塔", "洱海生态廊道", "宽窄巷子", "兵马俑", "鼓浪屿", "亚龙湾"} {
		if strings.Contains(joined, name) {
			candidates = append(candidates, name)
		}
	}
	for len(candidates) < dayCount {
		candidates = append(candidates, destination+" 推荐景点 "+itoa(len(candidates)+1))
	}
	return candidates[:dayCount]
}

func cleanUserTips(tips []string, destination string) []string {
	technical := []string{"LLM", "RAG", "LangChain", "Chroma", "演示", "测试", "规则", "模型", "源码"}
	result := []string{}
	for _, tip := range tips {
		tip = strings.TrimSpace(tip)
		if tip == "" {
			continue
		}
		skip := false
		for _, keyword := range technical {
			if strings.Contains(tip, keyword) {
				skip = true
				break
			}
		}
		if !skip && !contains(result, tip) {
			result = append(result, tip)
		}
	}
	if len(result) > 0 {
		return result
	}
	return []string{
		"建议根据" + destination + "当天实时天气准备雨具或薄外套，早晚和临水区域体感可能偏凉。",
		"热门景点建议错峰出发，给拍照、用餐和交通预留更从容的缓冲时间。",
	}
}

func prorate(total float64, count int) []float64 {
	if count <= 0 {
		return []float64{}
	}
	cents := int(math.Round(total * 100))
	base := cents / count
	remainder := cents % count
	values := make([]float64, count)
	for i := range values {
		value := base
		if i < remainder {
			value++
		}
		values[i] = float64(value) / 100
	}
	return values
}

func firstN(values []string, n int) []string {
	if len(values) < n {
		n = len(values)
	}
	return values[:n]
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func joinOrDefault(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, "、")
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func stableBucket(text string, modulo int) int {
	sum := 0
	for _, char := range text {
		sum += int(char)
	}
	if modulo <= 0 {
		return 0
	}
	return sum % modulo
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func floatToString(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func fmtSscanf(text, format string, args ...any) (int, error) {
	return fmt.Sscanf(text, format, args...)
}
