package bootstrap

import (
	"net/http"
	"strings"

	"travel-agent-go/internal/agent"
	"travel-agent-go/internal/application"
	"travel-agent-go/internal/config"
	infraamap "travel-agent-go/internal/infrastructure/amap"
	infrallm "travel-agent-go/internal/infrastructure/llm"
	infrarag "travel-agent-go/internal/infrastructure/rag"
	infrastorage "travel-agent-go/internal/infrastructure/storage"
	"travel-agent-go/internal/logging"
	corerag "travel-agent-go/internal/rag"
	"travel-agent-go/internal/services"
	"travel-agent-go/internal/tools"
	transporthttp "travel-agent-go/internal/transport/http"
	"travel-agent-go/internal/validators"
)

// App 是启动阶段组装好的应用对象。
// main.go 只需要拿到 HTTPHandler 和配置即可，不关心内部怎么装配。
type App struct {
	Config      config.Config
	HTTPHandler http.Handler
}

func NewApp(cfg config.Config) *App {
	repository := infrastorage.NewJSONTripRepository(cfg.StorageFile)
	var retriever corerag.Retriever
	if strings.EqualFold(cfg.RAGBackend, "hybrid") {
		retriever = infrarag.NewHybridRetriever(cfg)
	} else if strings.EqualFold(cfg.RAGBackend, "qdrant") {
		retriever = infrarag.NewQdrantRetriever(cfg)
	} else {
		retriever = infrarag.NewMarkdownRetriever(cfg.DataDir)
	}
	logging.Info(nil, "rag retriever configured",
		"backend", cfg.RAGBackend,
		"data_dir", cfg.DataDir,
		"qdrant_collection", cfg.QdrantCollection,
		"embedding_provider", "ollama",
		"embedding_model", cfg.EmbeddingModel,
		"lexical_tokenizer", "unicode_han_bigram_trigram_ascii",
		"candidate_k", cfg.RAGCandidateK,
		"rrf_k", cfg.RAGRRFK,
		"query_variants", cfg.RAGQueryVariants,
		"max_context_chars", cfg.RAGMaxContextChars,
		"reranker_enabled", strings.TrimSpace(cfg.RAGRerankerURL) != "",
		"reranker_model", cfg.RAGRerankerModel,
	)
	llmClient := infrallm.NewOpenAICompatibleClient(cfg)

	var planner services.Planner
	if cfg.LLMAPIKey != "" {
		planner = services.NewLLMPlanner(cfg, llmClient)
	} else {
		planner = services.NewRulePlanner()
	}

	ragTool := tools.NewRAGTool(retriever)
	webResearchTool := tools.NewWebResearchTool(services.NewWebResearchService(cfg))
	plannerTool := tools.NewPlannerTool(planner)
	amapClient := infraamap.NewClient(cfg)
	mapTool := tools.NewMapTool(cfg, amapClient)
	weatherService := services.NewWeatherService(cfg, amapClient)
	routeService := services.NewRoutePlanningService(cfg, amapClient)
	routeTool := tools.NewRouteTool(routeService)
	assembler := services.NewItineraryAssembler()
	validatorSet := validators.NewDefaultSet()
	defaultAgent := agent.NewDefaultTravelPlanningAgent(
		ragTool,
		webResearchTool,
		plannerTool,
		mapTool,
		routeTool,
		weatherService,
		assembler,
		validatorSet,
	)
	toolCallingAgent := agent.NewToolCallingTravelPlanningAgent(
		cfg,
		llmClient,
		ragTool,
		webResearchTool,
		plannerTool,
		mapTool,
		routeTool,
		weatherService,
		assembler,
		validatorSet,
		defaultAgent,
	)
	multiAgent := agent.NewMultiAgentTravelPlanningAgent(
		ragTool,
		webResearchTool,
		plannerTool,
		mapTool,
		routeTool,
		weatherService,
		assembler,
		validatorSet,
		defaultAgent,
	)
	var travelAgent application.ItineraryGenerator
	switch strings.ToLower(strings.TrimSpace(cfg.AgentMode)) {
	case "multi", "multi-agent", "multi_agent":
		travelAgent = multiAgent
	case "default", "fixed", "rule":
		travelAgent = defaultAgent
	default:
		travelAgent = toolCallingAgent
	}
	logging.Info(nil, "travel agent configured",
		"mode", cfg.AgentMode,
		"selected", selectedAgentName(cfg.AgentMode),
	)

	tripEditor := services.NewTripService(
		planner,
		travelAgent,
		weatherService,
		mapTool,
		routeTool,
		validatorSet,
	)
	tripUsecase := application.NewTripUsecase(travelAgent, tripEditor, repository)
	weatherUsecase := application.NewWeatherUsecase(weatherService)
	router := transporthttp.NewRouter(tripUsecase, weatherUsecase)

	return &App{
		Config:      cfg,
		HTTPHandler: router.Routes(),
	}
}

func selectedAgentName(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "multi", "multi-agent", "multi_agent":
		return "multi_agent"
	case "default", "fixed", "rule":
		return "default_agent"
	default:
		return "tool_calling_agent"
	}
}
