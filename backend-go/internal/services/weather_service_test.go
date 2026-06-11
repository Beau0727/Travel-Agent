package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"travel-agent-go/internal/config"
	infraamap "travel-agent-go/internal/infrastructure/amap"
)

func TestWeatherServiceForecastUsesAmap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/geocode/geo":
			if got := r.URL.Query().Get("address"); got != "大理" {
				t.Fatalf("geocode address = %s, want 大理", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "1",
				"info":   "OK",
				"count":  "1",
				"geocodes": []map[string]any{
					{
						"formatted_address": "云南省大理白族自治州大理市",
						"province":          "云南省",
						"city":              "大理白族自治州",
						"adcode":            "532901",
						"citycode":          "0872",
						"location":          "100.267638,25.606486",
					},
				},
			})
		case "/weather/weatherInfo":
			if got := r.URL.Query().Get("city"); got != "532901" {
				t.Fatalf("weather city = %s, want 532901", got)
			}
			if got := r.URL.Query().Get("extensions"); got != "all" {
				t.Fatalf("weather extensions = %s, want all", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "1",
				"info":   "OK",
				"count":  "1",
				"forecasts": []map[string]any{
					{
						"city":       "大理市",
						"province":   "云南",
						"adcode":     "532901",
						"reporttime": "2026-06-04 10:00:00",
						"casts": []map[string]any{
							{
								"date":         "2026-06-04",
								"week":         "4",
								"dayweather":   "小雨",
								"nightweather": "阴",
								"daytemp":      "25",
								"nighttemp":    "16",
								"daywind":      "东南",
								"nightwind":    "东南",
								"daypower":     "3",
								"nightpower":   "3",
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		EnableAmapWeather:  true,
		AmapAPIKey:         "test-key",
		AmapBaseV3URL:      server.URL,
		AmapTimeoutSeconds: 5,
	}
	service := NewWeatherService(cfg, infraamap.NewClient(cfg))

	got := service.Forecast(context.Background(), "大理")
	if got.Source != "amap" {
		t.Fatalf("Source = %s, want amap", got.Source)
	}
	if got.Adcode != "532901" {
		t.Fatalf("Adcode = %s, want 532901", got.Adcode)
	}
	if len(got.Days) != 1 {
		t.Fatalf("Days length = %d, want 1", len(got.Days))
	}
	if got.Days[0].DayWeather != "小雨" {
		t.Fatalf("DayWeather = %s, want 小雨", got.Days[0].DayWeather)
	}
	if !containsString(got.Risks, "rain") {
		t.Fatalf("Risks = %#v, want rain", got.Risks)
	}
	if len(got.Advice) == 0 {
		t.Fatalf("Advice is empty")
	}
}
