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

// TestBodyBreakoutRejectedAsSweep memvalidasi kasus EURUSD H1 lilin 00:00:
// Lilin yang menembus ekor lilin acuan tetapi bodynya ditutup menembus ke luar (body breakdown/breakout)
// BUKAN merupakan sweep (rejection wick), dan WAJIB ditolak oleh evaluator.
func TestBodyBreakoutRejectedAsSweep(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// 1. Bullish FVG: 1.11566 - 1.11625
	// Candle A (cPrev): Low 1.11620 (parkir di dalam FVG)
	// Candle B: Low menusuk ke 1.11590, TAPI Close di 1.11604 (body ditutup di bawah cPrev.Low 1.11620)
	// Ini adalah BODY BREAKDOWN, bukan wick sweep!
	htfBullishBreakdown := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 1.11500, High: 1.11566, Low: 1.11480, Close: 1.11560, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 1.11560, High: 1.11640, Low: 1.11530, Close: 1.11630, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 1.11630, High: 1.11670, Low: 1.11625, Close: 1.11650, IsClosed: true}, // C3
		{Timestamp: now.Add(-8 * time.Hour), Open: 1.11650, High: 1.11680, Low: 1.11620, Close: 1.11655, IsClosed: true},  // cPrev (Low=1.11620)
		// ck: Low=1.11590 < cPrev.Low (1.11620), tapi Close=1.11604 < cPrev.Low -> Body Breakdown!
		{Timestamp: now.Add(-4 * time.Hour), Open: 1.11655, High: 1.11672, Low: 1.11590, Close: 1.11604, IsClosed: true},
		{Timestamp: now, Open: 1.11604, High: 1.11606, Low: 1.11571, Close: 1.11580, IsClosed: false},
	}

	sweepBull := eval.DetectHTFIlaSweep(htfBullishBreakdown)
	if sweepBull != nil && sweepBull.Detected {
		t.Fatalf("FAILED: Body breakdown (Close < cPrev.Low) seharusnya DITOLAK sebagai sweep!")
	}

	// 2. Bearish FVG: 1.12000 - 1.12500
	// Candle A (cPrev): High 1.12200 (parkir di dalam FVG)
	// Candle B: High menusuk ke 1.12350, TAPI Close di 1.12250 (body ditutup di atas cPrev.High 1.12200)
	// Ini adalah BODY BREAKOUT ke atas, bukan wick sweep!
	htfBearishBreakout := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 1.12600, High: 1.12650, Low: 1.12500, Close: 1.12520, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 1.12520, High: 1.12520, Low: 1.11900, Close: 1.11950, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 1.11950, High: 1.12000, Low: 1.11850, Close: 1.11900, IsClosed: true}, // C3
		{Timestamp: now.Add(-8 * time.Hour), Open: 1.11900, High: 1.12200, Low: 1.11880, Close: 1.12050, IsClosed: true},  // cPrev (High=1.12200)
		// ck: High=1.12350 > cPrev.High (1.12200), tapi Close=1.12250 > cPrev.High -> Body Breakout!
		{Timestamp: now.Add(-4 * time.Hour), Open: 1.12050, High: 1.12350, Low: 1.12000, Close: 1.12250, IsClosed: true},
		{Timestamp: now, Open: 1.12250, High: 1.12280, Low: 1.12200, Close: 1.12230, IsClosed: false},
	}

	sweepBear := eval.DetectHTFIlaSweep(htfBearishBreakout)
	if sweepBear != nil && sweepBear.Detected {
		t.Fatalf("FAILED: Body breakout (Close > cPrev.High) seharusnya DITOLAK sebagai sweep!")
	}
}

func TestILASetupExpiredWhenCandle6Closed(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Skenario: Candle 4 (Sweep) sudah tutup, Candle 5 dan Candle 6 sudah TUTUP tanpa CISD,
	// dan sekarang kita sudah berada di Candle 7 (running).
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-24 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-20 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-16 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		// Candle A (masuk FVG)
		{Timestamp: now.Add(-12 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},
		// Candle B (Sweep Candle / Candle 4)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},
		// Candle C (Candle 5 SUDAH CLOSED tanpa sweep Candle B)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4190, High: 4220, Low: 4180, Close: 4210, IsClosed: true},
		// Candle D (Candle 6 SUDAH CLOSED tanpa CISD -> Kadaluarsa!)
		{Timestamp: now.Add(0), Open: 4210, High: 4215, Low: 4205, Close: 4212, IsClosed: true},
	}

	setup := eval.EvaluateILA(htfCandles, nil, "H4", "M15", "XAUUSD")
	if setup.Status != "WAIT_HTF_SWEEP" {
		t.Fatalf("expected status WAIT_HTF_SWEEP (expired), got %s", setup.Status)
	}
	if !strings.Contains(setup.NextAction, "kadaluarsa") {
		t.Errorf("expected NextAction to contain 'kadaluarsa', got %s", setup.NextAction)
	}
}

