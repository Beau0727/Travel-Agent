package services

import (
	"fmt"
	"strings"

	"travel-agent-go/internal/geo"
)

const (
	PlaceKindAttraction = "attraction"
	PlaceKindMeal       = "meal"
)

type PlaceCandidate struct {
	Name       string
	Kind       string
	City       string
	Adcode     string
	Address    string
	Latitude   *float64
	Longitude  *float64
	Rating     float64
	Tags       []string
	SourceIDs  []string
	Confidence float64
}

type PlaceCandidateBundle struct {
	Attractions []PlaceCandidate
	Meals       []PlaceCandidate
}

func BuildAttractionCandidates(destination string, contexts []string, limit int) []PlaceCandidate {
	names := LocalSpotCandidates(destination, contexts, limit)
	if len(names) < limit {
		names = appendUniqueStrings(names, pickDemoSpots(destination, contexts, limit)...)
	}
	return buildCandidates(destination, PlaceKindAttraction, firstNStrings(names, limit), nil)
}

func BuildMealCandidates(destination string, contexts []string, limit int) []PlaceCandidate {
	names := LocalMealCandidates(destination, contexts, limit)
	if len(names) < limit {
		names = appendUniqueStrings(names, pickDemoMeals(destination, contexts, limit)...)
	}
	return buildCandidates(destination, PlaceKindMeal, firstNStrings(names, limit), inferMealTags)
}

func CandidateNames(candidates []PlaceCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.Name)
		if name != "" && !contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func FormatPlaceCandidateContext(bundle PlaceCandidateBundle) string {
	parts := []string{}
	if len(bundle.Attractions) > 0 {
		parts = append(parts, "结构化候选景点（已按目的地过滤）："+formatCandidateList(bundle.Attractions, 16))
	}
	if len(bundle.Meals) > 0 {
		parts = append(parts, "结构化候选餐厅（已按目的地过滤，优先本地特色/榜单来源）："+formatCandidateList(bundle.Meals, 16))
	}
	return strings.Join(parts, "\n")
}

func MergeCandidateBundles(base PlaceCandidateBundle, additions ...PlaceCandidateBundle) PlaceCandidateBundle {
	out := base
	for _, addition := range additions {
		out.Attractions = mergeCandidates(out.Attractions, addition.Attractions)
		out.Meals = mergeCandidates(out.Meals, addition.Meals)
	}
	return out
}

func buildCandidates(destination, kind string, names []string, tagger func(string) []string) []PlaceCandidate {
	candidates := make([]PlaceCandidate, 0, len(names))
	for index, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || geo.HasConflictingCityMention(destination, name) {
			continue
		}
		tags := []string{}
		if tagger != nil {
			tags = tagger(name)
		}
		candidates = append(candidates, PlaceCandidate{
			Name:       name,
			Kind:       kind,
			City:       destination,
			Rating:     syntheticCandidateRating(index),
			Tags:       tags,
			Confidence: syntheticCandidateConfidence(kind, index, tags),
		})
	}
	return candidates
}

func formatCandidateList(candidates []PlaceCandidate, limit int) string {
	if limit <= 0 || len(candidates) < limit {
		limit = len(candidates)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		candidate := candidates[i]
		detail := candidate.Name
		if candidate.Rating > 0 {
			detail += fmt.Sprintf("(评分参考 %.1f)", candidate.Rating)
		}
		if len(candidate.Tags) > 0 {
			detail += "[" + strings.Join(candidate.Tags, "、") + "]"
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, "、")
}

func mergeCandidates(base []PlaceCandidate, additions []PlaceCandidate) []PlaceCandidate {
	out := append([]PlaceCandidate(nil), base...)
	for _, candidate := range additions {
		name := strings.TrimSpace(candidate.Name)
		if name == "" {
			continue
		}
		exists := false
		for _, current := range out {
			if current.Name == name && current.Kind == candidate.Kind {
				exists = true
				break
			}
		}
		if !exists {
			out = append(out, candidate)
		}
	}
	return out
}

func inferMealTags(name string) []string {
	tags := []string{}
	patterns := map[string]string{
		"海鲜": "本地海鲜",
		"米线": "地方小吃",
		"火锅": "地方风味",
		"菌":  "地方风味",
		"羊肉": "地方小吃",
		"煮饼": "地方小吃",
		"麻花": "地方小吃",
		"白族": "本地菜",
		"粑粑": "地方小吃",
		"砂锅鱼": "本地菜",
		"乳扇": "地方小吃",
		"小吃": "地方小吃",
		"老字号": "榜单优先",
	}
	for keyword, tag := range patterns {
		if strings.Contains(name, keyword) && !contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		tags = append(tags, "本地特色")
	}
	return tags
}

func syntheticCandidateRating(index int) float64 {
	value := 4.8 - float64(index%8)*0.06
	if value < 4.2 {
		value = 4.2
	}
	return round2(value)
}

func syntheticCandidateConfidence(kind string, index int, tags []string) float64 {
	confidence := 0.72 - float64(index%6)*0.03
	if kind == PlaceKindMeal && len(tags) > 0 {
		confidence += 0.08
	}
	if confidence < 0.45 {
		confidence = 0.45
	}
	if confidence > 0.92 {
		confidence = 0.92
	}
	return round2(confidence)
}

func appendUniqueStrings(values []string, additions ...string) []string {
	out := append([]string(nil), values...)
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition != "" && !contains(out, addition) {
			out = append(out, addition)
		}
	}
	return out
}
