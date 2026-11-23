package ila

// CISD di LTF. Sumber aturan: "New ILA 2.0/cisd.md", "New ILA 2.0/alur-entry.md" §4-5.
//
// Logika ditulis sekali untuk -CISD. +CISD memakai logika yang sama pada candle
// yang dicerminkan (harga dinegasikan, high<->low ditukar): rangkaian bearish
// menjadi bullish, swing low menjadi swing high, BISI menjadi SIBI.
//
// -CISD (bearish):
//   rangkaian  = candle bullish berurutan;
//                level CISD = open candle pertama (Kondisi 1 & 2),
//                             atau low candle pertama (Kondisi 3).
//   puncak     = high tertinggi sejak awal rangkaian sampai candle break (Stop Loss).
//   break      = candle pertama setelah rangkaian yang close < level CISD (close body).
//   key level  = swing high LTF terbaru sebelum puncak (ERL):
//     Kondisi 1: valid dan terkena wick puncak (level = open)
//     Kondisi 2: FTM + BISI terbaru di rangkaian sudah iFVG (level = open)
//     Kondisi 3: FTM + tanpa iFVG + swing low terbaru FTM setelah puncak (level = low)
//   konfirmasi = candle (break atau sesudahnya, di dalam C3, close masih < level)
//                saat key level dan break sama-sama terpenuhi. Entry = close-nya.
//   Struktur gugur bila sebelum konfirmasi ada high > puncak (SL tersentuh).

import (
	"fmt"
	"time"

	"ila-engine/pkg/indicators"
)

// DetectCISD mencari -CISD (dir BEARISH) atau +CISD (dir BULLISH) yang
// candle konfirmasinya berada di dalam C3 [c3Start, c3End). Hanya candle
// closed yang dievaluasi; setiap key level di candle k hanya membaca candle 0..k.
// Hasil nil berarti belum ada rangkaian yang relevan.
func DetectCISD(ltf []indicators.Candle, dir string, c3Start, c3End time.Time) *CISD {
	bull := dir == DirBullish
	cs := ltf
	if bull {
		cs = mirrorCandles(ltf)
	}
	o := detectBearishCISD(cs, c3Start, c3End)
	if o == nil {
		return nil
	}
	return o.toCISD(ltf, dir)
}

type cisdOutcome struct {
	runStart, peak, breakIdx, confirm int
	level                             float64
	useExtreme                        bool
	cond                              CISDCondition
	key                               *SwingERL
	pendingReason                     string
}

