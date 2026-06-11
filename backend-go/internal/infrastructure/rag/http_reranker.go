package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
)

// HTTPReranker calls an optional sidecar reranker service. The service contract
// is intentionally small so local FlagEmbedding/Qwen services and hosted APIs
// can be adapted without changing the Go application.
type HTTPReranker struct {
	url        string
	model      string
	httpClient *http.Client
}

func NewHTTPReranker(cfg config.Config) *HTTPReranker {
	timeout := cfg.RAGRerankerTimeout
	if timeout <= 0 {
		timeout = 30
	}
	return &HTTPReranker{
		url:   strings.TrimSpace(cfg.RAGRerankerURL),
		model: strings.TrimSpace(cfg.RAGRerankerModel),
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (r *HTTPReranker) Rerank(query corerag.Query, chunks []corerag.RetrievedChunk, topK int) ([]corerag.RetrievedChunk, error) {
	start := time.Now()
	if strings.TrimSpace(r.url) == "" {
		return chunks, nil
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	if topK <= 0 || topK > len(chunks) {
		topK = len(chunks)
	}

	logging.Info(nil, "reranker request started",
		"provider", "http",
		"model", r.model,
		"endpoint", r.url,
		"candidates", len(chunks),
		"top_k", topK,
	)

	documents := make([]rerankDocument, 0, len(chunks))
	for i, chunk := range chunks {
		documents = append(documents, rerankDocument{
			Index: i,
			ID:    chunk.Key(),
			Text:  strings.TrimSpace(chunk.Title + "\n" + chunk.Text),
		})
	}

	body, err := json.Marshal(rerankRequest{
		Model:     r.model,
		Query:     query.SearchText(),
		TopK:      topK,
		Documents: documents,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.httpClient.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logging.Warn(nil, "reranker request failed",
			"provider", "http",
			"model", r.model,
			"endpoint", r.url,
			"candidates", len(chunks),
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		logging.Warn(nil, "reranker response failed",
			"provider", "http",
			"model", r.model,
			"endpoint", r.url,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, fmt.Errorf("reranker failed: %s: %s", resp.Status, string(data))
	}

	var out rerankResponse
	if err := json.Unmarshal(data, &out); err != nil {
		logging.Warn(nil, "reranker response parse failed",
			"provider", "http",
			"model", r.model,
			"endpoint", r.url,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	if len(out.Results) == 0 {
		logging.Warn(nil, "reranker returned empty results",
			"provider", "http",
			"model", r.model,
			"endpoint", r.url,
			"candidates", len(chunks),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return chunks, nil
	}

	topScore, averageScore := rerankScoreSummary(out.Results)
	scored := make([]corerag.RetrievedChunk, 0, len(out.Results))
	used := map[int]bool{}
	for _, item := range out.Results {
		index := item.Index
		if index < 0 || index >= len(chunks) || used[index] {
			continue
		}
		next := chunks[index]
		if next.Scores == nil {
			next.Scores = map[string]float64{}
		}
		next.Scores["rerank"] = item.Score
		next.Channels = appendUnique(next.Channels, "rerank")
		scored = append(scored, next)
		used[index] = true
	}

	for i, chunk := range chunks {
		if used[i] {
			continue
		}
		scored = append(scored, chunk)
	}

	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i].Scores["rerank"]
		right := scored[j].Scores["rerank"]
		if left == right {
			return scored[i].Scores["rrf"] > scored[j].Scores["rrf"]
		}
		return left > right
	})
	if len(scored) > topK {
		scored = scored[:topK]
	}
	for i := range scored {
		scored[i].Rank = i + 1
	}
	logging.Info(nil, "reranker request completed",
		"provider", "http",
		"model", r.model,
		"endpoint", r.url,
		"input_candidates", len(chunks),
		"returned_scores", len(out.Results),
		"output_candidates", len(scored),
		"top_score", topScore,
		"average_score", averageScore,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return scored, nil
}

func rerankScoreSummary(results []rerankResult) (float64, float64) {
	if len(results) == 0 {
		return 0, 0
	}
	top := results[0].Score
	sum := 0.0
	for _, result := range results {
		if result.Score > top {
			top = result.Score
		}
		sum += result.Score
	}
	return top, sum / float64(len(results))
}

type rerankRequest struct {
	Model     string           `json:"model,omitempty"`
	Query     string           `json:"query"`
	TopK      int              `json:"top_k,omitempty"`
	Documents []rerankDocument `json:"documents"`
}

type rerankDocument struct {
	Index int    `json:"index"`
	ID    string `json:"id,omitempty"`
	Text  string `json:"text"`
}

type rerankResponse struct {
	Results []rerankResult `json:"results"`
}

type rerankResult struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}
