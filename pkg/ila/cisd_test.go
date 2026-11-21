package ila

import (
	"strings"
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// Rujukan: New ILA 2.0/cisd.md (Kasus 1-5, -CISD dan +CISD).
// LTF M15 mulai 09:30 NY; C3 = 10:00-14:00 NY.

var (
	cisdBase  = time.Date(2025, 6, 10, 9, 30, 0, 0, nyLocation)
	cisdC3    = cisdBase.Add(30 * time.Minute)
	cisdC3End = cisdC3.Add(4 * time.Hour)
)

func cisdSeries(cs ...C) []indicators.Candle { return series(cisdBase, 15*time.Minute, cs...) }

// Kerangka: swing high 103 (idx1), rangkaian bullish mulai idx4 (level CISD = open idx4 = 99.5).
var cisdHead = []C{
	{101, 102, 99.5, 100}, {100, 103, 99.8, 102}, {102, 102.5, 100.5, 101},
	{101, 101.5, 99, 99.5}, {99.5, 100.5, 99.2, 100.3},
}

func with(tail ...C) []C { return append(append([]C{}, cisdHead...), tail...) }

func detectBoth(t *testing.T, cs []indicators.Candle) (*CISD, *CISD) {
	return DetectCISD(cs, DirBearish, cisdC3, cisdC3End), DetectCISD(mirror200(cs), DirBullish, cisdC3, cisdC3End)
}

func TestCISD_Kondisi1_ValidTerkenaWick(t *testing.T) {
	cs := cisdSeries(with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})...)
	bear, bull := detectBoth(t, cs)
	if !bear.Detected || bear.Condition != CISDKondisi1 || bear.EntryPrice != 99.2 || bear.ExtremeSL != 103.5 || bear.CISDLevel != 99.5 {
		t.Fatalf("-CISD: %+v", bear)
	}
	if !bull.Detected || bull.Condition != CISDKondisi1 || bull.EntryPrice != 100.8 || bull.ExtremeSL != 96.5 || bull.CISDLevel != 100.5 {
		t.Fatalf("+CISD (cermin): %+v", bull)
	}
}

func TestCISD_Kasus4_HanyaWickMenembusLevel(t *testing.T) {
	cs := cisdSeries(with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.6})...)
	bear, bull := detectBoth(t, cs)
	if bear != nil && bear.Detected || bull != nil && bull.Detected {
		t.Fatal("close di atas level CISD tidak boleh terkonfirmasi")
	}
}

func TestCISD_Kondisi2_FTMDenganiFVG(t *testing.T) {
	cs := cisdSeries(with(
		C{100.3, 102, 100.2, 101.8}, C{101.8, 104, 101.2, 103.5}, // BISI (idx4-6), close 103.5 > 103 = swing high FTM
		C{103.5, 104, 99, 99.3})...) // close 99.3 < bawah BISI (iFVG) dan < level CISD
	bear, bull := detectBoth(t, cs)
	if !bear.Detected || bear.Condition != CISDKondisi2 || bear.EntryPrice != 99.3 || bear.ExtremeSL != 104 {
		t.Fatalf("-CISD: %+v", bear)
	}
	if !bull.Detected || bull.Condition != CISDKondisi2 {
		t.Fatalf("+CISD: %+v", bull)
	}
}

func TestCISD_Kondisi3_FTMTanpaiFVG_WajibSWLFTM(t *testing.T) {
	head := []C{{101, 102, 99.5, 100}, {100, 103, 99.8, 102}, {102, 102.5, 100.5, 101}, {101, 101.5, 99, 99.5}, {99.5, 100.5, 99.2, 100.3}}
	ok := cisdSeries(append(append([]C{}, head...), C{100.3, 104, 100.2, 103.5}, C{103.5, 103.8, 98.5, 98.8})...)
	bear, bull := detectBoth(t, ok)
	if !bear.Detected || bear.Condition != CISDKondisi3 || bear.EntryPrice != 98.8 || bear.CISDLevel != 99.2 {
		t.Fatalf("-CISD (level harus low candle pertama 99.2): %+v", bear)
	}
	if !bull.Detected || bull.Condition != CISDKondisi3 || bull.CISDLevel != 100.8 {
		t.Fatalf("+CISD (level harus high candle pertama 100.8): %+v", bull)
	}
	// -CISD terjadi tetapi swing low (99) belum di-close body di bawahnya: belum terkonfirmasi.
	no := cisdSeries(append(append([]C{}, head...), C{100.3, 104, 100.2, 103.5}, C{103.5, 103.8, 98.5, 99.2})...)
	bear, _ = detectBoth(t, no)
	if bear.Detected || !strings.Contains(bear.Details, "SWL") {
		t.Fatalf("tanpa SWL FTM tidak boleh terkonfirmasi: %+v", bear)
	}
}

