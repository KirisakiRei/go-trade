package keylevels

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func TestFindERLAndIRL(t *testing.T) {
	now := time.Now()
	candles := []indicators.Candle{
		{Timestamp: now.Add(-5 * time.Hour), Open: 100, High: 105, Low: 95, Close: 102, IsClosed: true},
		{Timestamp: now.Add(-4 * time.Hour), Open: 102, High: 110, Low: 101, Close: 108, IsClosed: true}, // Swing High at 110
		{Timestamp: now.Add(-3 * time.Hour), Open: 108, High: 107, Low: 98, Close: 100, IsClosed: true},
		{Timestamp: now.Add(-2 * time.Hour), Open: 100, High: 103, Low: 90, Close: 92, IsClosed: true},  // Swing Low at 90
		{Timestamp: now.Add(-1 * time.Hour), Open: 92, High: 99, Low: 91, Close: 98, IsClosed: true},
		{Timestamp: now, Open: 98, High: 102, Low: 96, Close: 101, IsClosed: true},
	}

	detector := NewDetector()
	erls := detector.FindERLs(candles)

	if len(erls) != 2 {
		t.Fatalf("expected 2 ERLs, got %d", len(erls))
	}
	if erls[0].Type != TypeERLSwingHigh || erls[0].Price != 110 {
		t.Errorf("expected Swing High at 110, got %v at %.2f", erls[0].Type, erls[0].Price)
	}
	if erls[1].Type != TypeERLSwingLow || erls[1].Price != 90 {
		t.Errorf("expected Swing Low at 90, got %v at %.2f", erls[1].Type, erls[1].Price)
	}
}

func TestBullishFVG(t *testing.T) {
	now := time.Now()
	candles := []indicators.Candle{
		{Timestamp: now.Add(-4 * time.Hour), Open: 100, High: 102, Low: 98, Close: 101, IsClosed: true}, // C1 High = 102
		{Timestamp: now.Add(-3 * time.Hour), Open: 101, High: 115, Low: 101, Close: 114, IsClosed: true}, // C2 Large Bullish
		{Timestamp: now.Add(-2 * time.Hour), Open: 114, High: 118, Low: 106, Close: 117, IsClosed: true}, // C3 Low = 106
		{Timestamp: now.Add(-1 * time.Hour), Open: 117, High: 117, Low: 104, Close: 108, IsClosed: true}, // C4 Retraces into gap [102, 106]
	}

	detector := NewDetector()
	irls := detector.FindIRLs(candles)

	if len(irls) != 1 {
		t.Fatalf("expected 1 IRL, got %d", len(irls))
	}
	irl := irls[0]
	if irl.Type != TypeIRLBullish || irl.Bottom != 102 || irl.Top != 106 {
		t.Errorf("expected Bullish FVG [102 - 106], got %v [%.2f - %.2f]", irl.Type, irl.Bottom, irl.Top)
	}
	if !irl.Mitigated {
		t.Errorf("expected FVG to be mitigated by C4 low 104")
	}
}
