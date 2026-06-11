package agent

import "travel-agent-go/internal/logging"

func stepStateLogArgs(state *State) []any {
	if state == nil {
		return nil
	}

	evidenceSources := 0
	evidenceClaims := 0
	evidenceWarnings := 0
	if state.EvidenceReport != nil {
		evidenceSources = len(state.EvidenceReport.Sources)
		evidenceClaims = len(state.EvidenceReport.Claims)
		evidenceWarnings = len(state.EvidenceReport.Warnings)
	}

	weatherDays := 0
	weatherSource := ""
	if state.WeatherForecast != nil {
		weatherDays = len(state.WeatherForecast.Days)
		weatherSource = state.WeatherForecast.Source
	}

	return []any{
		"destination", state.Request.Destination,
		"day_count", state.DayCount,
		"contexts", len(state.RAGContexts),
		"context_chars", totalContextChars(state.RAGContexts),
		"evidence_sources", evidenceSources,
		"evidence_claims", evidenceClaims,
		"evidence_warnings", evidenceWarnings,
		"weather_days", weatherDays,
		"weather_source", weatherSource,
		"draft_days", len(state.PlannerDraft.Days),
		"final_trip_id", state.FinalItinerary.TripID,
		"final_days", len(state.FinalItinerary.Days),
		"transport_legs", countTransportLegs(state.FinalItinerary),
		"validation_issues", len(state.ValidationIssues),
		"tool_observations", len(state.ToolObservations),
		"trace_steps", len(state.Trace),
	}
}

func totalContextChars(contexts []string) int {
	total := 0
	for _, context := range contexts {
		total += len([]rune(context))
	}
	return total
}

func toolCallLogArgs(call toolCall) []any {
	args := call.Function.Arguments
	return []any{
		"tool", call.Function.Name,
		"tool_call_id", call.ID,
		"tool_call_type", call.Type,
		"args", logging.SafeText(args, 360),
		"args_chars", len([]rune(args)),
	}
}

func selectedToolNames(calls []toolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Function.Name)
	}
	return names
}

func evidenceSourceCount(state *State) int {
	if state == nil || state.EvidenceReport == nil {
		return 0
	}
	return len(state.EvidenceReport.Sources)
}

func evidenceClaimCount(state *State) int {
	if state == nil || state.EvidenceReport == nil {
		return 0
	}
	return len(state.EvidenceReport.Claims)
}