func TestCISD_KonfirmasiHarusDiDalamC3(t *testing.T) {
	cs := cisdSeries(with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})...)
	if c := DetectCISD(cs, DirBearish, cisdC3.Add(4*time.Hour), cisdC3End.Add(4*time.Hour)); c != nil && c.Detected {
		t.Fatal("break di luar jendela C3 tidak berlaku")
	}
}

func TestCISD_KausalTidakMelihatCandleSesudahnya(t *testing.T) {
	// Candle sesudah konfirmasi (membentuk swing/FVG baru) tidak boleh mengubah hasil.
	base := with(C{100.3, 103.5, 100, 102.8}, C{102.8, 103, 99, 99.2})
	a := DetectCISD(cisdSeries(base...), DirBearish, cisdC3, cisdC3End)
	b := DetectCISD(cisdSeries(append(base, C{99.2, 99.5, 98, 98.5}, C{98.5, 101, 98.4, 100.9})...), DirBearish, cisdC3, cisdC3End)
	if !a.Detected || !b.Detected || a.EntryPrice != b.EntryPrice || a.Condition != b.Condition {
		t.Fatalf("hasil berubah karena candle masa depan: %+v vs %+v", a, b)
	}
}

func TestCISD_SwingHighLamaYangDigantikanTidakDipakai(t *testing.T) {
	// Swing high 103 digantikan swing high 101 yang lebih baru (idx3, kanan idx4).
	cs := cisdSeries(
		C{101, 102, 99.5, 100}, C{100, 103, 99.8, 102}, C{102, 102.5, 100.5, 101},
		C{100.9, 101.5, 100, 100.5}, C{100.5, 100.9, 99.9, 100.2}, // swing high 101.5 (idx3) menggantikan 103
		C{99.9, 100.1, 99.5, 99.6}, C{99.6, 101.0, 99.4, 100.8}, C{100.8, 101.2, 100.6, 101.1}, C{101.1, 101.15, 99.0, 99.2})
	bear := DetectCISD(cs, DirBearish, cisdC3, cisdC3End)
	if bear != nil && bear.Detected && bear.KeySwing != nil && bear.KeySwing.Level == 103 {
		t.Fatal("swing high lama yang digantikan tidak boleh jadi key level")
	}
}

func TestCISD_Kondisi2_RangkaianDuaCandleDanBreakWajibBearish(t *testing.T) {
	// Rangkaian bullish 2 candle (idx4-5) membentuk BISI (c1=idx3 high 101.5 < c3=idx5 low 102.0) + FTM swing high 103.
	twoBar := cisdSeries(
		C{101, 102, 99.5, 100}, C{100, 103, 99.8, 102}, C{102, 102.5, 100.5, 101},
		C{101, 101.5, 99, 99.5},
		C{99.5, 102.5, 99.4, 102.2}, // c2 BISI (candle bullish pertama rangkaian)
		C{102.2, 104, 102.0, 103.5}, // c3 BISI & close > 103 (FTM)
		C{103.5, 103.8, 99, 99.2},   // bearish close < 101.5 (iFVG) & < 99.5 (level CISD)
	)
	bear, bull := detectBoth(t, twoBar)
	if !bear.Detected || bear.Condition != CISDKondisi2 || !bull.Detected || bull.Condition != CISDKondisi2 {
		t.Fatalf("rangkaian 2 candle BISI harus Kondisi 2: %+v | %+v", bear, bull)
	}
	// Gap-down candle BULLISH yang close di bawah level bukan body bearish -> tidak konfirmasi.
	gapBull := cisdSeries(with(C{100.3, 103.5, 100, 102.8}, C{98.5, 99.4, 98.4, 99.2})...)
	if c := DetectCISD(gapBull, DirBearish, cisdC3, cisdC3End); c != nil && c.Detected {
		t.Fatalf("candle bullish gap-down tidak boleh jadi konfirmasi -CISD: %+v", c)
	}
}

