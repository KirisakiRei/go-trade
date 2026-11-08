package repository

import (
	"context"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
)

type JournalRepository interface {
	Create(ctx context.Context, journal *model.TradeJournal) error
	List(ctx context.Context, symbol string, pagination dto.PaginationQuery) ([]model.TradeJournal, int64, error)
}

type DailyTradeStatus struct {
	DateWIB       string `json:"date_wib"`
	TotalTrades   int    `json:"total_trades"`
	WinningTrades int    `json:"winning_trades"`
	LosingTrades  int    `json:"losing_trades"`
	OpenTrades    int    `json:"open_trades"`
	CanTrade      bool   `json:"can_trade"`
	LockReason    string `json:"lock_reason"`
}

type PositionRepository interface {
	Create(ctx context.Context, pos *model.Position) error
	FindByTicket(ctx context.Context, ticket string) (*model.Position, error)
	FindOpenBySymbol(ctx context.Context, symbol string) ([]model.Position, error)
	FindAllOpen(ctx context.Context) ([]model.Position, error)
	List(ctx context.Context, symbol, status string, p dto.PaginationQuery) ([]model.Position, int64, error)
	ClosePosition(ctx context.Context, ticket string, exitPrice, pnl, outcomeR float64, status, notes string) error
	CountTodayLosses(ctx context.Context) (int64, error)
	GetDailyTradeStatusWIB(ctx context.Context, now time.Time) (*DailyTradeStatus, error)
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
