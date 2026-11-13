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

// TestILASetupLanjutDiCandle6Running mengunci jendela eksekusi dinamis ILA 2.0:
// setup TIDAK lagi kadaluarsa seketika Candle 5 berganti ke Candle 6. Jendela
// pencarian CISD tetap terbuka selama Candle 6 masih berjalan.
//
// Alasan nyata (EURUSD H4-M15): candle M15 08:45 NY ditutup 09:00 NY menembus origin
// CISD by body, tetapi feed H4 09:00:00 NY sudah memunculkan Candle 6 running,
// sehingga setup dibatalkan tepat di detik entry seharusnya sah.
func TestILASetupLanjutDiCandle6Running(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Candle 4 (Sweep) closed, Candle 5 SUDO closed tanpa CISD, sekarang di Candle 6 running.
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},  // Candle A (masuk FVG)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},  // Candle 4 (sweep)
		{Timestamp: now.Add(0), Open: 4190, High: 4220, Low: 4180, Close: 4210, IsClosed: true},               // Candle 5 closed
		{Timestamp: now.Add(4 * time.Hour), Open: 4210, High: 4215, Low: 4205, Close: 4212, IsClosed: false},  // Candle 6 running
	}

	setup := eval.EvaluateILA(htfCandles, nil, "H4", "M15", "XAUUSD")
	if setup.Status == "WAIT_HTF_SWEEP" {
		t.Fatalf("FAILED: setup tidak boleh kadaluarsa saat Candle 6 masih berjalan, dapat %s (%s)", setup.Status, setup.NextAction)
	}
	if setup.StopLoss != 4220 {
		t.Fatalf("FAILED: SL harus dikunci di High Candle 5 (4220), dapat %.2f", setup.StopLoss)
	}
}

// TestILASetupExpiredWhenCandle6Closed mengunci batas akhir jendela: setelah
// Candle 6 CLOSED resmi tanpa CISD, setup barulah hangus.
func TestILASetupExpiredWhenCandle6Closed(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true},  // Candle A
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4230, Low: 4175, Close: 4190, IsClosed: true},  // Candle 4 (sweep)
		{Timestamp: now.Add(0), Open: 4190, High: 4220, Low: 4180, Close: 4210, IsClosed: true},               // Candle 5 closed
		{Timestamp: now.Add(4 * time.Hour), Open: 4210, High: 4215, Low: 4205, Close: 4212, IsClosed: true},   // Candle 6 CLOSED
	}

	setup := eval.EvaluateILA(htfCandles, nil, "H4", "M15", "XAUUSD")
	if setup.Status != "WAIT_HTF_SWEEP" {
		t.Fatalf("expected status WAIT_HTF_SWEEP (expired setelah Candle 6 closed), got %s", setup.Status)
	}
	if !strings.Contains(setup.NextAction, "kadaluarsa") {
		t.Errorf("expected NextAction mengandung 'kadaluarsa', got %s", setup.NextAction)
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

// TestDoubleFVGAllowsCandle3Sweep memvalidasi aturan Double / Stacked FVG:
// pada dua displacement bearish yang bersambung, Candle 3 milik FVG terbaru SAH
// disapu oleh candle berikutnya saat harga retrace masuk ke area FVG.
// Tanpa dukungan Double FVG, kasus ini ditolak oleh guard Candle 3.
func TestDoubleFVGAllowsCandle3Sweep(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// Dua bearish FVG bersambung (Candle1 FVG-B == Candle3 FVG-A).
	// Sweep terjadi TEPAT pada Candle 3 FVG terbaru (k-1 == Candle3Index),
	// satu-satunya kondisi yang diloloskan oleh aturan Double FVG.
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-5 * time.Hour), Open: 4200, High: 4205, Low: 4196, Close: 4200, IsClosed: true}, // C1 FVG-A (Low 4196)
		{Timestamp: now.Add(-4 * time.Hour), Open: 4200, High: 4200, Low: 4150, Close: 4155, IsClosed: true}, // C2 FVG-A displacement turun
		{Timestamp: now.Add(-3 * time.Hour), Open: 4155, High: 4178, Low: 4180, Close: 4182, IsClosed: true}, // C3 FVG-A (High 4178) -> FVG-A: 4178-4196
		{Timestamp: now.Add(-2 * time.Hour), Open: 4182, High: 4182, Low: 4130, Close: 4135, IsClosed: true}, // C2 FVG-B displacement turun
		{Timestamp: now.Add(-1 * time.Hour), Open: 4135, High: 4170, Low: 4172, Close: 4175, IsClosed: true}, // C3 FVG-B (High 4170) -> FVG-B: 4170-4180
		// ck menyapu High Candle 3 FVG-B (4179 > 4170) dan ditutup kembali di bawahnya (4169 <= 4170).
		{Timestamp: now.Add(-30 * time.Minute), Open: 4175, High: 4179, Low: 4168, Close: 4169, IsClosed: true},
		{Timestamp: now, Open: 4169, High: 4172, Low: 4167, Close: 4170, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("FAILED: Double FVG seharusnya mengizinkan sweep terhadap Candle 3 FVG terbaru!")
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("arah sweep seharusnya BEARISH, dapat %s", sweep.Direction)
	}
}

