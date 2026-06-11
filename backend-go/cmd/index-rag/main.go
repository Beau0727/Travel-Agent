package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"time"

	"travel-agent-go/internal/config"
	infrarag "travel-agent-go/internal/infrastructure/rag"
	"travel-agent-go/internal/logging"
)

const batchSize = 32

func main() {
	logging.Configure()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	start := time.Now()
	cfg := config.Load()
	ctx := context.Background()
	logging.Info(ctx, "rag index started",
		"data_dir", cfg.DataDir,
		"qdrant_collection", cfg.QdrantCollection,
		"embedding_provider", "ollama",
		"embedding_model", cfg.EmbeddingModel,
		"embedding_dim", cfg.EmbeddingDim,
		"batch_size", batchSize,
	)

	chunks, err := infrarag.LoadMarkdownChunks(cfg.DataDir)
	if err != nil {
		logging.Error(ctx, "rag index load chunks failed",
			"data_dir", cfg.DataDir,
			"error", err,
		)
		return err
	}
	fmt.Printf("loaded %d markdown chunks from %s\n", len(chunks), cfg.DataDir)
	logging.Info(ctx, "rag index chunks loaded",
		"data_dir", cfg.DataDir,
		"chunks", len(chunks),
	)
	if len(chunks) == 0 {
		logging.Warn(ctx, "rag index skipped empty chunk set",
			"data_dir", cfg.DataDir,
		)
		return nil
	}

	embedder := infrarag.NewEmbeddingClient(cfg)
	qdrant := infrarag.NewQdrantClient(cfg.QdrantURL)
	if err := qdrant.EnsureCollection(ctx, cfg.QdrantCollection, cfg.EmbeddingDim); err != nil {
		logging.Error(ctx, "rag index ensure qdrant collection failed",
			"collection", cfg.QdrantCollection,
			"embedding_dim", cfg.EmbeddingDim,
			"error", err,
		)
		return err
	}

	batch := make([]infrarag.QdrantPoint, 0, batchSize)
	for i, chunk := range chunks {
		vector, err := embedder.Embed(ctx, chunk.Title+"\n"+chunk.Text)
		if err != nil {
			logging.Error(ctx, "rag index embed chunk failed",
				"chunk_index", i+1,
				"source", chunk.Source,
				"title", chunk.Title,
				"error", err,
			)
			return fmt.Errorf("embed chunk %d %s/%s: %w", i+1, chunk.Source, chunk.Title, err)
		}
		if len(vector) != cfg.EmbeddingDim {
			logging.Error(ctx, "rag index embedding dim mismatch",
				"chunk_index", i+1,
				"source", chunk.Source,
				"title", chunk.Title,
				"got_dim", len(vector),
				"want_dim", cfg.EmbeddingDim,
			)
			return fmt.Errorf("embedding dim mismatch: got %d, want %d", len(vector), cfg.EmbeddingDim)
		}

		payload := map[string]any{
			"chunk_id": chunk.ID,
			"title":    chunk.Title,
			"text":     chunk.Text,
			"source":   chunk.Source,
		}
		for key, value := range chunk.Metadata {
			if _, exists := payload[key]; !exists {
				payload[key] = value
			}
		}

		batch = append(batch, infrarag.QdrantPoint{
			ID:      stableUUID(chunk.ID),
			Vector:  vector,
			Payload: payload,
		})

		if len(batch) >= batchSize {
			if err := qdrant.Upsert(ctx, cfg.QdrantCollection, batch); err != nil {
				logging.Error(ctx, "rag index qdrant upsert failed",
					"collection", cfg.QdrantCollection,
					"indexed", i+1,
					"batch_points", len(batch),
					"error", err,
				)
				return err
			}
			fmt.Printf("indexed %d/%d chunks\n", i+1, len(chunks))
			logging.Info(ctx, "rag index batch completed",
				"collection", cfg.QdrantCollection,
				"indexed", i+1,
				"total", len(chunks),
				"batch_points", len(batch),
			)
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := qdrant.Upsert(ctx, cfg.QdrantCollection, batch); err != nil {
			logging.Error(ctx, "rag index final qdrant upsert failed",
				"collection", cfg.QdrantCollection,
				"batch_points", len(batch),
				"error", err,
			)
			return err
		}
		logging.Info(ctx, "rag index final batch completed",
			"collection", cfg.QdrantCollection,
			"batch_points", len(batch),
		)
	}

	fmt.Printf("indexed all %d chunks into qdrant collection %s\n", len(chunks), cfg.QdrantCollection)
	logging.Info(ctx, "rag index completed",
		"collection", cfg.QdrantCollection,
		"chunks", len(chunks),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func stableUUID(text string) string {
	sum := sha1.Sum([]byte(text))
	b := make([]byte, 16)
	copy(b, sum[:16])

	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
