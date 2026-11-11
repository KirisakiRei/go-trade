package marketfeed

import "testing"

// TestMapSymbolWithFeed memastikan feed pilihan admin dipakai apa adanya, dan
// feed kosong tetap memakai pemetaan bawaan.
func TestMapSymbolWithFeed(t *testing.T) {
	p := NewTradingViewProvider()

	cases := []struct {
		symbol string
		feed   string
		want   string
	}{
		{"NAS100", "ICMARKETS", "ICMARKETS:NAS100"},
		{"xauusd", "icmarkets", "ICMARKETS:XAUUSD"},
		{"EUR/USD", "OANDA", "OANDA:EURUSD"},
		{"XAUUSD", "", "FOREXCOM:XAUUSD"},
		{"USTEC", "", "FOREXCOM:NAS100"},
	}

	for _, c := range cases {
		if got := p.MapSymbolWithFeed(c.symbol, c.feed); got != c.want {
			t.Errorf("MapSymbolWithFeed(%q, %q) = %q, ingin %q", c.symbol, c.feed, got, c.want)
		}
	}
}

// TestMultiFeedSymbolFeed memastikan penimpa feed per simbol tersimpan, bisa
// dihapus, dan FeedFor melaporkan feed efektifnya.
func TestMultiFeedSymbolFeed(t *testing.T) {
	p := NewMultiFeedProvider()

	// Bawaan: kripto ke Binance, sisanya FOREXCOM.
	if got := p.FeedFor("BTCUSD"); got != "BINANCE" {
		t.Errorf("FeedFor(BTCUSD) = %q, ingin BINANCE", got)
	}
	if got := p.FeedFor("NAS100"); got != "FOREXCOM" {
		t.Errorf("FeedFor(NAS100) = %q, ingin FOREXCOM", got)
	}

	p.SetSymbolFeed("nas100", "icmarkets")
	if got := p.SymbolFeed("NAS100"); got != "ICMARKETS" {
		t.Errorf("SymbolFeed(NAS100) = %q, ingin ICMARKETS", got)
	}
	if got := p.FeedFor("NAS100"); got != "ICMARKETS" {
		t.Errorf("FeedFor(NAS100) = %q, ingin ICMARKETS", got)
	}

	// Penimpa Binance memaksa rute kripto tetap ke Binance.
	p.SetSymbolFeed("BTCUSD", "BINANCE")
	if got := p.FeedFor("BTCUSD"); got != "BINANCE" {
		t.Errorf("FeedFor(BTCUSD) = %q, ingin BINANCE", got)
	}

	// Nilai kosong mengembalikan simbol ke pemetaan bawaan.
	p.SetSymbolFeed("NAS100", "")
	if got := p.FeedFor("NAS100"); got != "FOREXCOM" {
		t.Errorf("FeedFor(NAS100) setelah hapus = %q, ingin FOREXCOM", got)
	}
}

// TestNormalizeFeed memastikan nama feed diseragamkan sebelum dipakai.
func TestNormalizeFeed(t *testing.T) {
	if got := NormalizeFeed("  icmarkets "); got != "ICMARKETS" {
		t.Errorf("NormalizeFeed = %q, ingin ICMARKETS", got)
	}
	if got := NormalizeFeed(""); got != "" {
		t.Errorf("NormalizeFeed kosong = %q, ingin kosong", got)
	}
}