func detectBearishCISD(cs []indicators.Candle, c3Start, c3End time.Time) *cisdOutcome {
	inC3 := func(i int) bool {
		t := cs[i].Timestamp
		return !t.Before(c3Start) && t.Before(c3End)
	}
	last := len(cs) - 1
	for last >= 0 && !cs[last].IsClosed {
		last--
	}
	bullish := func(i int) bool { return cs[i].Close > cs[i].Open }

	var best, pending *cisdOutcome
	for rs := 1; rs <= last; rs++ {
		if !cs[rs].Timestamp.Before(c3End) {
			break
		}
		if !bullish(rs) || bullish(rs-1) {
			continue
		}
		openLevel, lowLevel := cs[rs].Open, cs[rs].Low
		o := &cisdOutcome{runStart: rs, peak: rs, breakIdx: -1, confirm: -1, level: openLevel}
		breakLowIdx := -1
		alive, inRun := true, true
		for k := rs + 1; k <= last && alive; k++ {
			c := cs[k]
			if o.breakIdx < 0 {
				inRun = inRun && bullish(k)
				// Rangkaian bullish sudah terputus candle bearish sebelum menembus CISD,
				// lalu muncul candle bullish baru: rangkaian lama selesai agar tidak menjadi
				// "zombie" yang menimpa rangkaian & key level lebih baru.
				if !inRun && bullish(k) {
					alive = false
					break
				}
				if c.High > cs[o.peak].High {
					o.peak = k
				}
				if c.Close >= openLevel || c.Close >= c.Open {
					continue
				}
				o.breakIdx = k
				if c.Close < lowLevel {
					breakLowIdx = k
				}
			} else {
				// Sesudah break: SL tersentuh -> gugur. Retrace di atas level hanya menunda konfirmasi.
				if c.High > cs[o.peak].High {
					alive = false
					break
				}
				if breakLowIdx < 0 && c.Close < lowLevel && c.Close < c.Open {
					breakLowIdx = k
				}
				if c.Close >= openLevel || c.Close >= c.Open {
					if !c.Timestamp.Before(c3End) {
						break
					}
					continue
				}
			}
			// Puncak sapuan & break awal wajib berada di dalam C3; bila di luar C3,
			// rangkaian ini tidak pernah bisa terkonfirmasi (hindari evaluasi key level historis).
			if !inC3(o.peak) || !inC3(o.breakIdx) {
				alive = false
				break
			}
			cond, key, reason := bearishKeyCondition(cs, o.runStart, o.peak, k)
			o.key, o.pendingReason = key, reason
			if cond == CISDKondisi1 || cond == CISDKondisi2 {
				if !inC3(o.peak) || !inC3(k) || !inC3(o.breakIdx) {
					if !inC3(o.peak) || k == o.breakIdx || !c.Timestamp.Before(c3End) {
						if !inC3(o.peak) || k == o.breakIdx {
							alive = false // sapuan/break di luar C3 tidak berlaku
						}
						break
					}
					continue
				}
				o.level, o.useExtreme, o.cond, o.confirm = openLevel, false, cond, k
				break
			}
			if cond == CISDKondisi3 {
				o.level, o.useExtreme = lowLevel, true
				if c.Close >= lowLevel {
					o.pendingReason = "tanpa iFVG, SWL FTM terpenuhi, menunggu close body menembus level CISD"
					if !c.Timestamp.Before(c3End) {
						break
					}
					continue
				}
				if !inC3(o.peak) || !inC3(o.breakIdx) || !inC3(k) || !inC3(breakLowIdx) {
					if !inC3(o.peak) || !inC3(o.breakIdx) || k == breakLowIdx || !inC3(breakLowIdx) {
						alive = false // sapuan/break di luar C3 tidak berlaku
					}
					break
				}
				o.cond, o.confirm = CISDKondisi3, k
				break
			}
			if key != nil && reason == reasonWaitKondisi3 {
				o.level, o.useExtreme = lowLevel, true
				if breakLowIdx >= 0 && !inC3(breakLowIdx) {
					alive = false
					break
				}
			}
			if !c.Timestamp.Before(c3End) {
				if k == o.breakIdx {
					alive = false
				}
				break
			}
			if !inC3(k) && breakLowIdx >= 0 && k == breakLowIdx {
				alive = false
				break
			}
		}
		if o.confirm >= 0 {
			if best == nil || o.confirm < best.confirm {
				best = o
			}
			continue
		}
		if alive && o.breakIdx < 0 && o.pendingReason == "" && inC3(o.peak) {
			cond, key, reason := bearishKeyCondition(cs, o.runStart, o.peak, last)
			if cond == CISDKondisi3 || (key != nil && reason == reasonWaitKondisi3) {
				o.level, o.useExtreme = lowLevel, true
			}
			o.key, o.pendingReason = key, "menunggu close body menembus level CISD; "+reason
		}
		if alive && inC3(o.peak) {
			pending = o
		}
	}
	if best != nil {
		return best
	}
	return pending
}

const reasonWaitKondisi3 = "swing high FTM tanpa iFVG, menunggu SWL FTM"

