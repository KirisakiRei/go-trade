package ila

import (
	"math"
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// Rujukan: New ILA 2.0/alur-entry.md. Cycle Daily (HTF H4 -> LTF M15), hari trading 9 Jun 18:00 - 10 Jun 18:00 NY.

func htfH4(running bool, cs ...C) []indicators.Candle {
	out := series(testBase, 4*time.Hour, cs...)
	if running {
		out[len(out)-1].IsClosed = false
	}
	return out
}

// Swing High 106 (idx1, kanan idx2). C2 = idx3 (06:00 NY, wick 106.5, close 102.5). C3 = idx4 (10:00 NY).
var htfSwingHighC2 = []C{
	{100, 102, 99, 101}, {101, 106, 100.5, 104}, {104, 105, 102, 103},
	{103, 106.5, 102, 102.5}, {102.5, 103, 101.5, 102},
}

func ltfCISD(cs ...C) []indicators.Candle { return cisdSeries(cs...) }

func TestEvaluate_HTFERL_C3Berjalan_CISD_Ready_RR2(t *testing.T) {
	s := NewEvaluator().EvaluateILA(htfH4(true, htfSwingHighC2...),
		ltfCISD(with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})...), "H4", "M15", "XAUUSD")
	if s.Status != StatusReady || s.Bias != DirBearish || s.Cycle != string(CycleDaily) {
		t.Fatalf("status/bias/cycle: %s %s %s | %s", s.Status, s.Bias, s.Cycle, s.Summary)
	}
	if s.EntryPrice != 99.2 || s.StopLoss != 103.5 || math.Abs(s.TakeProfit-90.6) > 1e-9 || s.RiskReward != 2 || !s.PassedMinRR {
		t.Fatalf("posisi: entry %v sl %v tp %v rr %v", s.EntryPrice, s.StopLoss, s.TakeProfit, s.RiskReward)
	}
	h := s.HTFSweep
	if h == nil || h.KeyLevelKind != KeyLevelERL || !h.Detected || h.SweepInProgress || h.TargetDOL != 0 || h.SweepCandleIndex != 3 {
		t.Fatalf("htf sweep: %+v", h)
	}
	if s.CISD == nil || !s.CISD.Detected || s.CISD.Condition != CISDKondisi1 {
		t.Fatalf("cisd: %+v", s.CISD)
	}
}

func TestEvaluate_C3Berjalan_BelumCISD_Menunggu(t *testing.T) {
	s := NewEvaluator().EvaluateILA(htfH4(true, htfSwingHighC2...), ltfCISD(with()...), "H4", "M15", "XAUUSD")
	if s.Status != StatusWaitCISD || s.EntryPrice != 0 {
		t.Fatalf("harus menunggu CISD: %s", s.Status)
	}
}

func TestEvaluate_C2DiLuarCycle_TidakBerlaku(t *testing.T) {
	// Pakai cycle Weekly (HTF D1) dengan C2 Kamis & C3 Jumat 18:00 NY: C3 di luar cycle.
	htf := series(ny(2025, time.June, 9, 18, 0), 24*time.Hour,
		C{100, 102, 99, 101}, C{101, 106, 100.5, 104}, C{104, 105, 102, 103},
		C{103, 106.5, 102, 102.5}, C{102.5, 103, 101.5, 102})
	htf[4].IsClosed = false // C3 = Jumat 13 Jun 18:00 (running)
	s := NewEvaluator().EvaluateILA(htf, ltfCISD(with()...), "D1", "H1", "XAUUSD")
	if s.Status == StatusReady || s.Status == StatusWaitCISD {
		t.Fatalf("C2/C3 di luar cycle tidak boleh jadi setup: %s", s.Status)
	}
}

func TestEvaluate_HTFIRL_C3Closed_TanpaSetup_Invalid(t *testing.T) {
	htf := htfH4(false, C{110, 112, 108, 109}, C{101, 109, 100, 108}, C{101, 104, 99, 100}, // SIBI 108/104
		C{102, 106, 101, 103},   // C2: wick 106 >= 104, close 103 < 104
		C{103, 104, 102, 103.5}) // C3 sudah close
	flat := make([]C, 12)
	for i := range flat {
		flat[i] = C{100, 100.5, 99.5, 100}
	}
	s := NewEvaluator().EvaluateILA(htf, series(ny(2025, time.June, 10, 10, 0), 15*time.Minute, flat...), "H4", "M15", "XAUUSD")
	if s.Status != StatusInvalid || s.Bias != DirBearish || s.HTFSweep.KeyLevelKind != KeyLevelIRL {
		t.Fatalf("C3 close tanpa setup harus INVALIDATED: %s | %s", s.Status, s.Summary)
	}
}

func TestEvaluate_C4TidakMembukaEntry(t *testing.T) {
	htf := htfH4(false, C{110, 112, 108, 109}, C{101, 109, 100, 108}, C{101, 104, 99, 100},
		C{102, 106, 101, 103}, C{103, 104, 102, 103.5}, C{103.5, 104, 102.5, 103})
	htf[5].IsClosed = false // C4 berjalan
	s := NewEvaluator().EvaluateILA(htf, ltfCISD(with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})...), "H4", "M15", "XAUUSD")
	if s.Status == StatusReady || s.Status == StatusWaitCISD {
		t.Fatalf("C4 tidak boleh membuka entry: %s", s.Status)
	}
}

