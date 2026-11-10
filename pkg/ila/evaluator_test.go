package ila

import (
	"strings"
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func TestFindFVG(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// 1. Test Bearish FVG: Low(C1)=4240, C2 drops, High(C3)=4212. Gap: 4212 - 4240
	candles := []indicators.Candle{
		{Timestamp: now.Add(-3 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true},
		{Timestamp: now.Add(-2 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true},
		{Timestamp: now.Add(-1 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true},
	}

	fvgs := eval.FindFVGs(candles)
	if len(fvgs) != 1 {
		t.Fatalf("expected 1 FVG, got %d", len(fvgs))
	}
	fvg := fvgs[0]
	if fvg.Type != FVGBearish {
		t.Errorf("expected Bearish FVG, got %s", fvg.Type)
	}
	if fvg.Top != 4240 || fvg.Bottom != 4212 {
		t.Errorf("expected Top=4240, Bottom=4212, got Top=%.2f, Bottom=%.2f", fvg.Top, fvg.Bottom)
	}
}

func TestHTFSweepAndCISD(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// HTF Candles: Bearish FVG formed, Candle A enters FVG, Candle B sweeps Candle A inside FVG
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-10 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-9 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true},  // C2 (Drop)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true},  // C3 (High=4212, FVG: 4212-4240)
		// Candle A: Retraces into FVG (High=4220), forms internal liquidity inside FVG
		{Timestamp: now.Add(-6 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},
		// Candle B (Sweep Candle): Sweeps Candle A (High=4230 > 4220), stays inside FVG (Close=4190 <= 4240)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},
		// Candle C (Running Candle 5): High=4225, Low=4185 (does not breach Sweep Candle High 4230)
		{Timestamp: now.Add(0), Open: 4190, High: 4225, Low: 4185, Close: 4200, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("expected HTF sweep to be detected")
	}
	if sweep.Direction != "BEARISH" {
		t.Errorf("expected BEARISH direction, got %s", sweep.Direction)
	}
	if sweep.StopLoss != 4230 {
		t.Errorf("expected SL=4230, got %.2f", sweep.StopLoss)
	}
	if sweep.TargetDOL != 4150 {
		t.Errorf("expected TargetDOL=4150, got %.2f", sweep.TargetDOL)
	}

	// LTF Candles: Bearish CISD (Bullish candle body broken by bearish body) inside Candle 5 (after now)
	ltfCandles := []indicators.Candle{
		// Bullish candle: Open=4210, Close=4218
		{Timestamp: now.Add(10 * time.Minute), Open: 4210, High: 4220, Low: 4208, Close: 4218, IsClosed: true},
		// Bearish candle breaks bullish Open (4210) by body: Open=4218, Close=4205 (< 4210)
		{Timestamp: now.Add(20 * time.Minute), Open: 4218, High: 4219, Low: 4204, Close: 4205, IsClosed: true},
	}

	cisd := eval.DetectCISD(ltfCandles, "BEARISH", now)
	if cisd == nil || !cisd.Detected {
		t.Fatalf("expected CISD to be detected")
	}
	if cisd.BreakCandle.Close != 4205 {
		t.Errorf("expected BreakPrice=4205, got %.2f", cisd.BreakCandle.Close)
	}

	// Full ILA Setup
	setup := eval.EvaluateILA(htfCandles, ltfCandles, "H4", "M15", "XAUUSD")
	if setup.Status != "READY_TO_ENTRY" {
		t.Errorf("expected READY_TO_ENTRY, got %s", setup.Status)
	}
	if setup.EntryPrice != 4205 {
		t.Errorf("expected EntryPrice=4205, got %.2f", setup.EntryPrice)
	}
	// Stop Loss placed at Candle 5 High (4220)
	if setup.StopLoss != 4220 {
		t.Errorf("expected StopLoss=4220 (Candle 5 High), got %.2f", setup.StopLoss)
	}
	// Option A Clamping: DOL was 4150 (raw RR = 55/15 = 3.67 >= 2.30), so TP is capped to 1:2.30 RR:
	// TP = 4205 - (2.30 * 15) = 4170.50
	if setup.TakeProfit != 4170.50 {
		t.Errorf("expected TakeProfit=4170.50 (capped at 1:2.30 RR), got %.2f", setup.TakeProfit)
	}
	if setup.RiskReward != 2.30 {
		t.Errorf("expected R:R=2.30 (capped), got %.2f", setup.RiskReward)
	}
	t.Logf("Setup summary: %s", setup.Summary)
}

func TestFakeSweepCandle3Rejected(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Candle 4 baru pertama kali masuk ke FVG dan menembus High C3.
	// Ini BUKAN sweep sah karena C3 adalah pembentuk FVG, bukan likuiditas yang sudah parkir di FVG.
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-4 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-3 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-2 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3 (High=4212)
		// C4: baru masuk FVG, menyapu C3 High (4220 > 4212). Harus DITOLAK!
		{Timestamp: now.Add(-1 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},
		{Timestamp: now.Add(0), Open: 4180, High: 4215, Low: 4175, Close: 4190, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep != nil && sweep.Detected {
		t.Fatalf("seharusnya sweep terhadap Candle 3 ditolak sebagai sweep palsu!")
	}
}

// TestExternalToInternalSweep memvalidasi kasus nyata USTEC H4:
// Candle acuan (cPrev) berada tipis di atas/luar Bullish FVG (misal FVG Top=27786.75, cPrev Low=27829.00),
// lalu candle sweep datang dari luar, menyapu Low cPrev (27766.60 < 27829.00) dan menusuk masuk ke FVG.
// Pola ini WAJIB sah diakui sebagai HTF Sweep (External-to-Internal Sweep).
func TestExternalToInternalSweep(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	htfCandles := []indicators.Candle{
		// C1, C2, C3 membentuk Bullish FVG: 27275.40 - 27786.75
		{Timestamp: now.Add(-20 * time.Hour), Open: 27032.50, High: 27275.40, Low: 27004.90, Close: 27214.00, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 27214.00, High: 27908.05, Low: 27190.80, Close: 27835.05, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 27835.05, High: 28023.15, Low: 27786.75, Close: 27969.10, IsClosed: true}, // C3

		// Candle A (cPrev): parkir tipis di atas FVG (Low=27829.00 > Top FVG 27786.75)
		{Timestamp: now.Add(-8 * time.Hour), Open: 27969.10, High: 28002.00, Low: 27829.00, Close: 27905.60, IsClosed: true},

		// Candle B (Sweep Candle): menyapu cPrev (Low 27766.60 < 27829.00) dan menusuk masuk ke FVG (27766.60 <= 27786.75)
		{Timestamp: now.Add(-4 * time.Hour), Open: 27905.60, High: 27946.10, Low: 27766.60, Close: 27929.60, IsClosed: true},

		// Candle C: Candle 5 (running)
		{Timestamp: now.Add(0), Open: 27929.60, High: 27933.50, Low: 27899.10, Close: 27920.00, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("expected External-to-Internal Sweep to be detected, got nil or not detected")
	}
	if sweep.Direction != "BULLISH" {
		t.Errorf("expected BULLISH direction, got %s", sweep.Direction)
	}
	if sweep.StopLoss != 27766.60 {
		t.Errorf("expected StopLoss=27766.60, got %.2f", sweep.StopLoss)
	}
	if sweep.TargetDOL != 28023.15 {
		t.Errorf("expected TargetDOL=28023.15, got %.2f", sweep.TargetDOL)
	}
}

func TestILASetupExpiredWhenCandle5Closed(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Skenario: Candle 4 (Sweep) sudah tutup, Candle 5 sudah TUTUP tanpa CISD,
	// dan sekarang kita sudah berada di Candle 6 (running).
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		// Candle A (masuk FVG)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},
		// Candle B (Sweep Candle / Candle 4)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},
		// Candle C (Candle 5 SUDAH CLOSED tanpa sweep Candle B)
		{Timestamp: now.Add(0), Open: 4190, High: 4220, Low: 4180, Close: 4210, IsClosed: true},
		// Candle D (Candle 6 running saat ini)
		{Timestamp: now.Add(4 * time.Hour), Open: 4210, High: 4215, Low: 4205, Close: 4212, IsClosed: false},
	}

	setup := eval.EvaluateILA(htfCandles, nil, "H4", "M15", "XAUUSD")
	if setup.Status != "WAIT_HTF_SWEEP" {
		t.Fatalf("expected status WAIT_HTF_SWEEP (expired), got %s", setup.Status)
	}
	if !strings.Contains(setup.NextAction, "kadaluarsa") {
		t.Errorf("expected NextAction to contain 'kadaluarsa', got %s", setup.NextAction)
	}
}

func TestZombieSweepRejected(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Sweep terjadi lebih dari 3 candle yang lalu (k < n-3), harus diabaikan total sebagai zombie.
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-28 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-24 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-20 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		{Timestamp: now.Add(-16 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true}, // Candle A
		{Timestamp: now.Add(-12 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true}, // Candle B (Sweep lama di k = n-4)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4190, High: 4220, Low: 4180, Close: 4200, IsClosed: true},  // Candle n-3
		{Timestamp: now.Add(-4 * time.Hour), Open: 4200, High: 4215, Low: 4190, Close: 4205, IsClosed: true},  // Candle n-2
		{Timestamp: now.Add(0), Open: 4205, High: 4210, Low: 4195, Close: 4205, IsClosed: false},              // Candle n-1 (running)
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep != nil && sweep.Detected {
		t.Fatalf("sweep lama (> 3 candle lalu) seharusnya ditolak sebagai zombie!")
	}
}
