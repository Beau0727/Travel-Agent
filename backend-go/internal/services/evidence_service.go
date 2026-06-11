package services

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"travel-agent-go/internal/domain"
)

const (
	claimStatusSupported       = "supported"
	claimStatusWeaklySupported = "weakly_supported"
	claimStatusNeedsReview     = "needs_review"

	sourceTypeOfficial       = "official"
	sourceTypeMapLocal       = "map_or_local_service"
	sourceTypeTicketing      = "ticketing"
	sourceTypeTravelPlatform = "travel_platform"
	sourceTypeSocial         = "social_content"
	sourceTypeGeneralWeb     = "general_web"

	verificationRoleOfficial       = "official"
	verificationRoleMap            = "map"
	verificationRoleTicketing      = "ticketing"
	verificationRoleTravelPlatform = "travel_platform"
	verificationRoleCommunity      = "community"
	verificationRoleWeb            = "web"

	verificationOfficialCross = "official_cross_verified"
	verificationOfficialOnly  = "official_supported"
	verificationMapTicketing  = "map_ticketing_cross_verified"
	verificationMultiSource   = "multi_source_supported"
	verificationSingleSource  = "single_source"
	verificationUnverified    = "unverified"
)

var sentenceSplitRe = regexp.MustCompile(`[。！？!?；;\n]+`)

func buildEvidenceReport(destination, query string, sources []WebResearchSource, generatedAt time.Time) domain.EvidenceReport {
	report := domain.EvidenceReport{
		Destination: strings.TrimSpace(destination),
		Query:       strings.TrimSpace(query),
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Sources:     make([]domain.EvidenceSource, 0, len(sources)),
	}
	if len(sources) == 0 {
		report.Warnings = append(report.Warnings, "未检索到可用的在线来源，行程将主要依赖本地资料和通用规划规则。")
		return report
	}

	claimsByKey := map[string]*domain.EvidenceClaim{}
	sourceTypes := map[string]int{}
	for index, source := range sources {
		evidenceSource := buildEvidenceSource(source, index, generatedAt)
		report.Sources = append(report.Sources, evidenceSource)
		sourceTypes[evidenceSource.SourceType]++

		for _, candidate := range extractClaimsFromSource(destination, source) {
			key := claimKey(candidate)
			existing, ok := claimsByKey[key]
			if !ok {
				candidate.ID = "claim_" + shortHash(key)
				candidate.SourceIDs = []string{evidenceSource.ID}
				candidate.SourceURLs = []string{evidenceSource.URL}
				candidate.SourceTypes = []string{evidenceSource.SourceType}
				claimsByKey[key] = &candidate
				continue
			}

			previousBestPriority := bestSourceTypePriority(existing.SourceTypes)
			existing.SourceIDs = appendUnique(existing.SourceIDs, evidenceSource.ID)
			existing.SourceURLs = appendUnique(existing.SourceURLs, evidenceSource.URL)
			existing.SourceTypes = appendUnique(existing.SourceTypes, evidenceSource.SourceType)
			existing.RequiresReview = existing.RequiresReview || candidate.RequiresReview
			existing.RiskLevel = higherRiskLevel(existing.RiskLevel, candidate.RiskLevel)
			if sourcePriority(evidenceSource.SourceType) < previousBestPriority {
				existing.Claim = candidate.Claim
				existing.Name = defaultString(candidate.Name, existing.Name)
			}
		}
	}

	sortEvidenceSources(report.Sources)

	claims := make([]domain.EvidenceClaim, 0, len(claimsByKey))
	for _, claim := range claimsByKey {
		enriched := *claim
		enrichClaimVerification(&enriched, report.Sources)
		claims = append(claims, enriched)
	}
	sort.SliceStable(claims, func(i, j int) bool {
		left := claimSortScore(claims[i])
		right := claimSortScore(claims[j])
		if left != right {
			return left > right
		}
		return claims[i].Confidence > claims[j].Confidence
	})
	if len(claims) > 18 {
		claims = claims[:18]
	}

	report.Claims = claims
	report.VerificationSummary = buildVerificationSummary(report, sourceTypes)
	report.Summary = buildEvidenceSummary(report)
	report.Warnings = append(report.Warnings, buildEvidenceWarnings(report, sourceTypes)...)
	return report
}

