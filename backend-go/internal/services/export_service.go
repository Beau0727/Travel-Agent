package services

import (
	"strings"

	"zhilv-yuntu-go/internal/domain"
)

// RenderMarkdown 把 Itinerary 渲染成 Markdown。
// 这对应 Python 版 export_service.py 里的 itinerary_to_markdown。
func RenderMarkdown(detail domain.TripDetailResponse) string {
	itinerary := detail.Itinerary
	var b strings.Builder
	b.WriteString("# " + itinerary.Destination + " 行程单\n\n")
	b.WriteString("- 行程 ID：" + detail.TripID + "\n")
	b.WriteString("- 目的地：" + itinerary.Destination + "\n")
	b.WriteString("- 预计预算：" + floatToString(itinerary.EstimatedBudget) + " 元\n\n")
	b.WriteString("## 行程概述\n")
	b.WriteString(itinerary.Summary + "\n\n")
	b.WriteString("## 每日安排\n")

	for _, day := range itinerary.Days {
		b.WriteString("\n### Day " + itoa(day.DayIndex) + " " + day.Theme + "\n")
		if day.Date != "" {
			b.WriteString("- 日期：" + day.Date + "\n")
		}
		for _, spot := range day.Spots {
			b.WriteString("- 主要景点：" + spot.Name + "\n")
			b.WriteString("  - 时间：" + defaultString(spot.StartTime, "待定") + " - " + defaultString(spot.EndTime, "待定") + "\n")
			b.WriteString("  - 说明：" + defaultString(spot.Description, "无") + "\n")
		}
		for _, meal := range day.Meals {
			b.WriteString("- 餐饮建议：" + meal.Name + "（" + meal.MealType + "）\n")
			if meal.Address != "" || meal.Location != "" {
				b.WriteString("  - 位置：" + defaultString(meal.Address, meal.Location) + "\n")
			}
			b.WriteString("  - 说明：" + defaultString(meal.Notes, "无") + "\n")
		}
		if day.Hotel != nil {
			b.WriteString("- 住宿安排：" + day.Hotel.Name + "（" + defaultString(day.Hotel.Level, "未标注档次") + "）\n")
		}
		for _, note := range day.Notes {
			b.WriteString("- 备注：" + note + "\n")
		}
	}

	budget := itinerary.BudgetBreakdown
	b.WriteString("\n## 预算拆分\n")
	b.WriteString("- 交通：" + floatToString(budget.Transport) + " 元\n")
	b.WriteString("- 住宿：" + floatToString(budget.Hotel) + " 元\n")
	b.WriteString("- 餐饮：" + floatToString(budget.Meals) + " 元\n")
	b.WriteString("- 门票：" + floatToString(budget.Tickets) + " 元\n")
	b.WriteString("- 其他：" + floatToString(budget.Other) + " 元\n")
	b.WriteString("- 总计：" + floatToString(budget.Total) + " 元\n")

	if len(itinerary.Tips) > 0 {
		b.WriteString("\n## 旅行提示\n")
		for _, tip := range itinerary.Tips {
			b.WriteString("- " + tip + "\n")
		}
	}
	return b.String()
}