func TestEvaluate_PasanganTidakDikenal(t *testing.T) {
	e := NewEvaluator()
	if s := e.EvaluateILA(nil, nil, "H2", "M15", "X"); s.Status != StatusWaitC2 {
		t.Fatalf("HTF di luar tabel: %s", s.Status)
	}
	if s := e.EvaluateILA(htfH4(true, htfSwingHighC2...), nil, "H4", "M5", "X"); s.Status != StatusWaitC2 {
		t.Fatalf("LTF tidak sesuai pasangan: %s", s.Status)
	}
}

func TestEvaluate_KonflikOutsideBar_Neutral(t *testing.T) {
	// Swing High 106 (idx1) dan Swing Low 98 (idx2), lalu outside bar idx4 menyapu keduanya dan close di tengah -> konflik.
	htf := htfH4(true,
		C{100, 102, 99, 101},    // idx0
		C{101, 106, 100, 104},   // idx1: Swing High 106
		C{104, 105, 98, 100},    // idx2: Swing Low 98
		C{100, 104, 99, 102},    // idx3: candle kanan Swing Low 98
		C{102, 107, 97, 102},    // idx4: C2 bearish (high 107 >= 106, close < 106) & C2 bullish (low 97 <= 98, close > 98)
		C{102, 103, 101, 102.5}, // idx5: C3 running
	)
	// C2 = idx4 (10:00 NY), C3 = idx5 (14:00 NY): masih satu Daily cycle (9 Jun 18:00 - 10 Jun 18:00 NY).
	s := NewEvaluator().EvaluateILA(htf, ltfCISD(with()...), "H4", "M15", "XAUUSD")
	if s.Status != StatusWaitC2 || s.Bias != DirNeutral {
		t.Fatalf("konflik outside bar harus NEUTRAL / WAIT_HTF_SWEEP, got %s %s", s.Status, s.Bias)
	}
}

func TestEvaluate_CandlePenutupC3TetapReadyLaluBerhentiDiC4(t *testing.T) {
	// HTF SIBI (idx0-2), C2 = idx3 (06:00 NY), C3 = idx4 (10:00-14:00 NY) baru saja close bersamaan
	// dengan candle M15 penutup C3 (13:45-14:00 NY).
	htfClosed := htfH4(false,
		C{110, 112, 108, 109}, C{101, 109, 100, 108}, C{101, 104, 99, 100},
		C{102, 106, 101, 103}, C{103, 104, 102, 103.5})
	ltfBase := ny(2025, time.June, 10, 12, 15) // idx 6 = 13:45 NY (penutup C3!)
	ltf := series(ltfBase, 15*time.Minute, with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})...)
	s := NewEvaluator().EvaluateILA(htfClosed, ltf, "H4", "M15", "XAUUSD")
	if s.Status != StatusReady || s.EntryPrice != 99.2 {
		t.Fatalf("candle penutup C3 harus READY: %s | %s", s.Status, s.Summary)
	}
	// Begitu candle M15 berikutnya (14:00 NY, di dalam C4) sudah close, tidak boleh terbit ulang.
	ltfNext := append(ltf, indicators.Candle{Timestamp: ny(2025, time.June, 10, 14, 0), Open: 99.2, High: 99.5, Low: 98.8, Close: 99.0, IsClosed: true})
	s2 := NewEvaluator().EvaluateILA(htfClosed, ltfNext, "H4", "M15", "XAUUSD")
	if s2.Status == StatusReady {
		t.Fatalf("sesudah C3 lewat tidak boleh terbit READY ulang: %s", s2.Status)
	}
}

func TestEvaluate_C3Berjalan_SetupStaleTidakTerbitUlang(t *testing.T) {
	// CISD terkonfirmasi pada candle M15 idx6 di tengah C3 (running).
	// Begitu candle M15 berikutnya (idx7) selesai tutup (bahkan bila harga sudah berbalik jauh),
	// EvaluateILA tidak boleh menerbitkan StatusReady ulang untuk break lama idx6.
	base := with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})
	fresh := NewEvaluator().EvaluateILA(htfH4(true, htfSwingHighC2...), ltfCISD(base...), "H4", "M15", "XAUUSD")
	if fresh.Status != StatusReady {
		t.Fatalf("pada candle break terbaru harus READY, got %s", fresh.Status)
	}
	stale := NewEvaluator().EvaluateILA(
		htfH4(true, htfSwingHighC2...),
		ltfCISD(append(base, C{99.2, 104.2, 99.0, 103.9})...), // candle sesudahnya bahkan sudah tembus SL 103.5
		"H4", "M15", "XAUUSD",
	)
	if stale.Status == StatusReady {
		t.Fatalf("setup stale di C3 tidak boleh diterbitkan ulang sebagai READY: %s | %s", stale.Status, stale.Summary)
	}
}
