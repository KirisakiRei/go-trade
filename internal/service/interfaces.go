package service

import (
	"context"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/pkg/ila"
)

type BBMAService interface {
	Analyze(ctx context.Context, req dto.AnalysisRequest) (*dto.AnalysisResponse, error)
	AnalyzeMTF(ctx context.Context, req dto.MTFAnalysisRequest) (*dto.MTFAnalysisResponse, error)
	ListActiveSetups(ctx context.Context, symbol string, p dto.PaginationQuery) ([]model.SetupRecord, dto.PaginationMeta, error)
}

type MarketService interface {
	GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]model.MarketCandle, error)
}

type MonitorService interface {
	StartMonitoring(ctx context.Context, symbol, contextTF, entryTF string) error
	StopMonitoring()
	GetStatus() dto.MonitorStatusResponse
	SetAlertCallback(cb func(setup *dto.AnalysisResponse))
}

type ExecutionService interface {
	EvaluateAndExecute(ctx context.Context, req dto.TradeExecutionRequest) (*dto.TradeExecutionResult, error)
	EvaluateAndExecuteILA(ctx context.Context, req dto.ILA2ExecuteRequest) (*dto.TradeExecutionResult, error)
	GetOpenPositions(ctx context.Context) ([]dto.PositionResponse, error)
	ListPositions(ctx context.Context, symbol, status string, p dto.PaginationQuery) ([]dto.PositionResponse, dto.PaginationMeta, error)
	CheckAndManagePositions(ctx context.Context) error
	GetAccount(ctx context.Context) (any, error)
	GetPendingSignals() []*dto.PendingSignal
	ConfirmSignal(ctx context.Context, req dto.ConfirmSignalRequest) error
}

type EvaluationService interface {
	EvaluateClosedTrade(ctx context.Context, pos *model.Position) (*model.TradeEvaluation, error)
	EvaluateAllClosedPositions(ctx context.Context) ([]*model.TradeEvaluation, error)
	GetUnnotifiedEvaluations(ctx context.Context) ([]model.TradeEvaluation, error)
	MarkNotified(ctx context.Context, id string) error
	GetEvaluationSummary(ctx context.Context) (*dto.EvaluationStats, error)
	ListRecentEvaluations(ctx context.Context, limit int) ([]dto.TradeEvaluationDTO, error)
	PreFlightCheck(ctx context.Context, symbol, direction, setupType string, slDist float64) (*dto.PreFlightCheckResult, error)
}

type ILA2Service interface {
	AnalyzeILA2(ctx context.Context, req dto.ILA2AnalysisRequest) (*ila.ILASetup, error)
	AnalyzeAllPairs(ctx context.Context, symbol string) (*ila.ILAMultiResponse, error)
}
