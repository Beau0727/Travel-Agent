package rag

import (
	"fmt"
	"sort"
	"strings"
)

// Retriever is the legacy RAG retrieval port used by the agent layer.
type Retriever interface {
	Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error)
}

// DetailedRetriever is the richer retrieval port used by hybrid search,
// reranking, and automated evaluation.
type DetailedRetriever interface {
	RetrieveDetailed(query Query) ([]RetrievedChunk, error)
}

// Reranker reorders a retrieved candidate pool for a single RAG query.
type Reranker interface {
	Rerank(query Query, chunks []RetrievedChunk, topK int) ([]RetrievedChunk, error)
}

// Query is the domain-neutral query shape shared by RAG adapters.
type Query struct {
	Destination  string
	Preferences  []string
	Pace         string
	SpecialNotes string
	Text         string
	TopK         int
}

func NewQuery(destination string, preferences []string, pace string, specialNotes string, topK int) Query {
	return Query{
		Destination:  strings.TrimSpace(destination),
		Preferences:  append([]string(nil), preferences...),
		Pace:         strings.TrimSpace(pace),
		SpecialNotes: strings.TrimSpace(specialNotes),
		TopK:         topK,
	}
}

func (q Query) WithText(text string) Query {
	q.Text = strings.TrimSpace(text)
	return q
}

func (q Query) Limit(fallback int) int {
	if q.TopK > 0 {
		return q.TopK
	}
	if fallback > 0 {
		return fallback
	}
	return 5
}

func (q Query) SearchText() string {
	if strings.TrimSpace(q.Text) != "" {
		return strings.TrimSpace(q.Text)
	}

	parts := []string{
		q.Destination,
		"\u666f\u70b9", // attractions
		"\u884c\u7a0b", // itinerary
		"\u653b\u7565", // guide
		"\u63a8\u8350", // recommendation
		"\u7f8e\u98df", // food
		"\u4ea4\u901a", // transport
		"travel",
		"guide",
		"itinerary",
		"food",
		"transport",
	}
	parts = append(parts, q.Preferences...)
	if q.Pace != "" {
		parts = append(parts, q.Pace)
	}
	if q.SpecialNotes != "" {
		parts = append(parts, q.SpecialNotes)
	}
	return strings.Join(uniqueNonBlank(parts), " ")
}

func (q Query) ExpandedTexts() []string {
	base := q.SearchText()
	parts := []string{base}
	for _, preference := range q.Preferences {
		preference = strings.TrimSpace(preference)
		if preference != "" {
			parts = append(parts, strings.TrimSpace(q.Destination+" "+preference))
		}
	}
	if q.Pace != "" {
		parts = append(parts, strings.TrimSpace(q.Destination+" "+q.Pace))
	}
	if q.SpecialNotes != "" {
		parts = append(parts, strings.TrimSpace(q.Destination+" "+q.SpecialNotes))
	}
	return uniqueNonBlank(parts)
}

// Chunk is the domain-neutral unit indexed or retrieved by RAG adapters.
type Chunk struct {
	ID       string
	Title    string
	Text     string
	Source   string
	Metadata map[string]any
}

// RetrievedChunk carries retrieval metadata across multi-route recall,
// fusion, reranking, and evaluation.
type RetrievedChunk struct {
	ID       string
	Title    string
	Text     string
	Source   string
	Metadata map[string]any
	Scores   map[string]float64
	Channels []string
	Rank     int
}

func (c RetrievedChunk) ContextString() string {
	source := strings.TrimSpace(c.Source)
	if source == "" {
		source = "unknown"
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = "untitled"
	}
	channels := strings.Join(uniqueNonBlank(c.Channels), ",")
	if channels == "" {
		channels = "retrieval"
	}
	return fmt.Sprintf("[source: %s | title: %s | channels: %s]\n%s", source, title, channels, strings.TrimSpace(c.Text))
}

func (c RetrievedChunk) Key() string {
	if strings.TrimSpace(c.ID) != "" {
		return strings.TrimSpace(c.ID)
	}
	if strings.TrimSpace(c.Source) != "" || strings.TrimSpace(c.Title) != "" {
		return strings.TrimSpace(c.Source) + "#" + strings.TrimSpace(c.Title)
	}
	return strings.TrimSpace(c.Text)
}

func FromChunk(chunk Chunk, channel string, score float64) RetrievedChunk {
	scores := map[string]float64{}
	if channel != "" {
		scores[channel] = score
	}
	return RetrievedChunk{
		ID:       chunk.ID,
		Title:    chunk.Title,
		Text:     chunk.Text,
		Source:   chunk.Source,
		Metadata: cloneMetadata(chunk.Metadata),
		Scores:   scores,
		Channels: uniqueNonBlank([]string{channel}),
	}
}

func ContextsFromResults(results []RetrievedChunk, topK int) []string {
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	contexts := make([]string, 0, topK)
	for _, result := range results[:topK] {
		if strings.TrimSpace(result.Text) == "" {
			continue
		}
		contexts = append(contexts, result.ContextString())
	}
	return contexts
}

func SortByScore(results []RetrievedChunk, scoreKey string) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Scores[scoreKey] > results[j].Scores[scoreKey]
	})
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func uniqueNonBlank(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