// TestDoubleFVGWithMitigatedNeighbourRejected memastikan aturan Double FVG hanya aktif
// bila FVG sebelumnya masih aktif (belum termitigasi) dan pembentuknya bersambung.
// FVG sebelumnya yang sudah termitigasi berarti harganya sudah melewati area tersebut,
// sehingga Candle 3-nya tidak boleh dipakai sebagai alasan meloloskan sweep.
func TestDoubleFVGWithMitigatedNeighbourRejected(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// FVG-A terbentuk lalu TERMITIGASI (candle idx3 menembus atap 4296),
	// setelah itu FVG-B terbentuk dan Candle 3-nya disapu.
	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-8 * time.Hour), Open: 4300, High: 4305, Low: 4296, Close: 4300, IsClosed: true}, // C1 FVG-A (Low 4296)
		{Timestamp: now.Add(-7 * time.Hour), Open: 4300, High: 4300, Low: 4250, Close: 4255, IsClosed: true}, // C2 FVG-A displacement turun
		{Timestamp: now.Add(-6 * time.Hour), Open: 4255, High: 4270, Low: 4260, Close: 4265, IsClosed: true}, // C3 FVG-A (High 4270) -> FVG-A: 4270-4296
		{Timestamp: now.Add(-5 * time.Hour), Open: 4265, High: 4300, Low: 4264, Close: 4298, IsClosed: true}, // menembus atap FVG-A -> FVG-A TERMITIGASI
		{Timestamp: now.Add(-4 * time.Hour), Open: 4298, High: 4298, Low: 4240, Close: 4245, IsClosed: true}, // C2 FVG-B displacement turun
		{Timestamp: now.Add(-3 * time.Hour), Open: 4245, High: 4262, Low: 4250, Close: 4255, IsClosed: true}, // C3 FVG-B -> FVG-B: 4262-4264
		{Timestamp: now.Add(-2 * time.Hour), Open: 4258, High: 4260, Low: 4256, Close: 4258, IsClosed: true}, // cPrev parkir di FVG-B
		{Timestamp: now.Add(-1 * time.Hour), Open: 4258, High: 4262, Low: 4255, Close: 4256, IsClosed: true}, // ck menyapu Candle 3 FVG-B
		{Timestamp: now, Open: 4256, High: 4259, Low: 4253, Close: 4254, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep != nil && sweep.Detected {
		t.Fatalf("FAILED: FVG sebelumnya yang sudah termitigasi tidak boleh memicu aturan Double FVG!")
	}
}

