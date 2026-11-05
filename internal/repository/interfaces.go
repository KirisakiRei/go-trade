package repository

import (
	"context"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
)

type SetupRepository interface {
	Create(ctx context.Context, setup *model.SetupRecord) error
	FindByID(ctx context.Context, id string) (*model.SetupRecord, error)
	FindBySetupID(ctx context.Context, setupID string) (*model.SetupRecord, error)
	FindLatestBySymbol(ctx context.Context, symbol, timeframe string) (*model.SetupRecord, error)
	ListActive(ctx context.Context, symbol string, pagination dto.PaginationQuery) ([]model.SetupRecord, int64, error)
	UpdateState(ctx context.Context, setupID, state string) error
}

type JournalRepository interface {
	Create(ctx context.Context, journal *model.TradeJournal) error
	List(ctx context.Context, symbol string, pagination dto.PaginationQuery) ([]model.TradeJournal, int64, error)
}

type PositionRepository interface {
	Create(ctx context.Context, pos *model.Position) error
	FindByTicket(ctx context.Context, ticket string) (*model.Position, error)
	FindOpenBySymbol(ctx context.Context, symbol string) ([]model.Position, error)
	FindAllOpen(ctx context.Context) ([]model.Position, error)
	List(ctx context.Context, symbol, status string, p dto.PaginationQuery) ([]model.Position, int64, error)
	ClosePosition(ctx context.Context, ticket string, exitPrice, pnl, outcomeR float64, status, notes string) error
	CountTodayLosses(ctx context.Context) (int64, error)
}

type EvaluationRepository interface {
	Create(ctx context.Context, eval *model.TradeEvaluation) error
	FindByPositionID(ctx context.Context, posID string) (*model.TradeEvaluation, error)
	FindByTicket(ctx context.Context, ticket string) (*model.TradeEvaluation, error)
	FindUnnotified(ctx context.Context) ([]model.TradeEvaluation, error)
	MarkNotified(ctx context.Context, id string) error
	ListRecent(ctx context.Context, limit int) ([]model.TradeEvaluation, error)
	GetStats(ctx context.Context) (*dto.EvaluationStats, error)
}
