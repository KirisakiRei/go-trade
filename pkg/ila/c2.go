package ila

// C2 Closure pada key level IRL (SIBI / BISI) dan ERL (Swing High / Swing Low).
//
// Aturan mengikuti dokumen baku ILA 2.0 "C2 Closure pada Key Level IRL/ERL".
//
// Inti gagasan: sebuah candle disebut C2 bila wick-nya menyapu LEVEL ACUAN lalu
// ditolak dengan close di sisi yang benar.
//
// Yang membuat C2 berbeda dari sekadar "sapuan terhadap candle sebelumnya":
//
//  1. Level acuan BERGESER. Awalnya batas FVG (IRL) atau level swing (ERL).
//     Setelah ada C2, acuan pindah ke high/low candle C2 itu sendiri.
//  2. Status C2 BERPINDAH ke candle berikutnya yang menyapu acuan baru dan
//     ditolak lagi.
//  3. C2 bisa GUGUR tanpa FVG ikut mati: bila candle menyapu acuan baru namun
//     justru close melewatinya (masih di dalam zona), C2 lama gugur dan acuan
//     bergeser ke candle tersebut. Ini HANYA ada di IRL (Urutan 4).
//  4. FVG/Swing baru INVALID bila ada close melewati batas terjauhnya.
//
// Catatan kausal yang penting: begitu FVG/Swing dinyatakan invalid, evaluasi
// berhenti. Candle setelahnya tidak pernah bisa menjadi C2, walaupun harganya
// kembali ke dalam zona.
//
// Sumber kebenaran invalidasi TIDAK dikodekan ulang di sini:
//   - IRL memakai FVG.Invalidated (lihat FindFVGs)
//   - ERL memakai SwingERLStatus (lihat erl.go)

import (
	"time"

	"ila-engine/pkg/indicators"
)

// C2Status adalah hasil evaluasi C2 Closure untuk satu key level.
type C2Status string

const (
	// C2BelumAda: key level masih hidup, belum ada candle yang menyapu acuan.
	C2BelumAda C2Status = "BELUM_ADA_C2"
	// C2Aktif: ada candle C2 yang berlaku saat ini.
	C2Aktif C2Status = "C2_AKTIF"
	// C2Invalid: key level sudah mati karena close melewati batas invalidasi.
	C2Invalid C2Status = "INVALID"
)

// C2Result merangkum hasil evaluasi C2 Closure.
type C2Result struct {
	Status C2Status `json:"status"`

	// Reference adalah level acuan terakhir yang dipakai menguji candle
	// berikutnya. Awalnya batas FVG / level swing, lalu bergeser mengikuti
	// candle terakhir yang menyapu acuan.
	Reference float64 `json:"reference"`

	// CandleIndex adalah indeks candle C2 aktif. -1 bila tidak ada C2 aktif.
	CandleIndex int `json:"candle_index"`
	// CandleTime adalah waktu candle C2 aktif (nol bila tidak ada).
	CandleTime time.Time `json:"candle_time"`

	// InvalidIndex adalah indeks candle yang membatalkan key level. -1 bila belum.
	InvalidIndex int `json:"invalid_index"`

	// Pergeseran mencatat berapa kali acuan bergeser karena C2 baru/berpindah/gugur.
	Pergeseran int `json:"pergeseran"`

	// Detail ringkas untuk ditampilkan di ringkasan/alert.
	Detail string `json:"detail"`
}

// referenceAwalIRL menentukan level acuan awal sebuah FVG.
//
// Dokumen c2-closure-irl.md menegaskan acuan awal BUKAN batas zona terdekat,
// melainkan high/low candle 3 pembentuk FVG:
//   - Bearish / SIBI: high candle 3 (= Bottom zona)
//   - Bullish / BISI: low candle 3  (= Top zona)
func referenceAwalIRL(fvg FVG, candles []indicators.Candle) float64 {
	valid := fvg.Candle3Index >= 0 && fvg.Candle3Index < len(candles)
	if fvg.Type == FVGBearish {
		if valid {
			return candles[fvg.Candle3Index].High
		}
		return fvg.Bottom
	}
	if valid {
		return candles[fvg.Candle3Index].Low
	}
	return fvg.Top
}