// TestSingleFVGStillRejectsCandle3Sweep adalah penjaga regresi untuk perbaikan
// Double FVG: FVG tunggal tanpa pasangan bersambung TETAP menolak sweep Candle 3.
func TestSingleFVGStillRejectsCandle3Sweep(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-4 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1
		{Timestamp: now.Add(-3 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2
		{Timestamp: now.Add(-2 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3 (High=4212)
		{Timestamp: now.Add(-1 * time.Hour), Open: 4160, High: 4220, Low: 4160, Close: 4180, IsClosed: true}, // menyapu C3 tanpa pasangan Double FVG
		{Timestamp: now, Open: 4180, High: 4215, Low: 4175, Close: 4190, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep != nil && sweep.Detected {
		t.Fatalf("FAILED: FVG tunggal tetap harus menolak sweep terhadap Candle 3!")
	}
}

// TestDOLBreachedDoesNotInvalidateSweep memvalidasi Skenario 3:
// harga yang menembus DOL (SSL) setelah sweep tidak boleh membatalkan siklus sweep.
func TestDOLBreachedDoesNotInvalidateSweep(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	htfCandles := []indicators.Candle{
		{Timestamp: now.Add(-6 * time.Hour), Open: 4100, High: 4105, Low: 4098, Close: 4102, IsClosed: true},   // C1 (Low=4098)
		{Timestamp: now.Add(-5 * time.Hour), Open: 4102, High: 4102, Low: 4070, Close: 4075, IsClosed: true},   // C2 displacement turun
		{Timestamp: now.Add(-4 * time.Hour), Open: 4075, High: 4096, Low: 4092, Close: 4094, IsClosed: true},   // C3 (High=4096) -> Bearish FVG 4096-4098
		{Timestamp: now.Add(-3 * time.Hour), Open: 4094, High: 4097, Low: 4093, Close: 4095, IsClosed: true},   // cPrev (High=4097, parkir di FVG)
		{Timestamp: now.Add(-2 * time.Hour), Open: 4095, High: 4097.5, Low: 4092, Close: 4093, IsClosed: true}, // ck sweep wick, close di bawah cPrev.High
		// Candle berikutnya menembus jauh ke bawah DOL (SSL = bottom FVG 4096 => di bawahnya).
		{Timestamp: now.Add(-1 * time.Hour), Open: 4093, High: 4094, Low: 4040, Close: 4042, IsClosed: true},
		{Timestamp: now, Open: 4042, High: 4045, Low: 4035, Close: 4038, IsClosed: false},
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("FAILED: penembusan DOL tidak boleh membatalkan sweep (Skenario 3 DOL Breached)!")
	}
}

// TestCISDOriginCanFormInPriorCandleRun mengunci perbaikan dari PR hermes (00a7f44).
//
// Bug yang diperbaiki: penelusuran origin CISD dipotong di awal Candle 5. Bila deretan
// candle lawan sudah mulai di Candle 4 (lilin sweep HTF) dan bersambung turun/naiK menuju
// ekstrim di Candle 5, origin yang benar ikut terbuang. Akibatnya level CISD diambil dari
// candle pertama Candle 5 sehingga bergeser lebih tinggi (BEARISH) atau lebih rendah
// (BULLISH), dan entry terjadi terlalu dini.
func TestCISDOriginCanFormInPriorCandleRun(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	sweepCandle := indicators.Candle{
		Timestamp: now.Add(-4 * time.Hour),
		High:      1.11029,
		Low:       1.10757,
		Open:      1.10946,
		Close:     1.10891,
		IsClosed:  true,
	}
	afterTime := now // awal Candle 5

	ltfCandles := []indicators.Candle{
		// 08:00 NY: bearish, menyapu low lilin sweep
		{Timestamp: now.Add(-60 * time.Minute), Open: 1.10875, High: 1.10934, Low: 1.10757, Close: 1.10766, IsClosed: true},
		// 08:15 NY: bullish — ORIGIN sebenarnya, berada di siklus Candle 4
		{Timestamp: now.Add(-45 * time.Minute), Open: 1.10766, High: 1.10885, Low: 1.10761, Close: 1.10852, IsClosed: true},
		{Timestamp: now.Add(-30 * time.Minute), Open: 1.10852, High: 1.10899, Low: 1.10814, Close: 1.10868, IsClosed: true},
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.10868, High: 1.10898, Low: 1.10806, Close: 1.10891, IsClosed: true},
		// --- batas Candle 5 (09:00 NY) ---
		{Timestamp: now, Open: 1.10891, High: 1.10967, Low: 1.10886, Close: 1.10924, IsClosed: true},
		{Timestamp: now.Add(15 * time.Minute), Open: 1.10924, High: 1.10985, Low: 1.10916, Close: 1.10961, IsClosed: true},
		// 09:30 NY: bearish. Close 1.10825 < 1.10891 tapi BELUM menembus origin 1.10766.
		{Timestamp: now.Add(30 * time.Minute), Open: 1.10961, High: 1.10968, Low: 1.10824, Close: 1.10825, IsClosed: true},
	}

	cisd1, _ := eval.DetectDynamicCISD(ltfCandles, "BEARISH", afterTime, sweepCandle)
	if cisd1 == nil {
		t.Fatalf("expected non-nil cisd object")
	}
	if cisd1.CISDLevel != 1.10766 {
		t.Errorf("CISDLevel = %.5f; ingin 1.10766 (origin 08:15 NY, bukan 09:00 NY)", cisd1.CISDLevel)
	}
	if cisd1.Detected {
		t.Errorf("candle 09:30 NY belum menembus 1.10766; seharusnya Detected=false")
	}

	// 09:45 NY menembus 1.10766 -> CISD sah.
	ltfCandles = append(ltfCandles, indicators.Candle{
		Timestamp: now.Add(45 * time.Minute), Open: 1.10825, High: 1.10829, Low: 1.10740, Close: 1.10744, IsClosed: true,
	})

	cisd2, _ := eval.DetectDynamicCISD(ltfCandles, "BEARISH", afterTime, sweepCandle)
	if cisd2 == nil || !cisd2.Detected {
		t.Fatalf("candle 09:45 NY seharusnya memicu CISD sah")
	}
	if cisd2.EntryPrice != 1.10744 {
		t.Errorf("EntryPrice = %.5f; ingin 1.10744", cisd2.EntryPrice)
	}
}

// TestCISDOriginBullishCanFormInPriorCandleRun adalah cermin BULLISH dari test di atas.
// PR hermes hanya menguji sisi BEARISH; kode BULLISH terpisah, jadi ikut dikunci.
//
// Arah BULLISH: cari low terendah di Candle 5, lalu telusuri MUNDUR deretan candle
// bearish dan pakai Open candle bearish pertama sebagai level CISD. Deretan itu boleh
// berawal di Candle 4 (siklus sweep). Kalau dipotong di awal Candle 5, origin bergeser
// ke candle bearish pertama DI DALAM Candle 5 sehingga level CISD lebih rendah dan
// CISD terpicu terlalu dini.
func TestCISDOriginBullishCanFormInPriorCandleRun(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	sweepCandle := indicators.Candle{
		Timestamp: now.Add(-4 * time.Hour),
		High:      1.11029,
		Low:       1.10757,
		Open:      1.10946,
		Close:     1.10891,
		IsClosed:  true,
	}

	ltfCandles := []indicators.Candle{
		// 08:00 NY: bearish besar, menyapu low lilin sweep. Origin sebenarnya = 1.10980.
		{Timestamp: now.Add(-60 * time.Minute), Open: 1.10980, High: 1.10990, Low: 1.10760, Close: 1.10810, IsClosed: true},
		{Timestamp: now.Add(-45 * time.Minute), Open: 1.10810, High: 1.10840, Low: 1.10770, Close: 1.10790, IsClosed: true},
		{Timestamp: now.Add(-30 * time.Minute), Open: 1.10790, High: 1.10820, Low: 1.10760, Close: 1.10780, IsClosed: true},
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.10780, High: 1.10800, Low: 1.10758, Close: 1.10770, IsClosed: true},
		// --- batas Candle 5 (09:00 NY) ---
		{Timestamp: now, Open: 1.10770, High: 1.10800, Low: 1.10758, Close: 1.10790, IsClosed: true},
		// 09:15 NY: bullish, low terdalam Candle 5 = 1.10758
		{Timestamp: now.Add(15 * time.Minute), Open: 1.10790, High: 1.10800, Low: 1.10758, Close: 1.10775, IsClosed: true},
		// 09:30 NY: bullish. Close 1.10900 > 1.10810 tapi BELUM menembus origin 1.10980.
		{Timestamp: now.Add(30 * time.Minute), Open: 1.10775, High: 1.10905, Low: 1.10770, Close: 1.10900, IsClosed: true},
	}

	cisd1, _ := eval.DetectDynamicCISD(ltfCandles, "BULLISH", now, sweepCandle)
	if cisd1 == nil {
		t.Fatalf("expected non-nil cisd object")
	}
	if cisd1.CISDLevel != 1.10980 {
		t.Errorf("CISDLevel = %.5f; ingin 1.10980 (origin 08:00 NY, bukan candle bearish pertama di Candle 5)", cisd1.CISDLevel)
	}
	if cisd1.Detected {
		t.Errorf("candle 09:30 NY belum menembus 1.10980; seharusnya Detected=false")
	}

	// 09:45 NY menembus 1.10980 -> CISD sah.
	ltfCandles = append(ltfCandles, indicators.Candle{
		Timestamp: now.Add(45 * time.Minute), Open: 1.10900, High: 1.11000, Low: 1.10895, Close: 1.10990, IsClosed: true,
	})

	cisd2, _ := eval.DetectDynamicCISD(ltfCandles, "BULLISH", now, sweepCandle)
	if cisd2 == nil || !cisd2.Detected {
		t.Fatalf("candle 09:45 NY seharusnya memicu CISD sah")
	}
	if cisd2.EntryPrice != 1.10990 {
		t.Errorf("EntryPrice = %.5f; ingin 1.10990", cisd2.EntryPrice)
	}
}

// TestCISDOriginStillStopsAtSweepCandle memastikan perbaikan tidak membuka terlalu jauh:
// penelusuran tidak boleh menyeberang ke sebelum lilin sweep HTF (Candle 4).
func TestCISDOriginStillStopsAtSweepCandle(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	sweepCandle := indicators.Candle{
		Timestamp: now.Add(-4 * time.Hour),
		High:      1.11029,
		Low:       1.10757,
		Open:      1.10946,
		Close:     1.10891,
		IsClosed:  true,
	}

	ltfCandles := []indicators.Candle{
		// SEBELUM lilin sweep: bearish panjang. Bila terpakai, level CISD akan 1.11200.
		{Timestamp: now.Add(-5 * time.Hour), Open: 1.11200, High: 1.11210, Low: 1.10600, Close: 1.10610, IsClosed: true},
		// Sesudah lilin sweep: deretan bearish berakhir di low Candle 5.
		{Timestamp: now.Add(-3 * time.Hour), Open: 1.10960, High: 1.10970, Low: 1.10900, Close: 1.10920, IsClosed: true},
		{Timestamp: now.Add(-2 * time.Hour), Open: 1.10920, High: 1.10930, Low: 1.10850, Close: 1.10870, IsClosed: true},
		{Timestamp: now, Open: 1.10870, High: 1.10890, Low: 1.10800, Close: 1.10830, IsClosed: true},
	}

	cisd, _ := eval.DetectDynamicCISD(ltfCandles, "BULLISH", now, sweepCandle)
	if cisd == nil {
		t.Fatalf("expected non-nil cisd object")
	}
	// Origin wajib deretan bearish sesudah sweep (1.10960), bukan candle 1.11200 di siklus lama.
	if cisd.CISDLevel != 1.10960 {
		t.Errorf("CISDLevel = %.5f; ingin 1.10960 (tidak menyeberang sebelum lilin sweep)", cisd.CISDLevel)
	}
}

// TestWickPierceWithinToleranceDoesNotMitigateFVG mengunci toleransi spread/noise
// pada mitigasi FVG (diambil dari PR hermes, c98a206).
//
// Bug yang dicegah: sentuhan ekor yang hanya melewati atap/dasar FVG sejauh
// noise quote broker sudah dianggap menutup FVG. Akibatnya FVG sah hilang dari
// pemantauan hanya karena selisih spread, dan setup yang seharusnya terdeteksi
// tidak pernah muncul. Toleransi tetap terbatas: tembusan yang jelas melampaui
// spread TETAP menghanguskan FVG.
func TestWickPierceWithinToleranceDoesNotMitigateFVG(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// --- 1. BEARISH, tusukan DALAM toleransi (38 sen <= 50 sen): TIDAK mitigated ---
	bearishDalamToleransi := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true}, // C1 (Low 4240 = Top)
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true}, // C2 displacement turun
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true}, // C3 (High 4212 = Bottom)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4230, Low: 4160, Close: 4180, IsClosed: true},
		// Ekor menusuk ke 4240.38, hanya 38 sen di atas Top.
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4240.38, Low: 4175, Close: 4200, IsClosed: true},
		{Timestamp: now, Open: 4200, High: 4205, Low: 4195, Close: 4202, IsClosed: false},
	}

	fvgs := eval.FindFVGs(bearishDalamToleransi)
	if len(fvgs) == 0 {
		t.Fatalf("expected FVG to be found")
	}
	if fvgs[0].Mitigated {
		t.Fatalf("FVG bearish TIDAK boleh termitigasi: tusukan 38 sen masih dalam toleransi 50 sen")
	}
	if sweep := eval.DetectHTFIlaSweep(bearishDalamToleransi); sweep == nil || !sweep.Detected {
		t.Fatalf("sweep dengan rejection wick dalam toleransi seharusnya sah terdeteksi")
	}

	// --- 2. BEARISH, tusukan MELAMPAUI toleransi ($2 > 50 sen): TETAP mitigated ---
	bearishLewatToleransi := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4250, High: 4260, Low: 4240, Close: 4245, IsClosed: true},
		{Timestamp: now.Add(-16 * time.Hour), Open: 4245, High: 4245, Low: 4150, Close: 4155, IsClosed: true},
		{Timestamp: now.Add(-12 * time.Hour), Open: 4155, High: 4212, Low: 4150, Close: 4160, IsClosed: true},
		{Timestamp: now.Add(-8 * time.Hour), Open: 4160, High: 4242, Low: 4160, Close: 4180, IsClosed: true},
		{Timestamp: now, Open: 4180, High: 4185, Low: 4175, Close: 4180, IsClosed: false},
	}

	fvgsMit := eval.FindFVGs(bearishLewatToleransi)
	if len(fvgsMit) == 0 || !fvgsMit[0].Mitigated {
		t.Fatalf("FVG bearish HARUS termitigasi: tusukan $2 melampaui toleransi 50 sen")
	}
}

