package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/logging"
)

// MarkdownRetriever 是一个不依赖向量数据库的 RAG 教学实现。
// Python 版优先 Chroma 向量检索，再 fallback 到关键词；这里先实现关键词检索，
// 方便你理解 RAG 的基本结构：读取知识库 -> 切片 -> 打分 -> 取 topK。
type MarkdownRetriever struct {
	DataDir string
}

func NewMarkdownRetriever(dataDir string) *MarkdownRetriever {
	return &MarkdownRetriever{DataDir: dataDir}
}

func (r *MarkdownRetriever) Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error) {
	start := time.Now()
	logging.Info(nil, "markdown retriever retrieve started",
		"data_dir", r.DataDir,
		"destination", destination,
		"preferences", len(preferences),
		"top_k", topK,
	)
	chunks, err := r.loadChunks()
	if err != nil {
		logging.Error(nil, "markdown retriever load chunks failed",
			"data_dir", r.DataDir,
			"destination", destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	queryTerms := buildQuery(destination, preferences, pace, specialNotes)

	type scoredChunk struct {
		Score int
		Chunk Chunk
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := scoreChunk(queryTerms, destination, chunk)
		if score > 0 {
			scored = append(scored, scoredChunk{Score: score, Chunk: chunk})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if topK <= 0 {
		topK = 3
	}
	if len(scored) < topK {
		topK = len(scored)
	}

	results := make([]string, 0, topK)
	for _, item := range scored[:topK] {
		results = append(results, fmt.Sprintf("[来源: %s | 标题: %s]\n%s", item.Chunk.Source, item.Chunk.Title, item.Chunk.Text))
	}
	logging.Info(nil, "markdown retriever retrieve completed",
		"data_dir", r.DataDir,
		"destination", destination,
		"chunks", len(chunks),
		"scored", len(scored),
		"results", len(results),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return results, nil
}

func (r *MarkdownRetriever) loadChunks() ([]Chunk, error) {
	start := time.Now()
	files, err := filepath.Glob(filepath.Join(r.DataDir, "*.md"))
	if err != nil {
		return nil, err
	}

	var chunks []Chunk
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		source := filepath.Base(file)
		chunks = append(chunks, splitMarkdown(string(data), source)...)
	}
	logging.Info(nil, "markdown retriever chunks loaded",
		"data_dir", r.DataDir,
		"files", len(files),
		"chunks", len(chunks),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return chunks, nil
}

func splitMarkdown(text string, source string) []Chunk {
	currentTitle := "文档开头"
	currentLines := []string{}
	chunks := []Chunk{}

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if body == "" {
			return
		}
		chunks = append(chunks, Chunk{
			Title:  currentTitle,
			Text:   body,
			Source: source,
		})
		currentLines = []string{}
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			flush()
			currentTitle = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		if trimmed != "" {
			currentLines = append(currentLines, trimmed)
		}
	}
	flush()
	return chunks
}

func scoreChunk(queryTerms []string, destination string, chunk Chunk) int {
	score := 0
	combined := chunk.Title + "\n" + chunk.Text + "\n" + chunk.Source
	for _, term := range queryTerms {
		if strings.Contains(chunk.Title, term) {
			score += 3
		}
		if strings.Contains(chunk.Text, term) {
			score += 1
		}
	}
	if destination != "" && strings.Contains(combined, destination) {
		score += 5
	}
	if chunk.Title == "文档开头" {
		score -= 4
	}
	return score
}
