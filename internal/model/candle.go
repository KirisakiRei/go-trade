package model

import "time"

type MarketCandle struct {
	Base
	Symbol    string    `gorm:"type:varchar(20);index:idx_sym_tf_time;not null" json:"symbol"`
	Timeframe string    `gorm:"type:varchar(10);index:idx_sym_tf_time;not null" json:"timeframe"`
	Timestamp time.Time `gorm:"index:idx_sym_tf_time;not null" json:"timestamp"`
	Open      float64   `gorm:"type:decimal(16,4);not null" json:"open"`
	High      float64   `gorm:"type:decimal(16,4);not null" json:"high"`
	Low       float64   `gorm:"type:decimal(16,4);not null" json:"low"`
	Close     float64   `gorm:"type:decimal(16,4);not null" json:"close"`
	Volume    float64   `gorm:"type:decimal(20,4)" json:"volume"`
	IsClosed  bool      `gorm:"default:true;not null" json:"is_closed"`
}

func (MarketCandle) TableName() string {
	return "market_candles"
}
