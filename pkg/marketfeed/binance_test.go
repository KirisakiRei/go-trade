package marketfeed

import (
	"context"
	"testing"
	"time"
)

func TestBinanceProvider(t *testing.T) {
	provider := NewBinanceProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	candles, err := provider.FetchCandles(ctx, "XAUUSD", "15m", 100)
	if err != nil {
		t.Fatalf("failed to fetch candles: %v", err)
	}

	if len(candles) < 50 {
		t.Fatalf("expected at least 50 candles, got %d", len(candles))
	}

	last := candles[len(candles)-1]
	if last.High < last.Low || last.High < last.Close || last.Low > last.Close {
		t.Fatalf("corrupted candle values: %+v", last)
	}

	t.Logf("Successfully fetched %d candles for XAUUSD. Latest close: %f at %s (is_closed=%v)",
		len(candles), last.Close, last.Timestamp.Format(time.RFC3339), last.IsClosed)
}