// TestWickPierceToleranceBullish adalah cermin BULLISH dari toleransi serupa.
// PR hermes hanya menguji sisi BEARISH; kode mitigasi BULLISH terpisah.
func TestWickPierceToleranceBullish(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// --- 1. BULLISH, tusukan 38 sen di bawah Bottom (dalam toleransi): TIDAK mitigated ---
	dalamToleransi := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4150, High: 4160, Low: 4150, Close: 4155, IsClosed: true}, // C1 (High 4160 = Bottom)
		{Timestamp: now.Add(-16 * time.Hour), Open: 4155, High: 4250, Low: 4155, Close: 4245, IsClosed: true}, // C2 displacement naik
		{Timestamp: now.Add(-12 * time.Hour), Open: 4245, High: 4290, Low: 4250, Close: 4285, IsClosed: true}, // C3 (Low 4250 = Top)
		{Timestamp: now.Add(-8 * time.Hour), Open: 4260, High: 4265, Low: 4170, Close: 4180, IsClosed: true},
		// Ekor menusuk ke 4159.62, hanya 38 sen di bawah Bottom 4160.
		{Timestamp: now.Add(-4 * time.Hour), Open: 4180, High: 4185, Low: 4159.62, Close: 4175, IsClosed: true},
		{Timestamp: now, Open: 4175, High: 4180, Low: 4170, Close: 4178, IsClosed: false},
	}

	fvgs := eval.FindFVGs(dalamToleransi)
	if len(fvgs) == 0 {
		t.Fatalf("expected FVG to be found")
	}
	if fvgs[0].Mitigated {
		t.Fatalf("FVG bullish TIDAK boleh termitigasi: tusukan 38 sen masih dalam toleransi 50 sen")
	}

	// --- 2. BULLISH, tusukan $2 di bawah Bottom: TETAP mitigated ---
	lewatToleransi := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 4150, High: 4160, Low: 4150, Close: 4155, IsClosed: true},
		{Timestamp: now.Add(-16 * time.Hour), Open: 4155, High: 4250, Low: 4155, Close: 4245, IsClosed: true},
		{Timestamp: now.Add(-12 * time.Hour), Open: 4245, High: 4290, Low: 4250, Close: 4285, IsClosed: true},
		{Timestamp: now.Add(-8 * time.Hour), Open: 4260, High: 4265, Low: 4158, Close: 4180, IsClosed: true},
		{Timestamp: now, Open: 4180, High: 4185, Low: 4170, Close: 4178, IsClosed: false},
	}

	fvgsMit := eval.FindFVGs(lewatToleransi)
	if len(fvgsMit) == 0 || !fvgsMit[0].Mitigated {
		t.Fatalf("FVG bullish HARUS termitigasi: tusukan $2 melampaui toleransi 50 sen")
	}
}

