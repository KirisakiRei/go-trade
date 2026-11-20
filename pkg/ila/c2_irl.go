package ila

// C2 closure pada key level IRL (SIBI / BISI). Sumber aturan: "New ILA 2.0/c2-closure-irl.md" A3/B3.

import (
	"fmt"

	"ila-engine/pkg/indicators"
)

// C2Status adalah status C2 closure pada satu key level.
type C2Status string

const (
	C2None    C2Status = "BELUM_ADA_C2"
	C2Active  C2Status = "C2_AKTIF"
	C2Invalid C2Status = "KEY_LEVEL_INVALID"
)

// C2Result adalah hasil evaluasi C2 closure pada satu key level.
type C2Result struct {
	Status C2Status `json:"status"`
	// C2Index = indeks candle C2 aktif, -1 bila tidak ada.
	C2Index int `json:"c2_index"`
	// Reference = level acuan terakhir.
	Reference float64 `json:"reference"`
	// RunningTouch: candle terakhir masih berjalan dan sudah menyentuh acuan (calon C2, belum final).
	RunningTouch bool `json:"running_touch"`
	// InvalidIndex = indeks candle yang membuat key level invalid, -1 bila tidak.
	InvalidIndex int    `json:"invalid_index"`
	Detail       string `json:"detail"`
}

// EvaluateC2IRL mengevaluasi C2 closure pada FVG hasil FindIRL (dengan candle yang sama).
func EvaluateC2IRL(fvg FVG, candles []indicators.Candle) C2Result {
	replacedAt := -1
	if fvg.Status == StatusReplaced {
		replacedAt = fvg.InvalidIndex
	}
	sibi := fvg.Type == FVGSIBI
	res := C2Result{Status: C2None, C2Index: -1, Reference: fvg.InitialReference(), InvalidIndex: -1}

	for i := fvg.Candle3Index + 1; i < len(candles); i++ {
		c := candles[i]
		// Urutan 1: candle ini = candle 3 FVG sejenis baru.
		if i == replacedAt {
			return c2Invalid(res, i, fmt.Sprintf("%s digantikan %s baru (bukan iFVG)", fvg.Type, fvg.Type))
		}
		if !c.IsClosed {
			res.RunningTouch = (sibi && c.High >= res.Reference) || (!sibi && c.Low <= res.Reference)
			break
		}
		// Urutan 2: close di luar batas zona -> iFVG.
		if (sibi && c.Close > fvg.Top) || (!sibi && c.Close < fvg.Bottom) {
			return c2Invalid(res, i, fmt.Sprintf("%s invalid (iFVG): close %s di luar batas zona", fvg.Type, formatILAVal(c.Close)))
		}
		// Urutan 3: wick tidak menyentuh acuan -> diabaikan.
		if (sibi && c.High < res.Reference) || (!sibi && c.Low > res.Reference) {
			continue
		}
		// Urutan 4: close ditolak dari acuan -> C2 baru / berpindah.
		if (sibi && c.Close < res.Reference) || (!sibi && c.Close > res.Reference) {
			res.Status, res.C2Index = C2Active, i
		} else {
			// Urutan 5: close tidak ditolak tapi masih di dalam zona -> C2 gugur, acuan bergeser.
			res.Status, res.C2Index = C2None, -1
		}
		if sibi {
			res.Reference = c.High
		} else {
			res.Reference = c.Low
		}
	}
	res.Detail = c2Detail(string(fvg.Type), res, candles)
	return res
}

func c2Invalid(res C2Result, i int, detail string) C2Result {
	res.Status, res.C2Index, res.InvalidIndex, res.RunningTouch, res.Detail = C2Invalid, -1, i, false, detail
	return res
}

func c2Detail(name string, res C2Result, candles []indicators.Candle) string {
	switch {
	case res.Status == C2Active:
		return fmt.Sprintf("C2 closure aktif pada %s di candle %s (acuan %s)",
			name, formatCandleDateTime(candles[res.C2Index].Timestamp), formatILAVal(res.Reference))
	case res.RunningTouch:
		return fmt.Sprintf("Candle berjalan menyentuh acuan %s pada %s, status C2 belum final", formatILAVal(res.Reference), name)
	default:
		return fmt.Sprintf("Belum ada C2 pada %s (acuan %s)", name, formatILAVal(res.Reference))
	}
}
