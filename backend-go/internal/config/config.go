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
	EnableAmapEnrich    bool
	AmapAPIKey          string
	AmapBaseURL         string
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	LLMTimeoutSeconds   int
	EnableWebResearch   bool
	WebSearchEndpoint   string
	WebSearchAPIKey     string
	WebResearchTimeout  int
	WebResearchMaxPages int
}

func Load() Config {
	loadDotEnv(".env")

	return Config{
		Port:                env("PORT", "8000"),
		DataDir:             env("DATA_DIR", "data"),
		StorageFile:         env("STORAGE_FILE", filepath.Join("data", "trips.json")),
		EnableAmapEnrich:    envBool("ENABLE_AMAP_ENRICHMENT", false),
		AmapAPIKey:          env("AMAP_API_KEY", ""),
		AmapBaseURL:         env("AMAP_BASE_URL", "https://restapi.amap.com/v3"),
		LLMAPIKey:           env("LLM_API_KEY", ""),
		LLMBaseURL:          env("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		LLMModel:            env("LLM_MODEL", "qwen-max"),
		LLMTimeoutSeconds:   envInt("LLM_TIMEOUT_SECONDS", 60),
		EnableWebResearch:   envBool("ENABLE_WEB_RESEARCH", false),
		WebSearchEndpoint:   env("WEB_SEARCH_ENDPOINT", ""),
		WebSearchAPIKey:     env("WEB_SEARCH_API_KEY", ""),
		WebResearchTimeout:  envInt("WEB_RESEARCH_TIMEOUT_SECONDS", 20),
		WebResearchMaxPages: envInt("WEB_RESEARCH_MAX_PAGES", 3),
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
