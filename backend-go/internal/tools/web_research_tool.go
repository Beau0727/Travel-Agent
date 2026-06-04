package tools

import (
	"context"

	"zhilv-yuntu-go/internal/services"
)

type WebResearchTool struct {
	service *services.WebResearchService
}

type WebResearchInput struct {
	Destination string
	Query       string
	TopK        int
}

func NewWebResearchTool(service *services.WebResearchService) *WebResearchTool {
	return &WebResearchTool{service: service}
}

func (t *WebResearchTool) Research(ctx context.Context, input WebResearchInput) (services.WebResearchResult, error) {
	if err := ctx.Err(); err != nil {
		return services.WebResearchResult{}, err
	}
	if t == nil || t.service == nil {
		return services.WebResearchResult{}, nil
	}
	return t.service.Research(ctx, services.WebResearchRequest{
		Destination: input.Destination,
		Query:       input.Query,
		TopK:        input.TopK,
	})
}
