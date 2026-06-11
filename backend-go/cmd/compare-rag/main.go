package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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

type ragProfile struct {
	Name                   string `json:"name"`
	Description            string `json:"description,omitempty"`
	Backend                string `json:"backend"`
	CandidateK             *int   `json:"candidate_k,omitempty"`
	RRFK                   *int   `json:"rrf_k,omitempty"`
	QueryVariants          *int   `json:"query_variants,omitempty"`
	MaxContextChars        *int   `json:"max_context_chars,omitempty"`
	QdrantCollection       string `json:"qdrant_collection,omitempty"`
	EmbeddingBaseURL       string `json:"embedding_base_url,omitempty"`
	EmbeddingModel         string `json:"embedding_model,omitempty"`
	EmbeddingDim           *int   `json:"embedding_dim,omitempty"`
	RerankerURL            string `json:"reranker_url,omitempty"`
	RerankerModel          string `json:"reranker_model,omitempty"`
	RerankerTimeoutSeconds *int   `json:"reranker_timeout_seconds,omitempty"`
	DisableReranker        bool   `json:"disable_reranker,omitempty"`
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
	Reranked      bool     `json:"reranked"`
	Error         string   `json:"error,omitempty"`
}

type profileReport struct {
	Name                  string       `json:"name"`
	Description           string       `json:"description,omitempty"`
	Backend               string       `json:"backend"`
	QdrantCollection      string       `json:"qdrant_collection,omitempty"`
	EmbeddingModel        string       `json:"embedding_model,omitempty"`
	EmbeddingDim          int          `json:"embedding_dim,omitempty"`
	RerankerModel         string       `json:"reranker_model,omitempty"`
	RerankerEnabled       bool         `json:"reranker_enabled"`
	RerankedCases         int          `json:"reranked_cases"`
	CandidateK            int          `json:"candidate_k"`
	RRFK                  int          `json:"rrf_k"`
	QueryVariants         int          `json:"query_variants"`
	Cases                 int          `json:"cases"`
	FailedCases           int          `json:"failed_cases"`
	HitRate               float64      `json:"hit_rate"`
	AverageRecall         float64      `json:"average_recall"`
	AverageMRR            float64      `json:"average_mrr"`
	AverageNDCG           float64      `json:"average_ndcg"`
	ForbiddenHits         int          `json:"forbidden_hits"`
	AverageLatencyMS      float64      `json:"average_latency_ms"`
	QualityScore          float64      `json:"quality_score"`
	QualityScoreDelta     float64      `json:"quality_score_delta"`
	AverageLatencyDeltaMS float64      `json:"average_latency_delta_ms"`
	AverageRecallDelta    float64      `json:"average_recall_delta"`
	AverageMRRDelta       float64      `json:"average_mrr_delta"`
	AverageNDCGDelta      float64      `json:"average_ndcg_delta"`
	ForbiddenHitsDelta    int          `json:"forbidden_hits_delta"`
	CaseReports           []caseReport `json:"case_reports"`
	ProfileError          string       `json:"profile_error,omitempty"`
}

