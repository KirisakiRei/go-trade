package sequence

import (
	"fmt"
	"math"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
	"ila-engine/pkg/keylevels"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/ssmt"
	"ila-engine/pkg/stdev"
)

type Evaluator struct {
	klDetector  *keylevels.Detector
	qtCalc      *quarterly.Calculator
	smtDetector *ssmt.Detector
	stdevProj   *stdev.Projector
}

func NewEvaluator() *Evaluator {
	return &Evaluator{
		klDetector:  keylevels.NewDetector(),
		qtCalc:      quarterly.NewCalculator(),
		smtDetector: ssmt.NewDetector(),
		stdevProj:   stdev.NewProjector(),
	}
}

// minSLFloorFor: jarak SL minimum agar tidak tersapu noise spread, sesuai desimal tiap instrumen.
// Forex 5 desimal (EURUSD) 15 pip = 0.00150; Forex 3 desimal (USDJPY) 15 pip = 0.150.
func minSLFloorFor(symbol string) float64 {
	upper := strings.ToUpper(symbol)
	switch {
	case strings.Contains(upper, "XAU") || strings.Contains(upper, "GOLD"):
		return 4.00
	case strings.Contains(upper, "TEC") || strings.Contains(upper, "NAS") || strings.Contains(upper, "US100") || strings.Contains(upper, "US500"):
		return 15.0
	case strings.Contains(upper, "JPY"):
		return 0.150
	default:
		return 0.00150
	}
}

