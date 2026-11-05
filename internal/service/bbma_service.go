package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/pkg/apperror"
	"ila-engine/pkg/indicators"
	"ila-engine/pkg/marketfeed"

	"github.com/google/uuid"
)

type bbmaService struct {
	feed      marketfeed.Provider
	setupRepo repository.SetupRepository
}

func NewBBMAService(feed marketfeed.Provider, setupRepo repository.SetupRepository) BBMAService {
	return &bbmaService{
		feed:      feed,
		setupRepo: setupRepo,
	}
}

func (s *bbmaService) Analyze(ctx context.Context, req dto.AnalysisRequest) (*dto.AnalysisResponse, error) {
	limit := req.Limit
	if limit < 70 {
		limit = 100
	}

	rawCandles, err := s.feed.FetchCandles(ctx, req.Symbol, req.Timeframe, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", apperror.ErrMarketFeedFailed, err)
	}

	// Rule 3.2 & Section 5.1: Only use closed candles for final confirmation!
	closedCandles := make([]indicators.Candle, 0, len(rawCandles))
	for _, c := range rawCandles {
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
	}

	if len(closedCandles) < 50 {
		return &dto.AnalysisResponse{
			Symbol:                 req.Symbol,
			Timeframe:              req.Timeframe,
			AsOf:                   time.Now().UTC().Format(time.RFC3339),
			DataQuality:            "INSUFFICIENT_DATA",
			MarketPhase:            "INSUFFICIENT_DATA",
			Bias:                   "UNCONFIRMED",
			SetupType:              "NONE",
			SetupStatus:            "WAIT",
			Evidence:               nil,
			MissingConditions:      []string{"Minimum 50 closed candles required for reliable indicator warm-up"},
			InvalidationConditions: nil,
			EntryPlan:              nil,
			RiskCheck: dto.RiskCheck{
				Passed:  false,
				Reasons: []string{"Insufficient history"},
			},
			NextAction:             "WAIT_FOR_MORE_DATA",
			RulebookVersion:        "1.0.0",
			IndicatorConfigVersion: "bbma-baseline-1",
		}, nil
	}

	indValues, err := indicators.CalculateBBMA(closedCandles)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", apperror.ErrInternalServer, err)
	}

	n := len(closedCandles)
	lastIdx := n - 1
	lastCandle := closedCandles[lastIdx]
	lastInd := indValues[lastIdx]

	isForex := lastCandle.Close < 50.0
	round := func(v float64) float64 {
		if isForex {
			return math.Round(v*100000) / 100000
		}
		return math.Round(v*100) / 100
	}

	resp := &dto.AnalysisResponse{
		Symbol:                 req.Symbol,
		Timeframe:              req.Timeframe,
		AsOf:                   lastCandle.Timestamp.UTC().Format(time.RFC3339),
		DataQuality:            "VALID",
		RulebookVersion:        "1.0.0",
		IndicatorConfigVersion: "bbma-baseline-1",
		IndicatorsSnapshot: &dto.IndicatorSnapshot{
			BBUpper:   round(lastInd.BBUpper),
			BBMiddle:  round(lastInd.BBMiddle),
			BBLower:   round(lastInd.BBLower),
			WMA5High:  round(lastInd.WMA5High),
			WMA10High: round(lastInd.WMA10High),
			WMA5Low:   round(lastInd.WMA5Low),
			WMA10Low:  round(lastInd.WMA10Low),
			EMA50:     round(lastInd.EMA50),
			LastClose: round(lastCandle.Close),
		},
	}

	// Evaluate Setups deterministically
	s.evaluateRulebook(closedCandles, indValues, resp)

	// Persist valid setup candidate if found
	if resp.SetupStatus == "SETUP_VALID" || resp.SetupStatus == "CANDIDATE" {
		_ = s.saveSetupRecord(ctx, resp, lastCandle.Timestamp)
	}

	return resp, nil
}

