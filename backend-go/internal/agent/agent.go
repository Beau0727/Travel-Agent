package agent

import (
	"context"
	"fmt"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// TravelPlanningAgent runs the default fixed-step planning workflow.
type TravelPlanningAgent struct {
	steps []Step
}

func NewTravelPlanningAgent(steps ...Step) *TravelPlanningAgent {
	return &TravelPlanningAgent{steps: steps}
}

func (a *TravelPlanningAgent) Run(ctx context.Context, state *State) error {
	for _, step := range a.steps {
		start := time.Now()
		logging.Info(ctx, "default agent step started", "step", step.Name())
		state.AddTrace(step.Name(), "started")

		if err := step.Run(ctx, state); err != nil {
			state.AddTrace(step.Name(), "failed: "+err.Error())
			logging.Error(ctx, "default agent step failed",
				"step", step.Name(),
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)
			return fmt.Errorf("%s: %w", step.Name(), err)
		}

		state.AddTrace(step.Name(), "completed")
		logging.Info(ctx, "default agent step completed",
			"step", step.Name(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
	return nil
}

// Generate lets the fixed-step agent satisfy the itinerary generator interface.
func (a *TravelPlanningAgent) Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error) {
	start := time.Now()
	logging.Info(ctx, "default agent generation started",
		"destination", request.Destination,
		"start_date", request.StartDate,
		"end_date", request.EndDate,
	)

	state := &State{Request: request}
	if err := a.Run(ctx, state); err != nil {
		logging.Error(ctx, "default agent generation failed",
			"destination", request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.Itinerary{}, err
	}

	logging.Info(ctx, "default agent generation completed",
		"trip_id", state.FinalItinerary.TripID,
		"days", len(state.FinalItinerary.Days),
		"trace_steps", len(state.Trace),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return state.FinalItinerary, nil
}
