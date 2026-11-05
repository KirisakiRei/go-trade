package marketfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
)

type YahooProvider struct {
	client *http.Client
}

func NewYahooProvider() *YahooProvider {
	return &YahooProvider{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *YahooProvider) Name() string {
	return "yahoo_finance"
}

func (p *YahooProvider) mapSymbol(symbol string) string {
	s := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
	switch s {
	case "XAGUSD", "SILVER", "XAG":
		return "SI=F"
	case "XAUUSD", "GOLD", "XAU":
		return "GC=F"
	case "EURUSD":
		return "EURUSD=X"
	case "GBPUSD":
		return "GBPUSD=X"
	default:
		return s
	}
}

func (p *YahooProvider) mapInterval(tf string) (string, string) {
	upper := strings.ToUpper(tf)
	switch upper {
	case "M15", "15M":
		return "15m", "5d"
	case "M30", "30M":
		return "30m", "10d"
	case "H1", "1H":
		return "60m", "30d"
	case "H4", "4H":
		return "1d", "90d" // Yahoo 60m aggregated or 1d
	case "D1", "1D":
		return "1d", "180d"
	default:
		return "15m", "5d"
	}
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (p *YahooProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	sym := p.mapSymbol(symbol)
	interval, rangeStr := p.mapInterval(timeframe)

	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s", sym, interval, rangeStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo returned status %d", resp.StatusCode)
	}

	var data yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode yahoo response: %w", err)
	}

	if data.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", data.Chart.Error.Description)
	}

	if len(data.Chart.Result) == 0 || len(data.Chart.Result[0].Timestamp) == 0 {
		return nil, fmt.Errorf("no candle data returned from yahoo")
	}

	res := data.Chart.Result[0]
	quotes := res.Indicators.Quote[0]
	n := len(res.Timestamp)

	candles := make([]indicators.Candle, 0, n)
	now := time.Now()

	for i := 0; i < n; i++ {
		if quotes.Open[i] == nil || quotes.High[i] == nil || quotes.Low[i] == nil || quotes.Close[i] == nil {
			continue
		}

		t := time.Unix(res.Timestamp[i], 0).UTC()
		tfDur := ParseTimeframeDuration(timeframe)
		isClosed := !now.Before(t.Add(tfDur))

		vol := 0.0
		if quotes.Volume != nil && quotes.Volume[i] != nil {
			vol = *quotes.Volume[i]
		}

		candles = append(candles, indicators.Candle{
			Timestamp: t,
			Open:      *quotes.Open[i],
			High:      *quotes.High[i],
			Low:       *quotes.Low[i],
			Close:     *quotes.Close[i],
			Volume:    vol,
			IsClosed:  isClosed,
		})
	}

	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}

	return ValidateCandles(candles)
}