func TestILASetupActiveInCandle6WithCandle5SL(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Skenario: Candle 4 (Sweep) tutup di 4230, Candle 5 tutup dengan High 4220,
	// dan Candle 6 sedang running dengan High 4215 (tidak sweep Candle 5).
	// Jendela eksekusi harus TETAP AKTIF, dan Stop Loss dikunci di High Candle 5 (4220)!
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		// Candle A (masuk FVG)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},
		// Candle B (Sweep Candle / Candle 4: High 4230)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},
		// Candle C (Candle 5: High 4220)
		{Timestamp: now.Add(0), Open: 4190, High: 4220, Low: 4180, Close: 4210, IsClosed: true},
		// Candle D (Candle 6 running saat ini)
		{Timestamp: now.Add(4 * time.Hour), Open: 4210, High: 4215, Low: 4205, Close: 4212, IsClosed: false},
	}

	// 1. Tanpa CISD -> Status WAIT_LTF_CISD dan Stop Loss = High Candle 5 (4220)
	setupNoCISD := eval.EvaluateILA(htfCandles, nil, "H4", "M15", "XAUUSD")
	if setupNoCISD.Status != "WAIT_LTF_CISD" {
		t.Fatalf("expected status WAIT_LTF_CISD in Candle 6, got %s", setupNoCISD.Status)
	}
	if setupNoCISD.StopLoss != 4220.0 {
		t.Errorf("expected StopLoss to be Candle 5 High (4220.0), got %.2f", setupNoCISD.StopLoss)
	}

	// 2. Dengan CISD yang terbentuk di akhir Candle 5 / Candle 6 -> Status READY_TO_ENTRY, SL = 4220.0
	ltfCandles := []indicators.Candle{
		// origin di dalam Candle 5:
		{Timestamp: now.Add(-2 * time.Hour), Open: 4205, High: 4218, Low: 4200, Close: 4215, IsClosed: true},
		{Timestamp: now.Add(-1 * time.Hour), Open: 4215, High: 4220, Low: 4210, Close: 4218, IsClosed: true},
		// break candle:
		{Timestamp: now.Add(1 * time.Hour), Open: 4218, High: 4218, Low: 4195, Close: 4200, IsClosed: true},
	}

	setupWithCISD := eval.EvaluateILA(htfCandles, ltfCandles, "H4", "M15", "XAUUSD")
	if setupWithCISD.Status != "READY_TO_ENTRY" {
		t.Fatalf("expected status READY_TO_ENTRY in Candle 6, got %s", setupWithCISD.Status)
	}
	if setupWithCISD.StopLoss != 4220.0 {
		t.Errorf("expected StopLoss to be Candle 5 High (4220.0), got %.2f", setupWithCISD.StopLoss)
	}
	if !setupWithCISD.PassedMinRR {
		t.Errorf("expected PassedMinRR to be true")
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

// TestWickPierceDoesNotMitigateFVG memvalidasi bahwa FVG TIDAK termitigasi jika tusukan ekor masih dalam toleransi spread/noise ($0.50),
// namun AKAN termitigasi jika ekor menembus jauh melebihi toleransi atau ditutup tembus body.
func TestWickPierceDoesNotMitigateFVG(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// 1. Kasus Dalam Toleransi (High 4240.38 vs Top 4240.00: selisih 38 sen <= 50 sen) -> TIDAK MITIGATED
	htfCandlesValid := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1 (Low=4240)
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3 (High=4212)
		// Candle A: Masuk FVG (High=4230)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4230, Low: 4160, Close: 4180, IsClosed: true},
		// Candle B: Ekor menusuk ke 4240.38 (38 sen di atas Top 4240), TAPI Close di 4200 (rejection kuat)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4240.38, Low: 4175, Close: 4200, IsClosed: true},
		// Candle C: Running
		{Timestamp: now.Add(0), Open: 4200, High: 4205, Low: 4195, Close: 4202, IsClosed: false},
	}

	fvgs := eval.FindFVGs(htfCandlesValid)
	if len(fvgs) == 0 {
		t.Fatalf("expected FVG to be found")
	}
	if fvgs[0].Mitigated {
		t.Fatalf("FVG seharusnya TIDAK termitigasi karena selisih 38 sen masih dalam batas toleransi spread 50 sen!")
	}

	sweep := eval.DetectHTFIlaSweep(htfCandlesValid)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("sweep dengan rejection wick seharusnya terdeteksi sah!")
	}

	// 2. Kasus Melebihi Toleransi (High 4242.00 vs Top 4240.00: selisih $2.00 > 50 sen) -> TERMITIGASI PENUH
	htfCandlesMitigated := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1 (Low=4240)
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3 (High=4212)
		// Candle A: Ekor tembus ke 4242 (> Top + tol)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4242, Low: 4160, Close: 4180, IsClosed: true},
		{Timestamp: now.Add(0), Open: 4180, High: 4185, Low: 4175, Close: 4180, IsClosed: false},
	}

	fvgsMit := eval.FindFVGs(htfCandlesMitigated)
	if len(fvgsMit) == 0 || !fvgsMit[0].Mitigated {
		t.Fatalf("FVG seharusnya TERMITIGASI jika ekor menusuk jauh melampaui toleransi!")
	}
}

