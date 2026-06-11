package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 集中管理应用配置。
// 这是 Go 项目常见的“配置对象”模式：启动时读环境变量，之后通过依赖注入传给服务。
type Config struct {
	Port                string
	DataDir             string
	StorageFile         string
	AgentMode           string
	EnableAmapEnrich    bool
	EnableAmapWeather   bool
	EnableAmapRouting   bool
	AmapAPIKey          string
	AmapBaseURL         string
	AmapBaseV3URL       string
	AmapBaseV5URL       string
	AmapTimeoutSeconds  int
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	LLMTimeoutSeconds   int
	EnableWebResearch   bool
	WebSearchProvider   string
	WebSearchEndpoint   string
	WebSearchAPIKey     string
	WebResearchTimeout  int
	WebResearchMaxPages int
	RAGBackend          string
	RAGCandidateK       int
	RAGRRFK             int
	RAGQueryVariants    int
	RAGMaxContextChars  int
	RAGRerankerURL      string
	RAGRerankerModel    string
	RAGRerankerTimeout  int
	QdrantURL           string
	QdrantCollection    string
	EmbeddingBaseURL    string
	EmbeddingModel      string
	EmbeddingDim        int
}

func Load() Config {
	loadDotEnv(".env")
	amapBaseV3 := env("AMAP_BASE_V3_URL", env("AMAP_BASE_URL", "https://restapi.amap.com/v3"))

	return Config{
		Port:                env("PORT", "8000"),
		DataDir:             env("DATA_DIR", filepath.Join("data", "guides")),
		StorageFile:         env("STORAGE_FILE", filepath.Join("data", "trips.json")),
		AgentMode:           env("AGENT_MODE", "tool"),
		EnableAmapEnrich:    envBool("ENABLE_AMAP_ENRICHMENT", false),
		EnableAmapWeather:   envBool("ENABLE_AMAP_WEATHER", false),
		EnableAmapRouting:   envBool("ENABLE_AMAP_ROUTING", false),
		AmapAPIKey:          env("AMAP_API_KEY", ""),
		AmapBaseURL:         amapBaseV3,
		AmapBaseV3URL:       amapBaseV3,
		AmapBaseV5URL:       env("AMAP_BASE_V5_URL", "https://restapi.amap.com/v5"),
		AmapTimeoutSeconds:  envInt("AMAP_TIMEOUT_SECONDS", 15),
		LLMAPIKey:           env("LLM_API_KEY", ""),
		LLMBaseURL:          env("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		LLMModel:            env("LLM_MODEL", "qwen-max"),
		LLMTimeoutSeconds:   envInt("LLM_TIMEOUT_SECONDS", 60),
		EnableWebResearch:   envBool("ENABLE_WEB_RESEARCH", false),
		WebSearchProvider:   env("WEB_SEARCH_PROVIDER", ""),
		WebSearchEndpoint:   env("WEB_SEARCH_ENDPOINT", ""),
		WebSearchAPIKey:     env("WEB_SEARCH_API_KEY", ""),
		WebResearchTimeout:  envInt("WEB_RESEARCH_TIMEOUT_SECONDS", 20),
		WebResearchMaxPages: envInt("WEB_RESEARCH_MAX_PAGES", 3),
		RAGBackend:          env("RAG_BACKEND", "markdown"),
		RAGCandidateK:       envInt("RAG_CANDIDATE_K", 40),
		RAGRRFK:             envInt("RAG_RRF_K", 60),
		RAGQueryVariants:    envInt("RAG_QUERY_VARIANTS", 3),
		RAGMaxContextChars:  envInt("RAG_MAX_CONTEXT_CHARS", 6000),
		RAGRerankerURL:      env("RAG_RERANKER_URL", ""),
		RAGRerankerModel:    env("RAG_RERANKER_MODEL", "bge-reranker-v2-m3"),
		RAGRerankerTimeout:  envInt("RAG_RERANKER_TIMEOUT_SECONDS", 30),
		QdrantURL:           env("QDRANT_URL", "http://127.0.0.1:6333"),
		QdrantCollection:    env("QDRANT_COLLECTION", "travel_guides"),
		EmbeddingBaseURL:    env("EMBEDDING_BASE_URL", "http://127.0.0.1:11434"),
		EmbeddingModel:      env("EMBEDDING_MODEL", "bge-m3"),
		EmbeddingDim:        envInt("EMBEDDING_DIM", 1024),
	}
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
