package marketfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
)

type BinanceProvider struct {
	client *http.Client
}

func NewBinanceProvider() *BinanceProvider {
	return &BinanceProvider{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *BinanceProvider) Name() string {
	return "binance_paxg"
}

func (p *BinanceProvider) mapTimeframe(tf string) string {
	tfUpper := strings.ToUpper(tf)
	switch tfUpper {
	case "M1":
		return "1m"
	case "M5":
		return "5m"
	case "M15":
		return "15m"
	case "M30":
		return "30m"
	case "H1":
		return "1h"
	case "H4":
		return "4h"
	case "D1":
		return "1d"
	default:
		return strings.ToLower(tf)
	}
}

func (p *BinanceProvider) mapSymbol(symbol string) string {
	s := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
	if s == "XAUUSD" || s == "GOLD" {
		return "PAXGUSDT"
	}
	if strings.HasSuffix(s, "USDT") {
		return s
	}
	if strings.HasSuffix(s, "USD") {
		return strings.TrimSuffix(s, "USD") + "USDT"
	}
	return s + "USDT"
}

func (p *BinanceProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	binanceSymbol := p.mapSymbol(symbol)
	interval := p.mapTimeframe(timeframe)

	url := fmt.Sprintf("https://api.binance.com/api/v3/klines?symbol=%s&interval=%s&limit=%d", binanceSymbol, interval, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance api returned status: %d", resp.StatusCode)
	}

	var raw [][]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	nowMs := time.Now().UnixMilli()
	candles := make([]indicators.Candle, 0, len(raw))

	for _, item := range raw {
		if len(item) < 7 {
			continue
		}

		openTimeMs, ok := item[0].(float64)
		if !ok {
			continue
		}
		closeTimeMs, ok := item[6].(float64)
		if !ok {
			continue
		}

		openStr, _ := item[1].(string)
		highStr, _ := item[2].(string)
		lowStr, _ := item[3].(string)
		closeStr, _ := item[4].(string)
		volStr, _ := item[5].(string)

		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		closePrice, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volStr, 64)

		isClosed := int64(closeTimeMs) <= nowMs

		candles = append(candles, indicators.Candle{
			Timestamp: time.UnixMilli(int64(openTimeMs)),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			IsClosed:  isClosed,
		})
	}

	return ValidateCandles(candles)
}
