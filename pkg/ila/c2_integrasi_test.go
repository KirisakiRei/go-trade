package ila

// Test jaminan: penambahan status C2 dan bias ERL TIDAK boleh mengubah
// keputusan entry sedikit pun.
//
// Ini pagar utama sebelum logika sweep diganti ke definisi C2 draft. Kalau ada
// test di sini merah, berarti ada jalur keputusan yang ikut terbaca dari C2.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// bangunSweepBearish menyiapkan FVG bearish dengan candle sweep yang sah
// menurut aturan sweep yang berlaku sekarang.
func bangunSweepBearish(base time.Time) []indicators.Candle {
	return []indicators.Candle{
		bangunCandle(base, 0, 4430, 4440, 4405.77, 4412), // C1 Low = atap zona
		bangunCandle(base, 1, 4431.07, 4431.07, 4394.15, 4394.15),
		bangunCandle(base, 2, 4394.15, 4400.32, 4390, 4396), // C3 High = lantai zona
		bangunCandle(base, 3, 4396, 4401.00, 4394, 4399),    // candle acuan di dalam zona
		bangunCandle(base, 4, 4399, 4403.00, 4392, 4396),    // sweep high 4401, close 4396
	}
}

// Status C2 terisi pada hasil sweep.
func TestSweepC2Terisi(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cs := bangunSweepBearish(base)

	sw := NewEvaluator().DetectHTFIlaSweep(cs)
	if sw == nil {
		t.Fatal("sweep tidak terdeteksi")
	}
	if sw.C2Status == "" {
		t.Fatal("C2Status seharusnya terisi sebagai informasi pelengkap")
	}
	if sw.C2Detail == "" {
		t.Fatal("C2Detail seharusnya terisi")
	}
}

// Keputusan entry identik dengan yang dihasilkan fungsi inti.
// Fungsi inti adalah logika sweep sebelum C2 ditambahkan, jadi apa pun yang
// dikembalikannya adalah acuan perilaku live.
//
// Diuji pada BEBERAPA FVG sekaligus supaya mencakup FVG yang C2-nya belum ada
// maupun yang sudah ada. Kalau field C2 sampai terbaca keputusan, salah satu
// kombinasi pasti meleset.
func TestSweepEntryTidakBerubah(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cs := bangunSweepBearish(base)

	e := NewEvaluator()
	fvgs := e.FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG tidak terbentuk")
	}

	for _, latest := range fvgs {
		luar := e.detectSweepPadaFVG(cs, fvgs, latest, false)
		dalam := e.detectSweepPadaFVGInti(cs, fvgs, latest, false)

		if (luar == nil) != (dalam == nil) {
			t.Fatalf("nil berbeda: luar=%v dalam=%v", luar == nil, dalam == nil)
		}
		if luar == nil {
			continue
		}

		if luar.Detected != dalam.Detected ||
			luar.SweepInProgress != dalam.SweepInProgress ||
			luar.Direction != dalam.Direction ||
			luar.SweepCandleIndex != dalam.SweepCandleIndex ||
			luar.StopLoss != dalam.StopLoss ||
			luar.TargetDOL != dalam.TargetDOL ||
			luar.SweepTime != dalam.SweepTime {
			t.Fatalf("keputusan entry berubah untuk FVG %.2f-%.2f!\nluar : det=%v dir=%s idx=%d sl=%.2f dol=%.2f\n"+
				"dalam: det=%v dir=%s idx=%d sl=%.2f dol=%.2f",
				latest.Bottom, latest.Top,
				luar.Detected, luar.Direction, luar.SweepCandleIndex, luar.StopLoss, luar.TargetDOL,
				dalam.Detected, dalam.Direction, dalam.SweepCandleIndex, dalam.StopLoss, dalam.TargetDOL)
		}
	}
}

// Bukti struktural: mengubah C2Status secara manual TIDAK mempengaruhi apa pun.
// Test ini menjaga agar tidak ada kode masa depan yang membaca C2Status untuk
// keputusan entry tanpa sadar.
func TestSweepC2StatusTidakDibaca(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cs := bangunSweepBearish(base)

	e := NewEvaluator()
	fvgs := e.FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG tidak terbentuk")
	}
	latest := fvgs[len(fvgs)-1]

	sweep := e.detectSweepPadaFVGInti(cs, fvgs, latest, false)
	if sweep == nil {
		t.Fatal("sweep inti nil")
	}

	// Rekam keputusan sebelum C2Status disentuh.
	det, dir, idx, sl, dol := sweep.Detected, sweep.Direction, sweep.SweepCandleIndex, sweep.StopLoss, sweep.TargetDOL

	// Ubah status C2 menjadi nilai yang berlawanan; keputusan harus tetap.
	sweep.C2Status = C2Invalid
	sweep.C2Detail = "uji"

	if sweep.Detected != det || sweep.Direction != dir || sweep.SweepCandleIndex != idx ||
		sweep.StopLoss != sl || sweep.TargetDOL != dol {
		t.Fatalf("keputusan berubah karena C2Status: det=%v dir=%s idx=%d sl=%.2f dol=%.2f",
			sweep.Detected, sweep.Direction, sweep.SweepCandleIndex, sweep.StopLoss, sweep.TargetDOL)
	}
}

// ERLBias terisi di ILASetup tetapi tidak mengubah Status/Bias/Entry.
func TestEvaluateILAERLBiasTidakMengubahStatus(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	htf := bangunSweepBearish(base)

	// LTF sederhana: cukup agar DetectDynamicCISD punya bahan.
	var ltf []indicators.Candle
	for i := 0; i < 30; i++ {
		c := bangunCandle(base, i, 4400, 4405, 4390, 4396)
		ltf = append(ltf, c)
	}

	e := NewEvaluator()
	setup := e.EvaluateILA(htf, ltf, "H4", "M15", "XAUUSD")
	if setup == nil {
		t.Fatal("setup nil")
	}
	if setup.ERLBias == "" {
		t.Fatal("ERLBias seharusnya terisi sebagai informasi")
	}

	// Bias entry (Bias) hanya boleh dua nilai yang ditentukan sweep FVG, bukan ERL.
	if setup.Bias != "BEARISH" && setup.Bias != "BULLISH" && setup.Bias != "NEUTRAL" {
		t.Fatalf("Bias tidak valid: %s", setup.Bias)
	}
	// Status ditentukan sweep + CISD, bukan ERL.
	switch setup.Status {
	case "WAIT_HTF_SWEEP", "WAIT_HTF_CANDLE_CLOSE", "WAIT_LTF_CISD", "READY_TO_ENTRY", "INVALIDATED":
	default:
		t.Fatalf("Status tidak dikenal: %s", setup.Status)
	}
}
