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
		{"US30", "ICMARKETS", "ICMARKETS:US30"},
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

// TestAliasSimbolPerFeed menutup kasus yang membuat candle tidak pernah masuk:
// simbol yang tidak dikenal feed dijawab TradingView dengan "invalid symbol",
// bukan dengan bar kosong.
//
// Setiap pasangan sudah diverifikasi langsung ke feed TradingView.
func TestAliasSimbolPerFeed(t *testing.T) {
	p := NewTradingViewProvider()

	cases := []struct {
		symbol string
		feed   string
		want   string
	}{
		// FOREX.com memakai NAS100 dan SPX500.
		{"USTEC", "FOREXCOM", "FOREXCOM:NAS100"},
		{"US500", "FOREXCOM", "FOREXCOM:SPX500"},
		// IC Markets memakai USTEC/US500, tapi DE40 dan XTIUSD.
		{"USTEC", "ICMARKETS", "ICMARKETS:USTEC"},
		{"US500", "ICMARKETS", "ICMARKETS:US500"},
		{"GER40", "ICMARKETS", "ICMARKETS:DE40"},
		{"USOIL", "ICMARKETS", "ICMARKETS:XTIUSD"},
		// Sama di kedua feed.
		{"GER40", "FOREXCOM", "FOREXCOM:GER40"},
		{"USOIL", "FOREXCOM", "FOREXCOM:USOIL"},
		// Kripto hanya memakai pasangan USDT di Binance.
		{"BTCUSD", "BINANCE", "BINANCE:BTCUSDT"},
		{"BTCUSD", "ICMARKETS", "ICMARKETS:BTCUSD"},
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