// TestFVGTolerancePerInstrumen memastikan ambang mengikuti skala harga.
// Satu angka tetap salah: 50 sen berarti $0.50 pada XAUUSD tetapi 500 pips
// pada EURUSD, sehingga toleransi harus dibedakan per kelas instrumen.
func TestFVGTolerancePerInstrumen(t *testing.T) {
	kasus := []struct {
		nama   string
		harga  float64
		ingin  float64
		kurang string
	}{
		{"Forex EURUSD", 1.0850, 0.00010, "1 pip (5 desimal)"},
		{"Forex GBPUSD", 1.2650, 0.00010, "1 pip (5 desimal)"},
		{"Forex USDJPY", 152.30, 0.01, "1 pip (3 desimal)"},
		{"Forex GBPJPY", 189.50, 0.01, "1 pip (3 desimal)"},
		{"Gold XAUUSD", 4240.0, 0.50, "50 sen"},
		{"Indeks USTEC", 20500.0, 2.00, "2 poin"},
		{"Indeks US100", 21000.0, 2.00, "2 poin"},
	}

	for _, k := range kasus {
		if dapat := getFVGTolerance(k.harga); dapat != k.ingin {
			t.Errorf("%s (harga %.4f): toleransi = %v; ingin %v (%s)", k.nama, k.harga, dapat, k.ingin, k.kurang)
		}
	}
}

