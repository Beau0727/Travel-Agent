package bootstrap

import (
	"net/http"

	"zhilv-yuntu-go/internal/agent"
	"zhilv-yuntu-go/internal/application"
	"zhilv-yuntu-go/internal/config"
	infrarag "zhilv-yuntu-go/internal/infrastructure/rag"
	infrastorage "zhilv-yuntu-go/internal/infrastructure/storage"
	"zhilv-yuntu-go/internal/services"
	"zhilv-yuntu-go/internal/tools"
	transporthttp "zhilv-yuntu-go/internal/transport/http"
	"zhilv-yuntu-go/internal/validators"
)

// App 是启动阶段组装好的应用对象。
// main.go 只需要拿到 HTTPHandler 和配置即可，不关心内部怎么装配。
type App struct {
	Config      config.Config
	HTTPHandler http.Handler
}

func NewApp(cfg config.Config) *App {
	repository := infrastorage.NewJSONTripRepository(cfg.StorageFile)
	retriever := infrarag.NewMarkdownRetriever(cfg.DataDir)

	var planner services.Planner
	if cfg.LLMAPIKey != "" {
		planner = services.NewLLMPlanner(cfg)
	} else {
		planner = services.NewRulePlanner()
	}

	ragTool := tools.NewRAGTool(retriever)
	webResearchTool := tools.NewWebResearchTool(services.NewWebResearchService(cfg))
	plannerTool := tools.NewPlannerTool(planner)
	mapTool := tools.NewMapTool(cfg)
	assembler := services.NewItineraryAssembler()
	validatorSet := validators.NewDefaultSet()
	defaultAgent := agent.NewDefaultTravelPlanningAgent(ragTool, plannerTool, mapTool, assembler, validatorSet)
	weatherService := services.NewWeatherService()
	travelAgent := agent.NewToolCallingTravelPlanningAgent(
		cfg,
		ragTool,
		webResearchTool,
		plannerTool,
		mapTool,
		weatherService,
		assembler,
		validatorSet,
		defaultAgent,
	)

	tripEditor := services.NewTripService(planner, travelAgent)
	tripUsecase := application.NewTripUsecase(travelAgent, tripEditor, repository)
	weatherUsecase := application.NewWeatherUsecase(weatherService)
	router := transporthttp.NewRouter(tripUsecase, weatherUsecase)

	return &App{
		Config:      cfg,
		HTTPHandler: router.Routes(),
	}
}
