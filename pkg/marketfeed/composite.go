package marketfeed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"ila-engine/pkg/indicators"
)

// MultiFeedProvider memilih sumber candle yang tepat untuk setiap simbol.
//
// Sejak 2026-11 pasangan mata uang, logam, dan indeks hanya diambil dari
// TradingView FOREX.com. Binance hanya dipakai untuk instrumen kripto yang
// memang ada di sana. Tidak ada lagi fallback ke sumber yang memakai instrumen
// berbeda: candle dari instrumen lain akan menghasilkan setup yang tampak sah
// padahal harganya bukan harga simbol yang diminta.
//
// Sejak multi-broker, admin platform boleh menetapkan feed per pair (mis.
// NAS100 dari ICMARKETS). Penetapan itu dikirim Runner lewat Subscribe dan
// disimpan di sini sebagai penimpa (override) per simbol.
type MultiFeedProvider struct {
	tv      *TradingViewProvider
	binance *BinanceProvider

	mu    sync.RWMutex
	feeds map[string]string
}

func NewMultiFeedProvider() *MultiFeedProvider {
	return &MultiFeedProvider{
		tv:      NewTradingViewProvider(),
		binance: NewBinanceProvider(),
		feeds:   make(map[string]string),
	}
}

func (p *MultiFeedProvider) Name() string {
	return "multi_broker_feed (TradingView FOREX.com + Binance kripto)"
}

// SetSymbolFeed menetapkan feed pilihan admin untuk satu simbol.
// Nilai kosong menghapus penimpa sehingga simbol kembali ke pemetaan bawaan.
func (p *MultiFeedProvider) SetSymbolFeed(symbol, feed string) {
	sym := normalizeSymbol(symbol)
	if sym == "" {
		return
	}
	normalized := NormalizeFeed(feed)

	p.mu.Lock()
	defer p.mu.Unlock()
	if normalized == "" {
		delete(p.feeds, sym)
		return
	}
	p.feeds[sym] = normalized
}

// SymbolFeed mengembalikan feed pilihan admin untuk satu simbol, atau string
// kosong bila simbol itu memakai pemetaan bawaan.
func (p *MultiFeedProvider) SymbolFeed(symbol string) string {
	sym := normalizeSymbol(symbol)
	if sym == "" {
		return ""
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.feeds[sym]
}

// FeedFor mengembalikan feed efektif untuk satu simbol: pilihan admin bila ada,
// jika tidak pemetaan bawaan (BINANCE untuk kripto, FOREXCOM untuk sisanya).
//
// Nilai ini ikut dikirim pada setiap sinyal agar dashboard bisa menampilkan
// chart dari feed yang sama persis.
func (p *MultiFeedProvider) FeedFor(symbol string) string {
	sym := normalizeSymbol(symbol)
	if feed := p.SymbolFeed(sym); feed != "" {
		return feed
	}
	if isCryptoSymbol(sym) {
		return "BINANCE"
	}
	return "FOREXCOM"
}

func (p *MultiFeedProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	sym := normalizeSymbol(symbol)

	minExpected := 50
	if limit > 0 && limit < 50 {
		minExpected = limit
	}

	// Feed pilihan admin diprioritaskan. Binance ditangani terpisah karena
	// protokolnya berbeda dengan TradingView.
	if feed := p.SymbolFeed(sym); feed == "BINANCE" {
		return p.fetchBinance(ctx, sym, timeframe, limit, minExpected)
	}

	// 1. Sumber utama: TradingView, memakai feed pilihan admin bila ada.
	feed := p.SymbolFeed(sym)
	candles, err := p.tv.FetchCandlesWithFeed(ctx, sym, feed, timeframe, limit)
	if err == nil && len(candles) >= minExpected {
		return candles, nil
	}
	tvErr := err
	if tvErr == nil {
		tvErr = fmt.Errorf("candle %s %s tidak lengkap: %d dari %d", sym, timeframe, len(candles), minExpected)
	}

	// Instrumen non-kripto tidak punya padanan di Binance. Mengembalikan error
	// lebih benar daripada mengevaluasi candle instrumen lain.
	if !isCryptoSymbol(sym) {
		return nil, fmt.Errorf("candle %s %s gagal diambil: %w", sym, timeframe, tvErr)
	}

	// 2. Kripto: Binance sebagai cadangan.
	candles, err = p.fetchBinance(ctx, sym, timeframe, limit, minExpected)
	if err == nil {
		return candles, nil
	}
	return nil, errors.Join(tvErr, err)
}

func (p *MultiFeedProvider) fetchBinance(ctx context.Context, sym, timeframe string, limit, minExpected int) ([]indicators.Candle, error) {
	candles, err := p.binance.FetchCandles(ctx, sym, timeframe, limit)
	if err == nil && len(candles) >= minExpected {
		return candles, nil
	}
	if err == nil {
		err = fmt.Errorf("candle Binance tidak lengkap: %d dari %d", len(candles), minExpected)
	}
	return nil, err
}

// NormalizeFeed menyeragamkan nama feed menjadi huruf besar tanpa spasi.
func NormalizeFeed(feed string) string {
	return strings.ToUpper(strings.TrimSpace(feed))
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(symbol), "/", ""))
}

// isCryptoSymbol menandai instrumen yang benar-benar tersedia di Binance.
// Daftar ini sengaja eksplisit: menebak dari akhiran "USD" akan membuat
// EURUSD dianggap kripto.
func isCryptoSymbol(sym string) bool {
	switch sym {
	case "BTCUSD", "BTCUSDT", "ETHUSD", "ETHUSDT",
		"BNBUSD", "BNBUSDT", "SOLUSD", "SOLUSDT",
		"XRPUSD", "XRPUSDT", "ADAUSD", "ADAUSDT",
		"DOGEUSD", "DOGEUSDT", "PAXGUSD", "PAXGUSDT":
		return true
	default:
		return false
	}
}
