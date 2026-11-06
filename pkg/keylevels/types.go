package keylevels

import "time"

type LevelType string

const (
	TypeERLSwingHigh LevelType = "ERL_SWING_HIGH"
	TypeERLSwingLow  LevelType = "ERL_SWING_LOW"
	TypeIRLBearish   LevelType = "IRL_BEARISH_FVG"
	TypeIRLBullish   LevelType = "IRL_BULLISH_FVG"
)

// ERL represents External Range Liquidity (purely Swing High / Swing Low)
type ERL struct {
	Type        LevelType `json:"type"`        // ERL_SWING_HIGH or ERL_SWING_LOW
	Price       float64   `json:"price"`       // Extreme high or low price
	CandleTime  time.Time `json:"candle_time"` // Timestamp of the swing candle
	CandleIndex int       `json:"candle_index"`
	Mitigated   bool      `json:"mitigated"`   // Level has been tested / reacted from
	Displaced   bool      `json:"displaced"`   // Level broken by candle body displacement (MSS)
}

// IRL represents Internal Range Liquidity (purely Fair Value Gap / FVG)
type IRL struct {
	Type        LevelType `json:"type"`          // IRL_BEARISH_FVG or IRL_BULLISH_FVG
	Top         float64   `json:"top"`           // Top boundary of gap
	Bottom      float64   `json:"bottom"`        // Bottom boundary of gap
	Candle1Time time.Time `json:"candle1_time"`
	Candle2Time time.Time `json:"candle2_time"`
	Candle3Time time.Time `json:"candle3_time"`
	CandleIndex int       `json:"candle_index"`
	Mitigated   bool      `json:"mitigated"`     // Retraced into & reacted
	FullyFilled bool      `json:"fully_filled"`  // 100% covered by price (invalidated)
	Invalidated bool      `json:"invalidated"`   // Broken through with displacement
}

// TargetRule enforces ILA 3.0 target destination
// Rule:
// - IRL start -> ALWAYS targets ERL (rigid)
// - ERL start -> targets IRL or another ERL (flexible)
type TargetRule struct {
	StartLevelType LevelType `json:"start_level_type"`
	TargetType     LevelType `json:"target_type"`
	TargetPrice    float64   `json:"target_price"`
}