// TestForexToleranceIsOnePip memastikan toleransi forex benar-benar 1 pip,
// bukan 50 sen yang akan melonggarkan EURUSD secara berlebihan.
func TestForexToleranceIsOnePip(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	basis := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), Open: 1.0850, High: 1.0860, Low: 1.0850, Close: 1.0855, IsClosed: true}, // C1 (Low 1.0850 = Top)
		{Timestamp: now.Add(-16 * time.Hour), Open: 1.0855, High: 1.0855, Low: 1.0800, Close: 1.0805, IsClosed: true}, // C2 displacement turun
		{Timestamp: now.Add(-12 * time.Hour), Open: 1.0805, High: 1.0820, Low: 1.0800, Close: 1.0815, IsClosed: true}, // C3 (High 1.0820 = Bottom)
	}

	// 1. Tusukan 0.5 pip (dalam toleransi): tidak mitigated.
	dalam := append(append([]indicators.Candle{}, basis...),
		indicators.Candle{Timestamp: now.Add(-8 * time.Hour), Open: 1.0815, High: 1.08505, Low: 1.0810, Close: 1.0820, IsClosed: true},
		indicators.Candle{Timestamp: now, Open: 1.0820, High: 1.0825, Low: 1.0815, Close: 1.0822, IsClosed: false},
	)
	fvgsDalam := eval.FindFVGs(dalam)
	if len(fvgsDalam) == 0 {
		t.Fatalf("expected FVG to be found")
	}
	if fvgsDalam[0].Mitigated {
		t.Fatalf("EURUSD: tusukan 0.5 pip masih dalam toleransi 1 pip, tidak boleh termitigasi")
	}

	// 2. Tusukan 1.5 pip (melampaui toleransi): mitigated.
	lewat := append(append([]indicators.Candle{}, basis...),
		indicators.Candle{Timestamp: now.Add(-8 * time.Hour), Open: 1.0815, High: 1.08515, Low: 1.0810, Close: 1.0820, IsClosed: true},
		indicators.Candle{Timestamp: now, Open: 1.0820, High: 1.0825, Low: 1.0815, Close: 1.0822, IsClosed: false},
	)
	fvgsLewat := eval.FindFVGs(lewat)
	if len(fvgsLewat) == 0 || !fvgsLewat[0].Mitigated {
		t.Fatalf("EURUSD: tusukan 1.5 pip melampaui toleransi 1 pip, harus termitigasi")
	}
}