type compareReport struct {
	Cases       int             `json:"cases"`
	Baseline    string          `json:"baseline"`
	Profiles    []profileReport `json:"profiles"`
	RankedBy    string          `json:"ranked_by"`
	GeneratedAt string          `json:"generated_at"`
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
	profilesPath := flag.String("profiles", "data/eval/rag_profiles.json", "JSON file with RAG comparison profiles")
	topK := flag.Int("top-k", 5, "default retrieval cutoff")
	outputPath := flag.String("output", "", "optional JSON comparison report output path")
	markdownOutputPath := flag.String("markdown-output", "", "optional Chinese Markdown comparison report output path")
	flag.Parse()

	baseCfg := config.Load()
	cases, err := loadCases(*casesPath)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("no eval cases loaded from %s", *casesPath)
	}
	profiles, err := loadProfiles(*profilesPath)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no rag profiles loaded from %s", *profilesPath)
	}

	logging.Info(nil, "rag comparison started",
		"cases_path", *casesPath,
		"profiles_path", *profilesPath,
		"profiles", len(profiles),
		"cases", len(cases),
		"default_top_k", *topK,
	)

	report := compareReport{
		Cases:       len(cases),
		Baseline:    profiles[0].Name,
		Profiles:    make([]profileReport, 0, len(profiles)),
		RankedBy:    "quality_score desc, latency asc",
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	for _, profile := range profiles {
		profileReport := evaluateProfile(baseCfg, profile, cases, *topK)
		report.Profiles = append(report.Profiles, profileReport)
		logging.Info(nil, "rag comparison profile completed",
			"profile", profileReport.Name,
			"backend", profileReport.Backend,
			"qdrant_collection", profileReport.QdrantCollection,
			"embedding_model", profileReport.EmbeddingModel,
			"embedding_dim", profileReport.EmbeddingDim,
			"reranker_enabled", profileReport.RerankerEnabled,
			"reranker_model", profileReport.RerankerModel,
			"reranked_cases", profileReport.RerankedCases,
			"cases", profileReport.Cases,
			"failed_cases", profileReport.FailedCases,
			"hit_rate", profileReport.HitRate,
			"average_recall", profileReport.AverageRecall,
			"average_mrr", profileReport.AverageMRR,
			"average_ndcg", profileReport.AverageNDCG,
			"quality_score", profileReport.QualityScore,
			"average_latency_ms", profileReport.AverageLatencyMS,
			"profile_error", profileReport.ProfileError,
		)
	}

	applyDeltas(&report)
	printComparison(report)

	if strings.TrimSpace(*outputPath) != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileEnsuringDir(*outputPath, data, 0o644); err != nil {
			return err
		}
	}

	if strings.TrimSpace(*markdownOutputPath) != "" {
		data := []byte(renderMarkdownReport(report, *casesPath, *profilesPath, *topK))
		if err := writeFileEnsuringDir(*markdownOutputPath, data, 0o644); err != nil {
			return err
		}
	}

	logging.Info(nil, "rag comparison completed",
		"profiles", len(report.Profiles),
		"baseline", report.Baseline,
		"json_output", strings.TrimSpace(*outputPath),
		"markdown_output", strings.TrimSpace(*markdownOutputPath),
	)
	return nil
}

func evaluateProfile(baseCfg config.Config, profile ragProfile, cases []evalCase, defaultTopK int) profileReport {
	cfg := applyProfile(baseCfg, profile)
	report := profileReport{
		Name:             profile.Name,
		Description:      profile.Description,
		Backend:          cfg.RAGBackend,
		RerankerEnabled:  usesRerankerBackend(cfg.RAGBackend) && strings.TrimSpace(cfg.RAGRerankerURL) != "",
		CandidateK:       cfg.RAGCandidateK,
		RRFK:             cfg.RAGRRFK,
		QueryVariants:    cfg.RAGQueryVariants,
		Cases:            len(cases),
		CaseReports:      make([]caseReport, 0, len(cases)),
		AverageLatencyMS: 0,
	}
	if usesDenseBackend(report.Backend) {
		report.QdrantCollection = cfg.QdrantCollection
		report.EmbeddingModel = cfg.EmbeddingModel
		report.EmbeddingDim = cfg.EmbeddingDim
	}
	if report.RerankerEnabled {
		report.RerankerModel = cfg.RAGRerankerModel
	}

	if strings.TrimSpace(report.Name) == "" {
		report.Name = cfg.RAGBackend
	}

	retriever := buildRetriever(cfg)
	var totalLatency int64
	successCases := 0

	for _, item := range cases {
		if item.TopK <= 0 {
			item.TopK = defaultTopK
		}

		query := corerag.NewQuery(item.Destination, item.Preferences, item.Pace, item.SpecialNotes, item.TopK).WithText(item.Query)
		start := time.Now()
		results, err := retriever.RetrieveDetailed(query)
		latency := time.Since(start).Milliseconds()
		totalLatency += latency

		caseResult := caseReport{ID: item.ID, LatencyMS: latency}
		if err != nil {
			caseResult.Error = err.Error()
			report.FailedCases++
			report.CaseReports = append(report.CaseReports, caseResult)
			continue
		}

		caseResult = evaluateCase(item, results, item.TopK)
		caseResult.LatencyMS = latency
		report.CaseReports = append(report.CaseReports, caseResult)
		successCases++

		if caseResult.Hit {
			report.HitRate += 1
		}
		report.AverageRecall += caseResult.Recall
		report.AverageMRR += caseResult.MRR
		report.AverageNDCG += caseResult.NDCG
		report.ForbiddenHits += caseResult.ForbiddenHits
		if caseResult.Reranked {
			report.RerankedCases++
		}
	}

	if successCases == 0 {
		report.ProfileError = "all cases failed"
		report.AverageLatencyMS = float64(totalLatency) / float64(maxInt(1, len(cases)))
		return report
	}

	report.HitRate /= float64(successCases)
	report.AverageRecall /= float64(successCases)
	report.AverageMRR /= float64(successCases)
	report.AverageNDCG /= float64(successCases)
	report.AverageLatencyMS = float64(totalLatency) / float64(len(cases))
	report.QualityScore = qualityScore(report)
	return report
}

