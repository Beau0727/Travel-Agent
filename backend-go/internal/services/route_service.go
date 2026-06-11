package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	infraamap "travel-agent-go/internal/infrastructure/amap"
	"travel-agent-go/internal/logging"
)

type RoutePoint struct {
	Name      string
	Latitude  float64
	Longitude float64
	POIID     string
}

type RoutePlanRequest struct {
	Origin      RoutePoint
	Destination RoutePoint
	Mode        string
	HasRain     bool
}

type RoutePlanResult struct {
	Provider        string
	Mode            string
	DistanceMeters  int
	DurationSeconds int
	Polyline        string
	Summary         string
	Warning         string
	TaxiCost        float64
}

type RoutePlanningService struct {
	cfg  config.Config
	amap *infraamap.Client
}

func NewRoutePlanningService(cfg config.Config, amap *infraamap.Client) *RoutePlanningService {
	return &RoutePlanningService{cfg: cfg, amap: amap}
}

func (s *RoutePlanningService) Plan(ctx context.Context, request RoutePlanRequest) (RoutePlanResult, error) {
	if s == nil || !s.cfg.EnableAmapRouting || s.amap == nil || !s.amap.Enabled() {
		return RoutePlanResult{}, errors.New("amap routing is disabled")
	}
	if !validRoutePoint(request.Origin) || !validRoutePoint(request.Destination) {
		return RoutePlanResult{}, errors.New("route point has invalid coordinate")
	}

	mode := normalizeRouteMode(request.Mode)
	if mode == "" {
		mode = chooseRouteMode(request.Origin, request.Destination, request.HasRain)
	}

	values := url.Values{}
	values.Set("origin", formatLngLat(request.Origin.Latitude, request.Origin.Longitude))
	values.Set("destination", formatLngLat(request.Destination.Latitude, request.Destination.Longitude))
	values.Set("show_fields", "cost,polyline")
	values.Set("alternative_route", "1")
	if request.Origin.POIID != "" {
		values.Set("origin_id", request.Origin.POIID)
	}
	if request.Destination.POIID != "" {
		values.Set("destination_id", request.Destination.POIID)
	}
	if mode == "walking" {
		values.Set("isindoor", "0")
	}

	start := time.Now()
	var raw amapRouteResponse
	if err := s.amap.GetV5(ctx, "/direction/"+mode, values, &raw); err != nil {
		logging.Warn(ctx, "amap route request failed",
			"mode", mode,
			"origin", request.Origin.Name,
			"destination", request.Destination.Name,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return RoutePlanResult{}, err
	}

	result, err := normalizeAmapRoute(mode, raw)
	if err != nil {
		return RoutePlanResult{}, err
	}
	logging.Info(ctx, "amap route request completed",
		"mode", mode,
		"origin", request.Origin.Name,
		"destination", request.Destination.Name,
		"distance_meters", result.DistanceMeters,
		"duration_seconds", result.DurationSeconds,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return result, nil
}

func (s *RoutePlanningService) EnrichItineraryRoutes(ctx context.Context, itinerary *domain.Itinerary, weather *domain.WeatherForecastResponse) error {
	if itinerary == nil || len(itinerary.Days) == 0 {
		return nil
	}
	if s == nil || !s.cfg.EnableAmapRouting || s.amap == nil || !s.amap.Enabled() {
		logging.Info(ctx, "route enrichment disabled",
			"trip_id", itinerary.TripID,
			"destination", itinerary.Destination,
		)
		return nil
	}

	hasRain := weatherHasRain(weather)
	for dayIndex := range itinerary.Days {
		points := routePointsFromDay(itinerary.Days[dayIndex])
		if len(points) < 2 {
			continue
		}

		transports := make([]domain.TransportItem, 0, len(points)-1)
		for i := 0; i < len(points)-1; i++ {
			from := points[i]
			to := points[i+1]
			result, err := s.Plan(ctx, RoutePlanRequest{
				Origin:      from,
				Destination: to,
				HasRain:     hasRain,
			})
			if err != nil {
				logging.Warn(ctx, "route leg enrichment failed",
					"trip_id", itinerary.TripID,
					"day", itinerary.Days[dayIndex].DayIndex,
					"from", from.Name,
					"to", to.Name,
					"error", err,
				)
				transports = append(transports, domain.TransportItem{
					Mode:          "交通待确认",
					FromPlace:     from.Name,
					ToPlace:       to.Name,
					EstimatedCost: 0,
					RouteProvider: "amap",
					RouteStatus:   "failed",
					RouteWarning:  err.Error(),
				})
				continue
			}
			transports = append(transports, result.ToTransportItem(from.Name, to.Name))
		}

		if len(transports) > 0 {
			itinerary.Days[dayIndex].Transport = transports
		}
	}
	*itinerary = refreshBudget(*itinerary, itinerary.EstimatedBudget)
	return nil
}

func (r RoutePlanResult) ToTransportItem(from, to string) domain.TransportItem {
	distanceKMValue := math.Round(float64(r.DistanceMeters)/10) / 100
	minutes := int(math.Round(float64(r.DurationSeconds) / 60))
	modeLabel := "打车/网约车"
	cost := r.TaxiCost
	if r.Mode == "walking" {
		modeLabel = "步行"
		cost = 0
	} else if r.DistanceMeters > 0 && r.DistanceMeters <= 10000 {
		modeLabel = "公交/地铁优先，打车备选"
	}
	if r.Mode != "walking" && cost <= 0 {
		cost = estimateTaxiCost(r.DistanceMeters)
	}

	return domain.TransportItem{
		Mode:             modeLabel,
		FromPlace:        from,
		ToPlace:          to,
		EstimatedCost:    cost,
		Duration:         humanDuration(r.DurationSeconds),
		DistanceKM:       &distanceKMValue,
		EstimatedMinutes: &minutes,
		RouteProvider:    r.Provider,
		RouteMode:        r.Mode,
		RouteStatus:      "ok",
		DistanceMeters:   r.DistanceMeters,
		DurationSeconds:  r.DurationSeconds,
		Polyline:         r.Polyline,
		RouteSummary:     r.Summary,
		RouteWarning:     r.Warning,
		RouteTaxiCost:    r.TaxiCost,
	}
}

type amapRouteResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	Infocode string `json:"infocode"`
	Count    string `json:"count"`
	Route    struct {
		Origin      string          `json:"origin"`
		Destination string          `json:"destination"`
		Paths       []amapRoutePath `json:"paths"`
	} `json:"route"`
}

type amapRoutePath struct {
	Distance string          `json:"distance"`
	Duration string          `json:"duration"`
	Strategy string          `json:"strategy"`
	Cost     amapRouteCost   `json:"cost"`
	Steps    []amapRouteStep `json:"steps"`
}

type amapRouteCost struct {
	Duration string `json:"duration"`
	Taxi     string `json:"taxi"`
	Tolls    string `json:"tolls"`
}

type amapRouteStep struct {
	Instruction  string        `json:"instruction"`
	RoadName     string        `json:"road_name"`
	StepDistance string        `json:"step_distance"`
	Polyline     string        `json:"polyline"`
	Cost         amapRouteCost `json:"cost"`
}

func normalizeAmapRoute(mode string, raw amapRouteResponse) (RoutePlanResult, error) {
	if raw.Status != "1" || len(raw.Route.Paths) == 0 {
		if raw.Info == "" {
			raw.Info = "empty route"
		}
		return RoutePlanResult{}, errors.New(raw.Info)
	}

	path := raw.Route.Paths[0]
	distance := parseAmapInt(path.Distance)
	duration := parseAmapInt(path.Cost.Duration)
	if duration <= 0 {
		duration = parseAmapInt(path.Duration)
	}
	if duration <= 0 {
		for _, step := range path.Steps {
			duration += parseAmapInt(step.Cost.Duration)
		}
	}

	return RoutePlanResult{
		Provider:        "amap",
		Mode:            mode,
		DistanceMeters:  distance,
		DurationSeconds: duration,
		Polyline:        combineRoutePolyline(path),
		Summary:         buildRouteSummary(path),
		TaxiCost:        parseAmapFloat(path.Cost.Taxi),
	}, nil
}

func routePointsFromDay(day domain.DayPlan) []RoutePoint {
	lookup := map[string]RoutePoint{}
	addLookup := func(name string, lat, lng *float64, poiID string) {
		name = strings.TrimSpace(name)
		if name == "" || lat == nil || lng == nil {
			return
		}
		lookup[name] = RoutePoint{
			Name:      name,
			Latitude:  *lat,
			Longitude: *lng,
			POIID:     poiID,
		}
	}
	if day.Hotel != nil {
		addLookup(day.Hotel.Name, day.Hotel.Latitude, day.Hotel.Longitude, "")
	}
	for _, spot := range day.Spots {
		addLookup(spot.Name, spot.Latitude, spot.Longitude, spot.POIID)
	}
	for _, meal := range day.Meals {
		addLookup(meal.Name, meal.Latitude, meal.Longitude, meal.POIID)
	}

	points := []RoutePoint{}
	appendPoint := func(name string) {
		point, ok := lookup[strings.TrimSpace(name)]
		if !ok {
			return
		}
		if len(points) > 0 && points[len(points)-1].Name == point.Name {
			return
		}
		points = append(points, point)
	}

	if len(day.Transport) > 0 {
		appendPoint(day.Transport[0].FromPlace)
		for _, leg := range day.Transport {
			appendPoint(leg.ToPlace)
		}
		if len(points) >= 2 {
			return points
		}
	}

	if day.Hotel != nil {
		appendPoint(day.Hotel.Name)
	}
	for _, spot := range day.Spots {
		appendPoint(spot.Name)
	}
	for _, meal := range day.Meals {
		appendPoint(meal.Name)
	}
	if day.Hotel != nil {
		appendPoint(day.Hotel.Name)
	}
	return points
}

func normalizeRouteMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "walk", "walking", "步行":
		return "walking"
	case "drive", "driving", "taxi", "打车", "驾车":
		return "driving"
	default:
		return ""
	}
}