// bearishKeyCondition mengevaluasi syarat key level -CISD pada candle k (kausal: hanya cs[0..k]).
func bearishKeyCondition(cs []indicators.Candle, runStart, peak, k int) (CISDCondition, *SwingERL, string) {
	// Swing high terbaru yang sudah terbentuk sebelum puncak (candle kanan < peak).
	key := LatestERL(FindERL(cs[:peak]), ERLSwingHigh)
	if key == nil {
		return "", nil, "belum ada swing high LTF sebagai key level"
	}
	// Status key level dinilai sampai candle k; swing baru sesudah puncak sengaja tidak dihitung
	// karena swing itu justru terbentuk oleh puncak sapuan.
	ftm := false
	for j := key.RightIndex + 1; j <= k; j++ {
		if cs[j].Close > key.Level {
			ftm = true
			break
		}
	}
	upToK := cs[:k+1]
	// Jika sesudah puncak sapuan terbentuk swing high baru (bukan puncak itu sendiri),
	// key level lama sudah digantikan (key-level-erl.md §4 & cisd.md) sehingga tidak boleh dipakai.
	allERL := FindERL(upToK)
	if latestSH := LatestERL(allERL, ERLSwingHigh); latestSH != nil && latestSH.MidIndex > peak {
		return "", nil, "swing high acuan sudah digantikan swing high baru setelah puncak"
	}
	if !ftm {
		if cs[peak].High >= key.Level {
			return CISDKondisi1, key, ""
		}
		return "", key, fmt.Sprintf("swing high %s valid tapi belum terkena wick", formatILAVal(key.Level))
	}
	var latestBISI *FVG
	for _, f := range FindIRL(upToK) {
		if f.Type == FVGBISI && f.Candle2Index >= runStart && f.Candle2Index <= peak {
			ff := f
			latestBISI = &ff
		}
	}
	if latestBISI != nil && latestBISI.Status == StatusIFVG {
		return CISDKondisi2, key, ""
	}
	if swl := LatestERL(allERL, ERLSwingLow); swl != nil && swl.Status == StatusFTM && swl.InvalidIndex > peak &&
		cs[swl.InvalidIndex].Close < cs[swl.InvalidIndex].Open {
		return CISDKondisi3, key, ""
	}
	if latestBISI != nil && latestBISI.Status == StatusValid {
		return "", key, "swing high FTM, BISI terbaru di rangkaian belum iFVG"
	}
	return "", key, reasonWaitKondisi3
}

func mirrorCandles(cs []indicators.Candle) []indicators.Candle {
	out := make([]indicators.Candle, len(cs))
	for i, c := range cs {
		out[i] = indicators.Candle{Timestamp: c.Timestamp, Open: -c.Open, High: -c.Low, Low: -c.High,
			Close: -c.Close, Volume: c.Volume, IsClosed: c.IsClosed}
	}
	return out
}

// toCISD menerjemahkan hasil (ruang -CISD) kembali ke harga asli.
func (o *cisdOutcome) toCISD(ltf []indicators.Candle, dir string) *CISD {
	sign := 1.0
	name, runName, keyName := "-CISD", "bullish", "swing high"
	if dir == DirBullish {
		sign = -1
		name, runName, keyName = "+CISD", "bearish", "swing low"
	}
	out := &CISD{
		Direction: dir,
		RefCandle: ltf[o.runStart],
		CISDLevel: o.level * sign,
		ExtremeSL: extremeOf(ltf[o.peak], dir),
	}
	if o.key != nil {
		k := *o.key
		k.Level *= sign
		if dir == DirBullish {
			k.Type = ERLSwingLow
		}
		out.KeySwing = &k
	}
	levelLabel := "open"
	if o.useExtreme {
		if dir == DirBullish {
			levelLabel = "high"
		} else {
			levelLabel = "low"
		}
	}
	if o.confirm < 0 {
		out.Details = fmt.Sprintf("%s belum terkonfirmasi: level CISD %s (%s candle %s pertama %s), SL %s. %s",
			name, formatILAVal(out.CISDLevel), levelLabel, runName, formatCandleDateTime(ltf[o.runStart].Timestamp),
			formatILAVal(out.ExtremeSL), o.pendingReason)
		return out
	}
	bc := ltf[o.confirm]
	out.Detected, out.Condition = true, o.cond
	out.BreakCandle, out.BreakTime, out.EntryPrice = bc, bc.Timestamp, bc.Close
	out.Details = fmt.Sprintf("%s terkonfirmasi (%s): close %s menembus level CISD (%s) %s pada %s; key level %s %s; SL %s",
		name, o.cond, formatILAVal(bc.Close), levelLabel, formatILAVal(out.CISDLevel), formatCandleDateTime(bc.Timestamp),
		keyName, formatILAVal(out.KeySwing.Level), formatILAVal(out.ExtremeSL))
	return out
}

func extremeOf(c indicators.Candle, dir string) float64 {
	if dir == DirBullish {
		return c.Low
	}
	return c.High
}
