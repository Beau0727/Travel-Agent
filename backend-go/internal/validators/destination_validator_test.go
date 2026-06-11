package validators

import (
	"testing"

	"travel-agent-go/internal/domain"
)

func TestDestinationConsistencyValidatorFlagsConflictingCityName(t *testing.T) {
	t.Parallel()

	issues := DestinationConsistencyValidator{}.Validate(domain.TripRequest{Destination: "北京"}, domain.Itinerary{
		Destination: "北京",
		Days: []domain.DayPlan{{
			DayIndex: 1,
			Spots: []domain.SpotItem{{
				Name:     "上海外滩",
				Location: "北京",
			}},
		}},
	})

	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %#v", issues)
	}
	if issues[0].Code != CodeDestinationMismatch {
		t.Fatalf("issue code = %s, want %s", issues[0].Code, CodeDestinationMismatch)
	}
}

func TestDestinationConsistencyValidatorAllowsDestinationAliases(t *testing.T) {
	t.Parallel()

	issues := DestinationConsistencyValidator{}.Validate(domain.TripRequest{Destination: "大理"}, domain.Itinerary{
		Destination: "大理",
		Days: []domain.DayPlan{{
			DayIndex: 1,
			Spots: []domain.SpotItem{{
				Name:    "大理古城",
				City:    "大理白族自治州",
				Address: "云南省大理白族自治州大理市",
			}},
		}},
	})

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
}
