package marketfeed

import (
	"context"
	"testing"
	"time"
)

// TestBinanceProvider adalah uji integrasi ke API publik Binance.
//
// Sejak Yahoo Finance dihapus, Binance hanya dipakai untuk instrumen kripto.
// Uji ini memakai pasangan kripto yang benar-benar dilayani jalur produksi.
// Bila lingkungan tidak dapat menjangkau internet, uji dilewati: kegagalan
// jaringan bukan bukti kode salah.
func TestBinanceProvider(t *testing.T) {
	provider := NewBinanceProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	candles, err := provider.FetchCandles(ctx, "BTCUSDT", "15m", 100)
	if err != nil {
		t.Skipf("Binance tidak terjangkau dari lingkungan ini: %v", err)
	}

	if len(candles) < 50 {
		t.Fatalf("expected at least 50 candles, got %d", len(candles))
	}

	last := candles[len(candles)-1]
	if last.High < last.Low || last.High < last.Close || last.Low > last.Close {
		t.Fatalf("corrupted candle values: %+v", last)
	}

	t.Logf("Successfully fetched %d candles for BTCUSDT. Latest close: %f at %s (is_closed=%v)",
		len(candles), last.Close, last.Timestamp.Format(time.RFC3339), last.IsClosed)
}