func (s *bbmaService) evaluateRulebook(candles []indicators.Candle, ind []indicators.IndicatorValues, resp *dto.AnalysisResponse) {
	n := len(candles)
	currIdx := n - 1
	c := candles[currIdx]
	iv := ind[currIdx]

	prevIdx := n - 2
	prevC := candles[prevIdx]
	prevIv := ind[prevIdx]

	isForex := c.Close < 50.0
	round := func(v float64) float64 {
		if isForex {
			return math.Round(v*100000) / 100000
		}
		return math.Round(v*100) / 100
	}
	fmtP := func(v float64) string {
		if isForex {
			return fmt.Sprintf("%.5f", v)
		}
		return fmt.Sprintf("%.2f", v)
	}

	buffer15 := 1.5
	buffer20 := 2.0
	if isForex {
		buffer15 = 0.0015
		buffer20 = 0.0020
	}
	_ = buffer15
	_ = buffer20

	// 0. Compute ATR(14) for dynamic volatility measurement
	atrPeriod := 14
	atr := 0.0
	if n >= atrPeriod+1 {
		trSum := 0.0
		for i := n - atrPeriod; i < n; i++ {
			h := candles[i].High
			l := candles[i].Low
			prevClose := candles[i-1].Close
			tr := math.Max(h-l, math.Max(math.Abs(h-prevClose), math.Abs(l-prevClose)))
			trSum += tr
		}
		atr = trSum / float64(atrPeriod)
	}
	if atr == 0 {
		if isForex {
			atr = 0.0015
		} else {
			atr = 4.0
		}
	}

	// Dynamic Safe Volatility Buffers:
	// For Gold (XAUUSD): minSLDist = max(atr*1.2, 4.50) [$4.50 = 45 pips], buffer = max(atr*0.4, 1.50)
	// For Forex (EURUSD): minSLDist = max(atr*1.2, 0.0020) [20 pips], buffer = max(atr*0.4, 0.0008)
	var buffer float64
	var minSLDist float64
	if isForex {
		buffer = math.Max(atr*0.4, 0.0008)
		minSLDist = math.Max(atr*1.2, 0.0020)
	} else {
		buffer = math.Max(atr*0.4, 1.50)
		minSLDist = math.Max(atr*1.2, 4.50)
	}

	// 1. Check Sideways (Flat Bollinger Bands)
	bbBandwidth := (iv.BBUpper - iv.BBLower) / iv.BBMiddle
	isFlatBB := bbBandwidth < 0.0035

	// 2. Check Momentum (CSM)
	if c.Close > iv.BBUpper {
		resp.MarketPhase = "MOMENTUM_CONFIRMED"
		resp.Bias = "BULLISH"
		resp.SetupType = "MOMENTUM"
		resp.SetupStatus = "WAIT" // Momentum is not an immediate entry per Rule 3.3 & Rule 6.6
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "close_above_upper_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close (%s) > Upper BB (%s)", fmtP(c.Close), fmtP(iv.BBUpper)),
		})
		resp.MissingConditions = append(resp.MissingConditions, "Tunggu retracement ke zona WMA 5/10 Low untuk Re-entry Momentum")
		resp.NextAction = "WAIT_FOR_REENTRY_PULLBACK"
		resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Do not chase breakout momentum candle"}}
		return
	}
	if c.Close < iv.BBLower {
		resp.MarketPhase = "MOMENTUM_CONFIRMED"
		resp.Bias = "BEARISH"
		resp.SetupType = "MOMENTUM"
		resp.SetupStatus = "WAIT"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "close_below_lower_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close (%s) < Lower BB (%s)", fmtP(c.Close), fmtP(iv.BBLower)),
		})
		resp.MissingConditions = append(resp.MissingConditions, "Tunggu retracement ke zona WMA 5/10 High untuk Re-entry Momentum")
		resp.NextAction = "WAIT_FOR_REENTRY_PULLBACK"
		resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Do not chase breakout momentum candle"}}
		return
	}

	// 3. Check Extreme Candidate & Confirmed
	// Extreme Sell: High broke Upper BB or WMA5/10 High > Upper BB, candle closes inside
	isExtremeSellCand := prevIv.WMA5High > prevIv.BBUpper || prevIv.WMA10High > prevIv.BBUpper || prevC.High > prevIv.BBUpper
	isExtremeSellConfirmed := isExtremeSellCand && c.Close < c.Open && c.Close < iv.BBUpper

	if isExtremeSellConfirmed {
		resp.MarketPhase = "EXTREME_CONFIRMED"
		resp.Bias = "BEARISH"
		resp.SetupType = "EXTREME"
		resp.SetupStatus = "CANDIDATE"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "wma_or_high_pierced_upper_bb",
			Passed:          true,
			CandleTimestamp: prevC.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("High %s > Upper BB %s", fmtP(prevC.High), fmtP(prevIv.BBUpper)),
		}, dto.EvidenceItem{
			Condition:       "reversal_bearish_close_inside_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close %s closed inside BB", fmtP(c.Close)),
		})
		resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close above Upper BB (%s)", fmtP(iv.BBUpper)))

		extremeHigh := math.Max(c.High, prevC.High)
		sl := extremeHigh + buffer
		entryRef := c.Close
		if sl-entryRef < minSLDist {
			sl = entryRef + minSLDist
		}
		tpTarget := iv.BBMiddle
		slDist := sl - entryRef
		tpDist := math.Abs(entryRef - tpTarget)
		rr := tpDist / slDist

		resp.EntryPlan = &dto.EntryPlan{
			EntryZone:       [2]float64{round(c.Close), round(iv.WMA5High)},
			StopLoss:        round(sl),
			TakeProfit:      round(tpTarget),
			RiskRewardRatio: math.Round(rr*100) / 100,
		}
		resp.RiskCheck = dto.RiskCheck{Passed: rr >= 1.5, Reasons: []string{"Target TP Wajib di Mid BB"}}
		resp.NextAction = "MONITOR_TP_WAJIB_MID_BB"
		return
	}

	// Extreme Buy: Low broke Lower BB or WMA5/10 Low < Lower BB, candle closes inside
	isExtremeBuyCand := prevIv.WMA5Low < prevIv.BBLower || prevIv.WMA10Low < prevIv.BBLower || prevC.Low < prevIv.BBLower
	isExtremeBuyConfirmed := isExtremeBuyCand && c.Close > c.Open && c.Close > iv.BBLower

	if isExtremeBuyConfirmed {
		resp.MarketPhase = "EXTREME_CONFIRMED"
		resp.Bias = "BULLISH"
		resp.SetupType = "EXTREME"
		resp.SetupStatus = "CANDIDATE"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "wma_or_low_pierced_lower_bb",
			Passed:          true,
			CandleTimestamp: prevC.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Low %s < Lower BB %s", fmtP(prevC.Low), fmtP(prevIv.BBLower)),
		}, dto.EvidenceItem{
			Condition:       "reversal_bullish_close_inside_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close %s closed inside BB", fmtP(c.Close)),
		})
		resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close below Lower BB (%s)", fmtP(iv.BBLower)))

		extremeLow := math.Min(c.Low, prevC.Low)
		sl := extremeLow - buffer
		entryRef := c.Close
		if entryRef-sl < minSLDist {
			sl = entryRef - minSLDist
		}
		tpTarget := iv.BBMiddle
		slDist := entryRef - sl
		tpDist := math.Abs(tpTarget - entryRef)
		rr := tpDist / slDist

		resp.EntryPlan = &dto.EntryPlan{
			EntryZone:       [2]float64{round(c.Close), round(iv.WMA5Low)},
			StopLoss:        round(sl),
			TakeProfit:      round(tpTarget),
			RiskRewardRatio: math.Round(rr*100) / 100,
		}
		resp.RiskCheck = dto.RiskCheck{Passed: rr >= 1.5, Reasons: []string{"Target TP Wajib di Mid BB"}}
		resp.NextAction = "MONITOR_TP_WAJIB_MID_BB"
		return
	}

	// 4. Check MHV (Market Hilang Volume) / Rejection of Top BB, Mid BB, or EMA 50
	lookbackMHV := 15
	startMHV := n - lookbackMHV
	if startMHV < 0 {
		startMHV = 0
	}

	extremeSellIdx := -1
	extremeBuyIdx := -1
	for i := startMHV; i < currIdx; i++ {
		if candles[i].High > ind[i].BBUpper || ind[i].WMA5High > ind[i].BBUpper || ind[i].WMA10High > ind[i].BBUpper {
			extremeSellIdx = i
		}
		if candles[i].Low < ind[i].BBLower || ind[i].WMA5Low < ind[i].BBLower || ind[i].WMA10Low < ind[i].BBLower {
			extremeBuyIdx = i
		}
	}

	// MHV Sell: Extreme Sell terjadi sebelumnya, harga retest Top/Mid/EMA50 tapi GAGAL break (Close < Top BB, candle bearish)
	if extremeSellIdx != -1 {
		hasCSMBreak := false
		for i := extremeSellIdx + 1; i <= currIdx; i++ {
			if candles[i].Close > ind[i].BBUpper {
				hasCSMBreak = true
				break
			}
		}

		distTopBB := (iv.BBUpper - c.High) / iv.BBUpper
		testedTopBB := c.High >= iv.BBUpper*0.997 || (distTopBB >= 0 && distTopBB <= 0.003)
		testedMidBB := c.High >= iv.BBMiddle && c.Close <= iv.BBMiddle
		testedEMA50 := c.High >= iv.EMA50 && c.Close <= iv.EMA50
		isRejection := (testedTopBB || testedMidBB || testedEMA50) && c.Close < iv.BBUpper && c.Close < c.Open

		if !hasCSMBreak && isRejection && currIdx-extremeSellIdx >= 2 {
			rejectionType := "Top BB (MHV)"
			if testedEMA50 {
				rejectionType = "EMA 50 (MHV)"
			} else if testedMidBB {
				rejectionType = "Mid BB (MHV)"
			}

			resp.MarketPhase = "MHV_CONFIRMED"
			resp.Bias = "BEARISH"
			resp.SetupType = "MHV"
			resp.SetupStatus = "SETUP_VALID"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "prior_extreme_sell",
				Passed:          true,
				CandleTimestamp: candles[extremeSellIdx].Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Terdeteksi Extreme Sell sebelumnya pada %s", candles[extremeSellIdx].Timestamp.UTC().Format("15:04")),
			}, dto.EvidenceItem{
				Condition:       "rejection_fail_to_break_bb",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Rejection %s: High (%s) gagal tembus Top BB (%s), candle close bearish", rejectionType, fmtP(c.High), fmtP(iv.BBUpper)),
			})
			resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close di atas Top BB (%s)", fmtP(iv.BBUpper)))

			// Structural Invalidation: Above the Highest High of the Extreme-MHV structure
			highestHigh := c.High
			for i := extremeSellIdx; i <= currIdx; i++ {
				if candles[i].High > highestHigh {
					highestHigh = candles[i].High
				}
			}
			structuralSL := highestHigh + buffer
			entryRef := c.Close
			if structuralSL-entryRef < minSLDist {
				structuralSL = entryRef + minSLDist
			}
			sl := structuralSL

			slDist := sl - entryRef
			tpDist := math.Max(math.Abs(entryRef-iv.BBLower), slDist*2.0)
			tp := entryRef - tpDist
			rr := tpDist / slDist

			resp.EntryPlan = &dto.EntryPlan{
				EntryZone:       [2]float64{round(c.Close), round(iv.WMA5High)},
				StopLoss:        round(sl),
				TakeProfit:      round(tp),
				RiskRewardRatio: math.Round(rr*100) / 100,
			}
			resp.RiskCheck = dto.RiskCheck{
				Passed:  rr >= 1.5,
				Reasons: []string{fmt.Sprintf("Target MHV ke Low BB, Risk/Reward: 1:%.2f", rr)},
			}
			resp.NextAction = "EXECUTE_OR_LIMIT_ENTRY_PLAN"
			return
		}
	}

	// MHV Buy: Extreme Buy terjadi sebelumnya, harga retest Low/Mid/EMA50 tapi GAGAL break (Close > Low BB, candle bullish)
	if extremeBuyIdx != -1 {
		hasCSMBreak := false
		for i := extremeBuyIdx + 1; i <= currIdx; i++ {
			if candles[i].Close < ind[i].BBLower {
				hasCSMBreak = true
				break
			}
		}

		distLowBB := (c.Low - iv.BBLower) / iv.BBLower
		testedLowBB := c.Low <= iv.BBLower*1.003 || (distLowBB >= 0 && distLowBB <= 0.003)
		testedMidBB := c.Low <= iv.BBMiddle && c.Close >= iv.BBMiddle
		testedEMA50 := c.Low <= iv.EMA50 && c.Close >= iv.EMA50
		isRejection := (testedLowBB || testedMidBB || testedEMA50) && c.Close > iv.BBLower && c.Close > c.Open

		if !hasCSMBreak && isRejection && currIdx-extremeBuyIdx >= 2 {
			rejectionType := "Low BB (MHV)"
			if testedEMA50 {
				rejectionType = "EMA 50 (MHV)"
			} else if testedMidBB {
				rejectionType = "Mid BB (MHV)"
			}

			resp.MarketPhase = "MHV_CONFIRMED"
			resp.Bias = "BULLISH"
			resp.SetupType = "MHV"
			resp.SetupStatus = "SETUP_VALID"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "prior_extreme_buy",
				Passed:          true,
				CandleTimestamp: candles[extremeBuyIdx].Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Terdeteksi Extreme Buy sebelumnya pada %s", candles[extremeBuyIdx].Timestamp.UTC().Format("15:04")),
			}, dto.EvidenceItem{
				Condition:       "rejection_fail_to_break_bb",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Rejection %s: Low (%s) gagal tembus Low BB (%s), candle close bullish", rejectionType, fmtP(c.Low), fmtP(iv.BBLower)),
			})
			resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close di bawah Low BB (%s)", fmtP(iv.BBLower)))

			lowestLow := c.Low
			for i := extremeBuyIdx; i <= currIdx; i++ {
				if candles[i].Low < lowestLow {
					lowestLow = candles[i].Low
				}
			}
			structuralSL := lowestLow - buffer
			entryRef := c.Close
			if entryRef-structuralSL < minSLDist {
				structuralSL = entryRef - minSLDist
			}
			sl := structuralSL

			slDist := entryRef - sl
			tpDist := math.Max(math.Abs(iv.BBUpper-entryRef), slDist*2.0)
			tp := entryRef + tpDist
			rr := tpDist / slDist

			resp.EntryPlan = &dto.EntryPlan{
				EntryZone:       [2]float64{round(c.Close), round(iv.WMA5Low)},
				StopLoss:        round(sl),
				TakeProfit:      round(tp),
				RiskRewardRatio: math.Round(rr*100) / 100,
			}
			resp.RiskCheck = dto.RiskCheck{
				Passed:  rr >= 1.5,
				Reasons: []string{fmt.Sprintf("Target MHV ke Top BB, Risk/Reward: 1:%.2f", rr)},
			}
			resp.NextAction = "EXECUTE_OR_LIMIT_ENTRY_PLAN"
			return
		}
	}

	// 4. Check Prior Context (CSAK / CSM in recent 10 bars) for Re-entry
	priorCSAKBullish := false
	priorCSAKBearish := false
	lookback := 12
	startBar := n - lookback
	if startBar < 0 {
		startBar = 0
	}

	for i := startBar; i < currIdx; i++ {
		// CSAK Bullish: Close > WMA5High && Close > WMA10High && Close > BBMiddle
		if candles[i].Close > ind[i].WMA5High && candles[i].Close > ind[i].WMA10High && candles[i].Close > ind[i].BBMiddle {
			priorCSAKBullish = true
		}
		// CSAK Bearish: Close < WMA5Low && Close < WMA10Low && Close < BBMiddle
		if candles[i].Close < ind[i].WMA5Low && candles[i].Close < ind[i].WMA10Low && candles[i].Close < ind[i].BBMiddle {
			priorCSAKBearish = true
		}
	}

	// 5. Check Re-entry Setup
	// Bullish Re-entry: Prior CSAK Bullish + Price touches/enters [WMA10Low, WMA5Low] zone and closes ABOVE Mid BB
	wmaLowMin := math.Min(iv.WMA5Low, iv.WMA10Low)
	wmaLowMax := math.Max(iv.WMA5Low, iv.WMA10Low)
	inBullishReentryZone := c.Low <= wmaLowMax && c.Close >= iv.BBMiddle

	if priorCSAKBullish && inBullishReentryZone {
		resp.MarketPhase = "REENTRY_CONFIRMED"
		resp.Bias = "BULLISH"
		resp.SetupType = "REENTRY"
		resp.SetupStatus = "SETUP_VALID"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "prior_csak_or_csm_bullish",
			Passed:          true,
			CandleTimestamp: candles[currIdx-1].Timestamp.UTC().Format(time.RFC3339),
			Details:         "Ditemukan CSAK/CSM Bullish dalam histori candle dekat",
		}, dto.EvidenceItem{
			Condition:       "pullback_to_ma5_10_low",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Low (%s) menyentuh zona WMA 5/10 Low (%s - %s)", fmtP(c.Low), fmtP(wmaLowMin), fmtP(wmaLowMax)),
		}, dto.EvidenceItem{
			Condition:       "closed_above_mid_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close (%s) bertahan di atas Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
		})
		resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close di bawah Low BB (%s)", fmtP(iv.BBLower)))

		startLook := currIdx - 12
		if startLook < 0 {
			startLook = 0
		}
		swingLow := c.Low
		for i := startLook; i <= currIdx; i++ {
			if candles[i].Low < swingLow {
				swingLow = candles[i].Low
			}
		}

		// Stop Loss di balik Low BB sesuai standar aturan BBMA Oma Ally
		structuralSL := math.Min(swingLow, iv.BBLower) - buffer
		entryMid := (wmaLowMin + wmaLowMax) / 2.0
		if entryMid-structuralSL < minSLDist {
			structuralSL = entryMid - minSLDist
		}
		sl := structuralSL

		slDist := entryMid - sl
		tpDist := math.Max(iv.BBUpper-entryMid, slDist*2.0)
		tp := entryMid + tpDist
		rr := tpDist / slDist

		resp.EntryPlan = &dto.EntryPlan{
			EntryZone:       [2]float64{round(wmaLowMin), round(wmaLowMax)},
			StopLoss:        round(sl),
			TakeProfit:      round(tp),
			RiskRewardRatio: math.Round(rr*100) / 100,
		}
		resp.RiskCheck = dto.RiskCheck{
			Passed:  rr >= 1.5,
			Reasons: []string{fmt.Sprintf("Risk/Reward rasio: 1:%.2f", rr)},
		}
		resp.NextAction = "EXECUTE_OR_LIMIT_ENTRY_PLAN"
		return
	}

	// Bearish Re-entry: Prior CSAK Bearish + Price touches/enters [WMA5High, WMA10High] zone and closes BELOW Mid BB
	wmaHighMin := math.Min(iv.WMA5High, iv.WMA10High)
	wmaHighMax := math.Max(iv.WMA5High, iv.WMA10High)
	inBearishReentryZone := c.High >= wmaHighMin && c.Close <= iv.BBMiddle

	if priorCSAKBearish && inBearishReentryZone {
		resp.MarketPhase = "REENTRY_CONFIRMED"
		resp.Bias = "BEARISH"
		resp.SetupType = "REENTRY"
		resp.SetupStatus = "SETUP_VALID"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "prior_csak_or_csm_bearish",
			Passed:          true,
			CandleTimestamp: candles[currIdx-1].Timestamp.UTC().Format(time.RFC3339),
			Details:         "Ditemukan CSAK/CSM Bearish dalam histori candle dekat",
		}, dto.EvidenceItem{
			Condition:       "pullback_to_ma5_10_high",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("High (%s) menyentuh zona WMA 5/10 High (%s - %s)", fmtP(c.High), fmtP(wmaHighMin), fmtP(wmaHighMax)),
		}, dto.EvidenceItem{
			Condition:       "closed_below_mid_bb",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Close (%s) bertahan di bawah Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
		})
		resp.InvalidationConditions = append(resp.InvalidationConditions, fmt.Sprintf("Candle close di atas Top BB (%s)", fmtP(iv.BBUpper)))

		startLook := currIdx - 12
		if startLook < 0 {
			startLook = 0
		}
		swingHigh := c.High
		for i := startLook; i <= currIdx; i++ {
			if candles[i].High > swingHigh {
				swingHigh = candles[i].High
			}
		}

		// Stop Loss di balik Top BB sesuai standar aturan BBMA Oma Ally
		structuralSL := math.Max(swingHigh, iv.BBUpper) + buffer
		entryMid := (wmaHighMin + wmaHighMax) / 2.0
		if structuralSL-entryMid < minSLDist {
			structuralSL = entryMid + minSLDist
		}
		sl := structuralSL

		slDist := sl - entryMid
		tpDist := math.Max(entryMid-iv.BBLower, slDist*2.0)
		tp := entryMid - tpDist
		rr := tpDist / slDist

		resp.EntryPlan = &dto.EntryPlan{
			EntryZone:       [2]float64{round(wmaHighMin), round(wmaHighMax)},
			StopLoss:        round(sl),
			TakeProfit:      round(tp),
			RiskRewardRatio: math.Round(rr*100) / 100,
		}
		resp.RiskCheck = dto.RiskCheck{
			Passed:  rr >= 1.5,
			Reasons: []string{fmt.Sprintf("Risk/Reward rasio: 1:%.2f", rr)},
		}
		resp.NextAction = "EXECUTE_OR_LIMIT_ENTRY_PLAN"
		return
	}

	// 6. Check Candlestick Arah (CSAK vs CS Awal)
	if c.Close > iv.WMA5High && c.Close > iv.WMA10High {
		if c.Close > iv.BBMiddle {
			resp.MarketPhase = "CS_KUKUH"
			resp.Bias = "BULLISH"
			resp.SetupType = "CSAK"
			resp.SetupStatus = "CANDIDATE"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "close_break_ma5_10_high_and_mid_bb",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Close (%s) > WMA 5/10 High dan Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
			})
			resp.MissingConditions = append(resp.MissingConditions, "Tunggu pullback ke MA 5/10 Low untuk setup Re-entry Buy")
			resp.NextAction = "WAIT_FOR_REENTRY"
			resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"CSAK adalah penentu arah, bukan titik entry langsung"}}
			return
		} else {
			resp.MarketPhase = "CS_AWAL"
			resp.Bias = "BULLISH"
			resp.SetupType = "CS_AWAL"
			resp.SetupStatus = "WAIT"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "close_break_ma5_10_high_only",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Close (%s) > WMA 5/10 High tapi belum tembus Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
			})
			resp.NextAction = "WAIT_FOR_CSAK_CONFIRMATION"
			resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Belum konfirmasi CSAK"}}
			return
		}
	}

	if c.Close < iv.WMA5Low && c.Close < iv.WMA10Low {
		if c.Close < iv.BBMiddle {
			resp.MarketPhase = "CS_KUKUH"
			resp.Bias = "BEARISH"
			resp.SetupType = "CSAK"
			resp.SetupStatus = "CANDIDATE"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "close_break_ma5_10_low_and_mid_bb",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Close (%s) < WMA 5/10 Low dan Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
			})
			resp.MissingConditions = append(resp.MissingConditions, "Tunggu pullback ke MA 5/10 High untuk setup Re-entry Sell")
			resp.NextAction = "WAIT_FOR_REENTRY"
			resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"CSAK adalah penentu arah, bukan titik entry langsung"}}
			return
		} else {
			resp.MarketPhase = "CS_AWAL"
			resp.Bias = "BEARISH"
			resp.SetupType = "CS_AWAL"
			resp.SetupStatus = "WAIT"
			resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
				Condition:       "close_break_ma5_10_low_only",
				Passed:          true,
				CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
				Details:         fmt.Sprintf("Close (%s) < WMA 5/10 Low tapi belum tembus Mid BB (%s)", fmtP(c.Close), fmtP(iv.BBMiddle)),
			})
			resp.NextAction = "WAIT_FOR_CSAK_CONFIRMATION"
			resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Belum konfirmasi CSAK"}}
			return
		}
	}

	// 7. Default: Sideways or Neutral
	if isFlatBB {
		resp.MarketPhase = "SIDEWAYS"
		resp.Bias = "NEUTRAL"
		resp.SetupType = "SIDEWAYS"
		resp.SetupStatus = "WAIT"
		resp.Evidence = append(resp.Evidence, dto.EvidenceItem{
			Condition:       "bollinger_bandwidth_flat",
			Passed:          true,
			CandleTimestamp: c.Timestamp.UTC().Format(time.RFC3339),
			Details:         fmt.Sprintf("Bandwidth %.4f < 0.0035 (BB Flat)", bbBandwidth),
		})
		resp.NextAction = "WAIT_FOR_BREAKOUT"
		resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Market sideways / tidak ada setup entry"}}
		return
	}

	resp.MarketPhase = "NEUTRAL"
	resp.Bias = "NEUTRAL"
	resp.SetupType = "NONE"
	resp.SetupStatus = "WAIT"
	resp.NextAction = "WAIT_FOR_SETUP"
	resp.RiskCheck = dto.RiskCheck{Passed: false, Reasons: []string{"Tidak ada setup BBMA yang aktif pada candle close"}}
}

