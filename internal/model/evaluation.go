package model

import "time"

// TradeEvaluation stores post-mortem analysis and learned lessons for each executed trade.
type TradeEvaluation struct {
	Base
	PositionID        string    `gorm:"type:varchar(36);index;not null" json:"position_id"`
	TicketNumber      string    `gorm:"type:varchar(64);index;not null" json:"ticket_number"`
	Symbol            string    `gorm:"type:varchar(20);index;not null" json:"symbol"`
	Direction         string    `gorm:"type:varchar(10);not null" json:"direction"`
	SetupType         string    `gorm:"type:varchar(30);not null" json:"setup_type"`
	ConfluenceGrade   string    `gorm:"type:varchar(30);not null" json:"confluence_grade"`
	OpenPrice         float64   `gorm:"type:decimal(16,5);not null" json:"open_price"`
	ExitPrice         float64   `gorm:"type:decimal(16,5);not null" json:"exit_price"`
	StopLoss          float64   `gorm:"type:decimal(16,5);not null" json:"stop_loss"`
	TakeProfit        float64   `gorm:"type:decimal(16,5);not null" json:"take_profit"`
	Outcome           string    `gorm:"type:varchar(20);index;not null" json:"outcome"` // HIT_TP, HIT_SL, CLOSED_MANUAL
	OutcomeR          float64   `gorm:"type:decimal(8,2);not null" json:"outcome_r"`
	ProfitLoss        float64   `gorm:"type:decimal(16,2);not null" json:"profit_loss"`
	DurationMinutes   int       `gorm:"not null" json:"duration_minutes"`
	RootCause         string    `gorm:"type:varchar(64);not null" json:"root_cause"`
	TechnicalAnalysis string    `gorm:"type:text;not null" json:"technical_analysis"`
	ActionableLesson  string    `gorm:"type:text;not null" json:"actionable_lesson"`
	NotifiedTelegram  bool      `gorm:"default:false;index" json:"notified_telegram"`
	EvaluatedAt       time.Time `gorm:"index;not null" json:"evaluated_at"`
}

func (TradeEvaluation) TableName() string {
	return "trade_evaluations"
}
