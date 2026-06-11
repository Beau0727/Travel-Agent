package agent

import (
	"context"
	"fmt"
	"time"

	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/logging"
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
		startArgs := append([]any{"step", step.Name()}, stepStateLogArgs(state)...)
		logging.Info(ctx, "default agent step started", startArgs...)
		state.AddTrace(step.Name(), "started")

		if err := step.Run(ctx, state); err != nil {
			state.AddTrace(step.Name(), "failed: "+err.Error())
			errorArgs := append([]any{
				"step", step.Name(),
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			}, stepStateLogArgs(state)...)
			logging.Error(ctx, "default agent step failed", errorArgs...)
			return fmt.Errorf("%s: %w", step.Name(), err)
		}

		state.AddTrace(step.Name(), "completed")
		completedArgs := append([]any{
			"step", step.Name(),
			"duration_ms", time.Since(start).Milliseconds(),
		}, stepStateLogArgs(state)...)
		logging.Info(ctx, "default agent step completed", completedArgs...)
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
		"travelers", request.Travelers,
		"budget", request.Budget,
		"preferences", len(request.Preferences),
		"dietary_preferences", len(request.DietaryPreferences),
		"pace", request.Pace,
		"hotel_level", request.HotelLevel,
		"special_notes_chars", len([]rune(request.SpecialNotes)),
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
