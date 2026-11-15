package ila

// Test bukti MERAH untuk dua kasus XAUUSD H4 yang dikonfirmasi trader.
//
// Kedua kasus adalah setup SELL yang sah menurut dokumen baku ILA 2.0
// (c2-closure-irl.md), tetapi DITOLAK oleh logika sweep yang berlaku sekarang
// karena logika itu menuntut sapuan terhadap high candle TEPAT SEBELUMNYA,
// sementara dokumen menuntut sapuan terhadap LEVEL ACUAN FVG.
//
// Test ini sengaja ditulis dengan logika sweep SEKARANG. Harapannya MERAH.
// Setelah logika diganti ke definisi C2 draft, test ini harus HIJAU tanpa
// mengubah data ujinya.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// Kasus 1 - XAUUSD H4, 28 Aug 2026 (waktu NEW YORK).
//
// FVG BEARISH 4471.71 - 4589.18
//
//	C1 = 28 Aug 06:00  Low  4589.18  (atap zona)
//	C2 = 28 Aug 10:00  Open 4604.94 -> Close 4469.52 (displacement turun)
//	C3 = 28 Aug 14:00  High 4471.71, Close 4454.93 (lantai zona = acuan awal)
//
// 28 Aug 18:00 tidak ada (Jumat, pasar tutup).
// Candle penyapu: 30 Aug 18:00
//
//	Open 4454.93  High 4472.14  Low 4434.81  Close 4440.30
//
// High 4472.14 menyapu acuan 4471.71, close 4440.30 ditolak ke bawah zona.
// Trader menyatakan: ini VALID SELL.
func TestSweepKasus1_H4_28Aug(t *testing.T) {
	// Timestamp dalam UTC; 06:00 NY = 10:00 UTC (EDT), 14:00 NY = 18:00 UTC.
	base := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	h4 := func(i int, o, h, l, c float64) indicators.Candle {
		return bangunCandle(base, i*4, o, h, l, c)
	}

	cs := []indicators.Candle{
		// 28 Aug 06:00 NY
		h4(0, 4570.00, 4620.00, 4589.18, 4600.00),
		// 28 Aug 10:00 NY (displacement turun)
		h4(1, 4604.94, 4604.94, 4460.00, 4469.52),
		// 28 Aug 14:00 NY (candle 3, High = acuan awal)
		h4(2, 4469.52, 4471.71, 4450.00, 4454.93),
		// 30 Aug 18:00 NY = 30 Aug 22:00 UTC. Diposisikan sebagai candle ke-3
		// setelah C3. Tidak ada candle di antara karena pasar tutup Jumat.
		{
			Timestamp: time.Date(2026, 8, 30, 22, 0, 0, 0, time.UTC),
			Open:      4454.93, High: 4472.14, Low: 4434.81, Close: 4440.30,
			IsClosed: true,
		},
		// Candle penutup (masih menjalankan harga turun) supaya candle sweep
		// bukan candle terakhir.
		{
			Timestamp: time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC),
			Open:      4440.30, High: 4445.00, Low: 4420.00, Close: 4425.00,
			IsClosed: true,
		},
	}

	e := NewEvaluator()
	fvgs := e.FindFVGs(cs)
	var fvgAcuan *FVG
	for i := range fvgs {
		if fvgs[i].Type == FVGBearish && fvgs[i].Bottom == 4471.71 {
			fvgAcuan = &fvgs[i]
			break
		}
	}
	if fvgAcuan == nil {
		t.Fatalf("FVG bearish 4471.71 tidak terbentuk; fvgs=%+v", fvgs)
	}

	// Menurut dokumen baku, C2 sah pada candle 30 Aug 18:00 NY.
	c2 := EvaluateC2IRL(*fvgAcuan, cs)
	if c2.Status != C2Aktif {
		t.Fatalf("dokumen baku: C2 seharusnya AKTIF (candle menyapu acuan 4471.71); dapat %s (%s)",
			c2.Status, c2.Detail)
	}

	// Inilah yang diuji: apakah logika sweep menerima setup ini sebagai SELL?
	sweep := e.detectSweepPadaFVGInti(cs, fvgs, *fvgAcuan, false)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("MERAH (bukti): logika sweep sekarang MENOLAK setup SELL yang sah menurut draft.\n"+
			"C2 draft = %s, acuan %.2f.\nsweep = %+v", c2.Status, c2.Reference, sweep)
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("arah salah: %s", sweep.Direction)
	}
}

