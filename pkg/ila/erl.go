package ila

// Key Level ERL (External Range Liquidity) untuk ILA 2.0.
//
// Aturan mengikuti dokumen baku ILA 2.0 "Key Level ERL (Swing High / Swing Low)":
//
//  1. Swing adalah pola TIGA candle berurutan; candle tengah adalah titiknya.
//  2. Swing Low  : Low tengah LEBIH RENDAH dari Low kiri DAN Low kanan.
//     Swing High : High tengah LEBIH TINGGI dari High kiri DAN High kanan.
//  3. Yang dibandingkan hanya wick (high/low), bukan body maupun close.
//     Warna candle tidak berpengaruh.
//  4. Level swing = low/high candle tengah, dan sekaligus batas invalidasi swing.
//  5. Swing baru dipastikan SETELAH candle kanan CLOSED.
//  6. Invalidasi HANYA dari close yang menembus level; wick boleh menembus.
//
// Pemetaan istilah ke dokumen:
//   - "level swing"   = Level di SwingERL (low/high candle tengah)
//   - "candle kanan"  = RightIndex, candle ketiga pembentuk swing
//
// Paket ini SENGAJA berdiri sendiri dan tidak memakai pkg/keylevels. pkg/keylevels
// adalah milik ILA 3.0 dan memakai fractal 5-bar dengan konsep mitigasi wick,
// sedangkan ILA 2.0 mengikuti draft trader yang memakai 3 candle dan invalidasi
// close murni. Menyatukan keduanya akan membuat dua engine dengan rules berbeda
// saling menimpa.

import (
	"time"

	"ila-engine/pkg/indicators"
)

// ERLType menandakan jenis key level ERL.
type ERLType string

const (
	// ERLSwingHigh adalah Swing High (sisi bearish).
	ERLSwingHigh ERLType = "SWING_HIGH"
	// ERLSwingLow adalah Swing Low (sisi bullish).
	ERLSwingLow ERLType = "SWING_LOW"
)

// SwingERL satu key level ERL hasil deteksi 3-candle.
//
// Struct ini hanya memuat HASIL PEMBENTUKAN. Status valid/invalid sengaja tidak
// disimpan di sini karena invalidasi adalah lapis aturan tersendiri; mintalah
// lewat SwingERLStatus agar hanya ada satu sumber kebenaran.
type SwingERL struct {
	// Type adalah jenis swing: Swing High atau Swing Low.
	Type ERLType `json:"type"`

	// Level adalah harga swing, yaitu Low (swing low) atau High (swing high)
	// candle tengah. Level ini juga menjadi batas invalidasi swing.
	Level float64 `json:"level"`

	// LeftIndex, MidIndex, RightIndex adalah indeks tiga candle pembentuk swing.
	LeftIndex  int `json:"left_index"`
	MidIndex   int `json:"mid_index"`
	RightIndex int `json:"right_index"`

	// MidTime adalah waktu candle tengah (candle titik swing).
	MidTime time.Time `json:"mid_time"`
}

// FindSwingERL mendeteksi seluruh Swing High dan Swing Low 3-candle pada deret
// candle yang diberikan. Fungsi ini HANYA melakukan pembentukan swing; ia tidak
// menilai validitas. Gunakan SwingERLStatus untuk itu.
//
// Candle yang belum closed tidak pernah dipakai sebagai pembentuk swing, karena
// high/low-nya belum final. Candle kanan yang masih berjalan berarti swing
// belum bisa dipastikan.
func FindSwingERL(candles []indicators.Candle) []SwingERL {
	n := len(candles)
	if n < 3 {
		return nil
	}

	var swings []SwingERL

	for i := 1; i < n-1; i++ {
		left, mid, right := candles[i-1], candles[i], candles[i+1]

		if !left.IsClosed || !mid.IsClosed || !right.IsClosed {
			continue
		}

		// Swing Low: low candle tengah paling rendah di antara tiga candle.
		if mid.Low < left.Low && mid.Low < right.Low {
			swings = append(swings, SwingERL{
				Type:       ERLSwingLow,
				Level:      mid.Low,
				LeftIndex:  i - 1,
				MidIndex:   i,
				RightIndex: i + 1,
				MidTime:    mid.Timestamp,
			})
		}

		// Swing High: high candle tengah paling tinggi di antara tiga candle.
		if mid.High > left.High && mid.High > right.High {
			swings = append(swings, SwingERL{
				Type:       ERLSwingHigh,
				Level:      mid.High,
				LeftIndex:  i - 1,
				MidIndex:   i,
				RightIndex: i + 1,
				MidTime:    mid.Timestamp,
			})
		}
	}

	return swings
}