// TestXAUUSDRegresiSweepFVGSebelumnya mengunci kasus nyata XAUUSD H4 (12-13 Nov).
//
// Data riil (jam WIB):
//
//	#53 14:00  O=3965.88 H=3972.51 L=3924.74 C=3954.01
//	#54 18:00  O=3954.01 H=3955.51 L=3866.33 C=3870.07
//	#55 22:00  O=3870.07 H=3887.64 L=3852.64 C=3882.88
//	#56 02:00  O=3882.88 H=3886.93 L=3872.16 C=3877.36
//	#57 06:00  O=3877.36 H=3916.74 L=3875.20 C=3915.86
//	#58 10:00  O=3915.86 H=3923.20 L=3888.47 C=3900.61
//	#59 14:00  O=3900.61 H=3905.50 L=3889.81 C=3896.15 (running)
//
// FVG-BEARISH 3887.64-3924.74 dibentuk C1=#53, C2=#54, C3=#55.
// Candle #58 menyapu ekor #57 di dalam area FVG itu -> sweep BEARISH SAH.
// Namun #58 SEKALIGUS membentuk FVG-BULLISH 3886.93-3888.47.
//
// Evaluator lama hanya memeriksa FVG aktif TERBARU (yang bullish), sehingga sweep
// bearish yang sah diabaikan dan XAUUSD tidak pernah memunculkan alert SELL.
// Evaluator harus mau memeriksa FVG sebelumnya bila FVG terbaru belum memberi konfirmasi.
func TestXAUUSDRegresiSweepFVGSebelumnya(t *testing.T) {
	eval := NewEvaluator()
	base := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)

	candle := func(jam int, o, h, l, c float64) indicators.Candle {
		return indicators.Candle{
			Timestamp: base.Add(time.Duration(jam) * time.Hour),
			Open:      o, High: h, Low: l, Close: c, IsClosed: true,
		}
	}

	htfCandles := []indicators.Candle{
		candle(0, 3965.88, 3972.51, 3924.74, 3954.01),  // #53 C1 FVG Bearish
		candle(4, 3954.01, 3955.51, 3866.33, 3870.07),  // #54 C2 displacement turun
		candle(8, 3870.07, 3887.64, 3852.64, 3882.88),  // #55 C3 -> FVG Bearish 3887.64-3924.74
		candle(12, 3882.88, 3886.93, 3872.16, 3877.36), // #56
		candle(16, 3877.36, 3916.74, 3875.20, 3915.86), // #57 cPrev: High 3916.74 parkir di dalam FVG
		candle(20, 3915.86, 3923.20, 3888.47, 3900.61), // #58 ck: sweep High #57 sekaligus bentuk FVG Bullish
	}

	running := indicators.Candle{
		Timestamp: base.Add(24 * time.Hour), Open: 3900.61, High: 3905.50,
		Low: 3889.81, Close: 3896.15, IsClosed: false, // #59 masih running
	}
	htfCandles = append(htfCandles, running)

	// Pastikan prasyarat data benar: FVG bearish 3887.64-3924.74 harus terbentuk.
	fvgs := eval.FindFVGs(htfCandles)
	var adaBearish bool
	for _, f := range fvgs {
		if f.Type == FVGBearish && f.Bottom == 3887.64 && f.Top == 3924.74 {
			adaBearish = true
			if f.Mitigated {
				t.Fatalf("prasyarat salah: FVG bearish 3887.64-3924.74 seharusnya belum termitigasi")
			}
		}
	}
	if !adaBearish {
		t.Fatalf("prasyarat salah: FVG bearish 3887.64-3924.74 tidak terbentuk dari data uji")
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil {
		t.Fatalf("FAILED: sweep nil; seharusnya terdeteksi BEARISH dari FVG sebelumnya")
	}
	if !sweep.Detected {
		t.Fatalf("FAILED: sweep bearish sah dari FVG sebelumnya tidak terdeteksi (detected=false, detail: %s)", sweep.Details)
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("FAILED: arah sweep seharusnya BEARISH, dapat %s", sweep.Direction)
	}
	if sweep.SweepCandleIndex != 5 {
		t.Fatalf("FAILED: sweep seharusnya terjadi di candle #58 (index 5), dapat index %d", sweep.SweepCandleIndex)
	}
	if sweep.FVG == nil || sweep.FVG.Top != 3924.74 {
		t.Fatalf("FAILED: sweep harus mengacu ke FVG bearish 3887.64-3924.74")
	}
}

