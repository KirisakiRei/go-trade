package ila

import (
	"fmt"
	"math"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
)

// Evaluator provides deterministic evaluation for the Imaginary Liquidity (ILA) method.
type Evaluator struct{}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// FindFVGs scans candle history and returns all 3-candle Fair Value Gaps.
func (e *Evaluator) FindFVGs(candles []indicators.Candle) []FVG {
	n := len(candles)
	if n < 3 {
		return nil
	}

	var fvgs []FVG

	for i := 2; i < n; i++ {
		c1 := candles[i-2]
		c2 := candles[i-1]
		c3 := candles[i]

		// FVG hanya sah dibentuk oleh 3 candle yang sudah CLOSED resmi
		if !c1.IsClosed || !c2.IsClosed || !c3.IsClosed {
			continue
		}

		// Bearish FVG: Low(C1) > High(C3) (C2 is a massive downward expansion candle)
		if c1.Low > c3.High && c2.Close < c2.Open {
			fvg := FVG{
				Type:         FVGBearish,
				Top:          c1.Low,
				Bottom:       c3.High,
				Candle1Index: i - 2,
				Candle2Index: i - 1,
				Candle3Index: i,
				Candle1Time:  c1.Timestamp,
				Candle2Time:  c2.Timestamp,
				Candle3Time:  c3.Timestamp,
			}

			// Check if subsequently mitigated by candles k > i
			// Berdasarkan Rule Opsi C: FVG termitigasi/hangus jika pernah tersentuh penuh (k.High >= fvg.Top atau k.Low <= fvg.Bottom)
			for k := i + 1; k < n; k++ {
				if candles[k].High >= fvg.Top {
					fvg.Mitigated = true
					break
				}
			}
			fvgs = append(fvgs, fvg)
		}

		// Bullish FVG: High(C1) < Low(C3) (C2 is a massive upward expansion candle)
		if c1.High < c3.Low && c2.Close > c2.Open {
			fvg := FVG{
				Type:         FVGBullish,
				Top:          c3.Low,
				Bottom:       c1.High,
				Candle1Index: i - 2,
				Candle2Index: i - 1,
				Candle3Index: i,
				Candle1Time:  c1.Timestamp,
				Candle2Time:  c2.Timestamp,
				Candle3Time:  c3.Timestamp,
			}

			// Check if subsequently mitigated by candles k > i
			for k := i + 1; k < n; k++ {
				if candles[k].Low <= fvg.Bottom {
					fvg.Mitigated = true
					break
				}
			}
			fvgs = append(fvgs, fvg)
		}
	}

	return fvgs
}