// Kasus 2 - XAUUSD H4, 09-11 Sep 2026 (waktu NEW YORK).
//
// FVG BEARISH 4400.32 - 4405.77
//
//	C1 = 09 Sep 22:00  Low  4405.77  (atap zona)
//	C2 = 10 Sep 02:00  Open 4431.07 -> Close 4394.15 (displacement turun)
//	C3 = 10 Sep 06:00  High 4400.32  (lantai zona = acuan awal)
//
// Candle penyapu: 11 Sep 10:00
//
//	Open 4401.74  High 4402.67  Low 4352.06  Close 4358.36
//
// High 4402.67 menembus acuan 4400.32, close 4358.36 ditolak jauh ke bawah.
// Trader menyatakan: ini SETUP SELL (draft benar).
//
// Catatan: high candle tepat sebelumnya adalah 4402.60, sehingga selisihnya
// hanya 0.07. Logika lama bergantung pada selisih sekecil itu sehingga setup
// yang jelas valid bisa hilang. Dokumen tidak memakai pembanding candle
// sebelumnya, melainkan level acuan FVG.
func TestSweepKasus2_H4_11Sep(t *testing.T) {
	// 09 Sep 22:00 NY = 10 Sep 02:00 UTC.
	base := time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC)

	cs := []indicators.Candle{
		// 09 Sep 22:00 NY
		bangunCandle(base, 0, 4420.00, 4440.00, 4405.77, 4412.00),
		// 10 Sep 02:00 NY (displacement turun)
		bangunCandle(base, 1, 4431.07, 4431.07, 4394.15, 4394.15),
		// 10 Sep 06:00 NY (candle 3, High = acuan awal 4400.32)
		bangunCandle(base, 2, 4394.15, 4400.32, 4390.00, 4396.00),
		// 10 Sep 10:00 NY
		bangunCandle(base, 3, 4396.00, 4399.00, 4393.00, 4397.00),
		// 10 Sep 14:00 NY
		bangunCandle(base, 4, 4397.00, 4402.60, 4395.00, 4399.00),
		// 10 Sep 18:00 NY
		bangunCandle(base, 5, 4399.00, 4401.00, 4396.00, 4398.00),
		// 10 Sep 22:00 NY
		bangunCandle(base, 6, 4398.00, 4400.50, 4395.00, 4397.00),
		// 11 Sep 02:00 NY
		bangunCandle(base, 7, 4397.00, 4399.50, 4394.00, 4396.00),
		// 11 Sep 06:00 NY
		bangunCandle(base, 8, 4396.00, 4398.00, 4392.00, 4394.00),
		// 11 Sep 10:00 NY - candle penyapu
		bangunCandle(base, 9, 4401.74, 4402.67, 4352.06, 4358.36),
		// candle penutup
		bangunCandle(base, 10, 4358.36, 4362.00, 4350.00, 4355.00),
	}

	e := NewEvaluator()
	fvgs := e.FindFVGs(cs)
	var fvgAcuan *FVG
	for i := range fvgs {
		if fvgs[i].Type == FVGBearish && fvgs[i].Bottom == 4400.32 {
			fvgAcuan = &fvgs[i]
			break
		}
	}
	if fvgAcuan == nil {
		t.Fatalf("FVG bearish 4400.32 tidak terbentuk; fvgs=%+v", fvgs)
	}

	// Menurut dokumen baku, C2 sah pada candle 11 Sep 10:00 NY.
	c2 := EvaluateC2IRL(*fvgAcuan, cs)
	if c2.Status != C2Aktif {
		t.Fatalf("dokumen baku: C2 seharusnya AKTIF (candle menyapu acuan 4400.32); dapat %s (%s)",
			c2.Status, c2.Detail)
	}

	sweep := e.detectSweepPadaFVGInti(cs, fvgs, *fvgAcuan, false)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("MERAH (bukti): logika sweep sekarang MENOLAK setup SELL yang sah menurut draft.\n"+
			"C2 draft = %s, acuan %.2f.\nsweep = %+v", c2.Status, c2.Reference, sweep)
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("arah salah: %s", sweep.Direction)
	}
}