// SwingERLStatus menilai apakah swing sudah invalid menurut dokumen baku ILA 2.0.
//
// Aturan invalidasi hanya memakai CLOSE:
//   - Swing Low  invalid bila ada close < Level
//   - Swing High invalid bila ada close > Level
//
// Wick yang menembus level TIDAK membatalkan swing. Evaluasi dimulai dari candle
// setelah candle kanan pembentuk swing, berurutan, dan berhenti pada candle
// running karena close-nya belum final.
//
// Mengembalikan invalid=true beserta indeks candle penyebabnya, atau
// invalid=false dengan index -1.
func SwingERLStatus(sw SwingERL, candles []indicators.Candle) (bool, int) {
	for k := sw.RightIndex + 1; k < len(candles); k++ {
		c := candles[k]
		if !c.IsClosed {
			break
		}
		if sw.Type == ERLSwingLow && c.Close < sw.Level {
			return true, k
		}
		if sw.Type == ERLSwingHigh && c.Close > sw.Level {
			return true, k
		}
	}
	return false, -1
}

// LatestSwingERL mengembalikan swing terbaru yang MASIH VALID untuk jenis
// tertentu. Iterasi dilakukan dari candle terbaru ke belakang, sehingga swing
// yang sudah invalid dilewati.
//
// Mengembalikan false bila tidak ada swing valid sama sekali.
func LatestSwingERL(candles []indicators.Candle, t ERLType) (SwingERL, bool) {
	swings := FindSwingERL(candles)
	for i := len(swings) - 1; i >= 0; i-- {
		if swings[i].Type != t {
			continue
		}
		if invalid, _ := SwingERLStatus(swings[i], candles); invalid {
			continue
		}
		return swings[i], true
	}
	return SwingERL{}, false
}

// ERLBiasOf menghitung bias dari Swing High dan Swing Low TERAKHIR, tanpa
// memandang status validasinya.
//
// Peran ERL di ILA 2.0 saat ini adalah PENENTU BIAS saja: tidak memicu entry dan
// tidak memfilter entry. Hasilnya dipakai untuk informasi di ringkasan.
//
// Aturan:
//   - close di atas Swing High terakhir -> BULLISH (harga sudah menembus atap)
//   - close di bawah Swing Low terakhir -> BEARISH (harga sudah menembus dasar)
//   - selain itu (harga masih di dalam rentang) -> NEUTRAL
//
// Catatan kausal: close yang menembus level swing sekaligus membuat swing itu
// invalid. Justru itulah tanda bias terkuat, karena harga sudah keluar dari
// rentang likuiditas sebelumnya. Karena itu di sini dipakai swing TERAKHIR,
// bukan "swing terakhir yang masih valid" — memakai yang valid saja akan membuat
// bias hilang tepat pada saat breakout terjadi.
func ERLBiasOf(candles []indicators.Candle) string {
	if len(candles) == 0 {
		return "NEUTRAL"
	}

	var closeTerakhir float64
	adaClose := false
	for i := len(candles) - 1; i >= 0; i-- {
		if candles[i].Close > 0 {
			closeTerakhir = candles[i].Close
			adaClose = true
			break
		}
	}
	if !adaClose {
		return "NEUTRAL"
	}

	sh, adaSH := terakhirSwing(candles, ERLSwingHigh)
	sl, adaSL := terakhirSwing(candles, ERLSwingLow)

	if adaSH && closeTerakhir > sh.Level {
		return "BULLISH"
	}
	if adaSL && closeTerakhir < sl.Level {
		return "BEARISH"
	}
	return "NEUTRAL"
}

// terakhirSwing mengembalikan swing terakhir berjenis t, tanpa menilai validasi.
func terakhirSwing(candles []indicators.Candle, t ERLType) (SwingERL, bool) {
	swings := FindSwingERL(candles)
	for i := len(swings) - 1; i >= 0; i-- {
		if swings[i].Type == t {
			return swings[i], true
		}
	}
	return SwingERL{}, false
}