func chooseRouteMode(from, to RoutePoint, hasRain bool) string {
	if hasRain {
		return "driving"
	}
	if haversineKM(from.Latitude, from.Longitude, to.Latitude, to.Longitude) <= 1.5 {
		return "walking"
	}
	return "driving"
}

func formatLngLat(lat, lng float64) string {
	return fmt.Sprintf("%.6f,%.6f", lng, lat)
}

func validRoutePoint(point RoutePoint) bool {
	return point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180
}

func parseAmapInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func parseAmapFloat(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return math.Round(parsed*100) / 100
}

func combineRoutePolyline(path amapRoutePath) string {
	parts := []string{}
	for _, step := range path.Steps {
		if strings.TrimSpace(step.Polyline) != "" {
			parts = append(parts, strings.TrimSpace(step.Polyline))
		}
	}
	return strings.Join(parts, ";")
}

func buildRouteSummary(path amapRoutePath) string {
	parts := []string{}
	for _, step := range path.Steps {
		text := strings.TrimSpace(step.Instruction)
		if text != "" {
			parts = append(parts, text)
		}
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 && path.Strategy != "" {
		return path.Strategy
	}
	return strings.Join(parts, "；")
}

func estimateTaxiCost(distanceMeters int) float64 {
	km := float64(distanceMeters) / 1000
	if km <= 0 {
		return 0
	}
	cost := 10.0
	if km > 3 {
		cost += (km - 3) * 2.6
	}
	return math.Round(cost*100) / 100
}

func humanDuration(seconds int) string {
	if seconds <= 0 {
		return "时长待确认"
	}
	minutes := int(math.Round(float64(seconds) / 60))
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", minutes/60, minutes%60)
}

func weatherHasRain(weather *domain.WeatherForecastResponse) bool {
	if weather == nil {
		return false
	}
	for _, risk := range weather.Risks {
		if risk == "rain" {
			return true
		}
	}
	for _, day := range weather.Days {
		if containsAnyText(day.DayWeather+day.NightWeather, []string{"雨", "雪", "雷", "暴"}) {
			return true
		}
	}
	return false
}

func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	toRad := func(value float64) float64 {
		return value * math.Pi / 180
	}
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
