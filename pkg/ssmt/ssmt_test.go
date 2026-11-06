package ssmt

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func TestBullishSMTDivergence(t *testing.T) {
	now := time.Now()
	// EURUSD: Candle 1 Low = 1.1000, Candle 2 Low = 1.1020 (Higher Low / GAGAL sweep)
	eur := []indicators.Candle{
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.1010, High: 1.1050, Low: 1.1000, Close: 1.1040, IsClosed: true},
		{Timestamp: now, Open: 1.1040, High: 1.1060, Low: 1.1020, Close: 1.1055, IsClosed: true},
	}

	// GBPUSD: Candle 1 Low = 1.3000, Candle 2 Low = 1.2980 (Lower Low / SWEEP!)
	gbp := []indicators.Candle{
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.3010, High: 1.3050, Low: 1.3000, Close: 1.3040, IsClosed: true},
		{Timestamp: now, Open: 1.3040, High: 1.3060, Low: 1.2980, Close: 1.3050, IsClosed: true},
	}

	detector := NewDetector()
	res := detector.EvaluateSSMT("EURUSD", "GBPUSD", eur, gbp, "BULLISH")

	if !res.Detected {
		t.Fatalf("expected SMT divergence to be detected")
	}
	if res.Type != SMTBullishDivergence {
		t.Errorf("expected BULLISH_SMT_DIVERGENCE, got %s", res.Type)
	}
	if res.ProbabilityGrade != GradeHighProbability {
		t.Errorf("expected Grade HIGH_PROBABILITY, got %s", res.ProbabilityGrade)
	}
}

func TestNeutralSweep(t *testing.T) {
	now := time.Now()
	// Both sweep low
	eur := []indicators.Candle{
		{Timestamp: now.Add(-15 * time.Minute), Low: 1.1000, IsClosed: true},
		{Timestamp: now, Low: 1.0980, IsClosed: true},
	}
	gbp := []indicators.Candle{
		{Timestamp: now.Add(-15 * time.Minute), Low: 1.3000, IsClosed: true},
		{Timestamp: now, Low: 1.2980, IsClosed: true},
	}

	detector := NewDetector()
	res := detector.EvaluateSSMT("EURUSD", "GBPUSD", eur, gbp, "BULLISH")

	if !res.Detected || res.Type != SMTNeutralSweep {
		t.Errorf("expected NEUTRAL_SWEEP, got %s", res.Type)
	}
	if res.ProbabilityGrade != GradeStandardProbability {
		t.Errorf("expected STANDARD_PROBABILITY, got %s", res.ProbabilityGrade)
	}
}
