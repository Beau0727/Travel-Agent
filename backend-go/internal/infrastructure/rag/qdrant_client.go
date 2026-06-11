package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"travel-agent-go/internal/logging"
)

type QdrantClient struct {
	baseURL    string
	httpClient *http.Client
}

type QdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type QdrantSearchResult struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func NewQdrantClient(baseURL string) *QdrantClient {
	return &QdrantClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, collection string, dim int) error {
	start := time.Now()
	if c.baseURL == "" {
		return fmt.Errorf("qdrant URL is empty")
	}
	if collection == "" {
		return fmt.Errorf("qdrant collection is empty")
	}
	if dim <= 0 {
		return fmt.Errorf("embedding dim must be positive")
	}

	path := "/collections/" + url.PathEscape(collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	logging.Info(ctx, "qdrant ensure collection started",
		"collection", collection,
		"embedding_dim", dim,
	)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.Warn(ctx, "qdrant ensure collection check failed",
			"collection", collection,
			"embedding_dim", dim,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		logging.Info(ctx, "qdrant collection already exists",
			"collection", collection,
			"embedding_dim", dim,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		logging.Warn(ctx, "qdrant ensure collection unexpected status",
			"collection", collection,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return fmt.Errorf("check qdrant collection failed: %s: %s", resp.Status, string(data))
	}

	body := map[string]any{
		"vectors": map[string]any{
			"size":     dim,
			"distance": "Cosine",
		},
	}
	if err := c.requestJSON(ctx, http.MethodPut, path, body, nil); err != nil {
		logging.Warn(ctx, "qdrant create collection failed",
			"collection", collection,
			"embedding_dim", dim,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}
	logging.Info(ctx, "qdrant collection created",
		"collection", collection,
		"embedding_dim", dim,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (c *QdrantClient) Upsert(ctx context.Context, collection string, points []QdrantPoint) error {
	if len(points) == 0 {
		return nil
	}

	start := time.Now()
	logging.Info(ctx, "qdrant upsert started",
		"collection", collection,
		"points", len(points),
	)
	body := map[string]any{"points": points}
	path := "/collections/" + url.PathEscape(collection) + "/points?wait=true"
	if err := c.requestJSON(ctx, http.MethodPut, path, body, nil); err != nil {
		logging.Warn(ctx, "qdrant upsert failed",
			"collection", collection,
			"points", len(points),
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return err
	}
	logging.Info(ctx, "qdrant upsert completed",
		"collection", collection,
		"points", len(points),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float32, limit int) ([]QdrantSearchResult, error) {
	start := time.Now()
	if len(vector) == 0 {
		return nil, fmt.Errorf("search vector is empty")
	}
	if limit <= 0 {
		limit = 3
	}

	body := map[string]any{
		"query":        vector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}

	var out struct {
		Result struct {
			Points []QdrantSearchResult `json:"points"`
		} `json:"result"`
	}
	path := "/collections/" + url.PathEscape(collection) + "/points/query"
	logging.Info(ctx, "qdrant search started",
		"collection", collection,
		"vector_dim", len(vector),
		"limit", limit,
	)
	if err := c.requestJSON(ctx, http.MethodPost, path, body, &out); err != nil {
		logging.Warn(ctx, "qdrant search failed",
			"collection", collection,
			"vector_dim", len(vector),
			"limit", limit,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	topScore := 0.0
	if len(out.Result.Points) > 0 {
		topScore = out.Result.Points[0].Score
	}
	logging.Info(ctx, "qdrant search completed",
		"collection", collection,
		"results", len(out.Result.Points),
		"top_score", topScore,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return out.Result.Points, nil
}

func (c *QdrantClient) requestJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant request failed: %s %s: %s: %s", method, path, resp.Status, string(data))
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}