func applyProfile(base config.Config, profile ragProfile) config.Config {
	cfg := base
	if strings.TrimSpace(profile.Backend) != "" {
		cfg.RAGBackend = strings.TrimSpace(profile.Backend)
	}
	if profile.CandidateK != nil {
		cfg.RAGCandidateK = *profile.CandidateK
	}
	if profile.RRFK != nil {
		cfg.RAGRRFK = *profile.RRFK
	}
	if profile.QueryVariants != nil {
		cfg.RAGQueryVariants = *profile.QueryVariants
	}
	if profile.MaxContextChars != nil {
		cfg.RAGMaxContextChars = *profile.MaxContextChars
	}
	if strings.TrimSpace(profile.QdrantCollection) != "" {
		cfg.QdrantCollection = strings.TrimSpace(profile.QdrantCollection)
	}
	if strings.TrimSpace(profile.EmbeddingBaseURL) != "" {
		cfg.EmbeddingBaseURL = strings.TrimSpace(profile.EmbeddingBaseURL)
	}
	if strings.TrimSpace(profile.EmbeddingModel) != "" {
		cfg.EmbeddingModel = strings.TrimSpace(profile.EmbeddingModel)
	}
	if profile.EmbeddingDim != nil {
		cfg.EmbeddingDim = *profile.EmbeddingDim
	}
	if strings.TrimSpace(profile.RerankerURL) != "" {
		cfg.RAGRerankerURL = strings.TrimSpace(profile.RerankerURL)
	}
	if strings.TrimSpace(profile.RerankerModel) != "" {
		cfg.RAGRerankerModel = strings.TrimSpace(profile.RerankerModel)
	}
	if profile.RerankerTimeoutSeconds != nil {
		cfg.RAGRerankerTimeout = *profile.RerankerTimeoutSeconds
	}
	if profile.DisableReranker {
		cfg.RAGRerankerURL = ""
	}
	return cfg
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

func usesDenseBackend(backend string) bool {
	return strings.EqualFold(backend, "qdrant") || strings.EqualFold(backend, "hybrid")
}

func usesRerankerBackend(backend string) bool {
	return strings.EqualFold(backend, "hybrid")
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

func loadProfiles(path string) ([]ragProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Profiles []ragProfile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Profiles) > 0 {
		return normalizeProfiles(wrapped.Profiles), nil
	}
	var profiles []ragProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return normalizeProfiles(profiles), nil
}

func normalizeProfiles(profiles []ragProfile) []ragProfile {
	for i := range profiles {
		if strings.TrimSpace(profiles[i].Name) == "" {
			profiles[i].Name = fmt.Sprintf("profile_%d", i+1)
		}
	}
	return profiles
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
	report.Reranked = containsString(report.Channels, "rerank")
	return report
}

func applyDeltas(report *compareReport) {
	if len(report.Profiles) == 0 {
		return
	}
	baseline := report.Profiles[0]
	for i := range report.Profiles {
		report.Profiles[i].QualityScoreDelta = report.Profiles[i].QualityScore - baseline.QualityScore
		report.Profiles[i].AverageLatencyDeltaMS = report.Profiles[i].AverageLatencyMS - baseline.AverageLatencyMS
		report.Profiles[i].AverageRecallDelta = report.Profiles[i].AverageRecall - baseline.AverageRecall
		report.Profiles[i].AverageMRRDelta = report.Profiles[i].AverageMRR - baseline.AverageMRR
		report.Profiles[i].AverageNDCGDelta = report.Profiles[i].AverageNDCG - baseline.AverageNDCG
		report.Profiles[i].ForbiddenHitsDelta = report.Profiles[i].ForbiddenHits - baseline.ForbiddenHits
	}
}

func qualityScore(report profileReport) float64 {
	// The score favors ranking quality first, then penalizes forbidden hits and failed cases.
	score := 0.4*report.AverageRecall + 0.3*report.AverageNDCG + 0.2*report.AverageMRR + 0.1*report.HitRate
	score -= float64(report.ForbiddenHits) * 0.02
	score -= float64(report.FailedCases) * 0.05
	if score < 0 {
		return 0
	}
	return score
}

func printComparison(report compareReport) {
	rows := append([]profileReport(nil), report.Profiles...)
	sortProfileReports(rows)

	fmt.Printf("RAG comparison report: baseline=%s cases=%d\n", report.Baseline, report.Cases)
	fmt.Println("rank\tprofile\tbackend\tcollection\tembedding\tdim\treranker\treranked_cases\tquality\tdelta_quality\trecall\tmrr\tndcg\tlatency_ms\tfailed")
	for i, row := range rows {
		reranker := "disabled"
		if row.RerankerEnabled {
			reranker = row.RerankerModel
		}
		fmt.Printf("%d\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%.3f\t%+.3f\t%.3f\t%.3f\t%.3f\t%.1f\t%d\n",
			i+1,
			row.Name,
			row.Backend,
			row.QdrantCollection,
			row.EmbeddingModel,
			row.EmbeddingDim,
			reranker,
			row.RerankedCases,
			row.QualityScore,
			row.QualityScoreDelta,
			row.AverageRecall,
			row.AverageMRR,
			row.AverageNDCG,
			row.AverageLatencyMS,
			row.FailedCases,
		)
	}
}

func renderMarkdownReport(report compareReport, casesPath string, profilesPath string, topK int) string {
	rows := append([]profileReport(nil), report.Profiles...)
	sortProfileReports(rows)

	var b strings.Builder
	b.WriteString("# RAG 效果对比实验报告\n\n")
	b.WriteString("## 实验配置\n\n")
	b.WriteString(fmt.Sprintf("- 生成时间：%s\n", report.GeneratedAt))
	b.WriteString(fmt.Sprintf("- 评测样例：`%s`\n", casesPath))
	b.WriteString(fmt.Sprintf("- Profile 配置：`%s`\n", profilesPath))
	b.WriteString(fmt.Sprintf("- Top K：%d\n", topK))
	b.WriteString(fmt.Sprintf("- Baseline：`%s`\n", report.Baseline))
	b.WriteString(fmt.Sprintf("- Profile 数量：%d\n", len(report.Profiles)))
	b.WriteString(fmt.Sprintf("- Case 数量：%d\n\n", report.Cases))

	b.WriteString("## 汇总结果\n\n")
	b.WriteString("| 排名 | Profile | Backend | Collection | Embedding | Reranker | 实际重排样例 | Quality | ΔQuality | Recall | MRR | nDCG | 平均延迟(ms) | 失败样例 |\n")
	b.WriteString("|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for i, row := range rows {
		b.WriteString(fmt.Sprintf(
			"| %d | `%s` | `%s` | %s | %s | %s | %d | %.3f | %+.3f | %.3f | %.3f | %.3f | %.1f | %d |\n",
			i+1,
			escapeMarkdownCell(row.Name),
			escapeMarkdownCell(row.Backend),
			markdownCodeOrDash(row.QdrantCollection),
			markdownEmbedding(row),
			markdownReranker(row),
			row.RerankedCases,
			row.QualityScore,
			row.QualityScoreDelta,
			row.AverageRecall,
			row.AverageMRR,
			row.AverageNDCG,
			row.AverageLatencyMS,
			row.FailedCases,
		))
	}
	b.WriteString("\n")

	b.WriteString("## 关键观察\n\n")
	if len(rows) > 0 {
		best := rows[0]
		b.WriteString(fmt.Sprintf("- 当前综合分最高的是 `%s`，quality_score=%.3f，平均延迟 %.1f ms。\n", best.Name, best.QualityScore, best.AverageLatencyMS))
	}
	if baseline, ok := findProfile(report.Profiles, report.Baseline); ok {
		b.WriteString(fmt.Sprintf("- Baseline `%s` 的 quality_score=%.3f，后续 profile 的 ΔQuality 都是相对它计算。\n", baseline.Name, baseline.QualityScore))
	}
	for _, row := range rows {
		if row.RerankerEnabled && row.RerankedCases == 0 {
			b.WriteString(fmt.Sprintf("- `%s` 配置了 reranker `%s`，但 `reranked_cases=0`，通常说明 reranker 服务没有启动、请求失败，或返回结果没有进入 `rerank` 通道。\n", row.Name, row.RerankerModel))
		}
		if row.FailedCases > 0 {
			b.WriteString(fmt.Sprintf("- `%s` 有 %d 个失败样例，需要优先查看逐 case 错误信息。\n", row.Name, row.FailedCases))
		}
	}
	b.WriteString("\n")

	for _, row := range rows {
		b.WriteString(fmt.Sprintf("## Profile：%s\n\n", row.Name))
		if strings.TrimSpace(row.Description) != "" {
			b.WriteString(fmt.Sprintf("- 说明：%s\n", row.Description))
		}
		b.WriteString(fmt.Sprintf("- Backend：`%s`\n", row.Backend))
		if strings.TrimSpace(row.QdrantCollection) != "" {
			b.WriteString(fmt.Sprintf("- Qdrant Collection：`%s`\n", row.QdrantCollection))
		}
		if strings.TrimSpace(row.EmbeddingModel) != "" {
			b.WriteString(fmt.Sprintf("- Embedding：`%s`，维度：%d\n", row.EmbeddingModel, row.EmbeddingDim))
		}
		b.WriteString(fmt.Sprintf("- Reranker：%s，实际重排样例：%d/%d\n", markdownReranker(row), row.RerankedCases, row.Cases))
		b.WriteString(fmt.Sprintf("- 指标：hit_rate=%.3f，recall=%.3f，mrr=%.3f，ndcg=%.3f，quality=%.3f，平均延迟=%.1f ms\n\n",
			row.HitRate,
			row.AverageRecall,
			row.AverageMRR,
			row.AverageNDCG,
			row.QualityScore,
			row.AverageLatencyMS,
		))
		b.WriteString("| Case | Hit | Recall | MRR | nDCG | 延迟(ms) | 通道 | Top Sources | 错误 |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---|---|---|\n")
		for _, item := range row.CaseReports {
			b.WriteString(fmt.Sprintf(
				"| `%s` | %t | %.3f | %.3f | %.3f | %d | %s | %s | %s |\n",
				escapeMarkdownCell(item.ID),
				item.Hit,
				item.Recall,
				item.MRR,
				item.NDCG,
				item.LatencyMS,
				markdownListOrDash(item.Channels),
				markdownListOrDash(uniqueStrings(item.ResultSources)),
				markdownTextOrDash(item.Error),
			))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func sortProfileReports(rows []profileReport) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].QualityScore == rows[j].QualityScore {
			return rows[i].AverageLatencyMS < rows[j].AverageLatencyMS
		}
		return rows[i].QualityScore > rows[j].QualityScore
	})
}

