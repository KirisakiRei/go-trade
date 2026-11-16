package ila

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// TestFVGTermudaGagalTidakMundurKeFVGLebihTua mengunci aturan pemilihan FVG acuan:
//
//  1. FVG aktif TERMUDA dipilih lebih dulu, bukan elemen terakhir daftar.
//  2. Bila FVG termuda masih dijangkau harga, dialah acuannya.
//  3. Bila FVG termuda TIDAK dijangkau harga, acuan diperiksa pada FVG lebih tua
//     dari yang termuda ke belakang, berhenti pada yang sweep-nya sah.
//  4. Bila tak satu pun menghasilkan sweep sah, tidak ada acuan sama sekali.
//
// Butir 4 yang dikunci di sini. Versi lama mampu mengembalikan acuan arah dari
// FVG tertua yang tersisa walau tak ada satu pun yang disapu pasar.
func TestFVGTermudaGagalTidakMundurKeFVGLebihTua(t *testing.T) {
	eval := NewEvaluator()
	awal := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	jam := func(i int) time.Time { return awal.Add(time.Duration(i) * 4 * time.Hour) }

	// FVG BEARISH 3887.64-3940.00 (Candle 3 = indeks 2), lalu harga TURUN dan
	// bertahan jauh di bawah zona sehingga zona itu tidak pernah disapu.
	candles := []indicators.Candle{
		{Timestamp: jam(0), Open: 3965.88, High: 3972.51, Low: 3940.00, Close: 3954.01, IsClosed: true},
		{Timestamp: jam(1), Open: 3954.01, High: 3955.51, Low: 3887.00, Close: 3890.00, IsClosed: true},
		{Timestamp: jam(2), Open: 3890.00, High: 3887.64, Low: 3852.64, Close: 3882.88, IsClosed: true},
		{Timestamp: jam(3), Open: 3820.00, High: 3830.00, Low: 3790.00, Close: 3800.00, IsClosed: true},
		{Timestamp: jam(4), Open: 3800.00, High: 3815.00, Low: 3770.00, Close: 3780.00, IsClosed: true},
		{Timestamp: jam(5), Open: 3775.00, High: 3810.00, Low: 3750.00, Close: 3800.00, IsClosed: true},
		{Timestamp: jam(6), Open: 3800.00, High: 3805.00, Low: 3740.00, Close: 3745.00, IsClosed: true},
		{Timestamp: jam(7), Open: 3745.00, High: 3760.00, Low: 3700.00, Close: 3705.00, IsClosed: true},
	}

	aktif := []FVG{}
	for _, f := range eval.FindFVGs(candles) {
		if !f.Mitigated {
			aktif = append(aktif, f)
		}
	}
	if len(aktif) == 0 {
		t.Fatalf("prasyarat setup: harus ada FVG aktif")
	}
	for _, f := range aktif {
		if fvgMasihDijangkauHarga(f, candles) {
			t.Fatalf("prasyarat setup: FVG c3=%d masih dijangkau harga", f.Candle3Index)
		}
		if len(candles)-1-f.Candle3Index > umurMaksimalFVG {
			t.Fatalf("prasyarat setup: FVG c3=%d sudah lewat batas umur", f.Candle3Index)
		}
	}

	if sweep := eval.DetectHTFIlaSweep(candles); sweep != nil {
		t.Fatalf("masih ada acuan %s dari FVG %s-%s; seharusnya nil karena tidak ada "+
			"FVG yang dijangkau harga maupun disapu",
			sweep.Direction, formatILAVal(sweep.FVG.Bottom), formatILAVal(sweep.FVG.Top))
	}
}

// TestFVGTerpilihYangTermuda mengunci pemilihan acuan: begitu FVG terbaru
// termitigasi, acuan jatuh ke FVG aktif yang paling MUDA, bukan elemen terakhir
// daftar (yang justru paling tua). Susunan candle mengikuti XAUUSD H4 12 Nov
// 2026: FVG bearish besar disapu candle 18:00 dan 22:00, sementara FVG bullish
// kecil di antaranya tidak pernah disentuh lagi.
func TestFVGTerpilihYangTermuda(t *testing.T) {
	eval := NewEvaluator()
	awal := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)

	candles := []indicators.Candle{
		{Timestamp: awal.Add(-16 * time.Hour), Open: 3965.88, High: 3972.51, Low: 3924.74, Close: 3954.01, IsClosed: true},
		{Timestamp: awal.Add(-12 * time.Hour), Open: 3954.01, High: 3955.51, Low: 3866.33, Close: 3870.07, IsClosed: true},
		{Timestamp: awal.Add(-8 * time.Hour), Open: 3870.07, High: 3887.64, Low: 3852.64, Close: 3882.88, IsClosed: true},
		{Timestamp: awal.Add(-4 * time.Hour), Open: 3882.88, High: 3886.93, Low: 3872.16, Close: 3877.36, IsClosed: true},
		{Timestamp: awal, Open: 3877.36, High: 3916.74, Low: 3875.20, Close: 3915.86, IsClosed: true},
		{Timestamp: awal.Add(4 * time.Hour), Open: 3915.86, High: 3923.20, Low: 3888.47, Close: 3900.61, IsClosed: true},
	}

	sweep := eval.DetectHTFIlaSweep(candles)
	if sweep == nil || !sweep.Detected {
		t.Fatalf("sweep bearish pada FVG 3887.64-3924.74 harus tetap sah")
	}
	if sweep.Direction != "BEARISH" {
		t.Fatalf("arah sweep = %s; ingin BEARISH", sweep.Direction)
	}
	if sweep.FVG == nil || sweep.FVG.Top != 3924.74 {
		t.Errorf("acuan harus FVG bearish 3924.74, dapat %+v", sweep.FVG)
	}
}
