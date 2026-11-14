package ila

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// TestCISDTidakTerbentukTanpaDeretan mengunci aturan CISD: dibutuhkan DERETAN
// candle berlawanan yang bersambung menuju titik ekstrem.
//
// Dulu ada blok cadangan yang, bila deretan bersambung tidak ditemukan,
// menelusuri mundur dan mengambil candle berlawanan PERTAMA yang ditemui --
// termasuk candle dari Candle 4 (sebelum lilin sweep HTF). Akibatnya lahir CISD
// dari deretan yang tidak pernah ada, dengan candle ekstrem sebagai penembusnya
// sendiri. Kasus nyata USTEC 9 Okt 2026 21:00 NY: entry SELL palsu dengan SL
// 8,7 poin.
//
// Fixture: Low baru dibuat di Candle 5, tetapi semua candle Candle 5 bullish
// sehingga tidak ada deretan bearish menuju Low itu. Tidak boleh ada CISD.
func TestCISDTidakTerbentukTanpaDeretan(t *testing.T) {
	eval := NewEvaluator()
	awal := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	m15 := func(i int) time.Time { return awal.Add(time.Duration(i) * 15 * time.Minute) }

	// Candle 4 (sweep HTF) di indeks 0; Candle 5 dimulai di indeks 2.
	//
	// Yang diuji: TIDAK ADA satu pun candle bearish di Candle 5, dan tidak ada
	// deretan bearish di Candle 4 yang bersambung ke bawah. Blok cadangan lama
	// akan menyisir mundur dan mengambil candle bearish mana saja yang ditemui,
	// lalu memakai candle terendah (yang bullish) sebagai penembusnya -- CISD
	// palsu. Tanpa deretan, aturan yang benar adalah tidak ada CISD.
	// Candle 4 (sweep HTF) sendiri BEARISH, supaya blok cadangan lama punya
	// candle untuk dipinjam dan benar-benar terpicu. Sweep-nya tetap sah: Low
	// 2990 lebih dalam dari Low Candle 5 (2994).
	sweepCandle := indicators.Candle{
		Timestamp: m15(0), Open: 3005, High: 3006, Low: 2990, Close: 2992, IsClosed: true,
	}

	candles := []indicators.Candle{
		sweepCandle,
		// Candle 4 BULLISH: tidak ada deretan bearish untuk dipinjam.
		{Timestamp: m15(1), Open: 2998, High: 3010, Low: 2996, Close: 3008, IsClosed: true},
		// Seluruh Candle 5 bullish (close > open). Low menurun sampai 2994.
		{Timestamp: m15(2), Open: 2999, High: 3001, Low: 2998, Close: 3000, IsClosed: true},
		{Timestamp: m15(3), Open: 3000, High: 3002, Low: 2994, Close: 3001, IsClosed: true},
		{Timestamp: m15(4), Open: 3001, High: 3005, Low: 2996, Close: 3004, IsClosed: true},
	}

	cisd, invalidated := eval.DetectDynamicCISD(candles, "BULLISH", m15(2), sweepCandle)
	if invalidated {
		t.Fatalf("prasyarat setup: Low Candle 5 (2994) masih di atas Low Candle 4 (2990)")
	}
	if cisd != nil {
		t.Fatalf("CISD tidak boleh terbentuk tanpa satu pun candle bearish; "+
			"dapat level=%.2f ref=%s", cisd.CISDLevel, cisd.RefCandle.Timestamp.Format("15:04"))
	}
}

// TestCISDTetapTerbentukDariDeretanSah memastikan aturan baru tidak membuang
// CISD yang memang punya deretan bersambung.
func TestCISDTetapTerbentukDariDeretanSah(t *testing.T) {
	eval := NewEvaluator()
	awal := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	m15 := func(i int) time.Time { return awal.Add(time.Duration(i) * 15 * time.Minute) }

	sweepCandle := indicators.Candle{
		Timestamp: m15(0), Open: 3000, High: 3005, Low: 2995, Close: 3002, IsClosed: true,
	}

	candles := []indicators.Candle{
		sweepCandle,
		// Candle indeks 1 BULLISH: penelusuran mundur wajib berhenti di sini dan
		// tidak merambah ke Candle 4.
		{Timestamp: m15(1), Open: 2998, High: 3010, Low: 2997, Close: 3008, IsClosed: true},
		// Dua candle BEARISH bersambung menuju Low. Origin = Open candle pertama
		// deretan = 3000.
		{Timestamp: m15(2), Open: 3000, High: 3001, Low: 2998, Close: 2999, IsClosed: true},
		{Timestamp: m15(3), Open: 2999, High: 3000, Low: 2996, Close: 2997, IsClosed: true},
		// Penembus: close di atas level CISD (3000).
		{Timestamp: m15(4), Open: 2997, High: 3003, Low: 2996, Close: 3002, IsClosed: true},
	}

	cisd, invalidated := eval.DetectDynamicCISD(candles, "BULLISH", m15(2), sweepCandle)
	if invalidated {
		t.Fatalf("setup tidak boleh invalidated; Low Candle 5 masih di atas Low Candle 4")
	}
	if cisd == nil {
		t.Fatalf("CISD dengan deretan bearish sah harus terbentuk")
	}
	if !cisd.Detected {
		t.Fatalf("CISD harus terdeteksi: %s", cisd.Details)
	}
	if cisd.CISDLevel != 3000 {
		t.Errorf("level CISD = %.2f; ingin 3000.00 (Open candle bearish pertama)", cisd.CISDLevel)
	}
}
