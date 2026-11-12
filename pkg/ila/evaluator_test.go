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
