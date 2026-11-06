package sequence

import (
	"fmt"
	"math"
	"time"

	"ila-engine/pkg/indicators"
	"ila-engine/pkg/keylevels"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/ssmt"
	"ila-engine/pkg/stdev"
)

type Evaluator struct {
	klDetector *keylevels.Detector
	qtCalc     *quarterly.Calculator
	smtDetector *ssmt.Detector
	stdevProj  *stdev.Projector
}

func NewEvaluator() *Evaluator {
	return &Evaluator{
		klDetector:  keylevels.NewDetector(),
		qtCalc:      quarterly.NewCalculator(),
		smtDetector: ssmt.NewDetector(),
		stdevProj:   stdev.NewProjector(),
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
	mtfERLs := e.klDetector.FindERLs(mtfCandles)
	var targetMTFERL *keylevels.ERL
	if len(mtfERLs) > 0 {
		targetMTFERL = &mtfERLs[len(mtfERLs)-1]
		setup.MTFKeyLevel = targetMTFERL
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
		setup.StopLoss = cisd1.ExtremeSL

		// Target rule: ERL start -> can target opposing ERL or IRL
		if htfTargetPrice > 0 {
			setup.TakeProfit = htfTargetPrice
		} else {
			if setup.Bias == "BULLISH" {
				setup.TakeProfit = setup.EntryPrice + (2.5 * math.Abs(setup.EntryPrice-setup.StopLoss))
			} else {
				setup.TakeProfit = setup.EntryPrice - (2.5 * math.Abs(setup.EntryPrice-setup.StopLoss))
			}
		}

		slDist := math.Abs(setup.EntryPrice - setup.StopLoss)
		if slDist > 0 {
			setup.RiskReward = math.Abs(setup.TakeProfit-setup.EntryPrice) / slDist
		}

		// Draw Standard Deviation Projection immediately
		dir := stdev.DirectionBullish
		if setup.Bias == "BEARISH" {
			dir = stdev.DirectionBearish
		}
		stdevLevels := e.stdevProj.CalculateProjection(dir, cisd1.ExtremeSL, cisd1.BreakPrice)
		setup.StDevProjections = &stdevLevels

		// Nested Execution Multi-Timeframe Risk/Reward (Section 12)
		if htfTargetPrice > 0 && slDist > 0 {
			setup.NestedExecution = &NestedExecutionInfo{
				HTFTargetPrice: htfTargetPrice,
				HTFTargetName:  htfTargetName,
				LTFSLPrice:     cisd1.ExtremeSL,
				RiskDistance:   slDist,
				RewardDistance: math.Abs(htfTargetPrice - setup.EntryPrice),
				NestedRR:       math.Abs(htfTargetPrice-setup.EntryPrice) / slDist,
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
			setup.StopLoss = cisd2.ExtremeSL

			if htfTargetPrice > 0 {
				setup.TakeProfit = htfTargetPrice
			} else {
				if setup.Bias == "BULLISH" {
					setup.TakeProfit = setup.EntryPrice + (2.5 * math.Abs(setup.EntryPrice-setup.StopLoss))
				} else {
					setup.TakeProfit = setup.EntryPrice - (2.5 * math.Abs(setup.EntryPrice-setup.StopLoss))
				}
			}

			slDist := math.Abs(setup.EntryPrice - setup.StopLoss)
			if slDist > 0 {
				setup.RiskReward = math.Abs(setup.TakeProfit-setup.EntryPrice) / slDist
			}

			dir := stdev.DirectionBullish
			if setup.Bias == "BEARISH" {
				dir = stdev.DirectionBearish
			}
			stdevLevels := e.stdevProj.CalculateProjection(dir, cisd2.ExtremeSL, cisd2.BreakPrice)
			setup.StDevProjections = &stdevLevels

			if htfTargetPrice > 0 && slDist > 0 {
				setup.NestedExecution = &NestedExecutionInfo{
					HTFTargetPrice: htfTargetPrice,
					HTFTargetName:  htfTargetName,
					LTFSLPrice:     cisd2.ExtremeSL,
					RiskDistance:   slDist,
					RewardDistance: math.Abs(htfTargetPrice - setup.EntryPrice),
					NestedRR:       math.Abs(htfTargetPrice-setup.EntryPrice) / slDist,
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
