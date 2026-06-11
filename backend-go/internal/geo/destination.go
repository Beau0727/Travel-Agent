package geo

import (
	"math"
	"strings"
	"unicode"
)

var destinationAliasGroups = [][]string{
	{"beijing", "bj", "北京", "北京市"},
	{"shanghai", "sh", "上海", "上海市"},
	{"xian", "xi'an", "西安", "西安市"},
	{"xiamen", "厦门", "厦门市"},
	{"sanya", "三亚", "三亚市"},
	{"hangzhou", "杭州", "杭州市"},
	{"guilin", "桂林", "桂林市"},
	{"dali", "大理", "大理市", "大理白族自治州"},
	{"chongqing", "重庆", "重庆市"},
	{"chengdu", "成都", "成都市"},
	{"kunming", "昆明", "昆明市"},
	{"dalian", "大连", "大连市"},
	{"yuncheng", "运城", "运城市"},
}

func Aliases(destination string) []string {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return nil
	}
	normalized := Normalize(destination)
	aliases := []string{destination, strings.TrimSuffix(destination, "市"), normalized}
	for _, group := range destinationAliasGroups {
		for _, alias := range group {
			if Normalize(alias) == normalized {
				aliases = append(aliases, group...)
				return uniqueAliases(aliases)
			}
		}
	}
	return uniqueAliases(aliases)
}

func TextMatchesDestination(destination, text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if destination == "" || text == "" {
		return false
	}
	for _, alias := range Aliases(destination) {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias != "" && strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

func CityMatchesDestination(city, destination string) bool {
	city = strings.TrimSpace(city)
	destination = strings.TrimSpace(destination)
	if city == "" || destination == "" {
		return false
	}
	cityNorm := Normalize(city)
	for _, alias := range Aliases(destination) {
		if Normalize(alias) == cityNorm {
			return true
		}
	}
	return TextMatchesDestination(destination, city)
}

func HasConflictingCityMention(destination, text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if destination == "" || text == "" {
		return false
	}
	destinationAliases := map[string]bool{}
	for _, alias := range Aliases(destination) {
		destinationAliases[Normalize(alias)] = true
	}
	for _, group := range destinationAliasGroups {
		groupMatchesDestination := false
		for _, alias := range group {
			if destinationAliases[Normalize(alias)] {
				groupMatchesDestination = true
				break
			}
		}
		if groupMatchesDestination {
			continue
		}
		for _, alias := range group {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias != "" && len([]rune(alias)) >= 2 && strings.Contains(text, alias) {
				return true
			}
		}
	}
	return false
}

func Normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "市")
	value = strings.TrimSuffix(value, "city")
	var b strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || r == '_' || r == '-' || r == '\'' || r == '’' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func DistanceKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := degreesToRadians(lat2 - lat1)
	dLng := degreesToRadians(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degreesToRadians(lat1))*math.Cos(degreesToRadians(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func uniqueAliases(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := Normalize(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}
