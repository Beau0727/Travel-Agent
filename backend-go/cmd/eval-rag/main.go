package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	infrarag "travel-agent-go/internal/infrastructure/rag"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
)

type evalCase struct {
	ID               string   `json:"id"`
	Destination      string   `json:"destination"`
	Preferences      []string `json:"preferences"`
	Pace             string   `json:"pace"`
	SpecialNotes     string   `json:"special_notes"`
	Query            string   `json:"query"`
	TopK             int      `json:"top_k"`
	RelevantChunkIDs []string `json:"relevant_chunk_ids"`
	RelevantSources  []string `json:"relevant_sources"`
	MustContain      []string `json:"must_contain"`
	Forbidden        []string `json:"forbidden"`
}

type caseReport struct {
	ID            string   `json:"id"`
	Hit           bool     `json:"hit"`
	Recall        float64  `json:"recall"`
	MRR           float64  `json:"mrr"`
	NDCG          float64  `json:"ndcg"`
	ForbiddenHits int      `json:"forbidden_hits"`
	LatencyMS     int64    `json:"latency_ms"`
	ResultIDs     []string `json:"result_ids"`
	ResultSources []string `json:"result_sources"`
	Channels      []string `json:"channels"`
}

type evalReport struct {
	Backend          string       `json:"backend"`
	Cases            int          `json:"cases"`
	HitRate          float64      `json:"hit_rate"`
	AverageRecall    float64      `json:"average_recall"`
	AverageMRR       float64      `json:"average_mrr"`
	AverageNDCG      float64      `json:"average_ndcg"`
	ForbiddenHits    int          `json:"forbidden_hits"`
	AverageLatencyMS float64      `json:"average_latency_ms"`
	CaseReports      []caseReport `json:"case_reports"`
}

func main() {
	logging.Configure()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	casesPath := flag.String("cases", "data/eval/rag_cases.jsonl", "JSONL file with RAG evaluation cases")
	backend := flag.String("backend", "", "RAG backend override: markdown, qdrant, or hybrid")
	topK := flag.Int("top-k", 5, "default retrieval cutoff")
	outputPath := flag.String("output", "", "optional JSON report output path")
	flag.Parse()

	cfg := config.Load()
	if strings.TrimSpace(*backend) != "" {
		cfg.RAGBackend = strings.TrimSpace(*backend)
	}

	logging.Info(nil, "rag evaluation started",
		"backend", cfg.RAGBackend,
		"cases_path", *casesPath,
		"default_top_k", *topK,
		"qdrant_collection", cfg.QdrantCollection,
		"embedding_model", cfg.EmbeddingModel,
		"reranker_enabled", strings.TrimSpace(cfg.RAGRerankerURL) != "",
		"reranker_model", cfg.RAGRerankerModel,
	)

	retriever := buildRetriever(cfg)
	cases, err := loadCases(*casesPath)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("no eval cases loaded from %s", *casesPath)
	}

	report := evalReport{
		Backend:     cfg.RAGBackend,
		Cases:       len(cases),
		CaseReports: make([]caseReport, 0, len(cases)),
	}

	var totalLatency int64
	for _, item := range cases {
		if item.TopK <= 0 {
			item.TopK = *topK
		}

		query := corerag.NewQuery(item.Destination, item.Preferences, item.Pace, item.SpecialNotes, item.TopK).WithText(item.Query)
		start := time.Now()
		results, err := retriever.RetrieveDetailed(query)
		if err != nil {
			return fmt.Errorf("eval case %s failed: %w", item.ID, err)
		}
		latency := time.Since(start).Milliseconds()
		totalLatency += latency

		caseResult := evaluateCase(item, results, item.TopK)
		caseResult.LatencyMS = latency
		report.CaseReports = append(report.CaseReports, caseResult)
		logging.Info(nil, "rag evaluation case completed",
			"case_id", item.ID,
			"backend", cfg.RAGBackend,
			"hit", caseResult.Hit,
			"recall", caseResult.Recall,
			"mrr", caseResult.MRR,
			"ndcg", caseResult.NDCG,
			"forbidden_hits", caseResult.ForbiddenHits,
			"latency_ms", caseResult.LatencyMS,
			"channels", strings.Join(uniqueStrings(caseResult.Channels), ","),
		)

		if caseResult.Hit {
			report.HitRate += 1
		}
		report.AverageRecall += caseResult.Recall
		report.AverageMRR += caseResult.MRR
		report.AverageNDCG += caseResult.NDCG
		report.ForbiddenHits += caseResult.ForbiddenHits
	}

	report.HitRate /= float64(len(cases))
	report.AverageRecall /= float64(len(cases))
	report.AverageMRR /= float64(len(cases))
	report.AverageNDCG /= float64(len(cases))
	report.AverageLatencyMS = float64(totalLatency) / float64(len(cases))
	logging.Info(nil, "rag evaluation completed",
		"backend", report.Backend,
		"cases", report.Cases,
		"hit_rate", report.HitRate,
		"average_recall", report.AverageRecall,
		"average_mrr", report.AverageMRR,
		"average_ndcg", report.AverageNDCG,
		"forbidden_hits", report.ForbiddenHits,
		"average_latency_ms", report.AverageLatencyMS,
	)

	printReport(report)
	if strings.TrimSpace(*outputPath) != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func buildRetriever(cfg config.Config) corerag.DetailedRetriever {
	if strings.EqualFold(cfg.RAGBackend, "hybrid") {
		return infrarag.NewHybridRetriever(cfg)
	}
	if strings.EqualFold(cfg.RAGBackend, "qdrant") {
		return infrarag.NewQdrantRetriever(cfg)
	}
	return infrarag.NewMarkdownRetriever(cfg.DataDir)
}