// EvaluateSequence executes the full ILA 3.0 3-Tier Sequence (HTF -> MTF -> LTF)
func (e *Evaluator) EvaluateSequence(
	htfCandles, mtfCandles, ltfCandles, corrCandles []indicators.Candle,
	htfName, mtfName, ltfName, symbol, corrSymbol string,
) *ILA3SequenceSetup {

	now := time.Now().UTC()
	qInfo := e.qtCalc.GetDailyQuarter(now)

	setup := &ILA3SequenceSetup{
		Symbol:      symbol,
		HTF:         htfName,
		MTF:         mtfName,
		LTF:         ltfName,
		Bias:        "NEUTRAL",
		State:       StateWaitingHTFKeyLevel,
		SetupType:   SetupNone,
		QuarterInfo: qInfo,
		EvaluatedAt: now,
	}

	if len(htfCandles) < 5 || len(mtfCandles) < 5 || len(ltfCandles) < 5 {
		setup.Summary = "Data candle tidak cukup untuk evaluasi sequence 3-tier."
		setup.NextAction = "Menunggu data candle terkumpul."
		return setup
	}

	// 1. Detect HTF Key Levels
	htfERLs := e.klDetector.FindERLs(htfCandles)
	htfIRLs := e.klDetector.FindIRLs(htfCandles)

	htfReached := false
	latestHTF := htfCandles[len(htfCandles)-1]

	// Determine Bias from HTF by checking interaction with recent ERLs
	for i := len(htfERLs) - 1; i >= 0; i-- {
		erl := &htfERLs[i]
		if erl.Type == keylevels.TypeERLSwingHigh && latestHTF.High >= erl.Price && !erl.Displaced {
			htfReached = true
			setup.Bias = "BEARISH" // Reversal from swing high
			setup.HTFStartLevel = erl
			break
		} else if erl.Type == keylevels.TypeERLSwingLow && latestHTF.Low <= erl.Price && !erl.Displaced {
			htfReached = true
			setup.Bias = "BULLISH" // Reversal from swing low
			setup.HTFStartLevel = erl
			break
		}
	}

	// If no ERL interacted, check IRLs (FVG)
	if !htfReached {
		for i := len(htfIRLs) - 1; i >= 0; i-- {
			irl := &htfIRLs[i]
			if !irl.FullyFilled && !irl.Invalidated {
				if irl.Type == keylevels.TypeIRLBearish && latestHTF.High >= irl.Bottom {
					htfReached = true
					setup.Bias = "BEARISH"
					break
				} else if irl.Type == keylevels.TypeIRLBullish && latestHTF.Low <= irl.Top {
					htfReached = true
					setup.Bias = "BULLISH"
					break
				}
			}
		}
	}

	// 2. Quarterly Theory Exception Check
	q1Bypass := e.qtCalc.CheckQ1Exception(now, htfReached)
	setup.IsQ1Bypass = q1Bypass

	if !htfReached && !q1Bypass {
		setup.State = StateWaitingHTFKeyLevel
		setup.Summary = fmt.Sprintf("Harga belum mencapai HTF (%s) Key Level dan bukan kondisi Q1 exception.", htfName)
		setup.NextAction = fmt.Sprintf("⏳ Tunggu harga mendekati dan berinteraksi dengan Key Level HTF (%s).", htfName)
		return setup
	}

	// If in Q1 bypass, we allow MTF ERL or MTF IRL directly
	if q1Bypass {
		setup.Summary = fmt.Sprintf("⭐ Q1 EXCEPTION AKTIF (%s): Harga belum menyentuh HTF, diizinkan langsung menggunakan MTF (%s) Key Level.", qInfo.Quarter, mtfName)
	}

	// 3. Find MTF Key Level (Entry 1 location: MTF ERL Reversal)
	// Bias BULLISH -> Reversal dari MTF Swing Low
	// Bias BEARISH -> Reversal dari MTF Swing High
	mtfERLs := e.klDetector.FindERLs(mtfCandles)
	var targetMTFERL *keylevels.ERL
	for i := len(mtfERLs) - 1; i >= 0; i-- {
		merl := &mtfERLs[i]
		if setup.Bias == "BULLISH" && merl.Type == keylevels.TypeERLSwingLow {
			targetMTFERL = merl
			setup.MTFKeyLevel = targetMTFERL
			break
		} else if setup.Bias == "BEARISH" && merl.Type == keylevels.TypeERLSwingHigh {
			targetMTFERL = merl
			setup.MTFKeyLevel = targetMTFERL
			break
		}
	}

	// Cross-Timeframe Validation (Section 6): Check if HTF Key Level has structural trace in MTF
	if setup.HTFStartLevel != nil && len(mtfERLs) > 0 {
		for _, merl := range mtfERLs {
			if math.Abs(merl.Price-setup.HTFStartLevel.Price) < 0.0010 || (merl.Price >= setup.HTFStartLevel.Price-0.5 && merl.Price <= setup.HTFStartLevel.Price+0.5) {
				setup.CrossTFValid = true
				break
			}
		}
	}

	// Validasi Sentuhan MTF Key Level (Section 8 & 10):
	// Harga di MTF WAJIB menyentuh/menguji level MTF ERL terlebih dahulu sebelum mencari CISD di LTF!
	latestMTF := mtfCandles[len(mtfCandles)-1]
	mtfReached := false
	if targetMTFERL != nil {
		if setup.Bias == "BULLISH" && latestMTF.Low <= targetMTFERL.Price {
			mtfReached = true
		} else if setup.Bias == "BEARISH" && latestMTF.High >= targetMTFERL.Price {
			mtfReached = true
		}
	}

	if targetMTFERL == nil || !mtfReached {
		setup.State = StateWaitingMTFReversal
		if targetMTFERL != nil {
			setup.Summary = fmt.Sprintf("Menunggu harga di MTF (%s) menyentuh level %s (%.5f).", mtfName, targetMTFERL.Type, targetMTFERL.Price)
			setup.NextAction = fmt.Sprintf("⏳ Tunggu interaksi harga di MTF (%s) dengan Key Level ERL %.5f.", mtfName, targetMTFERL.Price)
		} else {
			setup.Summary = fmt.Sprintf("Belum ada MTF (%s) Key Level yang selaras dengan bias %s.", mtfName, setup.Bias)
			setup.NextAction = fmt.Sprintf("⏳ Tunggu pembentukan Swing High/Low di MTF (%s).", mtfName)
		}
		return setup
	}

	// Determine HTF Target Level (ERL/IRL)
	var htfTargetPrice float64
	var htfTargetName string
	if len(htfERLs) > 0 {
		for i := len(htfERLs) - 1; i >= 0; i-- {
			if setup.Bias == "BULLISH" && htfERLs[i].Type == keylevels.TypeERLSwingHigh {
				setup.HTFTargetLevel = &htfERLs[i]
				htfTargetPrice = htfERLs[i].Price
				htfTargetName = fmt.Sprintf("HTF Swing High (%.5f)", htfTargetPrice)
				break
			} else if setup.Bias == "BEARISH" && htfERLs[i].Type == keylevels.TypeERLSwingLow {
				setup.HTFTargetLevel = &htfERLs[i]
				htfTargetPrice = htfERLs[i].Price
				htfTargetName = fmt.Sprintf("HTF Swing Low (%.5f)", htfTargetPrice)
				break
			}
		}
	}

	// 4. SSMT Correlation Evaluation
	if corrSymbol != "" && len(corrCandles) > 0 {
		setup.SSMTResult = e.smtDetector.EvaluateSSMT(symbol, corrSymbol, mtfCandles, corrCandles, setup.Bias)
	}

	// 5. Scan LTF for CISD 1 (Entry 1 Reversal Trigger)
	cisd1 := e.detectLTFCISD(ltfCandles, setup.Bias)
	if cisd1 != nil && cisd1.Confirmed {
		setup.State = StateReadyEntry1
		setup.SetupType = SetupEntry1Reversal
		setup.EntryPrice = cisd1.EntryPrice

		// Proteksi SL Struktural Minimal (Anti-Spread Noise):
		minSLFloor := minSLFloorFor(symbol)

		slCandidate := cisd1.ExtremeSL
		if setup.Bias == "BULLISH" {
			if targetMTFERL != nil && targetMTFERL.Price < slCandidate {
				slCandidate = targetMTFERL.Price
			}
			if math.Abs(setup.EntryPrice-slCandidate) < minSLFloor {
				slCandidate = setup.EntryPrice - minSLFloor
			}
			setup.StopLoss = slCandidate
		} else {
			if targetMTFERL != nil && targetMTFERL.Price > slCandidate {
				slCandidate = targetMTFERL.Price
			}
			if math.Abs(setup.EntryPrice-slCandidate) < minSLFloor {
				slCandidate = setup.EntryPrice + minSLFloor
			}
			setup.StopLoss = slCandidate
		}

		slDist := math.Abs(setup.EntryPrice - setup.StopLoss)

		// Draw Standard Deviation Projection immediately
		dir := stdev.DirectionBullish
		if setup.Bias == "BEARISH" {
			dir = stdev.DirectionBearish
		}
		stdevLevels := e.stdevProj.CalculateProjection(dir, cisd1.ExtremeSL, cisd1.BreakPrice)
		setup.StDevProjections = &stdevLevels

		// Target Eksekusi Terukur (Bagian 12, 14 & 16 Dokumen ILA 3.0):
		// Basis awal StDev Level -2.0 (Zona Kritis / TP 1), lalu dijepit ke rentang R:R 1:2.0 s.d. 1:4.5
		// agar tidak melepas TP ke target harian/mingguan yang menghasilkan R:R fantasi.
		targetTP := stdevLevels.LevelMinus2
		if setup.Bias == "BULLISH" {
			rawRR := (targetTP - setup.EntryPrice) / slDist
			if rawRR < 2.0 {
				targetTP = setup.EntryPrice + (2.5 * slDist)
			} else if rawRR > 4.5 {
				targetTP = setup.EntryPrice + (4.5 * slDist)
			}
		} else {
			rawRR := (setup.EntryPrice - targetTP) / slDist
			if rawRR < 2.0 {
				targetTP = setup.EntryPrice - (2.5 * slDist)
			} else if rawRR > 4.5 {
				targetTP = setup.EntryPrice - (4.5 * slDist)
			}
		}
		setup.TakeProfit = targetTP

		if slDist > 0 {
			setup.RiskReward = math.Round((math.Abs(setup.TakeProfit-setup.EntryPrice)/slDist)*100) / 100.0
		}

		// Nested Execution Multi-Timeframe (Ultra-Macro Reward dicatat di info nested):
		if htfTargetPrice > 0 && slDist > 0 {
			setup.NestedExecution = &NestedExecutionInfo{
				HTFTargetPrice: htfTargetPrice,
				HTFTargetName:  htfTargetName,
				LTFSLPrice:     setup.StopLoss,
				RiskDistance:   slDist,
				RewardDistance: math.Abs(htfTargetPrice - setup.EntryPrice),
				NestedRR:       math.Round((math.Abs(htfTargetPrice-setup.EntryPrice)/slDist)*100) / 100.0,
				CriticalZoneTP: stdevLevels.LevelMinus2,
				FinalZoneTP:    stdevLevels.LevelMinus3_5,
			}
		}

		setup.Summary = fmt.Sprintf("🎯 ENTRY 1 (REVERSAL) TERKONFIRMASI: CISD di LTF (%s) tembus body! StDev Fibo ditarik.", ltfName)
		setup.NextAction = fmt.Sprintf("🚀 Siap Entry 1 %s di harga pembuka berikutnya (SL: %.5f, TP: %.5f, R:R 1:%.2f).",
			setup.Bias, setup.StopLoss, setup.TakeProfit, setup.RiskReward)
		return setup
	}

	// 6. Check Entry 2 (Second Chance Retracement in MTF FVG)
	mtfIRLs := e.klDetector.FindIRLs(mtfCandles)
	var activeRetraceFVG *keylevels.IRL
	for i := len(mtfIRLs) - 1; i >= 0; i-- {
		fvg := &mtfIRLs[i]
		if !fvg.FullyFilled && !fvg.Invalidated {
			// Aturan Entry yang Dilarang (Section 10):
			// FVG fresh di MTF yang masih berada di dalam / dekat area HTF key level hanya waypoint, bukan entry point!
			if setup.HTFStartLevel != nil {
				htfP := setup.HTFStartLevel.Price
				if htfP >= fvg.Bottom && htfP <= fvg.Top {
					setup.WaypointFVG = fvg
					continue // Lewati, jadikan waypoint only!
				}
			}
			activeRetraceFVG = fvg
			break
		}
	}

	if activeRetraceFVG != nil {
		setup.MTFFVGRetrace = activeRetraceFVG
		// Check if current LTF price is inside this MTF FVG and triggered CISD 2
		cisd2 := e.detectLTFCISD(ltfCandles, setup.Bias)
		if cisd2 != nil && cisd2.Confirmed {
			setup.State = StateReadyEntry2
			setup.SetupType = SetupEntry2Retrace
			setup.EntryPrice = cisd2.EntryPrice
			slCandidate2 := cisd2.ExtremeSL
			minSLFloor := minSLFloorFor(symbol)

			if setup.Bias == "BULLISH" {
				if activeRetraceFVG.Bottom < slCandidate2 {
					slCandidate2 = activeRetraceFVG.Bottom
				}
				if math.Abs(setup.EntryPrice-slCandidate2) < minSLFloor {
					slCandidate2 = setup.EntryPrice - minSLFloor
				}
				setup.StopLoss = slCandidate2
			} else {
				if activeRetraceFVG.Top > slCandidate2 {
					slCandidate2 = activeRetraceFVG.Top
				}
				if math.Abs(setup.EntryPrice-slCandidate2) < minSLFloor {
					slCandidate2 = setup.EntryPrice + minSLFloor
				}
				setup.StopLoss = slCandidate2
			}

			slDist := math.Abs(setup.EntryPrice - setup.StopLoss)

			dir := stdev.DirectionBullish
			if setup.Bias == "BEARISH" {
				dir = stdev.DirectionBearish
			}
			stdevLevels := e.stdevProj.CalculateProjection(dir, cisd2.ExtremeSL, cisd2.BreakPrice)
			setup.StDevProjections = &stdevLevels

			targetTP2 := stdevLevels.LevelMinus2
			if setup.Bias == "BULLISH" {
				rawRR := (targetTP2 - setup.EntryPrice) / slDist
				if rawRR < 2.0 {
					targetTP2 = setup.EntryPrice + (2.5 * slDist)
				} else if rawRR > 4.5 {
					targetTP2 = setup.EntryPrice + (4.5 * slDist)
				}
			} else {
				rawRR := (setup.EntryPrice - targetTP2) / slDist
				if rawRR < 2.0 {
					targetTP2 = setup.EntryPrice - (2.5 * slDist)
				} else if rawRR > 4.5 {
					targetTP2 = setup.EntryPrice - (4.5 * slDist)
				}
			}
			setup.TakeProfit = targetTP2

			if slDist > 0 {
				setup.RiskReward = math.Round((math.Abs(setup.TakeProfit-setup.EntryPrice)/slDist)*100) / 100.0
			}

			if htfTargetPrice > 0 && slDist > 0 {
				setup.NestedExecution = &NestedExecutionInfo{
					HTFTargetPrice: htfTargetPrice,
					HTFTargetName:  htfTargetName,
					LTFSLPrice:     setup.StopLoss,
					RiskDistance:   slDist,
					RewardDistance: math.Abs(htfTargetPrice - setup.EntryPrice),
					NestedRR:       math.Round((math.Abs(htfTargetPrice-setup.EntryPrice)/slDist)*100) / 100.0,
					CriticalZoneTP: stdevLevels.LevelMinus2,
					FinalZoneTP:    stdevLevels.LevelMinus3_5,
				}
			}

			setup.Summary = fmt.Sprintf("🎯 ENTRY 2 (SECOND CHANCE RETRACE) TERKONFIRMASI: Retracement ke MTF FVG selesai dan CISD kedua valid!")
			setup.NextAction = fmt.Sprintf("🚀 Siap Entry 2 %s di harga pembuka berikutnya (SL: %.5f, TP: %.5f, R:R 1:%.2f).",
				setup.Bias, setup.StopLoss, setup.TakeProfit, setup.RiskReward)
			return setup
		}
	}

	setup.State = StateWaitingLTFCISD1
	setup.Summary = fmt.Sprintf("MTF (%s) key level aktif. Menunggu pemicu CISD body-break-body di LTF (%s).", mtfName, ltfName)
	setup.NextAction = fmt.Sprintf("⏳ Menunggu candle LTF (%s) ditutup menembus origin opposing leg.", ltfName)
	return setup
}