// DetectHTFIlaSweep checks if an HTF candle retraced into an active FVG and swept the wick of the prior candle.
func (e *Evaluator) DetectHTFIlaSweep(candles []indicators.Candle) *HTFSweep {
	n := len(candles)
	if n < 4 {
		return nil
	}

	fvgs := e.FindFVGs(candles)
	if len(fvgs) == 0 {
		return nil
	}

	// Strict ILA rule: Filter out fully mitigated FVGs, focus ONLY on the latest active UNMITIGATED FVG
	var activeFVGs []FVG
	for _, f := range fvgs {
		if !f.Mitigated {
			activeFVGs = append(activeFVGs, f)
		}
	}
	if len(activeFVGs) == 0 {
		return nil // Tidak ada FVG aktif yang valid (semua sudah termitigasi)
	}

	latestFVG := activeFVGs[len(activeFVGs)-1]

	// Check if any candle created at or after this FVG entered the FVG and swept prior wick
	// If the latest candle ck is already past the entry window of a sweep, check if it invalidated the setup
	for k := n - 1; k > latestFVG.Candle2Index; k-- {
		ck := candles[k]
		cPrev := candles[k-1] // Bersebelahan langsung (adjacent)

		// 0. Cek apakah FVG sudah terisi/tertutup penuh (100% Fully Filled/Mitigated) baik oleh ekor maupun body
		if latestFVG.Type == FVGBearish && ck.High >= latestFVG.Top {
			return nil // Bearish FVG sudah tertutup penuh hingga atap (termitigasi)
		}
		if latestFVG.Type == FVGBullish && ck.Low <= latestFVG.Bottom {
			return nil // Bullish FVG sudah tertutup penuh hingga dasar (termitigasi)
		}

		// 1. BEARISH ILA SWEEP:
		// - Latest FVG is Bearish
		// - cPrev wick must already be inside FVG: cPrev.High >= latestFVG.Bottom
		// - ck enters FVG: ck.High >= latestFVG.Bottom
		// - ck sweeps previous ADJACENT wick inside FVG: ck.High > cPrev.High
		// - ck does NOT close above FVG top (displacement holds): ck.Close <= latestFVG.Top
		if latestFVG.Type == FVGBearish && cPrev.High >= latestFVG.Bottom && ck.High >= latestFVG.Bottom && ck.High > cPrev.High && ck.Close <= latestFVG.Top {
			// VALIDASI SIKLUS AKTIF:
			// Sweep hanya berlaku jika ck adalah candle terakhir atau candle tepat sebelum candle terakhir (k >= n-2).
			// Jika sweep terjadi berminggu-minggu lalu (k < n-2), siklus tersebut sudah kadaluarsa!
			if k < n-2 {
				continue
			}
			ssl := latestFVG.Bottom
			for s := latestFVG.Candle2Index; s < k; s++ {
				if candles[s].Low < ssl {
					ssl = candles[s].Low
				}
			}

			if !ck.IsClosed {
				return &HTFSweep{
					Detected:         false,
					SweepInProgress:  true,
					Direction:        "BEARISH",
					FVG:              &latestFVG,
					SweepCandle:      ck,
					SweptCandle:      cPrev,
					SweepCandleIndex: k,
					SweepTime:        ck.Timestamp,
					StopLoss:         ck.High,
					TargetDOL:        ssl,
					Details: fmt.Sprintf("Candle %s sedang melakukan sweep wick di Bearish FVG (belum ganti candle / masih running). Menunggu candle HTF closed resmi untuk cari entry di candle berikutnya!",
						ck.Timestamp.Format("02 Jan 15:04")),
				}
			}

			return &HTFSweep{
				Detected:         true,
				Direction:        "BEARISH",
				FVG:              &latestFVG,
				SweepCandle:      ck,
				SweptCandle:      cPrev,
				SweepCandleIndex: k,
				SweepTime:        ck.Timestamp,
				StopLoss:         ck.High,
				TargetDOL:        ssl,
				Details: fmt.Sprintf("Candle %s masuk ke Bearish FVG Terakhir (%.2f - %.2f) dan menyapu High wick candle sebelumnya (%.2f > %.2f). Target DOL: %.2f (SSL).",
					ck.Timestamp.Format("02 Jan 15:04"), latestFVG.Bottom, latestFVG.Top, ck.High, cPrev.High, ssl),
			}
		}

		// 2. BULLISH ILA SWEEP:
		// - Latest FVG is Bullish
		// - cPrev wick must already be inside FVG: cPrev.Low <= latestFVG.Top
		// - ck enters FVG: ck.Low <= latestFVG.Top
		// - ck sweeps previous wick low inside FVG: ck.Low < cPrev.Low
		// - ck does NOT close below FVG bottom: ck.Close >= latestFVG.Bottom
		if latestFVG.Type == FVGBullish && cPrev.Low <= latestFVG.Top && ck.Low <= latestFVG.Top && ck.Low < cPrev.Low && ck.Close >= latestFVG.Bottom {
			// VALIDASI SIKLUS AKTIF:
			// Sweep hanya berlaku jika ck adalah candle terakhir atau candle tepat sebelum candle terakhir (k >= n-2).
			if k < n-2 {
				continue
			}
			bsl := latestFVG.Top
			for s := latestFVG.Candle2Index; s < k; s++ {
				if candles[s].High > bsl {
					bsl = candles[s].High
				}
			}

			if !ck.IsClosed {
				return &HTFSweep{
					Detected:         false,
					SweepInProgress:  true,
					Direction:        "BULLISH",
					FVG:              &latestFVG,
					SweepCandle:      ck,
					SweptCandle:      cPrev,
					SweepCandleIndex: k,
					SweepTime:        ck.Timestamp,
					StopLoss:         ck.Low,
					TargetDOL:        bsl,
					Details: fmt.Sprintf("Candle %s sedang melakukan sweep wick di Bullish FVG (belum ganti candle / masih running). Menunggu candle HTF closed resmi untuk cari entry di candle berikutnya!",
						ck.Timestamp.Format("02 Jan 15:04")),
				}
			}

			return &HTFSweep{
				Detected:         true,
				Direction:        "BULLISH",
				FVG:              &latestFVG,
				SweepCandle:      ck,
				SweptCandle:      cPrev,
				SweepCandleIndex: k,
				SweepTime:        ck.Timestamp,
				StopLoss:         ck.Low,
				TargetDOL:        bsl,
				Details: fmt.Sprintf("Candle %s masuk ke Bullish FVG Terakhir (%.2f - %.2f) dan menyapu Low wick candle sebelumnya (%.2f < %.2f). Target DOL: %.2f (BSL).",
					ck.Timestamp.Format("02 Jan 15:04"), latestFVG.Bottom, latestFVG.Top, ck.Low, cPrev.Low, bsl),
			}
		}
	}

	// If no sweep inside the latest FVG has occurred yet:
	dirStr := "BEARISH"
	detailStr := fmt.Sprintf("FVG terakhir adalah Bearish (%.2f - %.2f). Menunggu candle masuk dan menyapu High wick candle sebelumnya.", latestFVG.Bottom, latestFVG.Top)
	if latestFVG.Type == FVGBullish {
		dirStr = "BULLISH"
		detailStr = fmt.Sprintf("FVG terakhir adalah Bullish (%.2f - %.2f). Menunggu candle masuk dan menyapu Low wick candle sebelumnya.", latestFVG.Bottom, latestFVG.Top)
	}

	return &HTFSweep{
		Detected:  false,
		Direction: dirStr,
		FVG:       &latestFVG,
		Details:   detailStr,
	}
}

