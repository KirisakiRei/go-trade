package risk

import (
	"testing"
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