func buildEvidenceSource(source WebResearchSource, index int, generatedAt time.Time) domain.EvidenceSource {
	host := source.Host
	if host == "" {
		host = hostFromURL(source.URL)
	}
	sourceType, role, label, score := classifySource(source.Title, source.URL, host)
	retrievedAt := source.RetrievedAt
	if retrievedAt == "" {
		retrievedAt = generatedAt.Format(time.RFC3339)
	}
	return domain.EvidenceSource{
		ID:               fmt.Sprintf("src_%02d_%s", index+1, shortHash(source.URL)),
		Title:            strings.TrimSpace(source.Title),
		URL:              strings.TrimSpace(source.URL),
		Host:             host,
		SourceType:       sourceType,
		VerificationRole: role,
		SourcePriority:   sourcePriority(sourceType),
		ReliabilityLabel: label,
		ReliabilityScore: score,
		PublishedAt:      strings.TrimSpace(source.PublishedAt),
		RetrievedAt:      retrievedAt,
		Snippet:          trimRunes(source.Snippet, 260),
	}
}

func extractClaimsFromSource(destination string, source WebResearchSource) []domain.EvidenceClaim {
	text := strings.TrimSpace(source.Title + "。" + source.Snippet)
	if text == "" {
		return nil
	}
	sentences := sentenceSplitRe.Split(text, -1)
	claims := make([]domain.EvidenceClaim, 0, 4)
	for _, sentence := range sentences {
		sentence = normalizeSpace(sentence)
		if !isUsefulTravelSentence(destination, sentence) {
			continue
		}
		claimType := classifyClaim(sentence)
		claim := domain.EvidenceClaim{
			ClaimType:      claimType,
			Name:           extractClaimName(destination, sentence),
			Claim:          trimRunes(sentence, 180),
			RequiresReview: claimRequiresReview(claimType, sentence),
			RiskLevel:      claimRiskLevel(claimType, sentence),
		}
		claims = append(claims, claim)
		if len(claims) >= 4 {
			break
		}
	}
	if len(claims) == 0 && strings.TrimSpace(source.Snippet) != "" {
		snippet := trimRunes(normalizeSpace(source.Snippet), 180)
		claims = append(claims, domain.EvidenceClaim{
			ClaimType:      "general",
			Name:           extractClaimName(destination, snippet),
			Claim:          snippet,
			RequiresReview: claimRequiresReview("general", snippet),
			RiskLevel:      claimRiskLevel("general", snippet),
		})
	}
	return claims
}

