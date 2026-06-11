package services

import (
	"testing"

	"travel-agent-go/internal/domain"
)

func TestFormatLngLatUsesAmapOrder(t *testing.T) {
	got := formatLngLat(25.6065, 100.2676)
	want := "100.267600,25.606500"
	if got != want {
		t.Fatalf("formatLngLat() = %s, want %s", got, want)
	}
}

func TestChooseRouteMode(t *testing.T) {
	from := RoutePoint{Name: "A", Latitude: 25.6065, Longitude: 100.2676}
	nearby := RoutePoint{Name: "B", Latitude: 25.6070, Longitude: 100.2680}
	far := RoutePoint{Name: "C", Latitude: 25.7000, Longitude: 100.3500}

	if got := chooseRouteMode(from, nearby, false); got != "walking" {
		t.Fatalf("chooseRouteMode nearby dry = %s, want walking", got)
	}
	if got := chooseRouteMode(from, nearby, true); got != "driving" {
		t.Fatalf("chooseRouteMode nearby rainy = %s, want driving", got)
	}
	if got := chooseRouteMode(from, far, false); got != "driving" {
		t.Fatalf("chooseRouteMode far dry = %s, want driving", got)
	}
}

func TestNormalizeAmapRoute(t *testing.T) {
	raw := amapRouteResponse{
		Status: "1",
		Info:   "OK",
	}
	raw.Route.Paths = []amapRoutePath{
		{
			Distance: "1500",
			Cost: amapRouteCost{
				Duration: "600",
				Taxi:     "18.5",
			},
			Steps: []amapRouteStep{
				{
					Instruction: "沿人民路向东行驶",
					Polyline:    "100.100000,25.100000;100.200000,25.200000",
				},
				{
					Instruction: "右转进入古城路",
					Polyline:    "100.200000,25.200000;100.300000,25.300000",
				},
			},
		},
	}

	got, err := normalizeAmapRoute("driving", raw)
	if err != nil {
		t.Fatalf("normalizeAmapRoute returned error: %v", err)
	}
	if got.DistanceMeters != 1500 {
		t.Fatalf("DistanceMeters = %d, want 1500", got.DistanceMeters)
	}
	if got.DurationSeconds != 600 {
		t.Fatalf("DurationSeconds = %d, want 600", got.DurationSeconds)
	}
	if got.TaxiCost != 18.5 {
		t.Fatalf("TaxiCost = %.2f, want 18.50", got.TaxiCost)
	}
	if got.Polyline == "" {
		t.Fatalf("Polyline is empty")
	}
	if got.Summary == "" {
		t.Fatalf("Summary is empty")
	}
}

func TestRoutePointsFromDayFollowsHotelTimeline(t *testing.T) {
	t.Parallel()

	hotelLat, hotelLng := 25.6000, 100.2000
	spotLat, spotLng := 25.6100, 100.2100
	mealLat, mealLng := 25.6200, 100.2200
	day := domain.DayPlan{
		Hotel: &domain.HotelItem{
			Name:      "大理舒适型酒店（全程默认同住）",
			Latitude:  &hotelLat,
			Longitude: &hotelLng,
		},
		Spots: []domain.SpotItem{{
			Name:      "大理古城",
			Latitude:  &spotLat,
			Longitude: &spotLng,
		}},
		Meals: []domain.MealItem{{
			Name:      "大理白族风味餐厅",
			Latitude:  &mealLat,
			Longitude: &mealLng,
		}},
		Transport: []domain.TransportItem{
			{FromPlace: "大理舒适型酒店（全程默认同住）", ToPlace: "大理古城"},
			{FromPlace: "大理古城", ToPlace: "大理白族风味餐厅"},
			{FromPlace: "大理白族风味餐厅", ToPlace: "大理舒适型酒店（全程默认同住）"},
		},
	}

	points := routePointsFromDay(day)
	if len(points) != 4 {
		t.Fatalf("expected 4 route points, got %#v", points)
	}
	want := []string{"大理舒适型酒店（全程默认同住）", "大理古城", "大理白族风味餐厅", "大理舒适型酒店（全程默认同住）"}
	for i, name := range want {
		if points[i].Name != name {
			t.Fatalf("point %d = %q, want %q; all=%#v", i, points[i].Name, name, points)
		}
	}
}
