package ila

// Test C2 Closure. Nama kasus mengikuti dokumen baku ILA 2.0
// "C2 Closure pada Key Level IRL/ERL" sehingga mudah dicocokkan dengan aturan.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// bangunCandle membuat candle tertutup dengan timestamp berurutan.
func bangunCandle(base time.Time, i int, o, h, l, c float64) indicators.Candle {
	return indicators.Candle{
		Timestamp: base.Add(time.Duration(i) * time.Hour),
		Open:      o, High: h, Low: l, Close: c,
		IsClosed: true,
	}
}

// fvgBearishSIBI membentuk FVG bearish sederhana dengan zona 1980..1996.
// Candle 1 = index 0 (Low 1996), Candle 2 = index 1, Candle 3 = index 2 (High 1980).
// Acuan awal = High Candle 3 = 1980.
func fvgBearishSIBI() (FVG, time.Time) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	fvg := FVG{
		Type: FVGBearish, Top: 1996, Bottom: 1980,
		Candle1Index: 0, Candle2Index: 1, Candle3Index: 2,
	}
	return fvg, base
}

func tigaCandleBentuk(base time.Time) []indicators.Candle {
	return []indicators.Candle{
		bangunCandle(base, 0, 2000, 2010, 1996, 2005), // C1: Low 1996 = top FVG
		bangunCandle(base, 1, 2005, 2005, 1960, 1965), // C2: displacement turun
		bangunCandle(base, 2, 1965, 1980, 1975, 1978), // C3: High 1980 = bottom FVG / acuan awal
	}
}

// --- BAGIAN A: BEARISH FVG / SIBI ---

// Kasus A: candle setelah candle 3 langsung menjadi C2.
func TestC2SIBIKasusA_LangsungC2(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1985, 1970, 1977), // wick 1985 > acuan 1980, close 1977 < 1980
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF, dapat %s (%s)", res.Status, res.Detail)
	}
	if res.CandleIndex != 3 {
		t.Fatalf("C2 seharusnya di index 3, dapat %d", res.CandleIndex)
	}
	if res.Reference != 1985 {
		t.Fatalf("acuan baru harus high C2 = 1985, dapat %.2f", res.Reference)
	}
}

// Kasus B: beberapa candle tidak menyentuh acuan (diabaikan), lalu C2 muncul.
func TestC2SIBIKasusB_TidakMenyentuhLaluC2(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1979, 1970, 1975), // high 1979 < 1980 -> diabaikan
		bangunCandle(base, 4, 1975, 1978, 1968, 1972), // high 1978 < 1980 -> diabaikan
		bangunCandle(base, 5, 1972, 1984, 1966, 1975), // wick 1984 > 1980, close 1975 -> C2
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif || res.CandleIndex != 5 {
		t.Fatalf("C2 seharusnya di index 5, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
}

// Kasus C: acuan bergeser (Urutan 4), lalu C2 baru terbentuk di acuan baru.
func TestC2SIBIKasusC_AcuanBergeser(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1985, 1970, 1982), // sentuh 1980, close 1982 > acuan tapi < top 1996 -> gugur, acuan -> 1985
		bangunCandle(base, 4, 1982, 1990, 1975, 1980), // sentuh 1985, close 1980 < 1985 -> C2, acuan -> 1990
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF setelah acuan bergeser, dapat %s (%s)", res.Status, res.Detail)
	}
	if res.CandleIndex != 4 {
		t.Fatalf("C2 seharusnya di index 4, dapat %d", res.CandleIndex)
	}
	if res.Reference != 1990 {
		t.Fatalf("acuan baru harus 1990, dapat %.2f", res.Reference)
	}
}

// Kasus D: invalidasi langsung.
func TestC2SIBIKasusD_Invalidasi(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 2000, 1970, 1999), // close 1999 > top 1996
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID, dapat %s", res.Status)
	}
	if res.InvalidIndex != 3 {
		t.Fatalf("invalid di index 3, dapat %d", res.InvalidIndex)
	}
}

// Kasus E: invalidasi lalu candle turun kembali -> BUKAN C2 (evaluasi berhenti).
func TestC2SIBIKasusE_InvalidLaluKembaliBukanC2(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 2000, 1970, 1999), // FVG invalid di sini
		bangunCandle(base, 4, 1999, 1995, 1960, 1970), // turun kembali, close di bawah acuan
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Invalid {
		t.Fatalf("harapan tetap INVALID (bukan C2 baru), dapat %s", res.Status)
	}
}