func TestCISD_Kondisi3_RetraceMinorSebelumSWLFTM(t *testing.T) {
	// -CISD di idx6 (close 99.2 < 99.5), retrace minor di idx7 (close 99.6 tanpa tembus SL 104), lalu SWL FTM di idx8 (close 98.5 < 99).
	cs := cisdSeries(with(
		C{100.3, 104, 100.2, 103.5},
		C{103.5, 103.8, 99.1, 99.2},
		C{99.2, 99.8, 99.1, 99.6},
		C{99.6, 99.7, 98.4, 98.5},
	)...)
	bear, bull := detectBoth(t, cs)
	if !bear.Detected || bear.Condition != CISDKondisi3 || bear.EntryPrice != 98.5 || bear.CISDLevel != 99.2 || !bull.Detected || bull.Condition != CISDKondisi3 || bull.CISDLevel != 100.8 {
		t.Fatalf("Kondisi 3 setelah retrace minor harus terkonfirmasi: %+v | %+v", bear, bull)
	}
}

func TestCISD_Kondisi3_WajibTembusLowCandlePertama(t *testing.T) {
	// Satu rangkaian bullish utuh idx4..idx7 tanpa BISI:
	// idx4: bullish #1 dengan open=99.5, low=98.4 (wick bawah lebih rendah dari open).
	// idx6: tetap bullish tapi membentuk SWL terbaru di 100.0 (low 100.0 < low idx5=100.5 & idx7=100.3).
	// idx7: FTM swing high 103 (close 103.5 > 103) tanpa BISI/iFVG.
	// idx8: close 98.9 -> sudah < open (99.5) dan < SWL 100.0 (SWL FTM), tapi >= low idx4 (98.4) -> BELUM terkonfirmasi.
	// idx9: close 98.2 -> < low idx4 (98.4) -> baru terkonfirmasi Kondisi 3 dengan CISDLevel = 98.4 (+CISD = 101.6).
	base := []C{
		{101, 102, 99.5, 100},      // idx0
		{100, 103, 99.8, 102},      // idx1: Swing High 103
		{102, 102.5, 100.5, 101},   // idx2
		{101, 101.5, 99.3, 99.5},   // idx3
		{99.5, 101.2, 98.4, 101},   // idx4: bullish #1 (open=99.5, low=98.4)
		{101, 102.2, 100.5, 102},   // idx5: bullish
		{102, 102.8, 100.0, 102.5}, // idx6: bullish, bentuk SWL terbaru 100.0
		{102.5, 104, 100.3, 103.5}, // idx7: bullish, FTM swing high 103 tanpa BISI
		{103.5, 103.6, 98.6, 98.9}, // idx8: close < open 99.5 & < SWL 100.0, tapi >= low 98.4
	}
	bearNotYet, bullNotYet := detectBoth(t, cisdSeries(base...))
	if (bearNotYet != nil && bearNotYet.Detected) || (bullNotYet != nil && bullNotYet.Detected) {
		t.Fatalf("close di atas low/di bawah high candle pertama tidak boleh konfirmasi Kondisi 3: %+v | %+v", bearNotYet, bullNotYet)
	}
	confirmed := append(base, C{98.9, 99.0, 98.0, 98.2}) // idx9: close 98.2 < 98.4
	bear, bull := detectBoth(t, cisdSeries(confirmed...))
	if !bear.Detected || bear.Condition != CISDKondisi3 || bear.CISDLevel != 98.4 || bear.EntryPrice != 98.2 {
		t.Fatalf("-CISD Kondisi 3 harus terkonfirmasi setelah tembus low 98.4: %+v", bear)
	}
	if !bull.Detected || bull.Condition != CISDKondisi3 || bull.CISDLevel != 101.6 || bull.EntryPrice != 101.8 {
		t.Fatalf("+CISD Kondisi 3 harus terkonfirmasi setelah tembus high 101.6: %+v", bull)
	}
}
