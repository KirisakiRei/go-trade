package router

import (
	"context"
	"time"

	"ila-engine/internal/config"
	"ila-engine/internal/handler"
	"ila-engine/internal/middleware"
	"ila-engine/internal/repository"
	"ila-engine/internal/service"
	"ila-engine/pkg/broker"
	"ila-engine/pkg/marketfeed"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AppComponents struct {
	Engine           *gin.Engine
	ExecutionService service.ExecutionService
}

func SetupRouter(cfg *config.Config, db *gorm.DB) *AppComponents {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global Middlewares
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS(cfg.AllowedOrigins))

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "app": "ila-engine"})
	})

	// Dependencies
	feedProvider := marketfeed.NewMultiFeedProvider()
	journalRepo := repository.NewJournalRepository(db)
	positionRepo := repository.NewPositionRepository(db)
	evalRepo := repository.NewEvaluationRepository(db)
	cTraderBroker, _ := broker.NewCTraderBroker(broker.CTraderConfig{})
	mtrBroker := broker.NewMatchTraderBroker("")

	// Multi-Broker: Dual execution to both cTrader and Match-Trader simultaneously
	var activeBroker broker.Broker = broker.NewMultiBrokerManager(mtrBroker, cTraderBroker)

	ilaService := service.NewILAService(feedProvider)
	executionService := service.NewExecutionService(nil, ilaService, positionRepo, journalRepo, activeBroker, feedProvider)
	evalService := service.NewEvaluationService(evalRepo, positionRepo, feedProvider)

	// Backfill evaluations for any previously closed trades on startup
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = evalService.EvaluateAllClosedPositions(ctx)
	}()

	analysisHandler := handler.NewAnalysisHandler(nil, ilaService)
	tradeHandler := handler.NewTradeHandler(executionService, evalService)

	// API Routes (Murni ILA)
	api := r.Group("/api/v1")
	{
		api.GET("/analysis/ila", analysisHandler.AnalyzeILA)

		trade := api.Group("/trade")
		{
			trade.POST("/execute/ila", tradeHandler.ExecuteILA)
			trade.GET("/positions", tradeHandler.ListPositions)
			trade.GET("/account", tradeHandler.GetAccount)
			trade.GET("/evaluations", tradeHandler.ListEvaluations)
			trade.GET("/evaluations/stats", tradeHandler.GetEvaluationStats)
		}
	}

	return &AppComponents{
		Engine:           r,
		ExecutionService: executionService,
	}
}
