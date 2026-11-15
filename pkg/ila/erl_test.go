package ila

// Test Key Level ERL 3-candle sesuai dokumen baku ILA 2.0
// "Key Level ERL (Swing High / Swing Low)".
//
// Pembentukan (bagian 1-3) diuji terpisah dari invalidasi (bagian 4), karena di
// kode pun keduanya fungsi berbeda: FindSwingERL dan SwingERLStatus.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func erlBase() time.Time {
	return time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
}

// Helper: ambil swing pertama dengan jenis dan level tertentu, karena data uji
// bisa saja memicu swing tambahan yang tidak relevan bagi kasus yang diuji.
func cariSwing(cs []indicators.Candle, typ ERLType, level float64) (SwingERL, bool) {
	for _, s := range FindSwingERL(cs) {
		if s.Type == typ && s.Level == level {
			return s, true
		}
	}
	return SwingERL{}, false
}

// --- PEMBENTUKAN ---

// Swing Low: low candle tengah paling rendah dari kiri dan kanan.
func TestSwingLow3Candle(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102), // low kiri 98
		candleBerurutan(b, 1, 102, 103, 90, 95),  // low tengah 90  <- paling rendah
		candleBerurutan(b, 2, 95, 100, 94, 99),   // low kanan 94
	}

	sw, ok := cariSwing(cs, ERLSwingLow, 90)
	if !ok {
		t.Fatalf("Swing Low level 90 tidak terdeteksi: %+v", FindSwingERL(cs))
	}
	if sw.MidIndex != 1 || sw.LeftIndex != 0 || sw.RightIndex != 2 {
		t.Fatalf("indeks salah: kiri %d tengah %d kanan %d", sw.LeftIndex, sw.MidIndex, sw.RightIndex)
	}
}

// Swing High: high candle tengah paling tinggi dari kiri dan kanan.
func TestSwingHigh3Candle(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 120, 101, 115), // high tengah 120 <- paling tinggi
		candleBerurutan(b, 2, 115, 112, 108, 110),
	}

	sw, ok := cariSwing(cs, ERLSwingHigh, 120)
	if !ok {
		t.Fatalf("Swing High level 120 tidak terdeteksi: %+v", FindSwingERL(cs))
	}
	if sw.MidIndex != 1 {
		t.Fatalf("harapan candle tengah idx 1, dapat %d", sw.MidIndex)
	}
}

// Draft: warna candle tidak berpengaruh. Candle tengah bullish maupun bearish
// sama-sama bisa menjadi titik swing.
func TestSwingWarnaTidakBerpengaruh(t *testing.T) {
	b := erlBase()

	// Tengah bearish (close < open) sebagai swing low.
	bearish := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 91),
		candleBerurutan(b, 2, 91, 100, 94, 99),
	}
	if sw := FindSwingERL(bearish); len(sw) != 1 || sw[0].Type != ERLSwingLow {
		t.Fatalf("candle tengah bearish seharusnya tetap terdeteksi Swing Low, dapat %+v", sw)
	}

	// Tengah bullish (close > open) sebagai swing low.
	bullish := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 90, 103, 90, 100),
		candleBerurutan(b, 2, 100, 104, 94, 99),
	}
	if sw := FindSwingERL(bullish); len(sw) != 1 || sw[0].Type != ERLSwingLow {
		t.Fatalf("candle tengah bullish seharusnya juga terdeteksi Swing Low, dapat %+v", sw)
	}
}

// Draft mensyaratkan LEBIH rendah/tinggi secara ketat. Low yang sama tidak
// membentuk swing.
func TestSwingLowSamaDenganKananTidakSah(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 95),
		candleBerurutan(b, 2, 95, 100, 90, 99), // low kanan == low tengah
	}
	if sw := FindSwingERL(cs); len(sw) != 0 {
		t.Fatalf("low tengah == low kanan bukan swing (harus ketat), dapat %+v", sw)
	}
}

// Draft: swing baru dipastikan setelah candle kanan CLOSED.
func TestSwingKananRunningBelumSah(t *testing.T) {
	b := erlBase()
	kanan := candleBerurutan(b, 2, 95, 100, 94, 99)
	kanan.IsClosed = false

	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 95),
		kanan,
	}
	if sw := FindSwingERL(cs); len(sw) != 0 {
		t.Fatalf("candle kanan masih running, swing belum sah, dapat %+v", sw)
	}
}

// --- INVALIDASI (bagian 4: hanya close) ---

func TestSwingLowInvalidClose(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 95),
		candleBerurutan(b, 2, 95, 100, 94, 99),
		candleBerurutan(b, 3, 99, 100, 92, 95), // aman, wick 92 > 90? tidak, close 95 > 90
		candleBerurutan(b, 4, 95, 96, 80, 85),  // close 85 < 90 -> INVALID
	}
	sw := FindSwingERL(cs)
	if len(sw) != 1 {
		t.Fatalf("harapan 1 swing, dapat %d", len(sw))
	}

	invalid, idx := SwingERLStatus(sw[0], cs)
	if !invalid {
		t.Fatal("harapan invalid: close 85 di bawah level swing low 90")
	}
	if idx != 4 {
		t.Fatalf("harapan idx invalidasi 4, dapat %d", idx)
	}
}

