package services

import (
	"context"
	"strings"
	"testing"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/llm"
)

func TestLLMPlannerAcceptsStringTips(t *testing.T) {
	t.Parallel()

	planner := NewLLMPlanner(config.Config{
		LLMAPIKey: "test-key",
		LLMModel:  "test-model",
	}, fakeChatClient{content: `{
		"summary": "运城三日游",
		"tips": "出发前复核开放时间",
		"days": [
			{
				"day_index": "1",
				"theme": "盐湖与古建",
				"spot_name": "运城盐湖",
				"spot_description": "适合拍照和轻松游览。",
				"meal_name": "北相羊肉胡卜",
				"meal_notes": "本地风味。",
				"daily_note": "错峰出行。"
			}
		]
	}`})

	draft, ok, err := planner.GenerateDraft(domain.TripRequest{
		Destination: "运城",
		StartDate:   "2026-06-10",
		EndDate:     "2026-06-10",
		Travelers:   2,
	}, nil, 1)
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected primary planner to be usable")
	}
	if len(draft.Tips) != 1 || draft.Tips[0] != "出发前复核开放时间" {
		t.Fatalf("expected string tips to be normalized, got %#v", draft.Tips)
	}
	if draft.Days[0].DayIndex != 1 || draft.Days[0].SpotName != "运城盐湖" {
		t.Fatalf("unexpected day draft: %#v", draft.Days[0])
	}
}

func TestRulePlannerAvoidsPlaceholderForKnownDestination(t *testing.T) {
	t.Parallel()

	draft, ok, err := NewRulePlanner().GenerateDraft(domain.TripRequest{
		Destination: "运城",
		StartDate:   "2026-06-10",
		EndDate:     "2026-06-12",
	}, nil, 3)
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected rule planner to be usable")
	}
	for _, day := range draft.Days {
		if strings.Contains(day.SpotName, "推荐景点") || strings.Contains(day.MealName, "特色餐饮") {
			t.Fatalf("fallback produced placeholder day: %#v", day)
		}
	}
	if draft.Days[0].SpotName != "解州关帝庙" {
		t.Fatalf("expected known 运城 spot first, got %q", draft.Days[0].SpotName)
	}
}

func TestSanitizeItineraryContentRepairsPlaceholderNames(t *testing.T) {
	t.Parallel()

	itinerary := domain.Itinerary{
		Destination: "运城",
		Days: []domain.DayPlan{{
			DayIndex: 1,
			Spots: []domain.SpotItem{{
				Name: "运城 推荐景点 1",
			}},
			Meals: []domain.MealItem{{
				Name: "运城 特色餐饮 1",
			}},
			Transport: []domain.TransportItem{{
				FromPlace: "运城 出发点",
				ToPlace:   "运城 推荐景点 1",
			}},
		}},
	}

	changed := SanitizeItineraryContent(domain.TripRequest{Destination: "运城"}, nil, &itinerary)
	if !changed {
		t.Fatalf("expected itinerary to be repaired")
	}
	day := itinerary.Days[0]
	if day.Spots[0].Name != "解州关帝庙" {
		t.Fatalf("expected spot repair, got %q", day.Spots[0].Name)
	}
	if day.Meals[0].Name != "北相羊肉胡卜" {
		t.Fatalf("expected meal repair, got %q", day.Meals[0].Name)
	}
	if strings.Contains(day.Transport[0].FromPlace, "出发点") || strings.Contains(day.Transport[0].ToPlace, "推荐景点") {
		t.Fatalf("expected transport repair, got %#v", day.Transport[0])
	}
}

type fakeChatClient struct {
	content string
}

func (c fakeChatClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatMessage, error) {
	return llm.ChatMessage{Role: "assistant", Content: c.content}, nil
}
