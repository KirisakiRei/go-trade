package ila

// C2 closure pada key level ERL (Swing High / Swing Low). Sumber aturan: "New ILA 2.0/c2-closure-erl.md" A2/B2.

import (
	"fmt"

	"ila-engine/pkg/indicators"
)

// EvaluateC2ERL mengevaluasi C2 closure pada swing hasil FindERL (dengan candle yang sama).
func EvaluateC2ERL(sw SwingERL, candles []indicators.Candle) C2Result {
	replacedAt := -1
	if sw.Status == StatusReplaced {
		replacedAt = sw.InvalidIndex
	}
	high := sw.Type == ERLSwingHigh
	name := "Swing Low"
	if high {
		name = "Swing High"
	}
	res := C2Result{Status: C2None, C2Index: -1, Reference: sw.Level, InvalidIndex: -1}

	for i := sw.RightIndex + 1; i < len(candles); i++ {
		c := candles[i]
		// Urutan 1: candle ini = candle kanan swing sejenis baru.
		if i == replacedAt {
			return c2Invalid(res, i, fmt.Sprintf("%s digantikan swing sejenis baru (bukan FTM)", name))
		}
		if !c.IsClosed {
			res.RunningTouch = (high && c.High >= res.Reference) || (!high && c.Low <= res.Reference)
			break
		}
		// Urutan 2: close di luar level swing -> FTM.
		if closeBeyondSwing(sw, c) {
			return c2Invalid(res, i, fmt.Sprintf("%s invalid (FTM): close %s di luar level %s", name, formatILAVal(c.Close), formatILAVal(sw.Level)))
		}
		// Urutan 3: wick tidak menyentuh acuan -> diabaikan.
		if (high && c.High < res.Reference) || (!high && c.Low > res.Reference) {
			continue
		}
		// Urutan 4: close ditolak dari acuan -> C2 baru / berpindah.
		// Close tepat di acuan tidak mengubah state (pseudocode tidak punya cabang lain).
		if (high && c.Close < res.Reference) || (!high && c.Close > res.Reference) {
			res.Status, res.C2Index = C2Active, i
			if high {
				res.Reference = c.High
			} else {
				res.Reference = c.Low
			}
		}
	}
	res.Detail = c2Detail(name, res, candles)
	return res
}