// Wick menembus level swing low tetapi close kembali di atasnya -> TETAP VALID.
func TestSwingLowWickTembusTetapValid(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 95),
		candleBerurutan(b, 2, 95, 100, 94, 99),
		// Wick turun 80 (menembus level 90), tetapi close 95 di atas level.
		candleBerurutan(b, 3, 99, 99, 80, 95),
	}
	sw := FindSwingERL(cs)
	if len(sw) != 1 {
		t.Fatalf("harapan 1 swing, dapat %d", len(sw))
	}

	invalid, idx := SwingERLStatus(sw[0], cs)
	if invalid {
		t.Fatalf("wick menembus level tidak membatalkan swing; dapat invalid di idx %d", idx)
	}
}

// Cermin: Swing High invalid hanya oleh close di atas level.
func TestSwingHighWickTembusTetapValid(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 120, 101, 115),
		candleBerurutan(b, 2, 115, 112, 108, 110),
		// Wick naik 130 menembus level 120, close 118 di bawah.
		candleBerurutan(b, 3, 110, 130, 109, 118),
	}
	sw := FindSwingERL(cs)
	if len(sw) != 1 {
		t.Fatalf("harapan 1 swing, dapat %d", len(sw))
	}
	if invalid, idx := SwingERLStatus(sw[0], cs); invalid {
		t.Fatalf("wick 130 menembus 120 tapi close 118: swing harus tetap valid, dapat invalid di idx %d", idx)
	}
}

func TestSwingHighInvalidClose(t *testing.T) {
	b := erlBase()
	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 120, 101, 115),
		candleBerurutan(b, 2, 115, 112, 108, 110),
		candleBerurutan(b, 3, 110, 112, 105, 108),
		candleBerurutan(b, 4, 108, 125, 107, 122), // close 122 > 120 -> INVALID
		candleBerurutan(b, 5, 122, 124, 118, 120), // high 124 < 125, tidak membentuk swing baru
	}
	sw, ok := cariSwing(cs, ERLSwingHigh, 120)
	if !ok {
		t.Fatalf("Swing High level 120 tidak terdeteksi: %+v", FindSwingERL(cs))
	}
	invalid, idx := SwingERLStatus(sw, cs)
	if !invalid || idx != 4 {
		t.Fatalf("harapan invalid di idx 4, dapat invalid=%v idx=%d", invalid, idx)
	}
}

// Candle running tidak boleh menilai invalidasi.
func TestSwingInvalidasiLewatCandleRunning(t *testing.T) {
	b := erlBase()
	running := candleBerurutan(b, 3, 99, 99, 80, 85)
	running.IsClosed = false

	cs := []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 103, 90, 95),
		candleBerurutan(b, 2, 95, 100, 94, 99),
		running,
	}
	sw := FindSwingERL(cs)
	if len(sw) != 1 {
		t.Fatalf("harapan 1 swing, dapat %d", len(sw))
	}
	if invalid, idx := SwingERLStatus(sw[0], cs); invalid {
		t.Fatalf("candle running tidak boleh men-invalidasi swing, dapat invalid di idx %d", idx)
	}
}

// --- SOAL BIAS ---
//
// ERLBiasOf membandingkan close terakhir dengan swing valid terbaru.
// Data uji dijaga supaya hanya ada satu Swing High dan satu Swing Low valid.

// bangunBias membentuk deret dengan:
//   - Swing High level 120 (mid idx 1)
//   - Swing Low  level 100 (mid idx 4)
//   - candle terakhir ditentukan pemanggil (idx 6)
func bangunBias(b time.Time, h, l, c float64) []indicators.Candle {
	return []indicators.Candle{
		candleBerurutan(b, 0, 100, 105, 98, 102),
		candleBerurutan(b, 1, 102, 120, 101, 115), // swing high 120
		candleBerurutan(b, 2, 115, 112, 108, 110),
		candleBerurutan(b, 3, 110, 112, 105, 106),
		candleBerurutan(b, 4, 106, 107, 100, 104), // swing low 100
		candleBerurutan(b, 5, 104, 110, 103, 108),
		candleBerurutan(b, 6, 108, h, l, c),
	}
}

// Close di antara swing low 100 dan swing high 120 -> NEUTRAL.
func TestERLBiasNeutralDiAntara(t *testing.T) {
	cs := bangunBias(erlBase(), 110, 103, 108)
	if got := ERLBiasOf(cs); got != "NEUTRAL" {
		t.Fatalf("close 108 di antara swing low 100 dan swing high 120, harapan NEUTRAL, dapat %s", got)
	}
}

// Close menembus ke atas Swing High terakhir -> BULLISH.
//
// Perhatikan: close 130 sekaligus men-invalidasi swing high 120. Itu justru
// tanda bias bullish terkuat (harga keluar dari rentang likuiditas lama), jadi
// bias harus tetap BULLISH, bukan NEUTRAL. Inilah alasan ERLBiasOf memakai
// swing TERAKHIR, bukan swing terakhir yang masih valid.
func TestERLBiasSwingHighDitemnbus(t *testing.T) {
	cs := bangunBias(erlBase(), 132, 107, 130)
	if got := ERLBiasOf(cs); got != "BULLISH" {
		t.Fatalf("close 130 > swing high 120, harapan BULLISH, dapat %s", got)
	}
}

// Close turun di bawah swing low valid -> bias BEARISH.
// Candle idx6 turun ke 95 tanpa membentuk swing low baru (low kiri 103 lebih rendah).
func TestERLBiasBearish(t *testing.T) {
	cs := bangunBias(erlBase(), 110, 95, 96)
	if got := ERLBiasOf(cs); got != "BEARISH" {
		t.Fatalf("harapan BEARISH, dapat %s", got)
	}
}