func (s *bbmaService) saveSetupRecord(ctx context.Context, resp *dto.AnalysisResponse, candleTime time.Time) error {
	if s.setupRepo == nil {
		return nil
	}
	evidenceBytes, _ := json.Marshal(resp.Evidence)
	missingBytes, _ := json.Marshal(resp.MissingConditions)
	invStr := ""
	if len(resp.InvalidationConditions) > 0 {
		invStr = resp.InvalidationConditions[0]
	}

	var entryPrice, sl, tp float64
	if resp.EntryPlan != nil {
		entryPrice = (resp.EntryPlan.EntryZone[0] + resp.EntryPlan.EntryZone[1]) / 2.0
		sl = resp.EntryPlan.StopLoss
		tp = resp.EntryPlan.TakeProfit
	}

	record := &model.SetupRecord{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Symbol:                 resp.Symbol,
		Timeframe:              resp.Timeframe,
		SetupID:                fmt.Sprintf("%s-%s-%d", resp.Symbol, resp.Timeframe, candleTime.Unix()),
		SetupType:              resp.SetupType,
		State:                  resp.SetupStatus,
		Direction:              resp.Bias,
		CandleTimestamp:        candleTime,
		EvidenceJSON:           string(evidenceBytes),
		MissingConditionsJSON:  string(missingBytes),
		InvalidationConditions: invStr,
		EntryPrice:             entryPrice,
		StopLoss:               sl,
		TakeProfit:             tp,
		ExpiresAt:              time.Now().Add(24 * time.Hour),
	}

	return s.setupRepo.Create(ctx, record)
}

