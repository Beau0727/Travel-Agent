package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
)

const denseChannel = "dense"

type QdrantRetriever struct {
	embedder         *EmbeddingClient
	qdrant           *QdrantClient
	collection       string
	maxQueryVariants int
}

func NewQdrantRetriever(cfg config.Config) *QdrantRetriever {
	maxQueryVariants := cfg.RAGQueryVariants
	if maxQueryVariants <= 0 {
		maxQueryVariants = 3
	}
	return &QdrantRetriever{
		embedder:         NewEmbeddingClient(cfg),
		qdrant:           NewQdrantClient(cfg.QdrantURL),
		collection:       cfg.QdrantCollection,
		maxQueryVariants: maxQueryVariants,
	}
}

func (r *QdrantRetriever) Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error) {
	query := corerag.NewQuery(destination, preferences, pace, specialNotes, topK)
	results, err := r.RetrieveDetailed(query)
	if err != nil {
		return nil, err
	}
	return corerag.ContextsFromResults(results, query.Limit(3)), nil
}

func (r *QdrantRetriever) RetrieveDetailed(query corerag.Query) ([]corerag.RetrievedChunk, error) {
	start := time.Now()
	limit := query.Limit(5)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	texts := query.ExpandedTexts()
	if r.maxQueryVariants > 0 && len(texts) > r.maxQueryVariants {
		texts = texts[:r.maxQueryVariants]
	}
	if len(texts) == 0 {
		texts = []string{query.SearchText()}
	}

	logging.Info(nil, "qdrant retriever retrieve started",
		"destination", query.Destination,
		"collection", r.collection,
		"channel", denseChannel,
		"variants", len(texts),
		"top_k", limit,
	)

	merged := map[string]corerag.RetrievedChunk{}
	for variantIndex, text := range texts {
		variantStart := time.Now()
		vector, err := r.embedder.Embed(ctx, text)
		if err != nil {
			logging.Warn(nil, "qdrant retriever embedding failed",
				"destination", query.Destination,
				"collection", r.collection,
				"channel", denseChannel,
				"variant", variantIndex+1,
				"duration_ms", time.Since(variantStart).Milliseconds(),
				"error", err,
			)
			return nil, err
		}

		results, err := r.qdrant.Search(ctx, r.collection, vector, limit)
		if err != nil {
			logging.Warn(nil, "qdrant retriever search failed",
				"destination", query.Destination,
				"collection", r.collection,
				"channel", denseChannel,
				"variant", variantIndex+1,
				"duration_ms", time.Since(variantStart).Milliseconds(),
				"error", err,
			)
			return nil, err
		}
		topScore := 0.0
		if len(results) > 0 {
			topScore = results[0].Score
		}
		logging.Info(nil, "qdrant retriever variant completed",
			"destination", query.Destination,
			"collection", r.collection,
			"channel", denseChannel,
			"variant", variantIndex+1,
			"results", len(results),
			"top_score", topScore,
			"duration_ms", time.Since(variantStart).Milliseconds(),
		)
		for rank, item := range results {
			chunk := qdrantResultToChunk(item)
			if strings.TrimSpace(chunk.Text) == "" {
				continue
			}
			score := item.Score / float64(variantIndex+1)
			result := corerag.FromChunk(chunk, denseChannel, score)
			result.Rank = rank + 1
			result.Scores["dense_rank"] = float64(rank + 1)
			result.Scores["dense_variant"] = float64(variantIndex + 1)
			mergeRetrievedChunk(merged, result, denseChannel, score)
		}
	}

	results := mapValues(merged)
	corerag.SortByScore(results, denseChannel)
	if len(results) > limit {
		results = results[:limit]
	}
	for i := range results {
		results[i].Rank = i + 1
	}

	logging.Info(nil, "qdrant retriever retrieve completed",
		"destination", query.Destination,
		"collection", r.collection,
		"channel", denseChannel,
		"variants", len(texts),
		"results", len(results),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return results, nil
}

func qdrantResultToChunk(item QdrantSearchResult) corerag.Chunk {
	title := payloadString(item.Payload, "title")
	text := payloadString(item.Payload, "text")
	source := payloadString(item.Payload, "source")
	chunkID := payloadString(item.Payload, "chunk_id")
	if chunkID == "" {
		chunkID = payloadString(item.Payload, "id")
	}
	if chunkID == "" {
		chunkID = fmt.Sprint(item.ID)
	}
	if title == "" {
		title = "Untitled chunk"
	}
	if source == "" {
		source = "qdrant"
	}

	metadata := map[string]any{}
	for key, value := range item.Payload {
		metadata[key] = value
	}
	metadata["qdrant_id"] = item.ID

	return corerag.Chunk{
		ID:       chunkID,
		Title:    title,
		Text:     text,
		Source:   source,
		Metadata: metadata,
	}
}

func mergeRetrievedChunk(merged map[string]corerag.RetrievedChunk, next corerag.RetrievedChunk, channel string, score float64) {
	key := next.Key()
	if key == "" {
		return
	}
	current, exists := merged[key]
	if !exists {
		merged[key] = next
		return
	}
	if current.Scores == nil {
		current.Scores = map[string]float64{}
	}
	for scoreKey, scoreValue := range next.Scores {
		if scoreKey == channel {
			current.Scores[scoreKey] += score
			continue
		}
		if current.Scores[scoreKey] == 0 {
			current.Scores[scoreKey] = scoreValue
		}
	}
	current.Channels = appendUnique(current.Channels, next.Channels...)
	if len(current.Metadata) == 0 {
		current.Metadata = next.Metadata
	} else {
		for key, value := range next.Metadata {
			if _, exists := current.Metadata[key]; !exists {
				current.Metadata[key] = value
			}
		}
	}
	merged[key] = current
}

func mapValues(values map[string]corerag.RetrievedChunk) []corerag.RetrievedChunk {
	results := make([]corerag.RetrievedChunk, 0, len(values))
	for _, value := range values {
		results = append(results, value)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Key() < results[j].Key()
	})
	return results
}

func appendUnique(base []string, values ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}
