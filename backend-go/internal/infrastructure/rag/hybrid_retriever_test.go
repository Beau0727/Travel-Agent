package rag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"travel-agent-go/internal/config"
	corerag "travel-agent-go/internal/rag"
)

func TestRRFFuseMergesDuplicateChunksAcrossRoutes(t *testing.T) {
	t.Parallel()

	routes := [][]corerag.RetrievedChunk{
		{
			{
				ID:       "xiamen-ring-road",
				Title:    "Ring Road",
				Text:     "Good for sunset rides.",
				Source:   "xiamen_guide.md",
				Scores:   map[string]float64{"dense": 0.9},
				Channels: []string{"dense"},
			},
		},
		{
			{
				ID:       "xiamen-ring-road",
				Title:    "Ring Road",
				Text:     "Good for sunset rides.",
				Source:   "xiamen_guide.md",
				Scores:   map[string]float64{"lexical": 12},
				Channels: []string{"lexical"},
			},
			{
				ID:       "xiamen-food",
				Title:    "Food",
				Text:     "Try local snacks.",
				Source:   "xiamen_guide.md",
				Scores:   map[string]float64{"lexical": 8},
				Channels: []string{"lexical"},
			},
		},
	}

	results := rrfFuse(routes, 60)
	if len(results) != 2 {
		t.Fatalf("expected two unique chunks, got %d", len(results))
	}
	if results[0].ID != "xiamen-ring-road" {
		t.Fatalf("expected duplicate chunk to win after RRF, got %s", results[0].ID)
	}
	if results[0].Scores["rrf"] <= results[1].Scores["rrf"] {
		t.Fatalf("expected duplicate route score to be higher: %#v", results)
	}
	if !contains(results[0].Channels, "dense") || !contains(results[0].Channels, "lexical") {
		t.Fatalf("expected merged channels, got %#v", results[0].Channels)
	}
}

func TestHTTPRerankerOrdersByReturnedScores(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Fatalf("unexpected rerank path: %s", r.URL.Path)
		}
		var request rerankRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Query == "" || len(request.Documents) != 2 {
			t.Fatalf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(rerankResponse{
			Results: []rerankResult{
				{Index: 1, Score: 0.99},
				{Index: 0, Score: 0.10},
			},
		})
	}))
	defer server.Close()

	reranker := NewHTTPReranker(config.Config{
		RAGRerankerURL:     server.URL + "/rerank",
		RAGRerankerTimeout: 5,
	})

	results, err := reranker.Rerank(
		corerag.NewQuery("xiamen", nil, "", "", 2),
		[]corerag.RetrievedChunk{
			{ID: "first", Title: "First", Text: "Less relevant.", Scores: map[string]float64{"rrf": 1}},
			{ID: "second", Title: "Second", Text: "More relevant.", Scores: map[string]float64{"rrf": 0.5}},
		},
		2,
	)
	if err != nil {
		t.Fatalf("Rerank returned error: %v", err)
	}
	if len(results) != 2 || results[0].ID != "second" {
		t.Fatalf("expected second chunk first, got %#v", results)
	}
	if results[0].Scores["rerank"] != 0.99 {
		t.Fatalf("expected rerank score to be attached, got %#v", results[0].Scores)
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