// DetectCISD scans LTF candles for Change In State of Delivery (body breaks body).
// Strictly using the first candle of the consecutive opposing run tracing backwards from the extreme of Candle 5.
func (e *Evaluator) DetectCISD(candles []indicators.Candle, direction string, afterTime time.Time) *CISD {
	cisd, _ := e.DetectDynamicCISD(candles, direction, afterTime, indicators.Candle{})
	return cisd
}

// DetectDynamicCISD evaluates the dynamic consecutive leg CISD:
// Tracing backwards from the extreme of Candle 5 to the origin (first candle) of the opposing leg.
func (e *Evaluator) DetectDynamicCISD(candles []indicators.Candle, direction string, afterTime time.Time, sweepCandle indicators.Candle) (*CISD, bool) {
	n := len(candles)
	if n < 2 {
		return nil, false
	}

	if direction == "BULLISH" {
		// 1. Cari candle terendah (lowest Low) di Candle 5 (c.Timestamp >= afterTime)
		lowestIdx := -1
		lowestVal := math.MaxFloat64

		for i := 0; i < n; i++ {
			c := candles[i]
			if !afterTime.IsZero() && c.Timestamp.Before(afterTime) {
				continue
			}
			if c.Low < lowestVal {
				lowestVal = c.Low
				lowestIdx = i
			}
		}

		if lowestIdx == -1 {
			lowestIdx = n - 1
			lowestVal = candles[lowestIdx].Low
		}

		// 2. Invalidation Check: Apakah low di Candle 5 turun terlalu jauh menembus wick Candle 4?
		if sweepCandle.Low > 0 && lowestVal < sweepCandle.Low {
			return nil, true // Invalidated!
		}

	// 3. Telusuri MUNDUR dari lowestIdx untuk mencari candle bearish PERTAMA dari deretan penurunan ini.
		// SYARAT KETAT: Candle origin CISD WAJIB berada di dalam Candle 5 (Timestamp >= afterTime)!
		// Tidak boleh mengambil origin dari candle sebelum Candle 5 dibuka.
		// Dan lowestIdx wajib terjadi di dalam Candle 5 (afterTime)
		if !afterTime.IsZero() && candles[lowestIdx].Timestamp.Before(afterTime) {
			return nil, false // Low belum terbentuk di dalam Candle 5
		}

		curr := lowestIdx
		if candles[curr].Close > candles[curr].Open && curr > 0 {
			curr--
		}

		var firstBear *indicators.Candle
		for curr >= 0 && candles[curr].Close < candles[curr].Open {
			if !afterTime.IsZero() && candles[curr].Timestamp.Before(afterTime) {
				break // Stop jika mundur melewati batas awal Candle 5!
			}
			firstBear = &candles[curr]
			curr--
		}

		if firstBear == nil {
			for k := lowestIdx; k >= 0; k-- {
				if !afterTime.IsZero() && candles[k].Timestamp.Before(afterTime) {
					break
				}
				if candles[k].Close < candles[k].Open {
					firstBear = &candles[k]
					break
				}
			}
		}

		if firstBear == nil {
			// Belum ada deretan candle bearish internal di dalam Candle 5
			return nil, false
		}

		cisdLevel := firstBear.Open
		cisd := &CISD{
			Detected:  false,
			Direction: "BULLISH",
			RefCandle: *firstBear,
			CISDLevel: cisdLevel,
			ExtremeSL: lowestVal,
			Details: fmt.Sprintf("Origin penurunan di Candle %s (Open %.2f). Menunggu candle ditutup > %.2f by body (SL: %.2f).",
				firstBear.Timestamp.Format("15:04"), cisdLevel, cisdLevel, lowestVal),
		}

		// 4. Cari apakah ada candle closed setelah lowestIdx yang menembus cisdLevel by body
		for k := lowestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close > cisdLevel {
				cisd.Detected = true
				cisd.BreakCandle = c
				cisd.BreakTime = c.Timestamp
				cisd.EntryPrice = c.Close
				cisd.Details = fmt.Sprintf("Bullish CISD Valid: Candle %s (Close %.2f) resmi menembus origin bearish %s (Open %.2f) by body! SL di %.2f.",
					c.Timestamp.Format("15:04"), c.Close, firstBear.Timestamp.Format("15:04"), cisdLevel, lowestVal)
				return cisd, false
			}
		}

		return cisd, false

	} else if direction == "BEARISH" {
		// 1. Cari candle tertinggi (highest High) di Candle 5 (c.Timestamp >= afterTime)
		highestIdx := -1
		highestVal := -math.MaxFloat64

		for i := 0; i < n; i++ {
			c := candles[i]
			if !afterTime.IsZero() && c.Timestamp.Before(afterTime) {
				continue
			}
			if c.High > highestVal {
				highestVal = c.High
				highestIdx = i
			}
		}

		if highestIdx == -1 {
			highestIdx = n - 1
			highestVal = candles[highestIdx].High
		}

		// 2. Invalidation Check: Apakah high di Candle 5 menembus wick Candle 4?
		if sweepCandle.High > 0 && highestVal > sweepCandle.High {
			return nil, true // Invalidated!
		}

	// 3. Telusuri MUNDUR dari highestIdx untuk mencari candle bullish PERTAMA dari deretan kenaikan ini.
		// SYARAT KETAT: Candle origin CISD WAJIB berada di dalam Candle 5 (Timestamp >= afterTime)!
		// Tidak boleh mengambil origin dari candle sebelum Candle 5 dibuka.
		// Dan highestIdx wajib terjadi di dalam Candle 5 (afterTime)
		if !afterTime.IsZero() && candles[highestIdx].Timestamp.Before(afterTime) {
			return nil, false // High belum terbentuk di dalam Candle 5
		}

		curr := highestIdx
		if candles[curr].Close < candles[curr].Open && curr > 0 {
			curr--
		}

		var firstBull *indicators.Candle
		for curr >= 0 && candles[curr].Close > candles[curr].Open {
			if !afterTime.IsZero() && candles[curr].Timestamp.Before(afterTime) {
				break // Stop jika mundur melewati batas awal Candle 5!
			}
			firstBull = &candles[curr]
			curr--
		}

		if firstBull == nil {
			for k := highestIdx; k >= 0; k-- {
				if !afterTime.IsZero() && candles[k].Timestamp.Before(afterTime) {
					break
				}
				if candles[k].Close > candles[k].Open {
					firstBull = &candles[k]
					break
				}
			}
		}

		if firstBull == nil {
			// Belum ada deretan candle bullish internal di dalam Candle 5
			return nil, false
		}

		cisdLevel := firstBull.Open
		cisd := &CISD{
			Detected:  false,
			Direction: "BEARISH",
			RefCandle: *firstBull,
			CISDLevel: cisdLevel,
			ExtremeSL: highestVal,
			Details: fmt.Sprintf("Origin kenaikan di Candle %s (Open %.2f). Menunggu candle ditutup < %.2f by body (SL: %.2f).",
				firstBull.Timestamp.Format("15:04"), cisdLevel, cisdLevel, highestVal),
		}

		// 4. Cari apakah ada candle closed setelah highestIdx yang menembus cisdLevel by body
		for k := highestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close < cisdLevel {
				cisd.Detected = true
				cisd.BreakCandle = c
				cisd.BreakTime = c.Timestamp
				cisd.EntryPrice = c.Close
				cisd.Details = fmt.Sprintf("Bearish CISD Valid: Candle %s (Close %.2f) resmi menembus origin bullish %s (Open %.2f) by body! SL di %.2f.",
					c.Timestamp.Format("15:04"), c.Close, firstBull.Timestamp.Format("15:04"), cisdLevel, highestVal)
				return cisd, false
			}
		}

		return cisd, false
	}

	return nil, false
}

