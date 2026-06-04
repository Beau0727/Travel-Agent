package tools

import (
	"context"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
	"zhilv-yuntu-go/internal/services"
)

// PlannerTool 把 Planner 策略包装成 Agent 工具。
// 它内置 fallback：LLM 失败时使用 RulePlanner，保证 Agent 工作流能继续。
type PlannerTool struct {
	planner  services.Planner
	fallback services.Planner
}

type PlannerInput struct {
	Request  domain.TripRequest
	Contexts []string
	DayCount int
}

func NewPlannerTool(planner services.Planner) *PlannerTool {
	return &PlannerTool{
		planner:  planner,
		fallback: services.NewRulePlanner(),
	}
}

func (t *PlannerTool) Generate(ctx context.Context, input PlannerInput) (services.PlannerDraft, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return services.PlannerDraft{}, err
	}
	logging.Info(ctx, "planner tool generate started",
		"destination", input.Request.Destination,
		"day_count", input.DayCount,
		"contexts", len(input.Contexts),
	)
	draft, ok, err := t.planner.GenerateDraft(input.Request, input.Contexts, input.DayCount)
	if err == nil && ok {
		logging.Info(ctx, "planner tool generate completed",
			"destination", input.Request.Destination,
			"planner", "primary",
			"days", len(draft.Days),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return draft, nil
	}
	if err != nil {
		logging.Warn(ctx, "planner tool primary planner failed, using fallback",
			"destination", input.Request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
	} else {
		logging.Info(ctx, "planner tool primary planner unavailable, using fallback",
			"destination", input.Request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
	fallbackDraft, _, fallbackErr := t.fallback.GenerateDraft(input.Request, input.Contexts, input.DayCount)
	if fallbackErr != nil {
		logging.Warn(ctx, "planner tool fallback failed",
			"destination", input.Request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", fallbackErr,
		)
		return fallbackDraft, fallbackErr
	}
	logging.Info(ctx, "planner tool generate completed",
		"destination", input.Request.Destination,
		"planner", "fallback",
		"days", len(fallbackDraft.Days),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return fallbackDraft, fallbackErr
}