func loadCases(path string) ([]evalCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cases := []evalCase{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var item evalCase
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode %s line %d: %w", path, lineNo, err)
		}
		if strings.TrimSpace(item.ID) == "" {
			item.ID = fmt.Sprintf("case_%d", lineNo)
		}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

func evaluateCase(item evalCase, results []corerag.RetrievedChunk, topK int) caseReport {
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	report := caseReport{ID: item.ID}
	if topK == 0 {
		return report
	}

	relevantIDs := toSet(item.RelevantChunkIDs)
	relevantSources := toSet(item.RelevantSources)
	mustContain := normalizeList(item.MustContain)
	forbidden := normalizeList(item.Forbidden)

	relevanceByRank := make([]float64, 0, topK)
	coveredTerms := map[string]bool{}
	firstRelevantRank := 0
	relevantHits := 0

	for i, result := range results[:topK] {
		report.ResultIDs = append(report.ResultIDs, result.ID)
		report.ResultSources = append(report.ResultSources, result.Source)
		report.Channels = append(report.Channels, result.Channels...)
		text := normalizeText(result.Title + "\n" + result.Text + "\n" + result.Source)

		for _, term := range forbidden {
			if term != "" && strings.Contains(text, term) {
				report.ForbiddenHits++
			}
		}

		relevant := false
		if len(relevantIDs) > 0 && relevantIDs[normalizeText(result.ID)] {
			relevant = true
			relevantHits++
		}
		if len(relevantSources) > 0 && relevantSources[normalizeText(result.Source)] {
			relevant = true
		}
		for _, term := range mustContain {
			if term != "" && strings.Contains(text, term) {
				coveredTerms[term] = true
				relevant = true
			}
		}

		if relevant {
			report.Hit = true
			if firstRelevantRank == 0 {
				firstRelevantRank = i + 1
			}
			relevanceByRank = append(relevanceByRank, 1)
		} else {
			relevanceByRank = append(relevanceByRank, 0)
		}
	}

	if firstRelevantRank > 0 {
		report.MRR = 1 / float64(firstRelevantRank)
	}
	if len(relevantIDs) > 0 {
		report.Recall = float64(relevantHits) / float64(len(relevantIDs))
	} else if len(mustContain) > 0 {
		report.Recall = float64(len(coveredTerms)) / float64(len(mustContain))
	} else if report.Hit {
		report.Recall = 1
	}
	report.NDCG = ndcg(relevanceByRank)
	report.Channels = uniqueStrings(report.Channels)
	return report
}

func ndcg(relevance []float64) float64 {
	if len(relevance) == 0 {
		return 0
	}
	dcg := 0.0
	idealRelevant := 0
	for i, value := range relevance {
		if value > 0 {
			idealRelevant++
		}
		dcg += value / math.Log2(float64(i)+2)
	}
	if idealRelevant == 0 {
		return 0
	}
	idcg := 0.0
	for i := 0; i < idealRelevant; i++ {
		idcg += 1 / math.Log2(float64(i)+2)
	}
	return dcg / idcg
}

func printReport(report evalReport) {
	fmt.Printf("RAG eval backend: %s\n", report.Backend)
	fmt.Printf("cases: %d\n", report.Cases)
	fmt.Printf("hit@k: %.3f\n", report.HitRate)
	fmt.Printf("recall@k: %.3f\n", report.AverageRecall)
	fmt.Printf("mrr@k: %.3f\n", report.AverageMRR)
	fmt.Printf("ndcg@k: %.3f\n", report.AverageNDCG)
	fmt.Printf("forbidden hits: %d\n", report.ForbiddenHits)
	fmt.Printf("avg latency: %.1f ms\n", report.AverageLatencyMS)
	for _, item := range report.CaseReports {
		fmt.Printf("- %s hit=%t recall=%.3f mrr=%.3f ndcg=%.3f forbidden=%d latency=%dms channels=%s\n",
			item.ID,
			item.Hit,
			item.Recall,
			item.MRR,
			item.NDCG,
			item.ForbiddenHits,
			item.LatencyMS,
			strings.Join(uniqueStrings(item.Channels), ","),
		)
	}
}

func toSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = normalizeText(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeText(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
