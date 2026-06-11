package tools

import (
	"context"
	"time"

	"travel-agent-go/internal/logging"
	"travel-agent-go/internal/rag"
)

// RAGTool 把攻略检索包装成 Agent 可调用工具。
// 现在工具是 Go 代码直接调用；后续如果做 LLM tool calling，可以把 Name/Description/JSON schema 加进来。
type RAGTool struct {
	retriever rag.Retriever
}

type RAGSearchInput struct {
	Destination  string
	Preferences  []string
	Pace         string
	SpecialNotes string
	TopK         int
}

func NewRAGTool(retriever rag.Retriever) *RAGTool {
	return &RAGTool{retriever: retriever}
}

func (t *RAGTool) Search(ctx context.Context, input RAGSearchInput) ([]string, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logging.Info(ctx, "rag tool search started",
		"destination", input.Destination,
		"preferences", len(input.Preferences),
		"pace", input.Pace,
		"top_k", input.TopK,
	)
	contexts, err := t.retriever.Retrieve(
		input.Destination,
		input.Preferences,
		input.Pace,
		input.SpecialNotes,
		input.TopK,
	)
	if err != nil {
		logging.Warn(ctx, "rag tool search failed",
			"destination", input.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	logging.Info(ctx, "rag tool search completed",
		"destination", input.Destination,
		"contexts", len(contexts),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return contexts, nil
}
