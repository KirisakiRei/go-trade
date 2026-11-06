package risk

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	// MaxLotSize is the hard ceiling for any single position.
	MaxLotSize = 20.0
)

var (
	ErrInvalidStopLoss = errors.New("stop loss must be distinct from entry price")
	ErrZeroEquity      = errors.New("equity must be greater than zero")
	ErrLowRiskReward   = errors.New("risk reward ratio is below required minimum")
)

type RiskResult struct {
	LotSize        float64 `json:"lot_size"`
	RiskAmount     float64 `json:"risk_amount"`
	RiskPercentage float64 `json:"risk_percentage"`
	DistancePips   float64 `json:"distance_pips"`
	PipValue       float64 `json:"pip_value"`
	RiskReward     float64 `json:"risk_reward"`
	PassedRR       bool    `json:"passed_rr"`
	PassedRisk     bool    `json:"passed_risk"`
	Reason         string  `json:"reason"`
}

// ValidateRR checks if reward to risk ratio meets the minimum required (default 2.0).
func ValidateRR(entry, sl, tp float64, minRR float64) (float64, bool) {
	risk := math.Abs(entry - sl)
	if risk <= 0.000001 {
		return 0, false
	}
	reward := math.Abs(tp - entry)
	rr := reward / risk
	return math.Round(rr*100) / 100, rr >= minRR
}

// CalculateLot computes position size based on exact equity, risk percentage (e.g. 1.0%), and instrument pip value.
// Symbol determines contract specs:
// - Gold (XAUUSD): 1 Lot = 100 oz. 1 pip = $0.10 price move ($10/pip/lot). $1.00 move = 10 pips ($100/lot).
// - Forex Standard (EURUSD, GBPUSD): 1 Lot = 100,000 units. 1 pip = 0.00010 ($10/pip/lot).
func CalculateLot(equity, riskPercent, entry, sl, minRR, tp float64, symbol string) (*RiskResult, error) {
	if equity <= 0 {
		return nil, ErrZeroEquity
	}
	if math.Abs(entry-sl) <= 0.000001 {
		return nil, ErrInvalidStopLoss
	}
	if riskPercent <= 0 {
		riskPercent = 1.0 // Default 1%
	}
	if minRR <= 0 {
		minRR = 2.0 // Default 1:2
	}
	// Safety clamp: never allow a single trade to risk more than 2% of equity,
	// regardless of what the caller passes. Prevents catastrophic misconfig
	// (e.g. RiskRatio accidentally set to 100).
	if riskPercent > 2.0 {
		riskPercent = 2.0
	}

	sym := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
	isGold := strings.Contains(sym, "XAU") || strings.Contains(sym, "GOLD") || strings.Contains(sym, "PAXG")
	isIndex := strings.Contains(sym, "NAS") || strings.Contains(sym, "TEC") || strings.Contains(sym, "US100") || strings.Contains(sym, "US30") || strings.Contains(sym, "US500") || strings.Contains(sym, "SPX") || strings.Contains(sym, "NDX")

	riskAmount := equity * (riskPercent / 100.0)
	priceDist := math.Abs(entry - sl)

	var pipSize float64
	var pipValuePerLot float64

	if isGold {
		pipSize = 0.10        // 1 pip Gold = 0.10 USD
		pipValuePerLot = 10.0 // 1 standard lot (100 oz) * 0.10 = $10 per pip
	} else if isIndex {
		pipSize = 1.0        // 1 index point
		pipValuePerLot = 1.0 // 1 index CFD contract = $1 per point
	} else {
		pipSize = 0.00010     // 1 pip Forex = 0.00010
		pipValuePerLot = 10.0 // 1 standard lot (100,000) * 0.00010 = $10 per pip
	}

	pips := priceDist / pipSize
	if pips < 1.0 {
		pips = 1.0
	}

	// lot = riskAmount / (pips * pipValuePerLot)
	rawLot := riskAmount / (pips * pipValuePerLot)

	minLot := 0.01
	if isIndex {
		minLot = 0.10 // USTEC minimum volume is 0.10 lot
	}

	// Clamp lot size between minLot and 20.0 lots (dynamic based on equity)
	lot := math.Round(rawLot*100) / 100
	if isIndex {
		lot = math.Floor(rawLot*10) / 10.0 // cTrader volume step for USTEC is 0.10 lot
	}
	if lot < minLot {
		lot = minLot
	}
	if lot > MaxLotSize {
		lot = MaxLotSize
	}

	rr, passedRR := ValidateRR(entry, sl, tp, minRR)

	// Actual risk after lot clamping. Clamping UP to minLot (e.g. 0.01 on a
	// tight SL) can silently increase the real risk beyond the requested %.
	// We must report the ACTUAL risk and only mark PassedRisk when the final
	// lot does not exceed the requested risk budget.
	actualRiskAmount := lot * pips * pipValuePerLot
	actualRiskPercent := (actualRiskAmount / equity) * 100.0

	riskTolerance := 1.05 // allow 5% overshoot for broker lot-step rounding
	passedRisk := actualRiskPercent <= riskPercent*riskTolerance

	res := &RiskResult{
		LotSize:        lot,
		RiskAmount:     math.Round(actualRiskAmount*100) / 100,
		RiskPercentage: math.Round(actualRiskPercent*1000) / 1000,
		DistancePips:   math.Round(pips*10) / 10,
		PipValue:       pipValuePerLot,
		RiskReward:     rr,
		PassedRR:       passedRR,
		PassedRisk:     passedRisk,
	}

	switch {
	case !passedRR:
		res.Reason = "R:R ratio below mandatory 1:2"
	case !passedRisk:
		res.Reason = fmt.Sprintf("Lot terpaksa dibulatkan ke bawah/atas sehingga risiko aktual (%.3f%%) melebihi target (%.3f%%)", actualRiskPercent, riskPercent)
	default:
		res.Reason = "Risk and R:R rules passed"
	}

	return res, nil
}