func isUsefulTravelSentence(destination, sentence string) bool {
	if len([]rune(sentence)) < 12 {
		return false
	}
	if destination != "" && strings.Contains(strings.ToLower(sentence), strings.ToLower(destination)) {
		return true
	}
	keywords := []string{
		"景点", "古城", "古镇", "博物馆", "公园", "寺", "塔", "山", "湖", "海", "湾",
		"美食", "小吃", "餐厅", "夜市", "咖啡", "火锅", "米线",
		"路线", "交通", "地铁", "公交", "步行", "打车", "自驾",
		"预约", "门票", "开放时间", "营业", "闭馆", "限流", "避坑", "注意", "建议",
		"ticket", "opening", "official", "map", "route", "food", "restaurant",
	}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(sentence), strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func classifyClaim(sentence string) string {
	lower := strings.ToLower(sentence)
	switch {
	case containsAny(lower, []string{"门票", "票价", "价格", "开放时间", "闭馆", "闭园", "营业", "停运", "限流", "预约", "ticket", "opening hour", "closed", "reservation"}):
		return "volatile"
	case containsAny(lower, []string{"美食", "小吃", "餐厅", "夜市", "咖啡", "火锅", "米线", "烧烤", "food", "restaurant", "snack"}):
		return "food"
	case containsAny(lower, []string{"交通", "地铁", "公交", "打车", "自驾", "路线", "步行", "换乘", "车程", "route", "metro", "bus", "taxi", "walk"}):
		return "transport"
	case containsAny(lower, []string{"避坑", "注意", "建议", "防晒", "雨具", "排队", "节假日", "高峰", "warning", "avoid", "queue"}):
		return "caution"
	case containsAny(lower, []string{"景点", "古城", "古镇", "博物馆", "公园", "寺", "塔", "山", "湖", "海", "湾", "街", "old town", "lake", "museum", "park"}):
		return "attraction"
	default:
		return "general"
	}
}

func claimRequiresReview(claimType, sentence string) bool {
	if claimType == "volatile" {
		return true
	}
	return containsAny(strings.ToLower(sentence), []string{"门票", "开放时间", "营业时间", "价格", "闭馆", "限流", "停运", "预约", "以官方", "为准", "ticket", "opening hour", "reservation"})
}

func claimRiskLevel(claimType, sentence string) string {
	lower := strings.ToLower(sentence)
	if claimType == "volatile" {
		return "high"
	}
	if containsAny(lower, []string{"节假日", "高峰", "排队", "天气", "下雨", "防晒", "避坑", "queue", "rain", "holiday"}) {
		return "medium"
	}
	return "low"
}

func enrichClaimVerification(claim *domain.EvidenceClaim, reportSources []domain.EvidenceSource) {
	sources := sourcesForClaim(reportSources, claim.SourceIDs)
	sortEvidenceSources(sources)

	claim.SourceIDs = make([]string, 0, len(sources))
	claim.SourceURLs = make([]string, 0, len(sources))
	claim.SourceTypes = make([]string, 0, len(sources))
	claim.VerificationChannels = make([]string, 0, len(sources))
	claim.OfficialSourceURL = ""

	for _, source := range sources {
		claim.SourceIDs = appendUnique(claim.SourceIDs, source.ID)
		claim.SourceURLs = appendUnique(claim.SourceURLs, source.URL)
		claim.SourceTypes = appendUnique(claim.SourceTypes, source.SourceType)
		claim.VerificationChannels = appendUnique(claim.VerificationChannels, source.VerificationRole)
		if claim.OfficialSourceURL == "" && source.SourceType == sourceTypeOfficial {
			claim.OfficialSourceURL = source.URL
		}
	}

	claim.VerificationStatus, claim.VerificationSummary = claimVerificationStatus(*claim)
	claim.Confidence = claimConfidence(*claim, sources)
	claim.Status = claimStatus(*claim)
	claim.Reason = claimReason(*claim, sources)
}

func claimConfidence(claim domain.EvidenceClaim, sources []domain.EvidenceSource) float64 {
	reliability := maxSourceReliability(sources)
	supportCount := len(sources)
	score := reliability*0.62 + 0.16
	if supportCount > 1 {
		score += 0.06 * float64(minInt(supportCount-1, 3))
	}
	hasOfficial := hasVerificationChannel(claim, verificationRoleOfficial)
	hasMap := hasVerificationChannel(claim, verificationRoleMap)
	hasTicketing := hasVerificationChannel(claim, verificationRoleTicketing)
	switch {
	case hasOfficial && (hasMap || hasTicketing):
		score += 0.16
	case hasOfficial:
		score += 0.08
	case hasMap && hasTicketing:
		score += 0.08
	case hasMap || hasTicketing:
		score += 0.03
	}
	if claim.RequiresReview {
		if hasOfficial && (hasMap || hasTicketing) {
			score -= 0.08
		} else {
			score -= 0.18
		}
	}
	if claim.ClaimType == "general" {
		score -= 0.05
	}
	if claim.VerificationStatus == verificationSingleSource && !hasOfficial {
		score -= 0.04
	}
	return roundTo(score, 2, 0.15, 0.96)
}

func claimStatus(claim domain.EvidenceClaim) string {
	if claim.RequiresReview {
		return claimStatusNeedsReview
	}
	if claim.Confidence >= 0.72 {
		return claimStatusSupported
	}
	if claim.VerificationStatus == verificationOfficialCross || claim.VerificationStatus == verificationOfficialOnly || claim.VerificationStatus == verificationMapTicketing {
		return claimStatusSupported
	}
	return claimStatusWeaklySupported
}

func claimReason(claim domain.EvidenceClaim, sources []domain.EvidenceSource) string {
	reasonParts := []string{}
	if claim.VerificationSummary != "" {
		reasonParts = append(reasonParts, claim.VerificationSummary)
	}
	if len(sources) > 1 {
		reasonParts = append(reasonParts, fmt.Sprintf("%d 个来源支持", len(sources)))
	} else if len(sources) == 1 {
		reasonParts = append(reasonParts, "单一来源")
	}
	if len(claim.VerificationChannels) > 0 {
		reasonParts = append(reasonParts, "渠道："+strings.Join(channelLabels(claim.VerificationChannels), "、"))
	}
	if best := bestSourceLabel(sources); best != "" {
		reasonParts = append(reasonParts, "最高优先级来源："+best)
	}
	if claim.RequiresReview {
		reasonParts = append(reasonParts, "涉及开放时间、票价、营业或限流等易变信息，出发前仍需以官方或实时地图/票务页复核")
	}
	return strings.Join(reasonParts, "；")
}

func classifySource(title, rawURL, host string) (string, string, string, float64) {
	lowerTitle := strings.ToLower(title)
	text := strings.ToLower(title + " " + rawURL + " " + host)
	switch {
	case strings.Contains(host, ".gov.cn") || strings.HasSuffix(host, "gov.cn") || containsAny(title, []string{"文旅局", "旅游局", "文化和旅游", "官网", "官方"}) || containsAny(text, []string{"official site", "official website"}):
		return sourceTypeOfficial, verificationRoleOfficial, "官方/政务来源", 0.95
	case containsAny(text, []string{"tickets", "ticket", "门票", "票务", "预约购票"}) && containsAny(text, []string{"ctrip.com", "trip.com", "qunar.com", "fliggy.com", "meituan.com", "klook.com", "damai.cn"}):
		return sourceTypeTicketing, verificationRoleTicketing, "票务/预约来源", 0.86
	case containsAny(text, []string{"amap.com", "ditu.amap.com", "map.baidu.com", "map.qq.com", "dianping.com", "meituan.com"}):
		return sourceTypeMapLocal, verificationRoleMap, "地图/本地生活来源", 0.82
	case containsAny(text, []string{"ctrip.com", "trip.com", "qunar.com", "fliggy.com", "mafengwo.cn", "tripadvisor"}) || containsAny(lowerTitle, []string{"携程", "去哪儿", "飞猪", "马蜂窝"}):
		return sourceTypeTravelPlatform, verificationRoleTravelPlatform, "旅行平台来源", 0.74
	case containsAny(text, []string{"xiaohongshu.com", "douyin.com", "bilibili.com", "zhihu.com", "weibo.com"}):
		return sourceTypeSocial, verificationRoleCommunity, "社媒/社区来源", 0.52
	default:
		return sourceTypeGeneralWeb, verificationRoleWeb, "普通网页来源", 0.6
	}
}

func buildEvidenceSummary(report domain.EvidenceReport) []string {
	if len(report.Claims) == 0 {
		return []string{"未从在线来源中提取到足够明确的可用事实。"}
	}
	supported := 0
	needsReview := 0
	for _, claim := range report.Claims {
		if claim.Status == claimStatusSupported {
			supported++
		}
		if claim.RequiresReview {
			needsReview++
		}
	}
	summary := []string{
		fmt.Sprintf("在线资料共保留 %d 个来源、%d 条候选事实。", len(report.Sources), len(report.Claims)),
		fmt.Sprintf("%d 条可直接辅助行程生成，%d 条涉及易变信息并已标记待确认。", supported, needsReview),
	}
	return summary
}

func buildVerificationSummary(report domain.EvidenceReport, sourceTypes map[string]int) []string {
	summary := []string{}
	official := sourceTypes[sourceTypeOfficial]
	mapLocal := sourceTypes[sourceTypeMapLocal]
	ticketing := sourceTypes[sourceTypeTicketing]
	if official > 0 {
		summary = append(summary, fmt.Sprintf("已检索到 %d 个官方/官网来源，证据排序和生成上下文会优先使用。", official))
	} else {
		summary = append(summary, "未检索到明确官方/官网来源，关键事实不会被视为最终确认。")
	}
	if mapLocal > 0 || ticketing > 0 {
		summary = append(summary, fmt.Sprintf("地图/本地生活来源 %d 个，票务/预约来源 %d 个，用于核对位置、营业状态、门票和预约信息。", mapLocal, ticketing))
	}

	officialCross := 0
	mapTicketingCross := 0
	for _, claim := range report.Claims {
		switch claim.VerificationStatus {
		case verificationOfficialCross:
			officialCross++
		case verificationMapTicketing:
			mapTicketingCross++
		}
	}
	if officialCross > 0 {
		summary = append(summary, fmt.Sprintf("%d 条事实形成官网与地图/票务交叉验证。", officialCross))
	}
	if officialCross == 0 && mapTicketingCross > 0 {
		summary = append(summary, fmt.Sprintf("%d 条事实形成地图与票务渠道交叉验证，但仍缺少官网确认。", mapTicketingCross))
	}
	if officialCross == 0 && mapTicketingCross == 0 {
		summary = append(summary, "尚未形成官网、地图、票务之间的明确交叉验证。")
	}
	return summary
}

func buildEvidenceWarnings(report domain.EvidenceReport, sourceTypes map[string]int) []string {
	warnings := []string{}
	if len(report.Sources) > 0 && sourceTypes[sourceTypeOfficial] == 0 {
		warnings = append(warnings, "本轮在线资料未包含明确官方来源，开放时间、票价、限流和营业状态需以官方渠道为准。")
	}
	if len(report.Sources) > 0 && sourceTypes[sourceTypeMapLocal] == 0 && sourceTypes[sourceTypeTicketing] == 0 {
		warnings = append(warnings, "本轮在线资料未包含地图或票务来源，位置、营业状态、门票和预约信息缺少实时渠道核对。")
	}
	for _, claim := range report.Claims {
		if claim.RequiresReview {
			warnings = append(warnings, "部分在线信息涉及易变事实，已在证据中标记为待确认。")
			break
		}
	}
	for _, claim := range report.Claims {
		if claim.RequiresReview && claim.VerificationStatus != verificationOfficialCross {
			warnings = append(warnings, "存在尚未完成官网与地图/票务交叉验证的易变事实，不应在行程中表述为确定信息。")
			break
		}
	}
	return warnings
}

func FormatEvidenceContext(report domain.EvidenceReport) string {
	if len(report.Sources) == 0 && len(report.Claims) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Evidence-based online travel research for ")
	b.WriteString(defaultString(report.Destination, "destination"))
	if report.Query != "" {
		b.WriteString(" (query: ")
		b.WriteString(report.Query)
		b.WriteString(")")
	}
	b.WriteString("\nRules:\n")
	b.WriteString("- Prefer official/website sources over travel blogs or social content.\n")
	b.WriteString("- For opening hours, tickets, reservations, business status, and limits, require official + map/ticketing cross-check before using as a confident fact.\n")
	b.WriteString("- Do not present needs_review claims as certain facts; turn them into reminders or verification tasks.\n")
	if len(report.VerificationSummary) > 0 {
		b.WriteString("Verification summary:\n")
		for _, item := range report.VerificationSummary {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}
	if len(report.Sources) > 0 {
		b.WriteString("Prioritized sources:\n")
		for _, source := range firstNEvidenceSources(report.Sources, 6) {
			b.WriteString("- [")
			b.WriteString(source.VerificationRole)
			b.WriteString(" priority=")
			b.WriteString(itoa(source.SourcePriority))
			b.WriteString(" reliability=")
			b.WriteString(fmt.Sprintf("%.2f", source.ReliabilityScore))
			b.WriteString("] ")
			b.WriteString(defaultString(source.Title, source.Host))
			if source.URL != "" {
				b.WriteString(" <")
				b.WriteString(source.URL)
				b.WriteString(">")
			}
			b.WriteString("\n")
		}
	}
	for _, claim := range report.Claims {
		b.WriteString("- [")
		b.WriteString(claim.Status)
		b.WriteString(" verification=")
		b.WriteString(claim.VerificationStatus)
		b.WriteString(" confidence=")
		b.WriteString(fmt.Sprintf("%.2f", claim.Confidence))
		b.WriteString(" type=")
		b.WriteString(claim.ClaimType)
		if len(claim.VerificationChannels) > 0 {
			b.WriteString(" channels=")
			b.WriteString(strings.Join(claim.VerificationChannels, ","))
		}
		if claim.RequiresReview {
			b.WriteString(" requires_review")
		}
		b.WriteString("] ")
		b.WriteString(claim.Claim)
		if claim.OfficialSourceURL != "" {
			b.WriteString(" (official: ")
			b.WriteString(claim.OfficialSourceURL)
			b.WriteString(")")
		} else if len(claim.SourceURLs) > 0 {
			b.WriteString(" (source: ")
			b.WriteString(claim.SourceURLs[0])
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	if len(report.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range report.Warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func claimVerificationStatus(claim domain.EvidenceClaim) (string, string) {
	hasOfficial := hasVerificationChannel(claim, verificationRoleOfficial)
	hasMap := hasVerificationChannel(claim, verificationRoleMap)
	hasTicketing := hasVerificationChannel(claim, verificationRoleTicketing)
	supportCount := len(claim.SourceIDs)
	switch {
	case hasOfficial && (hasMap || hasTicketing):
		return verificationOfficialCross, "官网/官方来源已与地图或票务渠道交叉验证"
	case hasOfficial:
		return verificationOfficialOnly, "已获得官网/官方来源支持"
	case hasMap && hasTicketing:
		return verificationMapTicketing, "地图与票务渠道相互支持，缺少官网确认"
	case supportCount >= 2:
		return verificationMultiSource, "多个非官方来源支持"
	case supportCount == 1:
		return verificationSingleSource, "仅单一来源支持"
	default:
		return verificationUnverified, "缺少可用来源支持"
	}
}

func claimKey(claim domain.EvidenceClaim) string {
	name := strings.TrimSpace(claim.Name)
	if name == "" {
		name = extractKeyPhrase(claim.Claim)
	}
	if name == "" {
		name = trimRunes(claim.Claim, 36)
	}
	factKind := claimFactKind(claim)
	return strings.ToLower(claim.ClaimType + "|" + factKind + "|" + normalizeSpace(name))
}

func claimFactKind(claim domain.EvidenceClaim) string {
	lower := strings.ToLower(claim.Claim)
	switch {
	case containsAny(lower, []string{"开放时间", "营业时间", "闭馆", "闭园", "停运", "opening hour", "closed"}):
		return "opening_or_status"
	case containsAny(lower, []string{"门票", "票价", "价格", "ticket", "price"}):
		return "ticket_or_price"
	case containsAny(lower, []string{"预约", "限流", "reservation", "booking", "limit"}):
		return "reservation_or_limit"
	case containsAny(lower, []string{"交通", "地铁", "公交", "路线", "打车", "route", "metro", "bus", "taxi"}):
		return "transport"
	case containsAny(lower, []string{"餐厅", "美食", "小吃", "夜市", "restaurant", "food", "snack"}):
		return "food"
	default:
		return "general"
	}
}

func extractClaimName(destination, sentence string) string {
	keywords := []string{"古城", "古镇", "博物馆", "公园", "寺", "塔", "山", "湖", "海", "湾", "街", "夜市", "餐厅", "old town", "lake", "museum", "park"}
	runes := []rune(sentence)
	for _, keyword := range keywords {
		index := strings.Index(strings.ToLower(sentence), strings.ToLower(keyword))
		if index < 0 {
			continue
		}
		prefixRuneCount := len([]rune(sentence[:index]))
		start := maxInt(0, prefixRuneCount-6)
		end := minInt(len(runes), prefixRuneCount+len([]rune(keyword))+3)
		return strings.Trim(string(runes[start:end]), " ，、：:-“”\"'")
	}
	if destination != "" && strings.Contains(strings.ToLower(sentence), strings.ToLower(destination)) {
		return destination
	}
	return ""
}

func extractKeyPhrase(claim string) string {
	claim = normalizeSpace(claim)
	if claim == "" {
		return ""
	}
	runes := []rune(claim)
	limit := minInt(len(runes), 28)
	return strings.Trim(string(runes[:limit]), " ，、：:-“”\"'")
}

func sourcesForClaim(reportSources []domain.EvidenceSource, sourceIDs []string) []domain.EvidenceSource {
	ids := map[string]bool{}
	for _, id := range sourceIDs {
		ids[id] = true
	}
	sources := make([]domain.EvidenceSource, 0, len(sourceIDs))
	for _, source := range reportSources {
		if ids[source.ID] {
			sources = append(sources, source)
		}
	}
	return sources
}

func sortEvidenceSources(sources []domain.EvidenceSource) {
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].SourcePriority != sources[j].SourcePriority {
			return sources[i].SourcePriority < sources[j].SourcePriority
		}
		if sources[i].ReliabilityScore != sources[j].ReliabilityScore {
			return sources[i].ReliabilityScore > sources[j].ReliabilityScore
		}
		return sources[i].Host < sources[j].Host
	})
}

func firstNEvidenceSources(sources []domain.EvidenceSource, limit int) []domain.EvidenceSource {
	if limit <= 0 || len(sources) <= limit {
		return sources
	}
	return sources[:limit]
}

func sourcePriority(sourceType string) int {
	switch sourceType {
	case sourceTypeOfficial:
		return 1
	case sourceTypeMapLocal:
		return 2
	case sourceTypeTicketing:
		return 3
	case sourceTypeTravelPlatform:
		return 4
	case sourceTypeGeneralWeb:
		return 5
	case sourceTypeSocial:
		return 6
	default:
		return 9
	}
}

func bestSourceTypePriority(sourceTypes []string) int {
	best := 9
	for _, sourceType := range sourceTypes {
		best = minInt(best, sourcePriority(sourceType))
	}
	return best
}

func maxSourceReliability(sources []domain.EvidenceSource) float64 {
	maxScore := 0.5
	for _, source := range sources {
		if source.ReliabilityScore > maxScore {
			maxScore = source.ReliabilityScore
		}
	}
	return maxScore
}

func hasVerificationChannel(claim domain.EvidenceClaim, channel string) bool {
	for _, item := range claim.VerificationChannels {
		if item == channel {
			return true
		}
	}
	return false
}

func claimSortScore(claim domain.EvidenceClaim) float64 {
	score := claim.Confidence
	switch claim.VerificationStatus {
	case verificationOfficialCross:
		score += 0.35
	case verificationOfficialOnly:
		score += 0.24
	case verificationMapTicketing:
		score += 0.18
	case verificationMultiSource:
		score += 0.08
	}
	if claim.RequiresReview {
		score -= 0.12
	}
	return score
}

func bestSourceLabel(sources []domain.EvidenceSource) string {
	if len(sources) == 0 {
		return ""
	}
	sortEvidenceSources(sources)
	return sources[0].ReliabilityLabel
}

func channelLabels(channels []string) []string {
	labels := make([]string, 0, len(channels))
	for _, channel := range channels {
		switch channel {
		case verificationRoleOfficial:
			labels = append(labels, "官网/官方")
		case verificationRoleMap:
			labels = append(labels, "地图/本地生活")
		case verificationRoleTicketing:
			labels = append(labels, "票务/预约")
		case verificationRoleTravelPlatform:
			labels = append(labels, "旅行平台")
		case verificationRoleCommunity:
			labels = append(labels, "社区内容")
		case verificationRoleWeb:
			labels = append(labels, "普通网页")
		default:
			labels = append(labels, channel)
		}
	}
	return labels
}

func higherRiskLevel(left, right string) string {
	rank := map[string]int{"": 0, "low": 1, "medium": 2, "high": 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func hostFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func trimRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func roundTo(value float64, digits int, minValue float64, maxValue float64) float64 {
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	multiplier := 1.0
	for i := 0; i < digits; i++ {
		multiplier *= 10
	}
	if value >= 0 {
		return float64(int(value*multiplier+0.5)) / multiplier
	}
	return float64(int(value*multiplier-0.5)) / multiplier
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
