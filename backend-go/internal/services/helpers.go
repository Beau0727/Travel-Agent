package services

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"travel-agent-go/internal/domain"
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
	if strings.TrimSpace(draft.Theme) != "" {
		day.Theme = draft.Theme
	}
	if len(day.Spots) > 0 && strings.TrimSpace(draft.SpotName) != "" {
		if day.Spots[0].Name != draft.SpotName {
			day.Spots[0].Address = ""
			day.Spots[0].Latitude = nil
			day.Spots[0].Longitude = nil
			day.Spots[0].POIID = ""
			day.Spots[0].ImageURL = ""
		}
		day.Spots[0].Name = draft.SpotName
		if strings.TrimSpace(draft.SpotDescription) != "" {
			day.Spots[0].Description = draft.SpotDescription
		}
		day.Spots[0].EstimatedCost = estimateTicketCost(draft.SpotName, draft.SpotDescription)
	}
	if len(day.Meals) > 0 && strings.TrimSpace(draft.MealName) != "" {
		if day.Meals[0].Name != draft.MealName {
			day.Meals[0].Address = ""
			day.Meals[0].Latitude = nil
			day.Meals[0].Longitude = nil
			day.Meals[0].POIID = ""
		}
		day.Meals[0].Name = draft.MealName
		if strings.TrimSpace(draft.MealNotes) != "" {
			day.Meals[0].Notes = draft.MealNotes
		}
	}
	if strings.TrimSpace(draft.DailyNote) != "" && len(day.Notes) > 0 {
		day.Notes[len(day.Notes)-1] = draft.DailyNote
	} else if strings.TrimSpace(draft.DailyNote) != "" {
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
		day.Spots[0].Address = ""
		day.Spots[0].Latitude = nil
		day.Spots[0].Longitude = nil
		day.Spots[0].POIID = ""
		day.Spots[0].ImageURL = ""
	}
}

var (
	attractionNameRe = regexp.MustCompile(`[\p{Han}A-Za-z0-9·（）()]{1,24}(?:关帝庙|盐湖|鹳雀楼|普救寺|古城|古镇|博物馆|公园|寺|塔|山|湖|海|湾|街|广场|景区|楼|陵|院|宫|庙|泉|洞|峡|寨|园|湿地|遗址|老街|夜市)`)
	foodNameRe       = regexp.MustCompile(`[\p{Han}A-Za-z0-9·（）()]{1,24}(?:羊肉胡卜|煮饼|麻花|泡泡油糕|大盘鸡|餐厅|饭店|小吃|夜市|美食街|火锅|面馆|菜馆|烧烤|咖啡|食府|酒楼|茶餐厅)`)
)

func pickDemoSpots(destination string, contexts []string, dayCount int) []string {
	candidates := extractContextNames(destination, contexts, attractionNameRe, knownDestinationSpots(destination))
	for len(candidates) < dayCount {
		candidates = append(candidates, fallbackSpotName(destination, len(candidates)+1))
	}
	return candidates[:dayCount]
}

func pickDemoMeals(destination string, contexts []string, dayCount int) []string {
	candidates := extractContextNames(destination, contexts, foodNameRe, knownDestinationMeals(destination))
	for len(candidates) < dayCount {
		candidates = append(candidates, fallbackMealName(destination, len(candidates)+1))
	}
	return candidates[:dayCount]
}

func extractContextNames(destination string, contexts []string, pattern *regexp.Regexp, seeds []string) []string {
	joined := strings.Join(contexts, "\n")
	candidates := []string{}
	for _, match := range pattern.FindAllString(joined, -1) {
		addCandidateName(&candidates, destination, match)
	}
	for _, seed := range seeds {
		addCandidateName(&candidates, destination, seed)
	}
	return candidates
}

func addCandidateName(candidates *[]string, destination, name string) {
	name = cleanCandidateName(name)
	if name == "" || isPlaceholderName(name, destination) || isGenericCandidateName(name, destination) {
		return
	}
	for _, existing := range *candidates {
		if existing == name {
			return
		}
	}
	*candidates = append(*candidates, name)
}

func cleanCandidateName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " \t\r\n，。；、：:,.!?！？（）()[]【】<>《》\"'")
	if len([]rune(name)) > 24 {
		return ""
	}
	return name
}