// TestDuaFVGSweepTerlamaMenang mengunci aturan prioritas ILA 2.0: ketika dua FVG
// aktif sama-sama memberi sweep SAH, yang dieksekusi adalah sweep yang terjadi
// LEBIH DULU. Sesuai keputusan aturan: "sell yang terjadi duluan dieksekusi,
// buy-nya dilewati, kecuali yang sell sudah stopped out".
//
// Susunan (n=10, filter sweep zombie hanya mengizinkan k >= n-3 = 7):
//
//	FVG BEARISH A = 1980..1998 (C1=3, C2=4, C3=5) -> disapu di candle idx7  (lebih awal)
//	FVG BULLISH B = 1980..1985 (C1=5, C2=6, C3=7) -> disapu di candle idx9  (lebih akhir)
//
// Keduanya berbagi candle idx5 (C3 A sekaligus C1 B), formasi displacement
// bersambung yang sah. Engine harus memilih BEARISH di idx7, bukan BULLISH di idx9.
func TestDuaFVGSweepTerlamaMenang(t *testing.T) {
	eval := NewEvaluator()
	base := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	mk := func(jam int, o, h, l, c float64) indicators.Candle {
		return indicators.Candle{Timestamp: base.Add(time.Duration(jam) * time.Hour), Open: o, High: h, Low: l, Close: c, IsClosed: true}
	}

	htfCandles := []indicators.Candle{
		mk(0, 2010, 2015, 2000, 2008),  // idx0
		mk(4, 2008, 2010, 1990, 1994),  // idx1
		mk(8, 1994, 1998, 1990, 1995),  // idx2
		mk(12, 1995, 2005, 1998, 2000), // idx3 C1 FVG A
		mk(16, 2000, 2005, 1960, 1965), // idx4 C2 FVG A (displacement turun)
		mk(20, 1965, 1980, 1975, 1978), // idx5 C3 FVG A (High 1980) sekaligus C1 FVG B
		mk(24, 1978, 1990, 1981, 1987), // idx6 cPrev sweep A
		mk(28, 1987, 1994, 1985, 1989), // idx7 SWEEP BEARISH A (High 1994 > 1990, Close 1989 <= 1990)
		mk(32, 1989, 1993, 1985, 1991), // idx8 cPrev sweep B
		mk(36, 1991, 1993, 1983, 1990), // idx9 SWEEP BULLISH B (Low 1983 < 1985, Close 1990 >= 1985)
	}

	// Prasyarat: dua FVG aktif harus benar-benar terbentuk dari data ini.
	fvgs := eval.FindFVGs(htfCandles)
	var adaA, adaB bool
	for _, f := range fvgs {
		if f.Mitigated {
			continue
		}
		if f.Type == FVGBearish && f.Bottom == 1980 && f.Top == 1998 {
			adaA = true
		}
		if f.Type == FVGBullish && f.Bottom == 1980 && f.Top == 1985 {
			adaB = true
		}
	}
	if !adaA || !adaB {
		t.Fatalf("prasyarat salah: FVG bearish A=%v FVG bullish B=%v, keduanya harus aktif", adaA, adaB)
	}

	// Prasyarat: kedua FVG benar-benar menghasilkan sweep sah saat diuji terpisah.
	for _, f := range fvgs {
		if f.Mitigated {
			continue
		}
		s := eval.detectSweepPadaFVG(htfCandles, fvgs, f, false)
		if s == nil || !s.Detected {
			t.Fatalf("prasyarat salah: FVG %s %.2f-%.2f seharusnya punya sweep sah", f.Type, f.Bottom, f.Top)
		}
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("FAILED: seharusnya ada sweep sah terdeteksi")
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("FAILED: sweep bearish (idx7) terjadi lebih dulu dan harus dipilih; dapat %s (idx %d)",
			sweep.Direction, sweep.SweepCandleIndex)
	}
	if sweep.SweepCandleIndex != 7 {
		t.Fatalf("FAILED: candle sweep seharusnya idx7, dapat idx %d", sweep.SweepCandleIndex)
	}
}

// TestFVGSebelumnyaBolehBerbagiCandle menjaga agar FVG sebelumnya yang berbagi
// candle dengan FVG terbaru (C3 lama == C1 baru) tetap dianggap berurutan sah,
// bukan dilewati sebagai FVG usang.
func TestFVGSebelumnyaBolehBerbagiCandle(t *testing.T) {
	eval := NewEvaluator()
	base := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	mk := func(jam int, o, h, l, c float64) indicators.Candle {
		return indicators.Candle{Timestamp: base.Add(time.Duration(jam) * time.Hour), Open: o, High: h, Low: l, Close: c, IsClosed: true}
	}

	htfCandles := []indicators.Candle{
		mk(0, 2010, 2015, 2000, 2008),
		mk(4, 2008, 2010, 1990, 1994),
		mk(8, 1994, 1998, 1990, 1995),
		mk(12, 1995, 2005, 1998, 2000), // C1 FVG A
		mk(16, 2000, 2005, 1960, 1965), // C2 FVG A
		mk(20, 1965, 1980, 1975, 1978), // C3 FVG A == C1 FVG B
		mk(24, 1978, 1990, 1981, 1987), // cPrev
		mk(28, 1987, 1994, 1985, 1989), // sweep bearish A
	}

	sweep := eval.DetectHTFIlaSweep(htfCandles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("FAILED: FVG sebelumnya yang berbagi candle harus tetap dievaluasi")
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("FAILED: arah harus BEARISH, dapat %s", sweep.Direction)
	}
}
