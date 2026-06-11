package rag

import (
	"sort"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
)

// HybridRetriever combines dense vector recall, lexical recall, rank fusion,
// and an optional reranker behind the existing RAG port.
type HybridRetriever struct {
	retrievers      []namedRetriever
	reranker        corerag.Reranker
	rerankerName    string
	candidateK      int
	rrfK            float64
	maxContextChars int
}

type namedRetriever struct {
	name      string
	retriever corerag.DetailedRetriever
}

func NewHybridRetriever(cfg config.Config) *HybridRetriever {
	candidateK := cfg.RAGCandidateK
	if candidateK <= 0 {
		candidateK = 40
	}
	rrfK := cfg.RAGRRFK
	if rrfK <= 0 {
		rrfK = 60
	}

	var reranker corerag.Reranker
	rerankerName := "disabled"
	if strings.TrimSpace(cfg.RAGRerankerURL) != "" {
		reranker = NewHTTPReranker(cfg)
		rerankerName = strings.TrimSpace(cfg.RAGRerankerModel)
		if rerankerName == "" {
			rerankerName = "http-reranker"
		}
	}

	return &HybridRetriever{
		retrievers: []namedRetriever{
			{name: denseChannel, retriever: NewQdrantRetriever(cfg)},
			{name: lexicalChannel, retriever: NewMarkdownRetriever(cfg.DataDir)},
		},
		reranker:        reranker,
		rerankerName:    rerankerName,
		candidateK:      candidateK,
		rrfK:            float64(rrfK),
		maxContextChars: cfg.RAGMaxContextChars,
	}
}

func (r *HybridRetriever) Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error) {
	query := corerag.NewQuery(destination, preferences, pace, specialNotes, topK)
	results, err := r.RetrieveDetailed(query)
	if err != nil {
		return nil, err
	}
	return packContexts(results, query.Limit(5), r.maxContextChars), nil
}

func (r *HybridRetriever) RetrieveDetailed(query corerag.Query) ([]corerag.RetrievedChunk, error) {
	start := time.Now()
	finalK := query.Limit(5)
	candidateK := r.candidateK
	if candidateK < finalK*4 {
		candidateK = finalK * 4
	}

	recallQuery := query
	recallQuery.TopK = candidateK

	logging.Info(nil, "hybrid retriever retrieve started",
		"destination", query.Destination,
		"retrieval_mode", "hybrid",
		"routes_configured", len(r.retrievers),
		"candidate_k", candidateK,
		"final_k", finalK,
		"rrf_k", r.rrfK,
		"reranker", r.rerankerName,
	)

	routes := make([][]corerag.RetrievedChunk, 0, len(r.retrievers))
	var lastErr error
	for _, route := range r.retrievers {
		routeStart := time.Now()
		results, err := route.retriever.RetrieveDetailed(recallQuery)
		if err != nil {
			lastErr = err
			logging.Warn(nil, "hybrid retriever recall route failed",
				"destination", query.Destination,
				"route", route.name,
				"duration_ms", time.Since(routeStart).Milliseconds(),
				"error", err,
			)
			continue
		}
		logging.Info(nil, "hybrid retriever recall route completed",
			"destination", query.Destination,
			"route", route.name,
			"success", true,
			"results", len(results),
			"duration_ms", time.Since(routeStart).Milliseconds(),
		)
		if len(results) > 0 {
			routes = append(routes, results)
		}
	}
	if len(routes) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, nil
	}

	fused := rrfFuse(routes, r.rrfK)
	logging.Info(nil, "hybrid retriever fusion completed",
		"destination", query.Destination,
		"fusion", "rrf",
		"routes_succeeded", len(routes),
		"fused_candidates", len(fused),
		"rrf_k", r.rrfK,
	)
	if len(fused) > candidateK {
		fused = fused[:candidateK]
	}

	if r.reranker != nil && len(fused) > 1 {
		rerankStart := time.Now()
		reranked, err := r.reranker.Rerank(query, fused, finalK)
		if err != nil {
			logging.Warn(nil, "hybrid retriever reranker failed",
				"destination", query.Destination,
				"reranker", r.rerankerName,
				"candidates", len(fused),
				"duration_ms", time.Since(rerankStart).Milliseconds(),
				"error", err,
			)
		} else if len(reranked) > 0 {
			logging.Info(nil, "hybrid retriever reranker completed",
				"destination", query.Destination,
				"reranker", r.rerankerName,
				"input_candidates", len(fused),
				"output_candidates", len(reranked),
				"duration_ms", time.Since(rerankStart).Milliseconds(),
			)
			fused = reranked
		}
	} else {
		logging.Info(nil, "hybrid retriever reranker skipped",
			"destination", query.Destination,
			"reranker", r.rerankerName,
			"reason", rerankerSkipReason(r.reranker, len(fused)),
		)
	}

	if len(fused) > finalK {
		fused = fused[:finalK]
	}
	for i := range fused {
		fused[i].Rank = i + 1
	}

	logging.Info(nil, "hybrid retriever retrieve completed",
		"destination", query.Destination,
		"retrieval_mode", "hybrid",
		"routes", len(routes),
		"results", len(fused),
		"top_channels", topResultChannels(fused),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return fused, nil
}

func rerankerSkipReason(reranker corerag.Reranker, candidates int) string {
	if reranker == nil {
		return "not_configured"
	}
	if candidates <= 1 {
		return "not_enough_candidates"
	}
	return "unknown"
}

func topResultChannels(results []corerag.RetrievedChunk) string {
	if len(results) == 0 {
		return ""
	}
	return strings.Join(results[0].Channels, ",")
}

func rrfFuse(routes [][]corerag.RetrievedChunk, rrfK float64) []corerag.RetrievedChunk {
	if rrfK <= 0 {
		rrfK = 60
	}
	merged := map[string]corerag.RetrievedChunk{}
	for _, route := range routes {
		for rank, chunk := range route {
			if strings.TrimSpace(chunk.Text) == "" {
				continue
			}
			score := 1 / (rrfK + float64(rank+1))
			if chunk.Scores == nil {
				chunk.Scores = map[string]float64{}
			}
			chunk.Scores["rrf"] = score
			chunk.Scores["rrf_rank"] = float64(rank + 1)
			mergeRetrievedChunk(merged, chunk, "rrf", score)
		}
	}

	results := mapValues(merged)
	sort.SliceStable(results, func(i, j int) bool {
		left := results[i].Scores["rrf"]
		right := results[j].Scores["rrf"]
		if left == right {
			return results[i].Key() < results[j].Key()
		}
		return left > right
	})
	return results
}

func packContexts(results []corerag.RetrievedChunk, topK int, maxChars int) []string {
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	contexts := make([]string, 0, topK)
	usedChars := 0
	for _, result := range results {
		if len(contexts) >= topK {
			break
		}
		context := result.ContextString()
		if strings.TrimSpace(context) == "" {
			continue
		}
		if maxChars > 0 && usedChars+len([]rune(context)) > maxChars && len(contexts) > 0 {
			break
		}
		contexts = append(contexts, context)
		usedChars += len([]rune(context))
	}
	return contexts
}
