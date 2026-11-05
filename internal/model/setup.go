package model

import (
	"time"
)

// SetupRecord represents a detected BBMA setup instance according to Rule 7.4.
type SetupRecord struct {
	Base
	Symbol                 string    `gorm:"type:varchar(20);index;not null" json:"symbol"`
	Timeframe              string    `gorm:"type:varchar(10);index;not null" json:"timeframe"`
	SetupID                string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"setup_id"`
	SetupType              string    `gorm:"type:varchar(30);index;not null" json:"setup_type"`
	State                  string    `gorm:"type:varchar(30);index;not null" json:"state"` // CANDIDATE, CONFIRMED, INVALIDATED, EXPIRED
	Direction              string    `gorm:"type:varchar(10);not null" json:"direction"`   // BUY, SELL, NEUTRAL
	CandleTimestamp        time.Time `gorm:"index;not null" json:"candle_timestamp"`
	EvidenceJSON           string    `gorm:"type:text" json:"evidence_json"`
	MissingConditionsJSON  string    `gorm:"type:text" json:"missing_conditions_json"`
	InvalidationConditions string    `gorm:"type:text" json:"invalidation_conditions"`
	EntryPrice             float64   `gorm:"type:decimal(16,4)" json:"entry_price"`
	StopLoss               float64   `gorm:"type:decimal(16,4)" json:"stop_loss"`
	TakeProfit             float64   `gorm:"type:decimal(16,4)" json:"take_profit"`
	ExpiresAt              time.Time `json:"expires_at"`
}

func (SetupRecord) TableName() string {
	return "bbma_setups"
}
