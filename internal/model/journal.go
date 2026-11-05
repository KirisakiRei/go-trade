package model

import "time"

// TradeJournal stores trade decisions and audit records as specified in Section 9.13 & Section 13.6.
type TradeJournal struct {
	Base
	SetupID     string    `gorm:"type:varchar(64);index;not null" json:"setup_id"`
	Symbol      string    `gorm:"type:varchar(20);index;not null" json:"symbol"`
	Timeframe   string    `gorm:"type:varchar(10);not null" json:"timeframe"`
	Action      string    `gorm:"type:varchar(20);not null" json:"action"` // PLAN_CREATED, SETUP_INVALIDATED, TARGET_HIT, STOPPED_OUT
	State       string    `gorm:"type:varchar(30);not null" json:"state"`
	Price       float64   `gorm:"type:decimal(16,4)" json:"price"`
	StopLoss    float64   `gorm:"type:decimal(16,4)" json:"stop_loss"`
	TakeProfit  float64   `gorm:"type:decimal(16,4)" json:"take_profit"`
	OutcomeR    float64   `gorm:"type:decimal(8,2)" json:"outcome_r"`
	Notes       string    `gorm:"type:text" json:"notes"`
	EvaluatedAt time.Time `gorm:"index;not null" json:"evaluated_at"`
}

func (TradeJournal) TableName() string {
	return "trade_journals"
}