// TestCISDOriginCanFormInPriorCandleRun memvalidasi bahwa origin deretan opposing leg
// yang bersambung langsung diizinkan berawal dari candle sebelum Candle 5 (di dalam Candle 4 HTF).
// Kasus riil EURUSD H4-M15: deretan naik dimulai jam 08:15 NY (Open 1.10766), mencapai puncak 09:15 NY,
// dan candle 09:30 NY belum menembus 1.10766, baru candle 09:45 NY yang menembus by body.
func TestCISDOriginCanFormInPriorCandleRun(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	sweepCandle := indicators.Candle{
		Timestamp: now.Add(-4 * time.Hour), // 05:00 NY
		High:      1.11029,
		Low:       1.10757,
		Open:      1.10946,
		Close:     1.10891,
		IsClosed:  true,
	}
	afterTime := now // 09:00 NY (Candle 5 start)

	ltfCandles := []indicators.Candle{
		// 08:00 NY: Bearish
		{Timestamp: now.Add(-60 * time.Minute), Open: 1.10875, High: 1.10934, Low: 1.10757, Close: 1.10766, IsClosed: true},
		// 08:15 NY: Bullish (TRUE ORIGIN!)
		{Timestamp: now.Add(-45 * time.Minute), Open: 1.10766, High: 1.10885, Low: 1.10761, Close: 1.10852, IsClosed: true},
		// 08:30 NY: Bullish
		{Timestamp: now.Add(-30 * time.Minute), Open: 1.10852, High: 1.10899, Low: 1.10814, Close: 1.10868, IsClosed: true},
		// 08:45 NY: Bullish
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.10868, High: 1.10898, Low: 1.10806, Close: 1.10891, IsClosed: true},
		// --- BATAS CANDLE 5 (09:00 NY) ---
		// 09:00 NY: Bullish (Lanjutan)
		{Timestamp: now, Open: 1.10891, High: 1.10967, Low: 1.10886, Close: 1.10924, IsClosed: true},
		// 09:15 NY: Bullish (Peak)
		{Timestamp: now.Add(15 * time.Minute), Open: 1.10924, High: 1.10985, Low: 1.10916, Close: 1.10961, IsClosed: true},
		// 09:30 NY: Bearish (Close 1.10825 < 1.10891 tetapi > 1.10766 -> BELUM CISD!)
		{Timestamp: now.Add(30 * time.Minute), Open: 1.10961, High: 1.10968, Low: 1.10824, Close: 1.10825, IsClosed: true},
	}

	// 1. Pada candle 09:30 NY, CISD seharusnya BELUM terdeteksi karena origin sebenarnya adalah 1.10766
	cisd1, _ := eval.DetectDynamicCISD(ltfCandles, "BEARISH", afterTime, sweepCandle)
	if cisd1 == nil {
		t.Fatalf("expected non-nil cisd object")
	}
	if cisd1.CISDLevel != 1.10766 {
		t.Errorf("expected CISDLevel to be 1.10766 (from 08:15 NY), got %.5f", cisd1.CISDLevel)
	}
	if cisd1.Detected {
		t.Errorf("candle 09:30 NY belum menembus 1.10766, seharusnya Detected=false!")
	}

	// 2. Tambahkan candle 09:45 NY (Close 1.10744 < 1.10766 -> SAH CISD!)
	ltfCandles = append(ltfCandles, indicators.Candle{
		Timestamp: now.Add(45 * time.Minute), Open: 1.10825, High: 1.10829, Low: 1.10740, Close: 1.10744, IsClosed: true,
	})

	cisd2, _ := eval.DetectDynamicCISD(ltfCandles, "BEARISH", afterTime, sweepCandle)
	if cisd2 == nil || !cisd2.Detected {
		t.Fatalf("candle 09:45 NY seharusnya memicu CISD Valid!")
	}
	if cisd2.EntryPrice != 1.10744 {
		t.Errorf("expected EntryPrice=1.10744, got %.5f", cisd2.EntryPrice)
	}
}
