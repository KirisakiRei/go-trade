package risk

import (
	"testing"

	"ila-engine/pkg/broker"
)

func TestValidateRR(t *testing.T) {
	// Entry 100, SL 95 (risk 5), TP 110 (reward 10) -> RR = 2.0
	rr, passed := ValidateRR(100, 95, 110, 2.0)
	if !passed || rr != 2.0 {
		t.Fatalf("expected RR 2.0 to pass, got %f, passed=%v", rr, passed)
	}

	// Entry 100, SL 95 (risk 5), TP 108 (reward 8) -> RR = 1.6 (should fail minRR 2.0)
	rr2, passed2 := ValidateRR(100, 95, 108, 2.0)
	if passed2 || rr2 != 1.6 {
		t.Fatalf("expected RR 1.6 to fail, got %f, passed=%v", rr2, passed2)
	}
}

func TestCalculateLotForex(t *testing.T) {
	// Equity $10,000, 1% risk = $100 risk
	// EURUSD: Entry 1.11700, SL 1.11500 (20 pips), TP 1.12200 (50 pips, RR 2.5)
	res, err := CalculateLot(10000, 1.0, 1.11700, 1.11500, 2.0, 1.12200, "EURUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.PassedRR {
		t.Fatalf("expected RR to pass: %+v", res)
	}

	// 20 pips * $10/pip = $200 per lot. To risk $100 -> lot = 100 / 200 = 0.50 lot
	if res.LotSize != 0.50 {
		t.Fatalf("expected 0.50 lot, got %f", res.LotSize)
	}

	if res.RiskAmount != 100.0 {
		t.Fatalf("expected $100 risk amount, got %f", res.RiskAmount)
	}
}

func TestCalculateLotGold(t *testing.T) {
	// Equity $10,000, 1% risk = $100 risk
	// XAUUSD: Entry 2000.0, SL 1995.0 ($5 move = 50 pips), TP 2012.0 ($12 move = 120 pips, RR 2.4)
	res, err := CalculateLot(10000, 1.0, 2000.0, 1995.0, 2.0, 2012.0, "XAUUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.PassedRR {
		t.Fatalf("expected RR to pass: %+v", res)
	}

	// 50 pips * $10/pip = $500 per lot. To risk $100 -> lot = 100 / 500 = 0.20 lot
	if res.LotSize != 0.20 {
		t.Fatalf("expected 0.20 lot, got %f", res.LotSize)
	}
}

// Regression: PassedRisk harus mencerminkan risiko aktual setelah pembulatan lot.
func TestCalculateLotPassedRiskDetectsRoundingOvershoot(t *testing.T) {
	// Equity $1, risk 1% = $0.01, tapi min lot 0.01 berisiko $0.10 (10%).
	res, err := CalculateLot(1.0, 1.0, 1.10000, 1.09999, 2.0, 1.30000, "EURUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.PassedRisk {
		t.Fatalf("expected PassedRisk=false when min-lot rounding overshoots target risk: %+v", res)
	}
	if res.RiskPercentage <= 1.0 {
		t.Fatalf("expected actual risk > 1%%, got %.3f%%", res.RiskPercentage)
	}
}

// Regression: riskPercent harus di-clamp maksimal 2%.
func TestCalculateLotClampsRiskPercent(t *testing.T) {
	res, err := CalculateLot(100000, 100.0, 2000.0, 1995.0, 2.0, 2012.0, "XAUUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RiskPercentage > 2.0001 {
		t.Fatalf("expected clamped risk <= 2%%, got %.3f%%", res.RiskPercentage)
	}
}

// Validasi default riskPercent 1.0%.
func TestCalculateLotDefaultRiskOnePercent(t *testing.T) {
	// Equity $10,000, riskPercent <= 0 -> default 1.0% = $100
	// XAUUSD: Entry 2000.0, SL 1995.0 ($5 move = 50 pips)
	// 50 pips * $10/pip = $500 per lot. $100 / $500 = 0.20 lot
	res, err := CalculateLot(10000, 0, 2000.0, 1995.0, 2.0, 2012.0, "XAUUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.LotSize != 0.20 {
		t.Fatalf("expected 0.20 lot on 1.0%% risk, got %f", res.LotSize)
	}
	if res.RiskAmount != 100.0 {
		t.Fatalf("expected $100 risk amount, got %f", res.RiskAmount)
	}
}

func TestCalculateLotUS100MatchTraderMultiplier(t *testing.T) {
	// Equity $13,131, risk 1% = $131.31
	// US100 Match-Trader: Entry 27903, SL 27825 (78 points), TP 28059 (156 points, RR 2.0)
	// Spec: PointValue = 20.0 ($20/point), VolumeStep = 0.01, VolumeMin = 0.01
	// Dollar loss per lot = 78 * 20 = $1,560 per lot
	// Expected lot = 131.31 / 1560 = 0.0841 -> floor to 0.08 lot
	// Actual risk = 0.08 * 78 * 20 = $124.80 (< $131.31)
	spec := broker.SymbolSpec{
		Symbol:         "US100",
		PointValue:     20.0,
		VolumeMin:      0.01,
		VolumeStep:     0.01,
		VolumeMax:      20.0,
		PricePrecision: 2,
	}
	res, err := CalculateLotWithSpec(13131, 1.0, 27903.0, 27825.0, 2.0, 28059.0, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.LotSize != 0.08 {
		t.Fatalf("expected 0.08 lot on Match-Trader US100 with multiplier 20, got %.2f", res.LotSize)
	}
	if res.RiskAmount > 131.31 {
		t.Fatalf("risk amount %.2f exceeded 1%% risk budget 131.31", res.RiskAmount)
	}
	if !res.PassedRisk {
		t.Fatalf("expected PassedRisk=true, got false")
	}
}

func TestCalculateLotUSTECcTrader(t *testing.T) {
	// Equity $13,131, risk 1% = $131.31
	// USTEC cTrader: Entry 27903, SL 27825 (78 points), TP 28059 (156 points, RR 2.0)
	// Spec: PointValue = 1.0 ($1/point), VolumeStep = 0.10, VolumeMin = 0.10
	// Dollar loss per lot = 78 * 1 = $78 per lot
	// Expected lot = 131.31 / 78 = 1.683 -> floor to 1.60 lot
	// Actual risk = 1.60 * 78 * 1 = $124.80 (< $131.31)
	spec := broker.SymbolSpec{
		Symbol:         "USTEC",
		PointValue:     1.0,
		VolumeMin:      0.10,
		VolumeStep:     0.10,
		VolumeMax:      20.0,
		PricePrecision: 2,
	}
	res, err := CalculateLotWithSpec(13131, 1.0, 27903.0, 27825.0, 2.0, 28059.0, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.LotSize != 1.60 {
		t.Fatalf("expected 1.60 lot on cTrader USTEC, got %.2f", res.LotSize)
	}
	if res.RiskAmount > 131.31 {
		t.Fatalf("risk amount %.2f exceeded 1%% risk budget 131.31", res.RiskAmount)
	}
	if !res.PassedRisk {
		t.Fatalf("expected PassedRisk=true, got false")
	}
}