func findProfile(profiles []profileReport, name string) (profileReport, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return profileReport{}, false
}

func markdownEmbedding(row profileReport) string {
	if strings.TrimSpace(row.EmbeddingModel) == "" {
		return "-"
	}
	if row.EmbeddingDim > 0 {
		return fmt.Sprintf("`%s/%d`", escapeMarkdownCell(row.EmbeddingModel), row.EmbeddingDim)
	}
	return markdownCodeOrDash(row.EmbeddingModel)
}

func markdownReranker(row profileReport) string {
	if !row.RerankerEnabled {
		return "未启用"
	}
	if strings.TrimSpace(row.RerankerModel) == "" {
		return "已启用"
	}
	return fmt.Sprintf("`%s`", escapeMarkdownCell(row.RerankerModel))
}

func markdownCodeOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return fmt.Sprintf("`%s`", escapeMarkdownCell(value))
}

func markdownListOrDash(values []string) string {
	values = uniqueStrings(values)
	if len(values) == 0 {
		return "-"
	}
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		escaped = append(escaped, escapeMarkdownCell(value))
	}
	return strings.Join(escaped, "<br>")
}

func markdownTextOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return escapeMarkdownCell(value)
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

func writeFileEnsuringDir(path string, data []byte, perm os.FileMode) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, perm)
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