func (s *bbmaService) ListActiveSetups(ctx context.Context, symbol string, p dto.PaginationQuery) ([]model.SetupRecord, dto.PaginationMeta, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 || p.PerPage > 100 {
		p.PerPage = 10
	}

	setups, total, err := s.setupRepo.ListActive(ctx, symbol, p)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(p.PerPage)))

	meta := dto.PaginationMeta{
		CurrentPage: p.Page,
		PerPage:     p.PerPage,
		TotalItems:  total,
		TotalPages:  totalPages,
	}

	return setups, meta, nil
}

func (s *bbmaService) AnalyzeMTF(ctx context.Context, req dto.MTFAnalysisRequest) (*dto.MTFAnalysisResponse, error) {
	if req.TF1 == "" {
		req.TF1 = "H4"
	}
	if req.TF2 == "" {
		req.TF2 = "H1"
	}
	if req.TF3 == "" {
		req.TF3 = "M15"
	}

	res1, err := s.Analyze(ctx, dto.AnalysisRequest{Symbol: req.Symbol, Timeframe: req.TF1, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("tf1 (%s) analysis failed: %w", req.TF1, err)
	}

	res2, err := s.Analyze(ctx, dto.AnalysisRequest{Symbol: req.Symbol, Timeframe: req.TF2, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("tf2 (%s) analysis failed: %w", req.TF2, err)
	}

	res3, err := s.Analyze(ctx, dto.AnalysisRequest{Symbol: req.Symbol, Timeframe: req.TF3, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("tf3 (%s) analysis failed: %w", req.TF3, err)
	}

	asOfTime, _ := time.Parse(time.RFC3339, res3.AsOf)
	if asOfTime.IsZero() {
		asOfTime = time.Now().UTC()
	}

	symUpper := strings.ToUpper(req.Symbol)
	isCrypto := strings.Contains(symUpper, "BTC") || strings.Contains(symUpper, "ETH") || strings.Contains(symUpper, "SOL") || strings.Contains(symUpper, "BNB")

	utcHour := asOfTime.UTC().Hour()
	utcWeekday := asOfTime.UTC().Weekday()

	isWeekend := !isCrypto && ((utcWeekday == time.Saturday) ||
		(utcWeekday == time.Sunday && utcHour < 22) ||
		(utcWeekday == time.Friday && utcHour >= 22))

	// Jam pasar (Winter / Standard Time):
	// London: 08:00 - 16:30 GMT (08:00 - 16:30 UTC / 15:00 - 23:30 WIB)
	// New York: 08:00 - 17:00 EST (13:00 - 22:00 UTC / 20:00 - 05:00 WIB)
	// Overlap London - New York: 13:00 - 17:00 UTC (20:00 - 00:00 WIB)
	// Rollover Subuh: 22:00 - 24:00 UTC (05:00 - 07:00 WIB)
	isRollover := !isCrypto && (utcHour >= 22 || utcHour < 0)
	isLondonNY := (utcHour >= 8 && utcHour < 22) // 15:00 - 05:00 WIB
	isOverlap := (utcHour >= 13 && utcHour < 17)  // 20:00 - 00:00 WIB (Puncak Likuiditas)
	isLondonOnly := (utcHour >= 8 && utcHour < 13) // 15:00 - 20:00 WIB
	isNYOnly := (utcHour >= 17 && utcHour < 22)   // 00:00 - 05:00 WIB

	var sessionName string
	if isWeekend {
		sessionName = "WEEKEND_CLOSED (Libur Akhir Pekan - Buka Senin 05:00 WIB)"
	} else if isRollover {
		sessionName = "ROLLOVER_BLACKOUT (05:00 - 07:00 WIB)"
	} else if isOverlap {
		sessionName = "LONDON_NEWYORK_OVERLAP (20:00 - 00:00 WIB) [PUNCAK LIKUIDITAS]"
	} else if isLondonOnly {
		sessionName = "LONDON_SESSION (15:00 - 20:00 WIB)"
	} else if isNYOnly {
		sessionName = "NEWYORK_SESSION (00:00 - 05:00 WIB)"
	} else {
		sessionName = "ASIAN_SESSION (07:00 - 15:00 WIB)"
	}

	resp := &dto.MTFAnalysisResponse{
		Symbol:          req.Symbol,
		TF1:             req.TF1,
		TF2:             req.TF2,
		TF3:             req.TF3,
		AsOf:            res3.AsOf,
		MarketSession:   sessionName,
		IsHighLiquidity: isLondonNY,
		TF1Analysis:     res1,
		TF2Analysis:     res2,
		TF3Analysis:     res3,
	}

	// Pattern format: e.g. "REENTRY(H4) -> MHV(H1) -> EXTREME(M15)"
	resp.ConfluencePattern = fmt.Sprintf("%s(%s) -> %s(%s) -> %s(%s)",
		res1.SetupType, req.TF1, res2.SetupType, req.TF2, res3.SetupType, req.TF3)

	// Check Weekend Closed Lock
	if isWeekend {
		resp.ConfluenceGrade = "MARKET_CLOSED_WEEKEND"
		resp.Bias = "WAIT"
		resp.ActionRecommendation = "STAND_ASIDE"
		resp.RecommendedRisk = "0% (Pasar Tutup)"
		resp.Summary = "🏖️ PASAR FOREX/EMAS TUTUP (Akhir Pekan): Pasar tutup sejak Sabtu subuh dan baru buka kembali Senin pagi 05:00 WIB (22:00 UTC Minggu). Dilarang entry!"
		return resp, nil
	}

	// Check Rollover Blackout Lock (04:00 - 06:00 WIB)
	if isRollover {
		resp.ConfluenceGrade = "ROLLOVER_LOCK"
		resp.Bias = "WAIT"
		resp.ActionRecommendation = "STAND_ASIDE"
		resp.RecommendedRisk = "0% (Dilarang Trading)"
		resp.Summary = "🚫 JEDA ROLLOVER SUBUH (04:00 - 06:00 WIB): Spread broker melebar drastis. Dilarang entry untuk melindungi akun prop firm dari stop-out!"
		return resp, nil
	}

	// Check Bias conflict
	if (res1.Bias == "BULLISH" && res3.Bias == "BEARISH") || (res1.Bias == "BEARISH" && res3.Bias == "BULLISH") {
		resp.ConfluenceGrade = "CONFLICT"
		resp.Bias = "CONFLICT"
		resp.ActionRecommendation = "STAND_ASIDE"
		resp.Summary = fmt.Sprintf("Arah timeframe besar %s (%s) berlawanan dengan trigger %s (%s). Aturan Section 7 Rule 10: Terapkan WAIT.", req.TF1, res1.Bias, req.TF3, res3.Bias)
		return resp, nil
	}

	// Helper to finalize MTF Grade with strict 1:2 R:R gate
	applyConfluenceGrade := func(baseTitle string, bias string, plan *dto.EntryPlan) {
		resp.Bias = bias
		resp.EntryPlan = plan

		if plan == nil {
			resp.ConfluenceGrade = "GRADE_B_STRONG"
			resp.ActionRecommendation = "WAIT_FOR_SETUP"
			resp.RecommendedRisk = "0% (Belum Ada Entry Plan)"
			resp.Summary = fmt.Sprintf("Teknikal selaras (%s), namun belum terbentuk Entry Plan valid pada timeframe eksekusi.", bias)
			return
		}

		if plan.RiskRewardRatio < 2.0 {
			resp.ConfluenceGrade = "GRADE_A_PENDING_RR"
			resp.ActionRecommendation = "WAIT_FOR_BETTER_RR"
			resp.RecommendedRisk = fmt.Sprintf("0%% (R:R 1:%.2f < 1:2.0)", plan.RiskRewardRatio)
			resp.Summary = fmt.Sprintf("Struktur 3-TF %s selaras (%s). TETAPI Rasio R:R (1:%.2f) di bawah syarat minimal 1:2. DILARANG entry! Menunggu harga koreksi lebih dalam mendekati level SL.", baseTitle, sessionName, plan.RiskRewardRatio)
			return
		}

		// R:R >= 2.0 passed!
		if isLondonNY {
			resp.ConfluenceGrade = "GRADE_A_PLUS"
			resp.RecommendedRisk = "0.75% - 1.00% ($75 - $100 Full Risk)"
			resp.ActionRecommendation = "EXECUTE_ENTRY_FULL_RISK"
			resp.Summary = fmt.Sprintf("💎 GRADE A+ PRIME SETUP (%s - %s):\nSeluruh Hukum Kodrat 3-TF BBMA dan Filter R:R (1:%.2f >= 1:2) terpenuhi sempurna! Siap eksekusi di zona entry.", baseTitle, sessionName, plan.RiskRewardRatio)
		} else {
			resp.ConfluenceGrade = "GRADE_A"
			resp.RecommendedRisk = "0.35% - 0.50% ($35 - $50 Half Risk)"
			resp.ActionRecommendation = "EXECUTE_ENTRY_HALF_RISK"
			resp.Summary = fmt.Sprintf("🥇 GRADE A SETUP (%s - %s):\nSeluruh teknikal BBMA dan Filter R:R (1:%.2f >= 1:2) terpenuhi pada sesi Asia. Gunakan separuh risiko.", baseTitle, sessionName, plan.RiskRewardRatio)
		}
	}

	// 1. Hukum Kodrat Re-entry Oma Ally: Di TF1 Re-entry, TF2 PANTANG CSM!
	if res1.SetupType == "REENTRY" {
		if res2.SetupType == "MOMENTUM" {
			resp.ConfluenceGrade = "INVALID_CSM_AT_TF2"
			resp.Bias = "WAIT"
			resp.ActionRecommendation = "STAND_ASIDE"
			resp.Summary = fmt.Sprintf("⚠️ RE-ENTRY TIDAK VALID (Hukum Kodrat BBMA Oma Ally): Di %s terdapat Re-entry %s, tetapi di %s terbentuk Candle Momentum (CSM)! Di TF2 pantang ada CSM. TF2 wajib membentuk Rejection (Reject Top BB/Mid BB/EMA 50 atau MHV/Extreme), BUKAN CSM.", req.TF1, res1.Bias, req.TF2)
			return resp, nil
		}

		if (res2.SetupType == "MHV" || res2.SetupType == "EXTREME") && res2.Bias == res1.Bias {
			if (res3.SetupType == "EXTREME" || res3.SetupType == "REENTRY" || res3.SetupType == "MHV") && res3.Bias == res1.Bias {
				applyConfluenceGrade("Hukum Kodrat BBMA", res1.Bias, res3.EntryPlan)
				return resp, nil
			}
		}
	}

	// 2. Hukum Re-entry pada TF2: Jika TF2 Re-entry, TF3 pantang CSM berlawanan
	if res2.SetupType == "REENTRY" && res3.SetupType == "MOMENTUM" {
		resp.ConfluenceGrade = "INVALID_CSM_AT_TRIGGER"
		resp.Bias = "WAIT"
		resp.ActionRecommendation = "STAND_ASIDE"
		resp.Summary = fmt.Sprintf("⚠️ RE-ENTRY TIDAK VALID: Di %s terdapat Re-entry %s, tetapi timeframe trigger %s membentuk Candle Momentum (CSM) yang menembus BB! Wajib tunggu %s membentuk Rejection/Extreme, bukan CSM.", req.TF2, res2.Bias, req.TF3, req.TF3)
		return resp, nil
	}

	// Golden Setup Check: TF1 Reentry/CSAK + TF2 MHV/CSAK + TF3 Extreme
	isGolden := (res1.SetupType == "REENTRY" || res1.SetupType == "CSAK") &&
		(res2.SetupType == "MHV" || res2.SetupType == "REENTRY" || res2.SetupType == "CSAK") &&
		(res3.SetupType == "EXTREME" || res3.SetupType == "REENTRY" || res3.SetupType == "MHV") &&
		res1.Bias == res3.Bias

	if isGolden {
		applyConfluenceGrade("Golden Setup BBMA", res1.Bias, res3.EntryPlan)
		return resp, nil
	}

	// Strong Confluence: TF1 Trend/CSM + TF2 Re-entry/CSAK
	isStrong := (res1.Bias == res2.Bias || res2.Bias == res3.Bias) && (res3.SetupStatus == "SETUP_VALID" || res2.SetupStatus == "SETUP_VALID")
	if isStrong {
		if res3.Bias != "NEUTRAL" {
			resp.Bias = res3.Bias
		} else {
			resp.Bias = res2.Bias
		}

		allThreeAligned := res1.Bias == res2.Bias && res2.Bias == res3.Bias && res3.SetupStatus == "SETUP_VALID"
		if allThreeAligned {
			applyConfluenceGrade("Re-entry Momentum MTF", resp.Bias, res3.EntryPlan)
		} else {
			resp.ConfluenceGrade = "GRADE_B_STRONG"
			resp.ActionRecommendation = "MONITOR_ENTRY_ZONE"
			resp.Summary = fmt.Sprintf("Setup kuat terkonfirmasi searah %s. Pantau konfirmasi candle close di zona entry.", resp.Bias)
			resp.EntryPlan = res3.EntryPlan
		}
		return resp, nil
	}

	resp.ConfluenceGrade = "NEUTRAL"
	resp.Bias = "NEUTRAL"
	resp.ActionRecommendation = "WAIT_FOR_SETUP"
	resp.Summary = "Belum ada keselarasan multi-timeframe yang valid. Menunggu pembentukan setup lanjutan."
	return resp, nil
}
