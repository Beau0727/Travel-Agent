package rag

import (
	"testing"

	corerag "travel-agent-go/internal/rag"
)

func TestLexicalSearchFiltersChunksByDestinationCity(t *testing.T) {
	t.Parallel()

	chunks := []corerag.Chunk{
		{
			ID:     "dali",
			Title:  "大理核心景点",
			Text:   "大理古城和洱海生态廊道适合慢游。",
			Source: "dali_guide.md",
			Metadata: map[string]any{
				"city": "dali",
			},
		},
		{
			ID:     "xian",
			Title:  "西安核心景点",
			Text:   "西安城墙和大雁塔适合历史文化游。",
			Source: "xian_guide.md",
			Metadata: map[string]any{
				"city": "xian",
			},
		},
	}

	results := lexicalSearch(chunks, corerag.NewQuery("大理", nil, "", "", 5), 5)
	if len(results) != 1 {
		t.Fatalf("expected one destination-matched result, got %#v", results)
	}
	if results[0].ID != "dali" {
		t.Fatalf("expected dali chunk, got %#v", results[0])
	}
}
