package broker

import (
	"context"
	"time"
)

type AccountInfo struct {
	BrokerName   string  `json:"broker_name"`
	AccountNumber string `json:"account_number"`
	Currency     string  `json:"currency"`
	Balance      float64 `json:"balance"`
	Equity       float64 `json:"equity"`
	MarginUsed   float64 `json:"margin_used"`
	FreeMargin   float64 `json:"free_margin"`
}

type OrderRequest struct {
	Symbol     string  `json:"symbol"`
	Direction  string  `json:"direction"` // BUY or SELL
	OpenPrice  float64 `json:"open_price"`
	StopLoss   float64 `json:"stop_loss"`
	TakeProfit float64 `json:"take_profit"`
	LotSize    float64 `json:"lot_size"`
	Comment    string  `json:"comment"`
	SetupID    string  `json:"setup_id"`
}

type OrderResponse struct {
	TicketNumber string    `json:"ticket_number"`
	Symbol       string    `json:"symbol"`
	Direction    string    `json:"direction"`
	ExecutedPrice float64  `json:"executed_price"`
	StopLoss     float64   `json:"stop_loss"`
	TakeProfit   float64   `json:"take_profit"`
	LotSize      float64   `json:"lot_size"`
	ExecutedAt   time.Time `json:"executed_at"`
	Status       string    `json:"status"`
}

type PositionInfo struct {
	TicketNumber string    `json:"ticket_number"`
	Symbol       string    `json:"symbol"`
	Direction    string    `json:"direction"`
	OpenPrice    float64   `json:"open_price"`
	CurrentPrice float64   `json:"current_price"`
	StopLoss     float64   `json:"stop_loss"`
	TakeProfit   float64   `json:"take_profit"`
	LotSize      float64   `json:"lot_size"`
	FloatingPnL  float64   `json:"floating_pnl"`
	OpenedAt     time.Time `json:"opened_at"`
}

type Broker interface {
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error)
	CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error
	GetOpenPositions(ctx context.Context) ([]PositionInfo, error)
	Name() string
}