func timeframeDuration(tf string) time.Duration {
	switch strings.ToUpper(strings.TrimSpace(tf)) {
	case "M1", "1":
		return 1 * time.Minute
	case "M5", "5":
		return 5 * time.Minute
	case "M15", "15":
		return 15 * time.Minute
	case "M30", "30":
		return 30 * time.Minute
	case "H1", "60", "60M":
		return 1 * time.Hour
	case "H4", "240", "240M":
		return 4 * time.Hour
	case "D1", "D":
		return 24 * time.Hour
	case "W", "W1", "1W":
		return 7 * 24 * time.Hour
	default:
		return 4 * time.Hour
	}
}

// EvaluateILA executes the full paired multi-timeframe ILA logic.
func (e *Evaluator) EvaluateILA(htfCandles, ltfCandles []indicators.Candle, htfName, ltfName, symbol string) *ILASetup {
	setup := &ILASetup{
		Symbol:      symbol,
		HTF:         htfName,
		LTF:         ltfName,
		Bias:        "NEUTRAL",
		Status:      "WAIT_HTF_SWEEP",
		EvaluatedAt: time.Now().UTC(),
	}

	// 1. Evaluate HTF
	sweep := e.DetectHTFIlaSweep(htfCandles)
	if sweep == nil {
		setup.NextAction = fmt.Sprintf("⏳ Menunggu pembentukan FVG baru di timeframe %s.", htfName)
		setup.Summary = fmt.Sprintf("Belum ditemukan formasi FVG aktif di timeframe %s.", htfName)
		return setup
	}

	setup.HTFSweep = sweep
	setup.Bias = sweep.Direction

	if sweep.SweepInProgress {
		setup.Status = "WAIT_HTF_CANDLE_CLOSE"
		setup.NextAction = fmt.Sprintf("⏳ Candle sweep di %s masih berjalan (belum ganti candle). Menunggu candle %s closed resmi untuk cari entry di candle berikutnya!", htfName, htfName)
		setup.Summary = fmt.Sprintf("Timeframe %s: %s", htfName, sweep.Details)
		return setup
	}

	if !sweep.Detected {
		setup.Status = "WAIT_HTF_SWEEP"
		setup.NextAction = fmt.Sprintf("⏳ %s", sweep.Details)
		setup.Summary = fmt.Sprintf("Timeframe %s: %s", htfName, sweep.Details)
		return setup
	}

	// 2. Tentukan Candle Tempat Entry (Bisa Candle 5, Candle 6, dst selama tidak me-sweep candle sebelumnya)
	// Aturan Baku: Jika candle HTF berikutnya (misal Candle 6) me-sweep candle sebelumnya (Candle 5),
	// maka candle tersebut menjadi Candle Sweep baru. Jika TIDAK me-sweep dan TIDAK ada CISD di candle sebelumnya,
	// pencarian CISD berpindah murni ke dalam candle HTF saat ini!
	var activeEntryCandle *indicators.Candle
	if len(htfCandles) > sweep.SweepCandleIndex+1 {
		activeEntryCandle = &htfCandles[len(htfCandles)-1] // Candle HTF yang sedang aktif berjalan/terakhir
	}

	htfDur := timeframeDuration(htfName)
	entryCandleStart := sweep.SweepCandle.Timestamp.Add(htfDur)
	if activeEntryCandle != nil {
		entryCandleStart = activeEntryCandle.Timestamp
		if sweep.Direction == "BULLISH" {
			setup.StopLoss = activeEntryCandle.Low
		} else {
			setup.StopLoss = activeEntryCandle.High
		}
	} else {
		setup.StopLoss = sweep.StopLoss
	}
	sweep.StopLoss = setup.StopLoss

	setup.TakeProfit = sweep.TargetDOL

	// 3. Evaluate LTF for CISD murni di dalam jendela waktu candle entry saat ini (entryCandleStart)
	cisd, invalidated := e.DetectDynamicCISD(ltfCandles, sweep.Direction, entryCandleStart, sweep.SweepCandle)
	if invalidated {
		setup.Status = "INVALIDATED"
		setup.NextAction = fmt.Sprintf("🛑 Setup ILA Batal: Harga di Candle 5 menembus ekor Candle 4 (%s).", htfName)
		setup.Summary = fmt.Sprintf("Setup %s ILA batal karena ekor sweep HTF ditembus.", sweep.Direction)
		return setup
	}

	if cisd == nil || !cisd.Detected {
		setup.Status = "WAIT_LTF_CISD"
		setup.CISD = cisd
		if cisd != nil {
			setup.StopLoss = cisd.ExtremeSL
			digits := 2
			if strings.Contains(symbol, "EUR") || strings.Contains(symbol, "GBP") || strings.Contains(symbol, "AUD") {
				digits = 5
			}
			fStr := fmt.Sprintf("%%.%df", digits)
			cisdStr := fmt.Sprintf(fStr, cisd.CISDLevel)
			slStr := fmt.Sprintf(fStr, cisd.ExtremeSL)
			setup.NextAction = fmt.Sprintf("⏳ Level CISD Valid di %s (SL: %s). Menunggu closed candle %s menembus level tersebut by body.", cisdStr, slStr, ltfName)
			setup.Summary = fmt.Sprintf("HTF %s sweep valid. Menunggu CISD %s di level %s (SL: %s).", htfName, ltfName, cisdStr, slStr)
		} else {
			setup.NextAction = fmt.Sprintf("⏳ HTF %s Sweep FVG resmi CLOSED! Saat ini di Candle 5 (SL: %.2f), menunggu pembentukan CISD di %s.", htfName, setup.StopLoss, ltfName)
			setup.Summary = fmt.Sprintf("Internal Liquidity FVG %s sudah disapu (%s). Target: %.2f. Menunggu CISD di %s.", htfName, sweep.Direction, sweep.TargetDOL, ltfName)
		}
		return setup
	}

	// 4. Both HTF Sweep and LTF CISD confirmed -> READY TO ENTRY!
	setup.CISD = cisd
	setup.Status = "READY_TO_ENTRY"
	setup.EntryPrice = cisd.EntryPrice
	setup.StopLoss = cisd.ExtremeSL

	// Risk & Reward calculation (Opsi A: DOL-Bounded Clamping antara 1:2.0 s.d. 1:2.3 R:R)
	slDist := math.Abs(setup.EntryPrice - setup.StopLoss)

	// Check if DOL was already breached/passed by the sweep candle or current price
	dolBreached := false
	if setup.Bias == "BULLISH" {
		if setup.EntryPrice >= setup.TakeProfit || (sweep != nil && sweep.SweepCandle.High >= setup.TakeProfit) {
			dolBreached = true
		}
	} else if setup.Bias == "BEARISH" {
		if setup.EntryPrice <= setup.TakeProfit || (sweep != nil && sweep.SweepCandle.Low <= setup.TakeProfit) {
			dolBreached = true
		}
	}

	targetRR := 2.00
	if dolBreached {
		// Rule ILA: Jika DOL/Eksternal sudah jebol duluan, kunci TP ke batas bawah 1:2.00 R:R
		targetRR = 2.00
	} else {
		rawRR := 0.0
		if slDist > 0 {
			rawRR = math.Abs(setup.TakeProfit-setup.EntryPrice) / slDist
		}
		if rawRR >= 2.30 {
			// Skenario 1 (Jarak DOL Jauh / R:R >= 2.3): Cap tepat di 1:2.30 (anti-greed)
			targetRR = 2.30
		} else if rawRR >= 2.00 {
			// Skenario 2 (Jarak DOL di Rentang 2.0 - 2.3): Pakai DOL langsung
			targetRR = math.Round(rawRR*100) / 100
		} else {
			// Skenario 3 (Jarak DOL < 2.0): Kunci di batas dasar 1:2.00
			targetRR = 2.00
		}
	}

	if targetRR == 2.00 || targetRR == 2.30 || dolBreached {
		if setup.Bias == "BULLISH" {
			setup.TakeProfit = setup.EntryPrice + (targetRR * slDist)
		} else {
			setup.TakeProfit = setup.EntryPrice - (targetRR * slDist)
		}
	}

	// Round TakeProfit cleanly sesuai instrumen
	digits := 2
	if strings.Contains(symbol, "EUR") || strings.Contains(symbol, "GBP") || strings.Contains(symbol, "AUD") {
		digits = 5
	}
	pow := math.Pow10(digits)
	setup.TakeProfit = math.Round(setup.TakeProfit*pow) / pow

	setup.RiskReward = targetRR
	setup.PassedMinRR = setup.RiskReward >= 2.00

	actionEmoji := "🟢 BUY"
	if setup.Bias == "BEARISH" {
		actionEmoji = "🔴 SELL"
	}

	fmtPrice := "%.2f"
	if digits == 5 {
		fmtPrice = "%.5f"
	}
	setup.NextAction = fmt.Sprintf("🎯 SIAP EKSEKUSI %s: Masuk di candle pembuka berikutnya!", actionEmoji)
	setup.Summary = fmt.Sprintf("💎 SETUP ILA TERKONFIRMASI LENGKAP (%s ➡️ %s):\n• %s\n• %s\n• Level: Entry="+fmtPrice+", SL="+fmtPrice+", TP="+fmtPrice+" (R:R 1:%.2f)",
		htfName, ltfName, sweep.Details, cisd.Details, setup.EntryPrice, setup.StopLoss, setup.TakeProfit, setup.RiskReward)

	return setup
}
