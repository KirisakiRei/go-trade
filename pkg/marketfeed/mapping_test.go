package marketfeed

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
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

func TestMacroTimeframe_TanpaBentrokMenit(t *testing.T) {
	tv := NewTradingViewProvider()
	tvCases := map[string]string{
		"1M": "1", "3M": "3", // tetap menit
		"MN": "1M", "MN1": "1M", "monthly": "1M", "1MO": "1M",
		"Q": "3M", "Q1": "3M", "quarterly": "3M", "3MO": "3M",
	}
	for tf, want := range tvCases {
		if got := tv.MapResolution(tf); got != want {
			t.Errorf("TV MapResolution(%q) = %q, ingin %q", tf, got, want)
		}
	}

	bn := NewBinanceProvider()
	bnCases := map[string]string{
		"1M": "1m", "3M": "3m", // tetap menit
		"MN": "1M", "MN1": "1M", "monthly": "1M", "1MO": "1M",
	}
	for tf, want := range bnCases {
		if got := bn.mapTimeframe(tf); got != want {
			t.Errorf("Binance mapTimeframe(%q) = %q, ingin %q", tf, got, want)
		}
	}
}

func TestCandleCloseTime_Kalender(t *testing.T) {
	jan := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got := candleCloseTime(jan, "MN1"); !got.Equal(time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("close Jan MN1 = %v", got)
	}
	febLeap := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	if got := candleCloseTime(febLeap, "MN1"); !got.Equal(time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("close Feb kabisat MN1 = %v", got)
	}
	if got := candleCloseTime(jan, "Q1"); !got.Equal(time.Date(2025, time.April, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("close Q1 = %v", got)
	}
}

func TestAggregateQuarterly(t *testing.T) {
	m := func(month time.Month, o, h, l, c, v float64, closed bool) indicators.Candle {
		return indicators.Candle{
			Timestamp: time.Date(2025, month, 1, 0, 0, 0, 0, time.UTC),
			Open:      o, High: h, Low: l, Close: c, Volume: v, IsClosed: closed,
		}
	}
	monthly := []indicators.Candle{
		m(time.January, 100, 110, 95, 108, 10, true),
		m(time.February, 108, 120, 102, 115, 20, true),
		m(time.March, 115, 118, 105, 112, 30, true),
		m(time.April, 112, 125, 110, 122, 15, true),
		m(time.May, 122, 130, 118, 128, 25, false), // bulan ke-2 running -> Q2 belum close
	}
	q := aggregateQuarterly(monthly, 10)
	if len(q) != 2 {
		t.Fatalf("harus 2 kuartal, got %d", len(q))
	}
	if q[0].Open != 100 || q[0].High != 120 || q[0].Low != 95 || q[0].Close != 112 || q[0].Volume != 60 || !q[0].IsClosed {
		t.Fatalf("Q1 salah: %+v", q[0])
	}
	if q[1].Open != 112 || q[1].High != 130 || q[1].Low != 110 || q[1].Close != 128 || q[1].Volume != 40 || q[1].IsClosed {
		t.Fatalf("Q2 salah: %+v", q[1])
	}
	// Bulan ke-3 masih running -> tetap IsClosed=false.
	q3run := aggregateQuarterly(append(monthly[:4], m(time.May, 122, 130, 118, 128, 25, true), m(time.June, 128, 135, 120, 132, 12, false)), 10)
	if q3run[1].IsClosed {
		t.Fatal("kuartal dengan bulan ke-3 running tidak boleh closed")
	}
	// Limit memotong kuartal paling awal.
	if trimmed := aggregateQuarterly(monthly, 1); len(trimmed) != 1 || trimmed[0].Timestamp.Month() != time.April {
		t.Fatalf("trim limit=1 salah: %+v", trimmed)
	}
}