type cisdResult struct {
	Confirmed  bool
	EntryPrice float64
	ExtremeSL  float64
	BreakPrice float64
	Details    string
}

func (e *Evaluator) detectLTFCISD(candles []indicators.Candle, direction string) *cisdResult {
	n := len(candles)
	if n < 3 {
		return nil
	}

	if direction == "BULLISH" {
		lowestIdx := n - 1
		lowestVal := candles[lowestIdx].Low
		for i := n - 1; i >= 0 && i >= n-10; i-- {
			if candles[i].Low < lowestVal {
				lowestVal = candles[i].Low
				lowestIdx = i
			}
		}

		// Find first bearish candle of the downward leg
		curr := lowestIdx
		if candles[curr].Close > candles[curr].Open && curr > 0 {
			curr--
		}
		var firstBear *indicators.Candle
		for curr >= 0 && candles[curr].Close < candles[curr].Open {
			firstBear = &candles[curr]
			curr--
		}

		if firstBear == nil {
			return nil
		}

		cisdLevel := firstBear.Open
		for k := lowestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close > cisdLevel {
				return &cisdResult{
					Confirmed:  true,
					EntryPrice: c.Close,
					ExtremeSL:  lowestVal,
					BreakPrice: c.Close,
					Details:    fmt.Sprintf("Bullish CISD tembus body di %.5f (Origin Open %.5f, SL %.5f)", c.Close, cisdLevel, lowestVal),
				}
			}
		}
	} else if direction == "BEARISH" {
		highestIdx := n - 1
		highestVal := candles[highestIdx].High
		for i := n - 1; i >= 0 && i >= n-10; i-- {
			if candles[i].High > highestVal {
				highestVal = candles[i].High
				highestIdx = i
			}
		}

		// Find first bullish candle of the upward leg
		curr := highestIdx
		if candles[curr].Close < candles[curr].Open && curr > 0 {
			curr--
		}
		var firstBull *indicators.Candle
		for curr >= 0 && candles[curr].Close > candles[curr].Open {
			firstBull = &candles[curr]
			curr--
		}

		if firstBull == nil {
			return nil
		}

		cisdLevel := firstBull.Open
		for k := highestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close < cisdLevel {
				return &cisdResult{
					Confirmed:  true,
					EntryPrice: c.Close,
					ExtremeSL:  highestVal,
					BreakPrice: c.Close,
					Details:    fmt.Sprintf("Bearish CISD tembus body di %.5f (Origin Open %.5f, SL %.5f)", c.Close, cisdLevel, highestVal),
				}
			}
		}
	}

	return nil
}
