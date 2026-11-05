package marketfeed

import (
	"context"
	"errors"
	"time"

	"ila-engine/pkg/indicators"
)

var (
	ErrInvalidCandleData = errors.New("invalid candle data (high < low or irregular values)")
	ErrDuplicateTimestamp = errors.New("duplicate candle timestamp detected")
	ErrNonMonotonicTime  = errors.New("non-monotonic candle timestamps detected")
)

// Provider defines the interface for fetching market candlestick data.
type Provider interface {
	FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error)
	Name() string
}

// ValidateCandles performs data integrity checks as required by Section 5.1 & Section 15:
// - high >= max(open, close, low)
// - low <= min(open, close, high)
// - No duplicate timestamps
// - Monotonic ascending order
func ValidateCandles(candles []indicators.Candle) ([]indicators.Candle, error) {
	if len(candles) == 0 {
		return nil, nil
	}

	validated := make([]indicators.Candle, 0, len(candles))
	seenTimestamps := make(map[int64]bool)
	var lastTime time.Time

	for i, c := range candles {
		// Rule 1: High must be highest, Low must be lowest
		if c.High < c.Low || c.High < c.Open || c.High < c.Close || c.Low > c.Open || c.Low > c.Close {
			return nil, ErrInvalidCandleData
		}

		ts := c.Timestamp.Unix()
		// Rule 2: No duplicates
		if seenTimestamps[ts] {
			continue // Deduplicate
		}
		seenTimestamps[ts] = true

		// Rule 3: Monotonic order
		if i > 0 && !c.Timestamp.After(lastTime) {
			return nil, ErrNonMonotonicTime
		}

		lastTime = c.Timestamp
		validated = append(validated, c)
	}

	return validated, nil
}
