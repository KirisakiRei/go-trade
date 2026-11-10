package risk

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"ila-engine/pkg/broker"
)

const (
	// MaxLotSize: batas maksimal lot per posisi.
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

// CalculateLot computes position size based on fallback specs for symbol.
func CalculateLot(equity, riskPercent, entry, sl, minRR, tp float64, symbol string) (*RiskResult, error) {
	spec := broker.FallbackSymbolSpec(symbol)
	return CalculateLotWithSpec(equity, riskPercent, entry, sl, minRR, tp, *spec)
}

// CalculateLotWithSpec computes position size dynamically using exact contract specifications from broker.
// PointValue defines how many USD are won/lost per 1.00 full price move per 1.00 lot:
// - Forex Standard: PointValue = 100,000 (1 pip 0.00010 = $10/lot)
// - Gold Standard: PointValue = 100 (1 pip 0.10 = $10/lot, $1 move = $100/lot)
// - Match-Trader US100: Multiplier 20 * ContractSize 1 -> PointValue = 20.0 ($20 per 1.00 index point/lot)
// - cTrader USTEC: 1 CFD contract -> PointValue = 1.0 ($1 per 1.00 index point/lot)
func CalculateLotWithSpec(equity, riskPercent, entry, sl, minRR, tp float64, spec broker.SymbolSpec) (*RiskResult, error) {
	if equity <= 0 {
		return nil, ErrZeroEquity
	}
	priceDist := math.Abs(entry - sl)
	if priceDist <= 0.000001 {
		return nil, ErrInvalidStopLoss
	}
	if riskPercent <= 0 {
		riskPercent = 1.0 // Default 1.0% per user instruction
	}
	if minRR <= 0 {
		minRR = 2.0 // Default 1:2
	}
	// Clamp risiko maksimal 2% per trade (proteksi salah konfigurasi).
	if riskPercent > 2.0 {
		riskPercent = 2.0
	}

	sym := strings.ToUpper(strings.ReplaceAll(spec.Symbol, "/", ""))
	isGold := strings.Contains(sym, "XAU") || strings.Contains(sym, "GOLD") || strings.Contains(sym, "PAXG")
	isIndex := strings.Contains(sym, "NAS") || strings.Contains(sym, "TEC") || strings.Contains(sym, "US100") || strings.Contains(sym, "US30") || strings.Contains(sym, "US500") || strings.Contains(sym, "SPX") || strings.Contains(sym, "NDX")

	var pipSize float64
	if isGold {
		pipSize = 0.10
	} else if isIndex {
		pipSize = 1.0
	} else {
		pipSize = 0.00010
	}

	pointVal := spec.PointValue
	if pointVal <= 0 {
		if isGold {
			pointVal = 100.0
		} else if isIndex {
			pointVal = 1.0
		} else {
			pointVal = 100000.0
		}
	}

	pipValuePerLot := pointVal * pipSize
	if pipValuePerLot <= 0 {
		pipValuePerLot = 10.0
	}

	pips := priceDist / pipSize
	if pips < 1.0 {
		pips = 1.0
	}

	riskAmount := equity * (riskPercent / 100.0)
	dollarLossPerLot := pips * pipValuePerLot
	rawLot := (riskAmount / dollarLossPerLot) + 1e-8

	step := spec.VolumeStep
	if step <= 0 {
		if isIndex {
			step = 0.10
		} else {
			step = 0.01
		}
	}

	minLot := spec.VolumeMin
	if minLot <= 0 {
		if isIndex {
			minLot = 0.10
		} else {
			minLot = 0.01
		}
	}

	maxLot := spec.VolumeMax
	if maxLot <= 0 {
		maxLot = MaxLotSize
	}

	// Floor lot calculation to never overshoot risk
	lot := math.Floor(rawLot/step) * step
	if lot < minLot {
		lot = minLot
	}
	if lot > maxLot {
		lot = maxLot
	}

	// Max Risk Assertion Gate: If rounded lot overshoots risk budget by > 10%, clamp down by step
	actualRiskAmount := lot * dollarLossPerLot
	if lot > minLot && actualRiskAmount > (riskAmount*1.10) {
		lot = math.Floor((riskAmount/dollarLossPerLot)/step) * step
		actualRiskAmount = lot * dollarLossPerLot
	}

	actualRiskPercent := (actualRiskAmount / equity) * 100.0
	passedRisk := actualRiskPercent <= (riskPercent * 1.05)

	rr, passedRR := ValidateRR(entry, sl, tp, minRR)

	res := &RiskResult{
		LotSize:        math.Round(lot*100) / 100.0,
		RiskAmount:     math.Round(actualRiskAmount*100) / 100.0,
		RiskPercentage: math.Round(actualRiskPercent*1000) / 1000.0,
		DistancePips:   math.Round(pips*10) / 10.0,
		PipValue:       pipValuePerLot,
		RiskReward:     rr,
		PassedRR:       passedRR,
		PassedRisk:     passedRisk,
	}

	switch {
	case !passedRR:
		res.Reason = "R:R ratio below mandatory 1:2"
	case !passedRisk:
		res.Reason = fmt.Sprintf("Lot terpaksa dibulatkan sehingga risiko aktual (%.3f%%) melebihi target (%.3f%%)", actualRiskPercent, riskPercent)
	default:
		res.Reason = "Risk and R:R rules passed"
	}

	return res, nil
}