// Kasus F: wick menembus batas atas FVG tapi close di bawah acuan -> tetap C2.
// Invalidasi hanya dari close, bukan wick.
func TestC2SIBIKasusF_WickTembusTopTetapC2(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 2010, 1970, 1975), // high 2010 > top 1996, close 1975 < 1980
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("wick boleh tembus top selama close di bawah acuan; dapat %s (%s)", res.Status, res.Detail)
	}
}

// Kasus G: C2 berpindah ke candle berikutnya.
func TestC2SIBIKasusG_C2Berpindah(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1985, 1970, 1977), // C2, acuan -> 1985
		bangunCandle(base, 4, 1977, 1990, 1972, 1980), // wick 1990 > 1985, close 1980 < 1985 -> C2 pindah
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif || res.CandleIndex != 4 {
		t.Fatalf("C2 harus pindah ke index 4, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
	if res.Reference != 1990 {
		t.Fatalf("acuan baru 1990, dapat %.2f", res.Reference)
	}
}

// Kasus H: C2 gugur lalu C2 baru terbentuk (dua kali pergeseran).
func TestC2SIBIKasusH_GugurLaluC2Baru(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1985, 1970, 1977), // C2, acuan -> 1985
		bangunCandle(base, 4, 1977, 1990, 1972, 1988), // sentuh 1985, close 1988 > 1985 tapi < top -> gugur, acuan -> 1990
		bangunCandle(base, 5, 1988, 1994, 1980, 1987), // sentuh 1990, close 1987 < 1990 -> C2
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif || res.CandleIndex != 5 {
		t.Fatalf("C2 baru harus di index 5, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
}

// Kasus close tepat sama dengan acuan: bukan C2 (dokumen tegas).
func TestC2SIBICloseTepatDiAcuanBukanC2(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		bangunCandle(base, 3, 1978, 1985, 1970, 1980), // close tepat 1980 = acuan
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status == C2Aktif {
		t.Fatalf("close tepat di acuan bukan C2")
	}
}

// Candle yang masih berjalan tidak boleh dipakai (status belum final).
func TestC2SIBICandleRunningDiabaikan(t *testing.T) {
	fvg, base := fvgBearishSIBI()
	cs := append(tigaCandleBentuk(base),
		indicators.Candle{Timestamp: base.Add(3 * time.Hour), Open: 1978, High: 1985, Low: 1970, Close: 1977, IsClosed: false},
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status == C2Aktif {
		t.Fatalf("candle running tidak boleh menjadi C2")
	}
}

// --- BAGIAN B: BULLISH FVG / BISI (mirror) ---

// zona 2000..2020 ; C1 High 2000 (bottom), C3 Low 2020 (top), acuan awal 2020.
func fvgBullishBISI() (FVG, time.Time) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	fvg := FVG{
		Type: FVGBullish, Top: 2020, Bottom: 2000,
		Candle1Index: 0, Candle2Index: 1, Candle3Index: 2,
	}
	return fvg, base
}

func tigaCandleBentukBullish(base time.Time) []indicators.Candle {
	return []indicators.Candle{
		bangunCandle(base, 0, 1990, 2000, 1985, 1995), // C1: High 2000 = bottom FVG
		bangunCandle(base, 1, 1995, 2050, 1990, 2045), // C2: displacement naik
		bangunCandle(base, 2, 2045, 2055, 2020, 2050), // C3: Low 2020 = top FVG / acuan awal
	}
}

func TestC2BISIKasusA_LangsungC2(t *testing.T) {
	fvg, base := fvgBullishBISI()
	cs := append(tigaCandleBentukBullish(base),
		bangunCandle(base, 3, 2050, 2060, 2015, 2055), // low 2015 < acuan 2020, close 2055 > 2020
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF, dapat %s (%s)", res.Status, res.Detail)
	}
	if res.Reference != 2015 {
		t.Fatalf("acuan baru = low C2 2015, dapat %.2f", res.Reference)
	}
}

func TestC2BISIKasusG_C2Berpindah(t *testing.T) {
	fvg, base := fvgBullishBISI()
	cs := append(tigaCandleBentukBullish(base),
		bangunCandle(base, 3, 2050, 2060, 2015, 2055), // C2, acuan -> 2015
		bangunCandle(base, 4, 2055, 2062, 2008, 2050), // low 2008 < 2015, close 2050 > 2015 -> pindah
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif || res.CandleIndex != 4 {
		t.Fatalf("C2 harus pindah ke index 4, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
	if res.Reference != 2008 {
		t.Fatalf("acuan 2008, dapat %.2f", res.Reference)
	}
}

func TestC2BISIKasusD_Invalidasi(t *testing.T) {
	fvg, base := fvgBullishBISI()
	cs := append(tigaCandleBentukBullish(base),
		bangunCandle(base, 3, 2050, 2060, 1990, 1992), // close 1992 < bottom 2000
	)
	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID, dapat %s", res.Status)
	}
}

// --- ERL: SWING LOW / SWING HIGH ---

func TestC2ERLSwingLowKasusA(t *testing.T) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	// Swing low di index 1 (Low 100), candle kanan index 2.
	cs := []indicators.Candle{
		bangunCandle(base, 0, 105, 108, 103, 106),
		bangunCandle(base, 1, 106, 107, 100, 104), // level swing low = 100
		bangunCandle(base, 2, 104, 109, 102, 108), // candle kanan
		bangunCandle(base, 3, 108, 110, 98, 105),  // wick 98 <= 100, close 105 > 100 -> C2
	}
	res := EvaluateC2ERL(SwingERL{Type: ERLSwingLow, Level: 100, LeftIndex: 0, MidIndex: 1, RightIndex: 2}, cs)
	if res.Status != C2Aktif || res.CandleIndex != 3 {
		t.Fatalf("harapan C2 di index 3, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
	if res.Reference != 98 {
		t.Fatalf("acuan baru = low C2 98, dapat %.2f", res.Reference)
	}
}

func TestC2ERLSwingLowInvalid(t *testing.T) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cs := []indicators.Candle{
		bangunCandle(base, 0, 105, 108, 103, 106),
		bangunCandle(base, 1, 106, 107, 100, 104),
		bangunCandle(base, 2, 104, 109, 102, 108),
		bangunCandle(base, 3, 108, 110, 95, 97), // close 97 < 100 -> invalid
	}
	res := EvaluateC2ERL(SwingERL{Type: ERLSwingLow, Level: 100, LeftIndex: 0, MidIndex: 1, RightIndex: 2}, cs)
	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID, dapat %s", res.Status)
	}
}

func TestC2ERLSwingHighKasusA(t *testing.T) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cs := []indicators.Candle{
		bangunCandle(base, 0, 100, 102, 98, 101),
		bangunCandle(base, 1, 101, 110, 100, 105), // level swing high = 110
		bangunCandle(base, 2, 105, 108, 100, 102), // candle kanan
		bangunCandle(base, 3, 102, 115, 99, 104),  // wick 115 >= 110, close 104 < 110 -> C2
	}
	res := EvaluateC2ERL(SwingERL{Type: ERLSwingHigh, Level: 110, LeftIndex: 0, MidIndex: 1, RightIndex: 2}, cs)
	if res.Status != C2Aktif || res.CandleIndex != 3 {
		t.Fatalf("harapan C2 di index 3, dapat %s idx=%d", res.Status, res.CandleIndex)
	}
	if res.Reference != 115 {
		t.Fatalf("acuan baru = high C2 115, dapat %.2f", res.Reference)
	}
}

func TestC2ERLSwingHighInvalid(t *testing.T) {
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cs := []indicators.Candle{
		bangunCandle(base, 0, 100, 102, 98, 101),
		bangunCandle(base, 1, 101, 110, 100, 105),
		bangunCandle(base, 2, 105, 108, 100, 102),
		bangunCandle(base, 3, 102, 115, 99, 112), // close 112 > 110 -> invalid
	}
	res := EvaluateC2ERL(SwingERL{Type: ERLSwingHigh, Level: 110, LeftIndex: 0, MidIndex: 1, RightIndex: 2}, cs)
	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID, dapat %s", res.Status)
	}
}
