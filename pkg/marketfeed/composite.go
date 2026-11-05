package marketfeed

import (
	"context"
	"strings"

	"ila-engine/pkg/indicators"
)

type MultiFeedProvider struct {
	tv      *TradingViewProvider
	binance *BinanceProvider
	yahoo   *YahooProvider
}

func NewMultiFeedProvider() *MultiFeedProvider {
	return &MultiFeedProvider{
		tv:      NewTradingViewProvider(),
		binance: NewBinanceProvider(),
		yahoo:   NewYahooProvider(),
	}
}

func (p *MultiFeedProvider) Name() string {
	return "multi_broker_feed (TradingView FOREX.com + Binance + Yahoo)"
}

func (p *MultiFeedProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	sym := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))

	minExpected := 50
	if limit > 0 && limit < 50 {
		minExpected = limit
	}

	// 1. Primary Feed: TradingView FOREX.com
	candles, err := p.tv.FetchCandles(ctx, sym, timeframe, limit)
	if err == nil && len(candles) >= minExpected {
		return candles, nil
	}

	// 2. Secondary Fallback: Binance (for Crypto / PAXG)
	candles, err = p.binance.FetchCandles(ctx, symbol, timeframe, limit)
	if err == nil && len(candles) >= minExpected {
		return candles, nil
	}

	// 3. Tertiary Fallback: Yahoo Finance
	return p.yahoo.FetchCandles(ctx, symbol, timeframe, limit)
}
