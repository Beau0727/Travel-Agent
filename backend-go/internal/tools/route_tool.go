package tools

import (
	"context"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/logging"
	"travel-agent-go/internal/services"
)

type RouteTool struct {
	service *services.RoutePlanningService
}

func NewRouteTool(service *services.RoutePlanningService) *RouteTool {
	return &RouteTool{service: service}
}

func (t *RouteTool) Enrich(ctx context.Context, itinerary *domain.Itinerary, weather *domain.WeatherForecastResponse) error {
	if t == nil || t.service == nil || itinerary == nil {
		return nil
	}

	start := time.Now()
	logging.Info(ctx, "route tool enrich started",
		"trip_id", itinerary.TripID,
		"destination", itinerary.Destination,
		"days", len(itinerary.Days),
	)

	err := t.service.EnrichItineraryRoutes(ctx, itinerary, weather)
	if err != nil {
		logging.Warn(ctx, "route tool enrich failed",
			"trip_id", itinerary.TripID,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}

	logging.Info(ctx, "route tool enrich completed",
		"trip_id", itinerary.TripID,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}
