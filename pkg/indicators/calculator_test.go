package indicators

import (
	"testing"
	"time"
)

func TestCalculateWMA(t *testing.T) {
	// 5 prices: 1, 2, 3, 4, 5
	// weights: 1, 2, 3, 4, 5 (sum weights = 15)
	// sum = 1*1 + 2*2 + 3*3 + 4*4 + 5*5 = 1 + 4 + 9 + 16 + 25 = 55
	// wma = 55 / 15 = 3.6666666666666665
	prices := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	expected := 55.0 / 15.0
	actual := calculateWMA(prices)

	if actual < expected-0.0001 || actual > expected+0.0001 {
		t.Fatalf("expected %f, got %f", expected, actual)
	}
}

func TestCalculateBB(t *testing.T) {
	prices := make([]float64, 20)
	for i := 0; i < 20; i++ {
		prices[i] = 100.0
	}
	middle, upper, lower := calculateBB(prices, 2.0)
	if middle != 100.0 || upper != 100.0 || lower != 100.0 {
		t.Fatalf("flat prices should produce 0 stddev: %f, %f, %f", middle, upper, lower)
	}
}

func TestCalculateBBMA(t *testing.T) {
	now := time.Now()
	candles := make([]Candle, 60)
	for i := 0; i < 60; i++ {
		candles[i] = Candle{
			Timestamp: now.Add(time.Duration(i) * time.Hour),
			Open:      2000.0 + float64(i),
			High:      2005.0 + float64(i),
			Low:       1995.0 + float64(i),
			Close:     2002.0 + float64(i),
			Volume:    100,
			IsClosed:  true,
		}
	}

	res, err := CalculateBBMA(candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 60 {
		t.Fatalf("expected 60 results, got %d", len(res))
	}

	last := res[59]
	if !last.ValidBB || !last.ValidWMA || !last.ValidEMA {
		t.Fatalf("last candle should have all valid indicators: %+v", last)
	}

	if last.BBUpper <= last.BBMiddle || last.BBMiddle <= last.BBLower {
		t.Fatalf("invalid BB ordering: upper=%f, mid=%f, lower=%f", last.BBUpper, last.BBMiddle, last.BBLower)
	}
}
