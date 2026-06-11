package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
)

func TestWebResearchServiceSearchesAndExtractsPageText(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{
					{"url": server.URL + "/guide"},
				},
			})
		case "/guide":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><title>Dali Guide</title><script>ignore()</script></head><body><h1>Dali old town</h1><p>Try local food and walk around Erhai lake at sunset.</p></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewWebResearchService(config.Config{
		EnableWebResearch:   true,
		WebSearchEndpoint:   server.URL + "/search",
		WebResearchTimeout:  5,
		WebResearchMaxPages: 2,
	})

	result, err := service.Research(context.Background(), WebResearchRequest{
		Destination: "Dali",
		TopK:        1,
	})
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(result.Sources))
	}
	if result.Sources[0].Title != "Dali Guide" {
		t.Fatalf("unexpected title: %q", result.Sources[0].Title)
	}
	if result.Sources[0].Snippet == "" || result.Sources[0].Snippet == "ignore()" {
		t.Fatalf("expected extracted body text, got %q", result.Sources[0].Snippet)
	}
}

func TestWebResearchServiceUsesTavilyProviderAndFallsBackToSearchContent(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			if r.Method != http.MethodPost {
				t.Fatalf("expected Tavily POST request, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Tavily request: %v", err)
			}
			if payload["query"] != "Dali official tickets opening hours" {
				t.Fatalf("unexpected Tavily query: %#v", payload["query"])
			}
			if payload["max_results"] != float64(3) {
				t.Fatalf("expected max_results=3, got %#v", payload["max_results"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"title":   "Dali Official Travel Notice",
						"url":     server.URL + "/blocked",
						"content": "Dali official notice says travelers should verify tickets, opening hours, and reservations before departure.",
						"score":   0.92,
					},
				},
			})
		case "/blocked":
			http.Error(w, "blocked", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewWebResearchService(config.Config{
		EnableWebResearch:   true,
		WebSearchProvider:   "tavily",
		WebSearchEndpoint:   server.URL + "/search",
		WebSearchAPIKey:     "test-key",
		WebResearchTimeout:  5,
		WebResearchMaxPages: 2,
	})

	result, err := service.Research(context.Background(), WebResearchRequest{
		Destination: "Dali",
		Query:       "Dali official tickets opening hours",
		TopK:        1,
	})
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(result.Sources))
	}
	if result.Sources[0].Provider != "tavily" {
		t.Fatalf("expected Tavily provider marker, got %#v", result.Sources[0])
	}
	if !strings.Contains(result.Sources[0].Snippet, "verify tickets") {
		t.Fatalf("expected fallback Tavily content, got %q", result.Sources[0].Snippet)
	}
}

func TestBuildEvidenceReportCrossVerifiesOfficialAndMapSources(t *testing.T) {
	t.Parallel()

	report := buildEvidenceReport("大理", "大理古城 官网 地图 开放时间", []WebResearchSource{
		{
			Title:   "大理古城官网",
			URL:     "https://www.daligucheng.gov.cn/notice",
			Host:    "www.daligucheng.gov.cn",
			Snippet: "大理古城开放时间为08:30-17:30，门票预约以官方公告为准。",
		},
		{
			Title:   "大理古城 - 高德地图",
			URL:     "https://amap.com/place/dali-old-town",
			Host:    "amap.com",
			Snippet: "大理古城开放时间为08:30-17:30，可查看实时营业状态。",
		},
	}, time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC))

	if len(report.Sources) != 2 {
		t.Fatalf("expected two sources, got %d", len(report.Sources))
	}
	if report.Sources[0].SourceType != sourceTypeOfficial {
		t.Fatalf("expected official source first, got %#v", report.Sources[0])
	}

	var verified *domain.EvidenceClaim
	for i := range report.Claims {
		claim := &report.Claims[i]
		if claim.VerificationStatus == verificationOfficialCross {
			verified = claim
			break
		}
	}
	if verified == nil {
		t.Fatalf("expected an official cross-verified claim, got %#v", report.Claims)
	}
	if verified.OfficialSourceURL == "" {
		t.Fatalf("expected official source URL on verified claim: %#v", verified)
	}
	if len(verified.VerificationChannels) < 2 {
		t.Fatalf("expected multiple verification channels, got %#v", verified.VerificationChannels)
	}
	if !verified.RequiresReview || verified.Status != claimStatusNeedsReview {
		t.Fatalf("volatile claim should still require review, got %#v", verified)
	}
	if len(report.VerificationSummary) == 0 {
		t.Fatalf("expected verification summary")
	}
}
