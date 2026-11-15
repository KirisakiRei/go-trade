package marketfeed

import (
	"testing"
)

// TestMapResolutionMencakupTimeframeEngine memastikan semua timeframe yang
// dipakai ILA 2.0 dan ILA 3.0 punya resolusi eksplisit. Nilai kosong berarti
// provider menolak permintaan, bukan diam-diam memakai 15 menit.
func TestMapResolutionMencakupTimeframeEngine(t *testing.T) {
	p := NewTradingViewProvider()

	cases := map[string]string{
		"M5":  "5",
		"5m":  "5",
		"M15": "15",
		"15m": "15",
		"H1":  "60",
		"1H":  "60",
		"H4":  "240",
		"4H":  "240",
		"D1":  "1D",
		"D":   "1D",
		"W":   "1W",
		"1W":  "1W",
	}

	for tf, want := range cases {
		if got := p.MapResolution(tf); got != want {
			t.Errorf("MapResolution(%q) = %q, ingin %q", tf, got, want)
		}
	}
}

func TestMapResolutionTakDikenalKosong(t *testing.T) {
	p := NewTradingViewProvider()
	if got := p.MapResolution("X9"); got != "" {
		t.Errorf("MapResolution(X9) = %q, ingin kosong", got)
	}
}

// TestMapSymbolMencakupSimbolEngine memastikan simbol yang benar-benar
// dipakai engine dan pembanding SMT-nya punya padanan TradingView.
func TestMapSymbolMencakupSimbolEngine(t *testing.T) {
	p := NewTradingViewProvider()

	cases := map[string]string{
		"XAUUSD": "FOREXCOM:XAUUSD",
		"EURUSD": "FOREXCOM:EURUSD",
		"USTEC":  "FOREXCOM:NAS100",
		"US500":  "FOREXCOM:SPX500",
		"XAGUSD": "FOREXCOM:XAGUSD",
		"GBPUSD": "FOREXCOM:GBPUSD",
	}

	for in, want := range cases {
		if got := p.MapSymbol(in); got != want {
			t.Errorf("MapSymbol(%q) = %q, ingin %q", in, got, want)
		}
	}
}

// TestBinanceMapTimeframe memastikan timeframe ILA 3.0 tidak menjadi interval
// tidak sah ("d"/"w") seperti sebelumnya.
func TestBinanceMapTimeframe(t *testing.T) {
	p := NewBinanceProvider()

	cases := map[string]string{
		"M5":  "5m",
		"5m":  "5m",
		"M15": "15m",
		"15m": "15m",
		"H1":  "1h",
		"1H":  "1h",
		"H4":  "4h",
		"4H":  "4h",
		"D":   "1d",
		"D1":  "1d",
		"W":   "1w",
		"1W":  "1w",
	}

	for tf, want := range cases {
		if got := p.mapTimeframe(tf); got != want {
			t.Errorf("mapTimeframe(%q) = %q, ingin %q", tf, got, want)
		}
	}

	if got := p.mapTimeframe("X9"); got != "" {
		t.Errorf("mapTimeframe(X9) = %q, ingin kosong", got)
	}
}

func TestIsCryptoSymbol(t *testing.T) {
	kripto := []string{"BTCUSD", "BTCUSDT", "ETHUSD", "ETHUSDT", "PAXGUSDT"}
	for _, sym := range kripto {
		if !isCryptoSymbol(sym) {
			t.Errorf("isCryptoSymbol(%q) = false, ingin true", sym)
		}
	}

	bukan := []string{"XAUUSD", "EURUSD", "USTEC", "US500", "XAGUSD", "GBPUSD"}
	for _, sym := range bukan {
		if isCryptoSymbol(sym) {
			t.Errorf("isCryptoSymbol(%q) = true, ingin false", sym)
		}
	}
}
