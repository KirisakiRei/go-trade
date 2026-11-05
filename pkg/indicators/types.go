package indicators

import "time"

// Candle represents an OHLCV candle for indicators calculation.
type Candle struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	IsClosed  bool      `json:"is_closed"`
}

// IndicatorValues holds all calculated BBMA values for a specific candle.
type IndicatorValues struct {
	Timestamp time.Time `json:"timestamp"`
	Close     float64   `json:"close"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Open      float64   `json:"open"`

	// Bollinger Bands (20, 2)
	BBUpper  float64 `json:"bb_upper"`
	BBMiddle float64 `json:"bb_middle"`
	BBLower  float64 `json:"bb_lower"`

	// WMA Highs
	WMA5High  float64 `json:"wma5_high"`
	WMA10High float64 `json:"wma10_high"`

	// WMA Lows
	WMA5Low  float64 `json:"wma5_low"`
	WMA10Low float64 `json:"wma10_low"`

	// EMA 50
	EMA50 float64 `json:"ema50"`

	// Validity flags
	ValidBB  bool `json:"valid_bb"`
	ValidWMA bool `json:"valid_wma"`
	ValidEMA bool `json:"valid_ema"`
}
