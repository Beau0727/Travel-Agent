package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/config"
	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// MapTool 是 Agent 的地图工具。
// 它负责把 itinerary 里的景点名称转换成高德 POI 信息，并补充地址、经纬度和图片。
type MapTool struct {
	enabled bool
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewMapTool(cfg config.Config) *MapTool {
	return &MapTool{
		enabled: cfg.EnableAmapEnrich && cfg.AmapAPIKey != "",
		apiKey:  cfg.AmapAPIKey,
		baseURL: strings.TrimRight(cfg.AmapBaseURL, "/"),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
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
			meal.POIID = firstNonEmpty(poi.ID, meal.POIID)
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
	Latitude  *float64
	Longitude *float64
}

func (t *MapTool) searchFirstPOI(ctx context.Context, keyword string, city string) (*amapPOI, error) {
	if keyword == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("key", t.apiKey)
	params.Set("keywords", keyword)
	params.Set("city", city)
	params.Set("offset", "1")
	params.Set("page", "1")
	params.Set("extensions", "all")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/place/text?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	logging.Info(ctx, "amap poi request started",
		"keyword", keyword,
		"city", city,
	)
	resp, err := t.client.Do(req)
	if err != nil {
		logging.Warn(ctx, "amap poi request failed",
			"keyword", keyword,
			"city", city,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Status string `json:"status"`
		Info   string `json:"info"`
		POIs   []struct {
			ID       string `json:"id"`
			Address  any    `json:"address"`
			Location string `json:"location"`
			Photos   []struct {
				URL string `json:"url"`
			} `json:"photos"`
		} `json:"pois"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		logging.Warn(ctx, "amap poi response parse failed",
			"keyword", keyword,
			"city", city,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	if payload.Status != "1" {
		logging.Warn(ctx, "amap poi response rejected",
			"keyword", keyword,
			"city", city,
			"status", resp.StatusCode,
			"info", payload.Info,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, errors.New(payload.Info)
	}
	if len(payload.POIs) == 0 {
		logging.Info(ctx, "amap poi response empty",
			"keyword", keyword,
			"city", city,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, nil
	}

	first := payload.POIs[0]
	lat, lng := splitAmapLocation(first.Location)
	imageURL := ""
	if len(first.Photos) > 0 {
		imageURL = first.Photos[0].URL
	}
	logging.Info(ctx, "amap poi request completed",
		"keyword", keyword,
		"city", city,
		"status", resp.StatusCode,
		"pois", len(payload.POIs),
		"poi_id", first.ID,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return &amapPOI{
		ID:        first.ID,
		Address:   stringifyAmapAddress(first.Address),
		ImageURL:  imageURL,
		Latitude:  lat,
		Longitude: lng,
	}, nil
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
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				parts = append(parts, text)
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
