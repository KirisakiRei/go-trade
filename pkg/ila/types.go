package ila

import (
	"time"

	"ila-engine/pkg/indicators"
)

type FVGType string

const (
	FVGBearish FVGType = "BEARISH"
	FVGBullish FVGType = "BULLISH"
)

// FVG represents a 3-candle Fair Value Gap (imbalance).
type FVG struct {
	Type         FVGType   `json:"type"`
	Top          float64   `json:"top"`
	Bottom       float64   `json:"bottom"`
	Candle1Index int       `json:"candle1_index"`
	Candle2Index int       `json:"candle2_index"`
	Candle3Index int       `json:"candle3_index"`
	Candle1Time  time.Time `json:"candle1_time"`
	Candle2Time  time.Time `json:"candle2_time"`
	Candle3Time  time.Time `json:"candle3_time"`
	Mitigated    bool      `json:"mitigated"`
}

// HTFSweep represents an Imaginary Liquidity Sweep occurring inside an HTF FVG.
type HTFSweep struct {
	Detected         bool              `json:"detected"`
	SweepInProgress  bool              `json:"sweep_in_progress"`
	RemainingTime    string            `json:"remaining_time,omitempty"`
	Direction        string            `json:"direction"` // BEARISH or BULLISH
	FVG              *FVG              `json:"fvg"`
	SweepCandle      indicators.Candle `json:"sweep_candle"`
	SweptCandle      indicators.Candle `json:"swept_candle"`
	SweepCandleIndex int               `json:"sweep_candle_index"`
	SweepTime        time.Time         `json:"sweep_time"`
	StopLoss         float64           `json:"stop_loss"`  // Low (Buy) or High (Sell) of Candle 5 / current candle
	TargetDOL        float64           `json:"target_dol"` // Draw on Liquidity (SSL / BSL)
	Details          string            `json:"details"`
}

// CISD represents Change In State of Delivery on LTF (body breaks body).
type CISD struct {
	Detected    bool              `json:"detected"`
	Direction   string            `json:"direction"` // BEARISH or BULLISH
	RefCandle   indicators.Candle `json:"ref_candle"`   // The opposing candle being broken
	BreakCandle indicators.Candle `json:"break_candle"` // The candle that broke ref candle by body
	BreakTime   time.Time         `json:"break_time"`
	CISDLevel   float64           `json:"cisd_level"`  // Open price of the origin/first candle
	ExtremeSL   float64           `json:"extreme_sl"`  // The extreme Low/High of Candle 5
	EntryPrice  float64           `json:"entry_price"` // Close of break candle / Open of next candle
	Details     string            `json:"details"`
}

// ILASetup represents the complete multi-timeframe ILA evaluation.
type ILASetup struct {
	Symbol      string    `json:"symbol"`
	HTF         string    `json:"htf"`
	LTF         string    `json:"ltf"`
	Bias        string    `json:"bias"`   // BULLISH, BEARISH, NEUTRAL
	Status      string    `json:"status"` // WAIT_HTF_SWEEP, WAIT_LTF_CISD, READY_TO_ENTRY, INVALIDATED
	HTFSweep    *HTFSweep `json:"htf_sweep,omitempty"`
	CISD        *CISD     `json:"cisd,omitempty"`
	EntryPrice  float64   `json:"entry_price"`
	StopLoss    float64   `json:"stop_loss"`
	TakeProfit  float64   `json:"take_profit"`
	RiskReward  float64   `json:"risk_reward"`
	PassedMinRR bool      `json:"passed_min_rr"`
	NextAction  string    `json:"next_action"`
	Summary     string    `json:"summary"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

type ILAPairResult struct {
	PairName string    `json:"pair_name"` // e.g. "W ➡️ H4", "D1 ➡️ H1", "H4 ➡️ M15", "H1 ➡️ M5"
	HTF      string    `json:"htf"`
	LTF      string    `json:"ltf"`
	Setup    *ILASetup `json:"setup"`
}

type ILAMultiResponse struct {
	Symbol       string          `json:"symbol"`
	EvaluatedAt  time.Time       `json:"evaluated_at"`
	Pairs        []ILAPairResult `json:"pairs"`
	ActiveSetups int             `json:"active_setups"`
	Summary      string          `json:"summary"`
}
