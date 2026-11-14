package ila

// Test invalidasi FVG versi dokumen baku ILA 2.0 (key-level-irl.md bagian 3).
//
// Inti aturan: FVG hanya INVALID bila ada CLOSE yang menembus batas terjauh zona.
// Wick boleh menembus batas zona tanpa membatalkan FVG.
//
// Ini sengaja dibedakan dari Mitigated (aturan lama, berbasis wick + toleransi
// spread) yang tetap dipakai menyaring kandidat sweep.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// candleBerurutan membuat candle tertutup dengan timestamp berurutan per jam.
func candleBerurutan(base time.Time, i int, o, h, l, c float64) indicators.Candle {
	return indicators.Candle{
		Timestamp: base.Add(time.Duration(i) * time.Hour),
		Open:      o, High: h, Low: l, Close: c,
		IsClosed: true,
	}
}

// fvgBearish4185 membentuk SIBI (FVG bearish) zona 4185.26 - 4254.74.
// Sama dengan kasus XAUUSD D1 29 Sep pada dokumen analisa.
// Candle 1 = index 0 (Low 4254.74 = atap), Candle 3 = index 2 (High 4185.26 = lantai).
func fvgBearish4185(base time.Time) []indicators.Candle {
	return []indicators.Candle{
		candleBerurutan(base, 0, 4220, 4290, 4254.74, 4284), // C1: Low = atap FVG
		candleBerurutan(base, 1, 4284, 4284, 4115, 4119),    // C2: displacement turun
		candleBerurutan(base, 2, 4119, 4185.26, 4110, 4157), // C3: High = lantai FVG
	}
}

func evaluatorFF() *Evaluator { return NewEvaluator() }

// Wick menembus atap zona tetapi close tetap di bawah atap:
// FVG tetap HIDUP menurut dokumen (Invalidated=false).
func TestFVGInvalidatedWickTembusCloseTetap(t *testing.T) {
	base := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	cs := append(fvgBearish4185(base),
		// High 4270 menembus atap 4254.74, tetap close 4240 (di bawah atap).
		candleBerurutan(base, 3, 4157, 4270, 4150, 4240),
	)

	fvgs := evaluatorFF().FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG bearish tidak terbentuk")
	}
	f := fvgs[0]

	if !f.Mitigated {
		t.Fatalf("harapan Mitigated=true (wick 4270 >= atap 4254.74), dapat false")
	}
	if f.Invalidated {
		t.Fatalf("harapan Invalidated=false karena close 4240 masih di bawah atap, dapat true")
	}
	if f.InvalidatedIndex != -1 {
		t.Fatalf("harapan InvalidatedIndex=-1, dapat %d", f.InvalidatedIndex)
	}
}

// Close menembus atap zona: FVG INVALID menurut dokumen.
func TestFVGInvalidatedCloseTembus(t *testing.T) {
	base := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	cs := append(fvgBearish4185(base),
		// Close 4300 di atas atap 4254.74 -> SIBI invalid.
		candleBerurutan(base, 3, 4157, 4310, 4150, 4300),
	)

	fvgs := evaluatorFF().FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG bearish tidak terbentuk")
	}
	f := fvgs[0]

	if !f.Invalidated {
		t.Fatalf("harapan Invalidated=true karena close 4300 > atap 4254.74, dapat false")
	}
	if f.InvalidatedIndex != 3 {
		t.Fatalf("harapan InvalidatedIndex=3, dapat %d", f.InvalidatedIndex)
	}
	if f.InvalidatedTime.IsZero() {
		t.Fatal("InvalidatedTime seharusnya terisi")
	}
}

// Invalidasi dihitung dari candle PERTAMA yang close menembus atap.
func TestFVGInvalidatedIndexPertama(t *testing.T) {
	base := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	cs := append(fvgBearish4185(base),
		candleBerurutan(base, 3, 4157, 4170, 4150, 4160), // aman
		candleBerurutan(base, 4, 4160, 4200, 4150, 4190), // aman
		candleBerurutan(base, 5, 4190, 4260, 4180, 4255), // close 4255 > 4254.74 -> invalid di sini
		candleBerurutan(base, 6, 4255, 4300, 4250, 4290), // lebih jauh, tidak dipakai
	)

	fvgs := evaluatorFF().FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG bearish tidak terbentuk")
	}
	f := fvgs[0]

	if !f.Invalidated {
		t.Fatal("harapan Invalidated=true")
	}
	if f.InvalidatedIndex != 5 {
		t.Fatalf("harapan InvalidatedIndex=5 (candle pertama yang close menembus), dapat %d", f.InvalidatedIndex)
	}
}

// Cermin bullish (BISI): invalid hanya bila close di bawah batas bawah zona.
func TestFVGInvalidatedBullish(t *testing.T) {
	base := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)

	// BISI zona 4185.26 (bawah) - 4254.74 (atas):
	// C1 High 4185.26, C3 Low 4254.74, C2 naik cepat.
	cs := []indicators.Candle{
		candleBerurutan(base, 0, 4150, 4185.26, 4140, 4180),
		candleBerurutan(base, 1, 4180, 4270, 4175, 4265),
		candleBerurutan(base, 2, 4265, 4270, 4254.74, 4260),
	}

	// Wick turun menembus batas bawah 4185.26, close tetap di atasnya -> tetap hidup.
	hidup := append(append([]indicators.Candle{}, cs...),
		candleBerurutan(base, 3, 4260, 4265, 4170, 4200),
	)
	fvgsHidup := evaluatorFF().FindFVGs(hidup)
	if len(fvgsHidup) == 0 {
		t.Fatal("FVG bullish tidak terbentuk")
	}
	if fvgsHidup[0].Invalidated {
		t.Fatal("harapan Invalidated=false: wick 4170 menembus 4185.26 tapi close 4200 masih di atas")
	}

	// Close menembus batas bawah -> invalid.
	mati := append(append([]indicators.Candle{}, cs...),
		candleBerurutan(base, 3, 4260, 4265, 4160, 4170),
	)
	fvgsMati := evaluatorFF().FindFVGs(mati)
	if len(fvgsMati) == 0 {
		t.Fatal("FVG bullish tidak terbentuk")
	}
	if !fvgsMati[0].Invalidated {
		t.Fatal("harapan Invalidated=true: close 4170 < batas bawah 4185.26")
	}
}

// Candle running tidak boleh dipakai menilai invalidasi karena close belum final.
func TestFVGInvalidatedCandleRunning(t *testing.T) {
	base := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	running := candleBerurutan(base, 3, 4157, 4310, 4150, 4300)
	running.IsClosed = false

	cs := append(fvgBearish4185(base), running)

	fvgs := evaluatorFF().FindFVGs(cs)
	if len(fvgs) == 0 {
		t.Fatal("FVG bearish tidak terbentuk")
	}
	if fvgs[0].Invalidated {
		t.Fatal("candle running tidak boleh men-invalidasi FVG; close belum final")
	}
}