// EvaluateC2IRL mengevaluasi C2 Closure untuk satu FVG (SIBI / BISI).
//
// Urutan pemeriksaan per candle WAJIB seperti dokumen (c2-closure-irl.md bagian
// A3 untuk bearish, B3 untuk bullish):
//
//  1. close melewati batas invalidasi FVG -> FVG INVALID (berhenti)
//  2. wick tidak menyentuh acuan          -> diabaikan, C2 lama tetap
//  3. wick menyentuh acuan, close ditolak -> C2 baru / C2 berpindah
//  4. wick menyentuh acuan, close melewati acuan tetapi masih di dalam zona
//     -> bukan C2, C2 lama gugur, acuan bergeser
//
// Urutan 4 inilah yang membedakan definisi draft dari logika lama: acuan boleh
// bergeser ke high/low candle terakhir yang gagal ditolak, sehingga candle
// berikutnya cukup menyapu acuan baru dan tidak harus menyapu candle k-1.
func EvaluateC2IRL(fvg FVG, candles []indicators.Candle) C2Result {
	res := C2Result{
		Status:       C2BelumAda,
		Reference:    referenceAwalIRL(fvg, candles),
		CandleIndex:  -1,
		InvalidIndex: -1,
	}

	// Urutan 1 sudah dinilai saat FVG dibentuk. Bila invalid, tidak ada C2.
	if fvg.Invalidated {
		res.Status = C2Invalid
		res.InvalidIndex = fvg.InvalidatedIndex
		res.Detail = "FVG invalid: ada close yang menembus batas zona"
		return res
	}

	bearish := fvg.Type == FVGBearish
	start := fvg.Candle3Index + 1
	if start < 0 {
		return res
	}

	for i := start; i < len(candles); i++ {
		c := candles[i]
		// Close yang belum final tidak boleh dipakai: status C2 bersifat sementara
		// sampai candle benar-benar ditutup.
		if !c.IsClosed {
			break
		}

		if bearish {
			// Urutan 1: invalidasi SIBI hanya dari close di atas batas atas FVG.
			if c.Close > fvg.Top {
				res.Status = C2Invalid
				res.InvalidIndex = i
				res.CandleIndex = -1
				res.Detail = "SIBI invalid: ada close di atas batas atas FVG"
				return res
			}
			// Urutan 2: high tidak menyentuh acuan -> diabaikan, C2 lama tetap.
			if c.High < res.Reference {
				continue
			}
			// Urutan 3: close kembali di bawah acuan -> C2 baru / C2 berpindah.
			// Close yang TEPAT sama dengan acuan BUKAN C2 (dokumen tegas).
			if c.Close < res.Reference {
				res.Status = C2Aktif
				res.CandleIndex = i
				res.CandleTime = c.Timestamp
				res.Reference = c.High
				res.Pergeseran++
				continue
			}
			// Urutan 4: menyentuh acuan tetapi close di atas acuan dan masih di
			// dalam zona -> C2 lama gugur, acuan bergeser ke high candle ini.
			// Cabang "close di atas batas atas" sudah ditangani Urutan 1.
			res.Status = C2BelumAda
			res.CandleIndex = -1
			res.Reference = c.High
			res.Pergeseran++
			continue
		}

		// Bullish / BISI (cermin dari sisi bearish)
		if c.Close < fvg.Bottom {
			res.Status = C2Invalid
			res.InvalidIndex = i
			res.CandleIndex = -1
			res.Detail = "BISI invalid: ada close di bawah batas bawah FVG"
			return res
		}
		if c.Low > res.Reference {
			continue
		}
		if c.Close > res.Reference {
			res.Status = C2Aktif
			res.CandleIndex = i
			res.CandleTime = c.Timestamp
			res.Reference = c.Low
			res.Pergeseran++
			continue
		}
		res.Status = C2BelumAda
		res.CandleIndex = -1
		res.Reference = c.Low
		res.Pergeseran++
	}

	if res.Status == C2Aktif {
		res.Detail = "C2 Closure aktif"
	} else {
		res.Detail = "FVG masih hidup, belum ada C2"
	}
	return res
}

// EvaluateC2ERL mengevaluasi C2 Closure untuk Swing High / Swing Low (ERL).
//
// Berbeda dengan IRL, ERL hanya punya TIGA urutan (c2-closure-erl.md A2 / B2).
// Urutan 4 tidak ada karena acuan setelah ada C2 selalu berada di level swing
// atau di luarnya; candle yang close melewati acuan otomatis juga close melewati
// level swing, sehingga langsung masuk Urutan 1 (SWING INVALID).
//
// Invalidasi TIDAK dihitung ulang di sini. Sumber kebenarannya SwingERLStatus
// supaya aturan invalidasi hanya hidup di satu tempat.
//
// sw adalah swing yang dievaluasi; candles adalah deret HTF yang sama dengan
// yang dipakai mendeteksi sw.
func EvaluateC2ERL(sw SwingERL, candles []indicators.Candle) C2Result {
	res := C2Result{
		Status:       C2BelumAda,
		Reference:    sw.Level,
		CandleIndex:  -1,
		InvalidIndex: -1,
	}

	if invalid, idx := SwingERLStatus(sw, candles); invalid {
		res.Status = C2Invalid
		res.InvalidIndex = idx
		res.Detail = "Swing invalid: ada close yang menembus level swing"
		return res
	}

	swingHigh := sw.Type == ERLSwingHigh
	start := sw.RightIndex + 1
	if start < 0 {
		return res
	}

	for i := start; i < len(candles); i++ {
		c := candles[i]
		if !c.IsClosed {
			break
		}

		if swingHigh {
			// Urutan 1 dipastikan tidak terjadi oleh SwingERLStatus di atas.
			// Urutan 2: high tidak menyentuh acuan -> diabaikan.
			if c.High < res.Reference {
				continue
			}
			// Urutan 3: close di bawah acuan -> C2 baru / C2 berpindah.
			if c.Close < res.Reference {
				res.Status = C2Aktif
				res.CandleIndex = i
				res.CandleTime = c.Timestamp
				res.Reference = c.High
				res.Pergeseran++
			}
			continue
		}

		// Swing Low
		if c.Low > res.Reference {
			continue
		}
		if c.Close > res.Reference {
			res.Status = C2Aktif
			res.CandleIndex = i
			res.CandleTime = c.Timestamp
			res.Reference = c.Low
			res.Pergeseran++
		}
	}

	if res.Status == C2Aktif {
		res.Detail = "C2 Closure aktif"
	} else {
		res.Detail = "Swing masih valid, belum ada C2"
	}
	return res
}
