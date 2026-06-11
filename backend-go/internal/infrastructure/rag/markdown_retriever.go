package rag

import (
	"crypto/sha1"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"travel-agent-go/internal/geo"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
)

const lexicalChannel = "lexical"
const lexicalTokenizer = "unicode_han_bigram_trigram_ascii"

// MarkdownRetriever is a local-file adapter for the RAG Retriever port.
type MarkdownRetriever struct {
	DataDir string
}

func NewMarkdownRetriever(dataDir string) *MarkdownRetriever {
	return &MarkdownRetriever{DataDir: dataDir}
}

func (r *MarkdownRetriever) Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error) {
	query := corerag.NewQuery(destination, preferences, pace, specialNotes, topK)
	results, err := r.RetrieveDetailed(query)
	if err != nil {
		return nil, err
	}
	return corerag.ContextsFromResults(results, query.Limit(3)), nil
}

func (r *MarkdownRetriever) RetrieveDetailed(query corerag.Query) ([]corerag.RetrievedChunk, error) {
	start := time.Now()
	limit := query.Limit(5)

	logging.Info(nil, "markdown retriever retrieve started",
		"data_dir", r.DataDir,
		"destination", query.Destination,
		"preferences", len(query.Preferences),
		"top_k", limit,
		"channel", lexicalChannel,
		"tokenizer", lexicalTokenizer,
	)

	chunks, err := LoadMarkdownChunks(r.DataDir)
	if err != nil {
		logging.Error(nil, "markdown retriever load chunks failed",
			"data_dir", r.DataDir,
			"destination", query.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}

	results := lexicalSearch(chunks, query, limit)
	topScore := 0.0
	if len(results) > 0 {
		topScore = results[0].Scores[lexicalChannel]
	}
	logging.Info(nil, "markdown retriever retrieve completed",
		"data_dir", r.DataDir,
		"destination", query.Destination,
		"chunks", len(chunks),
		"results", len(results),
		"channel", lexicalChannel,
		"tokenizer", lexicalTokenizer,
		"top_score", topScore,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return results, nil
}

func LoadMarkdownChunks(dataDir string) ([]corerag.Chunk, error) {
	files, err := filepath.Glob(filepath.Join(dataDir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	chunks := make([]corerag.Chunk, 0)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		source := filepath.Base(file)
		chunks = append(chunks, splitMarkdown(string(data), source)...)
	}
	return chunks, nil
}

func splitMarkdown(text string, source string) []corerag.Chunk {
	currentTitle := "Document start"
	currentLines := []string{}
	chunks := []corerag.Chunk{}

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if body == "" {
			return
		}
		chunk := corerag.Chunk{
			ID:     stableChunkID(source, currentTitle, body),
			Title:  currentTitle,
			Text:   body,
			Source: source,
			Metadata: map[string]any{
				"source": source,
				"title":  currentTitle,
				"city":   inferCityFromSource(source),
			},
		}
		chunks = append(chunks, chunk)
		currentLines = []string{}
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
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

func lexicalSearch(chunks []corerag.Chunk, query corerag.Query, limit int) []corerag.RetrievedChunk {
	queryTokens := tokenize(query.SearchText())
	if len(queryTokens) == 0 {
		logging.Warn(nil, "lexical search skipped empty query tokens",
			"channel", lexicalChannel,
			"tokenizer", lexicalTokenizer,
			"destination", query.Destination,
		)
		return nil
	}

	docFreq := map[string]int{}
	docTokens := make([][]string, len(chunks))
	for i, chunk := range chunks {
		tokens := tokenize(chunk.Title + "\n" + chunk.Text + "\n" + chunk.Source)
		docTokens[i] = tokens
		seen := map[string]bool{}
		for _, token := range tokens {
			if !seen[token] {
				docFreq[token]++
				seen[token] = true
			}
		}
	}

	queryFreq := termFrequency(queryTokens)
	logging.Info(nil, "lexical search scoring started",
		"channel", lexicalChannel,
		"tokenizer", lexicalTokenizer,
		"destination", query.Destination,
		"query_tokens", len(queryTokens),
		"documents", len(chunks),
		"limit", limit,
	)
	type scoredChunk struct {
		score float64
		chunk corerag.Chunk
	}

	scored := make([]scoredChunk, 0, len(chunks))
	totalDocs := float64(len(chunks))
	for i, chunk := range chunks {
		if !chunkMatchesDestination(query.Destination, chunk) {
			continue
		}
		score := 0.0
		tf := termFrequency(docTokens[i])
		for token, qf := range queryFreq {
			frequency := tf[token]
			if frequency == 0 {
				continue
			}
			idf := math.Log((totalDocs+1)/(float64(docFreq[token])+1)) + 1
			score += math.Sqrt(float64(qf)) * math.Log1p(float64(frequency)) * idf
		}

		score += metadataBoost(query, chunk)
		if score > 0 {
			scored = append(scored, scoredChunk{score: score, chunk: chunk})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if limit <= 0 || limit > len(scored) {
		limit = len(scored)
	}
	results := make([]corerag.RetrievedChunk, 0, limit)
	for rank, item := range scored[:limit] {
		result := corerag.FromChunk(item.chunk, lexicalChannel, item.score)
		result.Rank = rank + 1
		results = append(results, result)
	}
	return results
}

func chunkMatchesDestination(destination string, chunk corerag.Chunk) bool {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return true
	}
	if city, ok := chunk.Metadata["city"].(string); ok && strings.TrimSpace(city) != "" {
		return geo.CityMatchesDestination(city, destination)
	}
	text := chunk.Title + "\n" + chunk.Text + "\n" + chunk.Source
	return geo.TextMatchesDestination(destination, text)
}

func metadataBoost(query corerag.Query, chunk corerag.Chunk) float64 {
	score := 0.0
	combined := strings.ToLower(chunk.Title + "\n" + chunk.Text + "\n" + chunk.Source)
	destination := strings.ToLower(strings.TrimSpace(query.Destination))
	if destination != "" && strings.Contains(combined, destination) {
		score += 6
	}
	for _, preference := range query.Preferences {
		preference = strings.ToLower(strings.TrimSpace(preference))
		if preference != "" && strings.Contains(combined, preference) {
			score += 1.5
		}
	}
	if query.Pace != "" && strings.Contains(combined, strings.ToLower(query.Pace)) {
		score += 1
	}
	return score
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	tokens := []string{}
	var ascii strings.Builder
	var han []rune

	flushASCII := func() {
		if ascii.Len() > 0 {
			tokens = append(tokens, ascii.String())
			ascii.Reset()
		}
	}
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) <= 6 {
			tokens = append(tokens, string(han))
		}
		for size := 2; size <= 3; size++ {
			if len(han) < size {
				continue
			}
			for i := 0; i <= len(han)-size; i++ {
				tokens = append(tokens, string(han[i:i+size]))
			}
		}
		han = han[:0]
	}

	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			flushASCII()
			han = append(han, r)
			continue
		}
		flushHan()
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			ascii.WriteRune(r)
		} else {
			flushASCII()
		}
	}
	flushASCII()
	flushHan()
	return uniqueStrings(tokens)
}

func termFrequency(tokens []string) map[string]int {
	freq := map[string]int{}
	for _, token := range tokens {
		if token != "" {
			freq[token]++
		}
	}
	return freq
}

func uniqueStrings(values []string) []string {
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

func inferCityFromSource(source string) string {
	source = strings.TrimSuffix(strings.ToLower(source), filepath.Ext(source))
	source = strings.TrimSuffix(source, "_guide")
	return source
}

func stableChunkID(source string, title string, text string) string {
	sum := sha1.Sum([]byte(source + "\n" + title + "\n" + text))
	return fmt.Sprintf("%x", sum[:])
}