func isGenericCandidateName(name, destination string) bool {
	if name == destination {
		return true
	}
	generic := []string{
		"旅游攻略", "推荐景点", "必去景点", "景点推荐", "本地攻略", "旅行攻略",
		"特色餐饮", "本地餐饮", "美食推荐", "在线资料", "候选事实", "结构化候选事实",
	}
	for _, keyword := range generic {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

func knownDestinationSpots(destination string) []string {
	switch strings.TrimSpace(destination) {
	case "运城", "运城市":
		return []string{"解州关帝庙", "运城盐湖", "鹳雀楼", "普救寺", "永乐宫", "李家大院", "舜帝陵"}
	case "大理", "大理市":
		return []string{"大理古城", "喜洲古镇", "崇圣寺三塔", "洱海生态廊道", "双廊古镇", "苍山"}
	case "大连", "大连市":
		return []string{"星海广场", "棒棰岛", "老虎滩海洋公园", "渔人码头", "俄罗斯风情街", "旅顺博物馆"}
	case "昆明", "昆明市":
		return []string{"滇池海埂公园", "云南民族村", "翠湖公园", "石林风景区", "金马碧鸡坊", "斗南花市"}
	default:
		return nil
	}
}

func knownDestinationMeals(destination string) []string {
	switch strings.TrimSpace(destination) {
	case "运城", "运城市":
		return []string{"北相羊肉胡卜", "闻喜煮饼", "稷山麻花", "运城泡泡油糕", "运城本地面馆"}
	case "大理", "大理市":
		return []string{"大理白族风味餐厅", "喜洲粑粑小吃店", "洱海砂锅鱼餐厅", "大理乳扇小吃"}
	case "大连", "大连市":
		return []string{"大连海鲜餐厅", "大连焖子小吃", "海胆水饺餐厅", "渔人码头海鲜餐厅"}
	case "昆明", "昆明市":
		return []string{"过桥米线餐厅", "昆明菌子火锅", "汽锅鸡餐厅", "篆新农贸市场小吃"}
	default:
		return nil
	}
}

func fallbackSpotName(destination string, index int) string {
	if strings.TrimSpace(destination) == "" {
		return "当地核心景点"
	}
	names := []string{
		destination + "核心景区",
		destination + "城市公园",
		destination + "历史文化街区",
		destination + "博物馆",
	}
	if index <= len(names) {
		return names[index-1]
	}
	return destination + "文化景区"
}

func fallbackMealName(destination string, index int) string {
	if strings.TrimSpace(destination) == "" {
		return "本地风味餐厅"
	}
	names := []string{
		destination + "本地风味餐厅",
		destination + "特色小吃店",
		destination + "老字号餐馆",
		destination + "地方菜馆",
	}
	if index <= len(names) {
		return names[index-1]
	}
	return destination + "本地餐馆"
}

func isPlaceholderName(name, destination string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	placeholders := []string{
		"推荐景点", "特色餐饮", "出发点", "待确认", "待定", "自由活动",
		"景点 1", "景点1", "餐饮 1", "餐饮1",
	}
	for _, placeholder := range placeholders {
		if strings.Contains(name, placeholder) {
			return true
		}
	}
	if destination != "" && (name == destination || name == destination+"市") {
		return true
	}
	return false
}

func cleanStringList(values []string) []string {
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func SanitizeItineraryContent(request domain.TripRequest, contexts []string, itinerary *domain.Itinerary) bool {
	if itinerary == nil {
		return false
	}
	dayCount := len(itinerary.Days)
	if dayCount == 0 {
		return false
	}
	spots := pickDemoSpots(request.Destination, contexts, dayCount)
	meals := pickDemoMeals(request.Destination, contexts, dayCount)
	changed := false
	for dayIndex := range itinerary.Days {
		if len(itinerary.Days[dayIndex].Spots) > 0 && isPlaceholderName(itinerary.Days[dayIndex].Spots[0].Name, request.Destination) {
			itinerary.Days[dayIndex].Spots[0].Name = spots[dayIndex]
			itinerary.Days[dayIndex].Spots[0].Description = defaultString(
				itinerary.Days[dayIndex].Spots[0].Description,
				"结合在线资料、本地攻略和旅行偏好安排，出发前建议再核对开放状态。",
			)
			itinerary.Days[dayIndex].Spots[0].Address = ""
			itinerary.Days[dayIndex].Spots[0].Latitude = nil
			itinerary.Days[dayIndex].Spots[0].Longitude = nil
			itinerary.Days[dayIndex].Spots[0].POIID = ""
			itinerary.Days[dayIndex].Spots[0].ImageURL = ""
			itinerary.Days[dayIndex].Spots[0].EstimatedCost = estimateTicketCost(spots[dayIndex], itinerary.Days[dayIndex].Spots[0].Description)
			changed = true
		}
		if len(itinerary.Days[dayIndex].Meals) > 0 && isPlaceholderName(itinerary.Days[dayIndex].Meals[0].Name, request.Destination) {
			itinerary.Days[dayIndex].Meals[0].Name = meals[dayIndex]
			itinerary.Days[dayIndex].Meals[0].Notes = defaultString(
				itinerary.Days[dayIndex].Meals[0].Notes,
				"可按当天路线和排队情况灵活选择同类本地餐饮。",
			)
			itinerary.Days[dayIndex].Meals[0].Address = ""
			itinerary.Days[dayIndex].Meals[0].Latitude = nil
			itinerary.Days[dayIndex].Meals[0].Longitude = nil
			itinerary.Days[dayIndex].Meals[0].POIID = ""
			changed = true
		}
		primarySpot := ""
		if len(itinerary.Days[dayIndex].Spots) > 0 {
			primarySpot = itinerary.Days[dayIndex].Spots[0].Name
		}
		for transportIndex := range itinerary.Days[dayIndex].Transport {
			transport := &itinerary.Days[dayIndex].Transport[transportIndex]
			if isPlaceholderName(transport.FromPlace, request.Destination) {
				transport.FromPlace = defaultString(request.Destination, "目的地") + "市区"
				changed = true
			}
			if isPlaceholderName(transport.ToPlace, request.Destination) {
				transport.ToPlace = defaultString(primarySpot, spots[dayIndex])
				changed = true
			}
		}
	}
	if changed {
		*itinerary = refreshBudget(*itinerary, request.Budget)
	}
	return changed
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

func evidenceSourceNotes(report domain.EvidenceReport) []string {
	notes := []string{}
	if len(report.Sources) > 0 {
		notes = append(notes, "在线资料来源："+itoa(len(report.Sources))+" 个；结构化候选事实："+itoa(len(report.Claims))+" 条。")
	}
	if len(report.Summary) > 0 {
		notes = append(notes, report.Summary...)
	}
	if len(report.VerificationSummary) > 0 {
		notes = append(notes, report.VerificationSummary...)
	}
	for _, warning := range report.Warnings {
		if !contains(notes, warning) {
			notes = append(notes, warning)
		}
	}
	return notes
}

func evidenceRiskTips(report domain.EvidenceReport) []string {
	tips := []string{}
	hasVolatile := false
	for _, claim := range report.Claims {
		if claim.RequiresReview || claim.ClaimType == "volatile" {
			hasVolatile = true
			break
		}
	}
	if hasVolatile {
		tips = append(tips, "开放时间、票价、营业状态和限流预约属于易变信息，出发前请以官网/官方渠道、地图实时信息或票务预约页交叉复核。")
	}
	if len(report.Sources) > 0 {
		hasOfficial := false
		hasOperationalChannel := false
		for _, source := range report.Sources {
			if source.SourceType == "official" {
				hasOfficial = true
			}
			if source.SourceType == "map_or_local_service" || source.SourceType == "ticketing" {
				hasOperationalChannel = true
			}
		}
		if !hasOfficial {
			tips = append(tips, "本次在线资料未发现明确官方来源，攻略类推荐可参考，但关键事实不要视为最终确认。")
		}
		if !hasOperationalChannel {
			tips = append(tips, "本次在线资料缺少地图或票务来源，点位状态、门票和预约规则建议临行前再查实时渠道。")
		}
	}
	return tips
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

func firstNEditMessages(values []domain.TripEditMessage, n int) []domain.TripEditMessage {
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
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
