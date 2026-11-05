package model

import (
	"time"
)

// Position represents an active or historical trading position.
type Position struct {
	Base
	TicketNumber string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"ticket_number"`
	SetupID      string     `gorm:"type:varchar(64);index;not null" json:"setup_id"`
	Symbol       string     `gorm:"type:varchar(20);index;not null" json:"symbol"`
	Timeframe    string     `gorm:"type:varchar(10);not null" json:"timeframe"`
	Direction    string     `gorm:"type:varchar(10);not null" json:"direction"` // BUY or SELL
	OpenPrice    float64    `gorm:"type:decimal(16,5);not null" json:"open_price"`
	StopLoss     float64    `gorm:"type:decimal(16,5);not null" json:"stop_loss"`
	TakeProfit   float64    `gorm:"type:decimal(16,5);not null" json:"take_profit"`
	LotSize      float64    `gorm:"type:decimal(8,2);not null" json:"lot_size"`
	RiskReward   float64    `gorm:"type:decimal(6,2);not null" json:"risk_reward"`
	Status       string     `gorm:"type:varchar(30);index;not null" json:"status"` // OPEN, CLOSED_TP, CLOSED_SL, CLOSED_INVALIDATED, CLOSED_MANUAL
	ExitPrice    float64    `gorm:"type:decimal(16,5)" json:"exit_price"`
	ProfitLoss   float64    `gorm:"type:decimal(16,2)" json:"profit_loss"`
	OutcomeR     float64    `gorm:"type:decimal(8,2)" json:"outcome_r"`
	OpenedAt     time.Time  `gorm:"index;not null" json:"opened_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	Notes        string     `gorm:"type:text" json:"notes"`
}

func (Position) TableName() string {
	return "positions"
}
