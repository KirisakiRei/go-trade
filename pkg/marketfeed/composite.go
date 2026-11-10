package marketfeed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ila-engine/pkg/indicators"
)

// MultiFeedProvider memilih sumber candle yang tepat untuk setiap simbol.
//
// Sejak 2026-11 pasangan mata uang, logam, dan indeks hanya diambil dari
// TradingView FOREX.com. Binance hanya dipakai untuk instrumen kripto yang
// memang ada di sana. Tidak ada lagi fallback ke sumber yang memakai instrumen
// berbeda: candle dari instrumen lain akan menghasilkan setup yang tampak sah
// padahal harganya bukan harga simbol yang diminta.
type MultiFeedProvider struct {
	tv      *TradingViewProvider
	binance *BinanceProvider
}

func NewMultiFeedProvider() *MultiFeedProvider {
	return &MultiFeedProvider{
		tv:      NewTradingViewProvider(),
		binance: NewBinanceProvider(),
	}
}

func (p *MultiFeedProvider) Name() string {
	return "multi_broker_feed (TradingView FOREX.com + Binance kripto)"
}

func (p *MultiFeedProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	sym := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))

	minExpected := 50
	if limit > 0 && limit < 50 {
		minExpected = limit
	}

	// 1. Sumber utama: TradingView FOREX.com.
	candles, err := p.tv.FetchCandles(ctx, sym, timeframe, limit)
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
	candles, err = p.binance.FetchCandles(ctx, sym, timeframe, limit)
	if err == nil && len(candles) >= minExpected {
		return candles, nil
	}
	if err == nil {
		err = fmt.Errorf("candle Binance tidak lengkap: %d dari %d", len(candles), minExpected)
	}
	return nil, errors.Join(tvErr, err)
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
