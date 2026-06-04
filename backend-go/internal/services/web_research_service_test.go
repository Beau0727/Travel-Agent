package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"zhilv-yuntu-go/internal/config"
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
