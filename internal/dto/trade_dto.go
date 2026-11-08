package dto

import "time"

type TradeExecutionResult struct {
	Executed        bool      `json:"executed"`
	Strategy        string    `json:"strategy,omitempty"`
	Timeframe       string    `json:"timeframe,omitempty"`
	TicketNumber    string    `json:"ticket_number,omitempty"`
	Symbol          string    `json:"symbol"`
	Direction       string    `json:"direction,omitempty"`
	LotSize         float64   `json:"lot_size,omitempty"`
	OpenPrice       float64   `json:"open_price,omitempty"`
	StopLoss        float64   `json:"stop_loss,omitempty"`
	TakeProfit      float64   `json:"take_profit,omitempty"`
	RiskReward      float64   `json:"risk_reward,omitempty"`
	RiskAmount      float64   `json:"risk_amount,omitempty"`
	RejectionReason string    `json:"rejection_reason,omitempty"`
	SetupID         string    `json:"setup_id,omitempty"`
	ExecutedAt      time.Time `json:"executed_at"`
}

type PositionResponse struct {
	ID           string  `json:"id"`
	TicketNumber string  `json:"ticket_number"`
	Symbol       string  `json:"symbol"`
	Direction    string  `json:"direction"`
	OpenPrice    float64 `json:"open_price"`
	StopLoss     float64 `json:"stop_loss"`
	TakeProfit   float64 `json:"take_profit"`
	LotSize      float64 `json:"lot_size"`
	RiskReward   float64 `json:"risk_reward"`
	Status       string  `json:"status"`
	ExitPrice    float64 `json:"exit_price"`
	ProfitLoss   float64 `json:"profit_loss"`
	OutcomeR     float64 `json:"outcome_r"`
	OpenedAt     string  `json:"opened_at"`
	ClosedAt     *string `json:"closed_at,omitempty"`
	Notes        string  `json:"notes"`
}

type PendingSignal struct {
	ID         string    `json:"id"`
	Symbol     string    `json:"symbol"`
	Direction  string    `json:"direction"` // BUY or SELL
	Volume     float64   `json:"volume"`
	OpenPrice  float64   `json:"open_price"`
	StopLoss   float64   `json:"stop_loss"`
	TakeProfit float64   `json:"take_profit"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

type ConfirmSignalRequest struct {
	SignalID      string  `json:"signal_id" binding:"required"`
	TicketNumber  string  `json:"ticket_number" binding:"required"`
	ExecutedPrice float64 `json:"executed_price" binding:"required"`
}

type ILA2ExecuteRequest struct {
	Symbol    string  `json:"symbol" binding:"required"`
	HTF       string  `json:"htf,default=H4"`
	LTF       string  `json:"ltf,default=M15"`
	RiskRatio float64 `json:"risk_ratio,default=1.0"` // Risk % of equity
	MinRR     float64 `json:"min_rr,default=2.0"`     // Minimal 1:2 R:R
	Force     bool    `json:"force"`                  // Bypass daily loss lock for manual testing
}
