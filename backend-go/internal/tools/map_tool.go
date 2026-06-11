package tools

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/geo"
	infraamap "travel-agent-go/internal/infrastructure/amap"
	"travel-agent-go/internal/logging"
)

// MapTool 是 Agent 的地图工具。
// 它负责把 itinerary 里的景点名称转换成高德 POI 信息，并补充地址、经纬度和图片。
type MapTool struct {
	enabled bool
	amap    *infraamap.Client
}

func NewMapTool(cfg config.Config, amap *infraamap.Client) *MapTool {
	return &MapTool{
		enabled: cfg.EnableAmapEnrich && cfg.AmapAPIKey != "",
		amap:    amap,
	}
}

func (t *MapTool) EnrichItinerary(ctx context.Context, itinerary *domain.Itinerary) error {
	if !t.enabled {
		logging.Info(ctx, "map tool enrichment disabled",
			"trip_id", itinerary.TripID,
			"destination", itinerary.Destination,
		)
		return nil
	}
	if t.amap == nil || !t.amap.Enabled() {
		return errors.New("amap client is disabled")
	}
	start := time.Now()
	lookups := 0
	enriched := 0
	skipped := 0
	logging.Info(ctx, "map tool enrichment started",
		"trip_id", itinerary.TripID,
		"destination", itinerary.Destination,
		"days", len(itinerary.Days),
	)
	for dayIndex := range itinerary.Days {
		if itinerary.Days[dayIndex].Hotel != nil {
			hotel := itinerary.Days[dayIndex].Hotel
			lookups++
			poi, err := t.searchFirstPOI(ctx, hotelSearchKeyword(*hotel, itinerary.Destination), itinerary.Destination)
			if err != nil || poi == nil {
				skipped++
				if err != nil {
					logging.Warn(ctx, "map tool hotel poi lookup failed",
						"trip_id", itinerary.TripID,
						"day", itinerary.Days[dayIndex].DayIndex,
						"keyword", hotel.Name,
						"error", err,
					)
				}
			} else {
				hotel.Address = firstNonEmpty(poi.Address, hotel.Address, hotel.Location)
				hotel.Adcode = firstNonEmpty(poi.Adcode, hotel.Adcode)
				hotel.City = firstNonEmpty(poi.City, hotel.City, itinerary.Destination)
				hotel.Latitude = poi.Latitude
				hotel.Longitude = poi.Longitude
				enriched++
			}
		}
		for spotIndex := range itinerary.Days[dayIndex].Spots {
			spot := &itinerary.Days[dayIndex].Spots[spotIndex]
			lookups++
			poi, err := t.searchFirstPOI(ctx, spot.Name, itinerary.Destination)
			if err != nil || poi == nil {
				skipped++
				if err != nil {
					logging.Warn(ctx, "map tool spot poi lookup failed",
						"trip_id", itinerary.TripID,
						"day", itinerary.Days[dayIndex].DayIndex,
						"keyword", spot.Name,
						"error", err,
					)
				} else {
					logging.Info(ctx, "map tool spot poi not found",
						"trip_id", itinerary.TripID,
						"day", itinerary.Days[dayIndex].DayIndex,
						"keyword", spot.Name,
					)
				}
				continue
			}
			spot.Address = firstNonEmpty(poi.Address, spot.Address, spot.Location)
			spot.ImageURL = firstNonEmpty(poi.ImageURL, spot.ImageURL)
			spot.POIID = firstNonEmpty(poi.ID, spot.POIID)
			spot.Adcode = firstNonEmpty(poi.Adcode, spot.Adcode)
			spot.City = firstNonEmpty(poi.City, spot.City, itinerary.Destination)
			spot.Latitude = poi.Latitude
			spot.Longitude = poi.Longitude
			enriched++
		}
		for mealIndex := range itinerary.Days[dayIndex].Meals {
			meal := &itinerary.Days[dayIndex].Meals[mealIndex]
			lookups++
			poi, err := t.searchFirstPOI(ctx, meal.Name, itinerary.Destination)
			if err != nil || poi == nil {
				skipped++
				if err != nil {
					logging.Warn(ctx, "map tool meal poi lookup failed",
						"trip_id", itinerary.TripID,
						"day", itinerary.Days[dayIndex].DayIndex,
						"keyword", meal.Name,
						"error", err,
					)
				} else {
					logging.Info(ctx, "map tool meal poi not found",
						"trip_id", itinerary.TripID,
						"day", itinerary.Days[dayIndex].DayIndex,
						"keyword", meal.Name,
					)
				}
				continue
			}
			meal.Location = firstNonEmpty(meal.Location, itinerary.Destination)
			meal.Address = firstNonEmpty(poi.Address, meal.Address, meal.Location)
			meal.ImageURL = firstNonEmpty(poi.ImageURL, meal.ImageURL)
			meal.POIID = firstNonEmpty(poi.ID, meal.POIID)
			meal.Adcode = firstNonEmpty(poi.Adcode, meal.Adcode)
			meal.City = firstNonEmpty(poi.City, meal.City, itinerary.Destination)
			meal.Latitude = poi.Latitude
			meal.Longitude = poi.Longitude
			enriched++
		}
	}
	logging.Info(ctx, "map tool enrichment completed",
		"trip_id", itinerary.TripID,
		"lookups", lookups,
		"enriched", enriched,
		"skipped", skipped,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

type amapPOI struct {
	ID        string
	Address   string
	ImageURL  string
	City      string
	Adcode    string
	Latitude  *float64
	Longitude *float64
}

func (t *MapTool) searchFirstPOI(ctx context.Context, keyword string, city string) (*amapPOI, error) {
	if keyword == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("keywords", keyword)
	params.Set("city", city)
	params.Set("citylimit", "true")
	params.Set("offset", "5")
	params.Set("page", "1")
	params.Set("extensions", "all")

	start := time.Now()
	logging.Info(ctx, "amap poi request started",
		"keyword", keyword,
		"city", city,
	)
	var payload struct {
		Status string           `json:"status"`
		Info   string           `json:"info"`
		POIs   []amapPOIPayload `json:"pois"`
	}
	if err := t.amap.GetV3(ctx, "/place/text", params, &payload); err != nil {
		logging.Warn(ctx, "amap poi response parse failed",
			"keyword", keyword,
			"city", city,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	if payload.Status != "1" {
		logging.Warn(ctx, "amap poi response rejected",
			"keyword", keyword,
			"city", city,
			"info", payload.Info,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, errors.New(payload.Info)
	}
	if len(payload.POIs) == 0 {
		logging.Info(ctx, "amap poi response empty",
			"keyword", keyword,
			"city", city,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, nil
	}

	filtered := filterAmapPOIs(payload.POIs, city)
	if len(filtered) == 0 {
		logging.Warn(ctx, "amap poi response rejected by city filter",
			"keyword", keyword,
			"city", city,
			"pois", len(payload.POIs),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, nil
	}

	first := bestAmapPOI(filtered)
	lat, lng := splitAmapLocation(first.Location)
	imageURL := firstAmapPhotoURL(first.Photos)
	poiCity := firstNonEmpty(stringifyAmapValue(first.CityName), stringifyAmapValue(first.ADName))
	logging.Info(ctx, "amap poi request completed",
		"keyword", keyword,
		"city", city,
		"pois", len(payload.POIs),
		"filtered_pois", len(filtered),
		"poi_id", first.ID,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return &amapPOI{
		ID:        first.ID,
		Address:   stringifyAmapAddress(first.Address),
		ImageURL:  imageURL,
		City:      poiCity,
		Adcode:    strings.TrimSpace(first.Adcode),
		Latitude:  lat,
		Longitude: lng,
	}, nil
}

type amapPOIPayload struct {
	ID       string `json:"id"`
	Address  any    `json:"address"`
	PName    any    `json:"pname"`
	CityName any    `json:"cityname"`
	ADName   any    `json:"adname"`
	Adcode   string `json:"adcode"`
	Location string `json:"location"`
	Photos   []struct {
		URL string `json:"url"`
	} `json:"photos"`
}

func filterAmapPOIs(pois []amapPOIPayload, destination string) []amapPOIPayload {
	if strings.TrimSpace(destination) == "" {
		return pois
	}
	filtered := make([]amapPOIPayload, 0, len(pois))
	for _, poi := range pois {
		cityText := strings.Join([]string{
			stringifyAmapValue(poi.CityName),
			stringifyAmapValue(poi.ADName),
			stringifyAmapValue(poi.PName),
			stringifyAmapAddress(poi.Address),
		}, " ")
		if geo.CityMatchesDestination(cityText, destination) || geo.TextMatchesDestination(destination, cityText) {
			filtered = append(filtered, poi)
		}
	}
	return filtered
}

func bestAmapPOI(pois []amapPOIPayload) amapPOIPayload {
	if len(pois) == 0 {
		return amapPOIPayload{}
	}
	for _, poi := range pois {
		if firstAmapPhotoURL(poi.Photos) != "" {
			return poi
		}
	}
	return pois[0]
}

func firstAmapPhotoURL(photos []struct {
	URL string `json:"url"`
}) string {
	for _, photo := range photos {
		url := strings.TrimSpace(photo.URL)
		if url == "" {
			continue
		}
		if strings.HasPrefix(url, "http://") {
			return "https://" + strings.TrimPrefix(url, "http://")
		}
		return url
	}
	return ""
}

func splitAmapLocation(location string) (*float64, *float64) {
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		return nil, nil
	}
	longitude, ok1 := parseFloatPointer(parts[0])
	latitude, ok2 := parseFloatPointer(parts[1])
	if !ok1 || !ok2 {
		return nil, nil
	}
	return latitude, longitude
}

func parseFloatPointer(value string) (*float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func stringifyAmapAddress(value any) string {
	return stringifyAmapValue(value)
}

func stringifyAmapValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hotelSearchKeyword(hotel domain.HotelItem, destination string) string {
	name := strings.TrimSpace(hotel.Name)
	if name != "" && !strings.Contains(name, "默认同住") && !strings.Contains(name, "住宿") {
		return name
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "酒店"
	}
	return destination + " 酒店"
}
