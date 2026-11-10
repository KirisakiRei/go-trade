package keylevels

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func TestFindERLAndIRL(t *testing.T) {
	now := time.Now()
	// Standar fraktal 5-bar ICT: membutuhkan minimal 2 candle kiri dan 2 candle kanan
	candles := []indicators.Candle{
		{Timestamp: now.Add(-6 * time.Hour), Open: 98, High: 103, Low: 94, Close: 100, IsClosed: true},   // idx 0
		{Timestamp: now.Add(-5 * time.Hour), Open: 100, High: 105, Low: 95, Close: 102, IsClosed: true},  // idx 1
		{Timestamp: now.Add(-4 * time.Hour), Open: 102, High: 110, Low: 101, Close: 108, IsClosed: true}, // idx 2: 5-bar Swing High at 110
		{Timestamp: now.Add(-3 * time.Hour), Open: 108, High: 107, Low: 98, Close: 100, IsClosed: true},  // idx 3
		{Timestamp: now.Add(-2 * time.Hour), Open: 100, High: 103, Low: 90, Close: 92, IsClosed: true},   // idx 4: 5-bar Swing Low at 90
		{Timestamp: now.Add(-1 * time.Hour), Open: 92, High: 99, Low: 91, Close: 98, IsClosed: true},     // idx 5
		{Timestamp: now, Open: 98, High: 102, Low: 96, Close: 101, IsClosed: true},                       // idx 6
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
	// C4 retrace ke dalam gap [102, 106] tapi belum fully filled melewati lantai 102
	candlesRetrace := []indicators.Candle{
		{Timestamp: now.Add(-4 * time.Hour), Open: 100, High: 102, Low: 98, Close: 101, IsClosed: true},  // C1 High = 102 (Lantai)
		{Timestamp: now.Add(-3 * time.Hour), Open: 101, High: 115, Low: 101, Close: 114, IsClosed: true}, // C2 Large Bullish
		{Timestamp: now.Add(-2 * time.Hour), Open: 114, High: 118, Low: 106, Close: 117, IsClosed: true}, // C3 Low = 106 (Atap)
		{Timestamp: now.Add(-1 * time.Hour), Open: 117, High: 117, Low: 104, Close: 108, IsClosed: true}, // C4 Retraces to 104 (inside gap)
	}

	detector := NewDetector()
	irls := detector.FindIRLs(candlesRetrace)

	if len(irls) != 1 {
		t.Fatalf("expected 1 IRL, got %d", len(irls))
	}
	irl := irls[0]
	if irl.Type != TypeIRLBullish || irl.Bottom != 102 || irl.Top != 106 {
		t.Errorf("expected Bullish FVG [102 - 106], got %v [%.2f - %.2f]", irl.Type, irl.Bottom, irl.Top)
	}
	// Aturan baku ILA 3.0: hanya masuk gap belum gugur/mitigated sampai 100% fully filled
	if irl.Mitigated {
		t.Errorf("expected FVG NOT to be mitigated when only touching 104 inside gap")
	}

	// C5 datang dan fully fill tembus ke lantai (Low <= 102)
	candlesFilled := append(candlesRetrace, indicators.Candle{
		Timestamp: now, Open: 108, High: 109, Low: 101.5, Close: 103, IsClosed: true,
	})
	irlsFilled := detector.FindIRLs(candlesFilled)
	if len(irlsFilled) != 1 {
		t.Fatalf("expected 1 IRL, got %d", len(irlsFilled))
	}
	if !irlsFilled[0].Mitigated || !irlsFilled[0].FullyFilled {
		t.Errorf("expected FVG to be mitigated and fully filled after C5 low 101.5 <= 102")
	}
}
