package indicators

import (
	"errors"
	"math"
)

var (
	ErrNotEnoughData = errors.New("not enough candles to calculate indicators (min 50 required)")
)

// CalculateBBMA computes Bollinger Bands (20, 2), WMA 5/10 High/Low, and EMA 50 for all candles.
// Returns an array of IndicatorValues with length equal to candles.
func CalculateBBMA(candles []Candle) ([]IndicatorValues, error) {
	n := len(candles)
	if n < 50 {
		return nil, ErrNotEnoughData
	}

	result := make([]IndicatorValues, n)

	// Pre-extract slices for fast sequential access
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		highs[i] = candles[i].High
		lows[i] = candles[i].Low
		closes[i] = candles[i].Close

		result[i].Timestamp = candles[i].Timestamp
		result[i].Open = candles[i].Open
		result[i].High = candles[i].High
		result[i].Low = candles[i].Low
		result[i].Close = candles[i].Close
	}

	// 1. Calculate WMA 5 High & Low
	for i := 4; i < n; i++ {
		result[i].WMA5High = calculateWMA(highs[i-4 : i+1])
		result[i].WMA5Low = calculateWMA(lows[i-4 : i+1])
	}

	// 2. Calculate WMA 10 High & Low
	for i := 9; i < n; i++ {
		result[i].WMA10High = calculateWMA(highs[i-9 : i+1])
		result[i].WMA10Low = calculateWMA(lows[i-9 : i+1])
		result[i].ValidWMA = true
	}

	// 3. Calculate Bollinger Bands (20, 2) on Close
	for i := 19; i < n; i++ {
		middle, upper, lower := calculateBB(closes[i-19:i+1], 2.0)
		result[i].BBMiddle = middle
		result[i].BBUpper = upper
		result[i].BBLower = lower
		result[i].ValidBB = true
	}

	// 4. Calculate EMA 50 on Close
	// Seed with SMA of first 50 candles
	var sum50 float64
	for i := 0; i < 50; i++ {
		sum50 += closes[i]
	}
	ema := sum50 / 50.0
	result[49].EMA50 = ema
	result[49].ValidEMA = true

	alpha := 2.0 / (50.0 + 1.0)
	for i := 50; i < n; i++ {
		ema = closes[i]*alpha + ema*(1.0-alpha)
		result[i].EMA50 = ema
		result[i].ValidEMA = true
	}

	return result, nil
}

// calculateWMA calculates Linear Weighted Moving Average for the given slice.
// Weights are 1 for oldest (idx 0), up to len(slice) for latest (idx len-1).
func calculateWMA(prices []float64) float64 {
	length := len(prices)
	if length == 0 {
		return 0
	}

	var sum float64
	weightSum := float64(length * (length + 1) / 2)

	for i := 0; i < length; i++ {
		weight := float64(i + 1)
		sum += prices[i] * weight
	}

	return sum / weightSum
}

// calculateBB calculates Bollinger Bands (Middle, Upper, Lower) for window slice with multiplier dev.
func calculateBB(prices []float64, dev float64) (middle, upper, lower float64) {
	length := len(prices)
	if length == 0 {
		return 0, 0, 0
	}

	var sum float64
	for _, p := range prices {
		sum += p
	}
	middle = sum / float64(length)

	var varianceSum float64
	for _, p := range prices {
		diff := p - middle
		varianceSum += diff * diff
	}
	// Population standard deviation (as standard in MetaTrader / TradingView)
	stdev := math.Sqrt(varianceSum / float64(length))

	upper = middle + dev*stdev
	lower = middle - dev*stdev
	return middle, upper, lower
}
