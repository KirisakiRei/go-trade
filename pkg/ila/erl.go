package ila

// Key Level ERL (Swing High / Swing Low). Sumber aturan: "New ILA 2.0/key-level-erl.md".
//   - Pola 3 candle closed (candle kiri, tengah, kanan), hanya membandingkan wick.
//   - Invalid hanya karena close body di luar level swing (FTM), wick tidak dihitung.
//   - Swing sejenis baru (candle kanan-nya close) langsung menggantikan yang lama (REPLACED, bukan FTM).
//
// Sengaja tidak memakai pkg/keylevels (milik ILA 3.0, fractal 5-bar + mitigasi wick).

import "ila-engine/pkg/indicators"

// FindERL mendeteksi seluruh swing high/low beserta statusnya, memakai hanya
// candle yang diberikan (kausal: tidak melihat candle di luar slice).
func FindERL(candles []indicators.Candle) []SwingERL {
	var swings []SwingERL
	for i := 1; i < len(candles)-1; i++ {
		kiri, tengah, kanan := candles[i-1], candles[i], candles[i+1]
		if !kiri.IsClosed || !tengah.IsClosed || !kanan.IsClosed {
			continue
		}
		base := SwingERL{LeftIndex: i - 1, MidIndex: i, RightIndex: i + 1, MidTime: tengah.Timestamp,
			Status: StatusValid, InvalidIndex: -1}
		if tengah.Low < kiri.Low && tengah.Low < kanan.Low {
			sw := base
			sw.Type, sw.Level = ERLSwingLow, tengah.Low
			swings = append(swings, sw)
		}
		if tengah.High > kiri.High && tengah.High > kanan.High {
			sw := base
			sw.Type, sw.Level = ERLSwingHigh, tengah.High
			swings = append(swings, sw)
		}
	}
	for i := range swings {
		resolveERLStatus(&swings[i], nextSameERL(swings, i), candles)
	}
	return swings
}

// nextSameERL mengembalikan RightIndex swing sejenis berikutnya, atau -1.
func nextSameERL(swings []SwingERL, i int) int {
	for j := i + 1; j < len(swings); j++ {
		if swings[j].Type == swings[i].Type {
			return swings[j].RightIndex
		}
	}
	return -1
}

// resolveERLStatus: urutan cek per candle = digantikan (1) lalu close tembus (2),
// sama dengan urutan di c2-closure-erl.md A2/B2.
func resolveERLStatus(sw *SwingERL, replacedAt int, candles []indicators.Candle) {
	for k := sw.RightIndex + 1; k < len(candles); k++ {
		if k == replacedAt {
			sw.Status, sw.InvalidIndex = StatusReplaced, k
			return
		}
		c := candles[k]
		if !c.IsClosed {
			return
		}
		if closeBeyondSwing(*sw, c) {
			sw.Status, sw.InvalidIndex = StatusFTM, k
			return
		}
	}
}

func closeBeyondSwing(sw SwingERL, c indicators.Candle) bool {
	if sw.Type == ERLSwingHigh {
		return c.Close > sw.Level
	}
	return c.Close < sw.Level
}

// LatestERL mengembalikan swing sejenis paling baru (tidak pernah mundur ke swing lama).
func LatestERL(swings []SwingERL, typ ERLType) *SwingERL {
	for i := len(swings) - 1; i >= 0; i-- {
		if swings[i].Type == typ {
			sw := swings[i]
			return &sw
		}
	}
	return nil
}
