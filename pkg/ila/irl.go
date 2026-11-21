package ila

// Key Level IRL (SIBI / BISI). Sumber aturan: "New ILA 2.0/key-level-irl.md".
//   - Pembentukan hanya dari gap 3 candle closed; warna candle 2 tidak disyaratkan.
//   - Invalid hanya karena close body di luar batas zona (iFVG), wick tidak dihitung.
//   - FVG sejenis baru (candle 3-nya close) langsung menggantikan yang lama (REPLACED, bukan iFVG).

import "ila-engine/pkg/indicators"

// FindIRL mendeteksi seluruh SIBI/BISI beserta statusnya, memakai hanya candle
// yang diberikan (kausal: tidak melihat candle di luar slice).
func FindIRL(candles []indicators.Candle) []FVG {
	var fvgs []FVG
	for i := 2; i < len(candles); i++ {
		c1, c2, c3 := candles[i-2], candles[i-1], candles[i]
		if !c1.IsClosed || !c2.IsClosed || !c3.IsClosed {
			continue
		}
		f := FVG{
			Candle1Index: i - 2, Candle2Index: i - 1, Candle3Index: i,
			Candle1Time: c1.Timestamp, Candle2Time: c2.Timestamp, Candle3Time: c3.Timestamp,
			Status: StatusValid, InvalidIndex: -1,
		}
		switch {
		case c1.Low > c3.High:
			f.Type, f.Top, f.Bottom = FVGSIBI, c1.Low, c3.High
		case c1.High < c3.Low:
			f.Type, f.Top, f.Bottom = FVGBISI, c3.Low, c1.High
		default:
			continue
		}
		fvgs = append(fvgs, f)
	}
	for i := range fvgs {
		resolveIRLStatus(&fvgs[i], nextSameIRL(fvgs, i), candles)
	}
	return fvgs
}

// nextSameIRL mengembalikan Candle3Index FVG sejenis berikutnya, atau -1.
func nextSameIRL(fvgs []FVG, i int) int {
	for j := i + 1; j < len(fvgs); j++ {
		if fvgs[j].Type == fvgs[i].Type {
			return fvgs[j].Candle3Index
		}
	}
	return -1
}

// resolveIRLStatus: urutan cek per candle = digantikan (1) lalu close tembus (2),
// sama dengan urutan di c2-closure-irl.md A3.
func resolveIRLStatus(f *FVG, replacedAt int, candles []indicators.Candle) {
	for k := f.Candle3Index + 1; k < len(candles); k++ {
		if k == replacedAt {
			f.Status, f.InvalidIndex = StatusReplaced, k
			return
		}
		c := candles[k]
		if !c.IsClosed {
			return
		}
		if (f.Type == FVGSIBI && c.Close > f.Top) || (f.Type == FVGBISI && c.Close < f.Bottom) {
			f.Status, f.InvalidIndex = StatusIFVG, k
			return
		}
	}
}

// LatestIRL mengembalikan FVG sejenis paling baru (tidak pernah mundur ke FVG lama).
func LatestIRL(fvgs []FVG, typ FVGType) *FVG {
	for i := len(fvgs) - 1; i >= 0; i-- {
		if fvgs[i].Type == typ {
			f := fvgs[i]
			return &f
		}
	}
	return nil
}
