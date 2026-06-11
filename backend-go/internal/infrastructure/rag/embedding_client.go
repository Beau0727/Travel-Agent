package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/logging"
)

// EmbeddingClient adapts Ollama's local embedding API.
type EmbeddingClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewEmbeddingClient(cfg config.Config) *EmbeddingClient {
	return &EmbeddingClient{
		baseURL: strings.TrimRight(cfg.EmbeddingBaseURL, "/"),
		model:   cfg.EmbeddingModel,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	start := time.Now()
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embedding text is empty")
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("embedding base URL is empty")
	}
	if c.model == "" {
		return nil, fmt.Errorf("embedding model is empty")
	}

	body, err := json.Marshal(map[string]any{
		"model": c.model,
		"input": text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	logging.Info(ctx, "embedding request started",
		"provider", "ollama",
		"model", c.model,
		"input_chars", len([]rune(text)),
	)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.Warn(ctx, "embedding request failed",
			"provider", "ollama",
			"model", c.model,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		logging.Warn(ctx, "embedding response failed",
			"provider", "ollama",
			"model", c.model,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, fmt.Errorf("ollama embed failed: %s: %s", resp.Status, string(data))
	}

	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned no embeddings")
	}
	if len(out.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama returned an empty embedding")
	}
	logging.Info(ctx, "embedding request completed",
		"provider", "ollama",
		"model", c.model,
		"dim", len(out.Embeddings[0]),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return out.Embeddings[0], nil
}
