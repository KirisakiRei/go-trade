package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MatchTraderBroker struct {
	token      string
	baseURL    string
	httpClient *http.Client
	login      string
	mu         sync.RWMutex
	specs      map[string]*SymbolSpec
}

func NewMatchTraderBroker(token string) *MatchTraderBroker {
	if token == "" {
		token = os.Getenv("MATCHTRADER_TOKEN")
	}
	return &MatchTraderBroker{
		token:   token,
		baseURL: "https://trader-api.match-trader.com/v1",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		login: "1089632",
		specs: make(map[string]*SymbolSpec),
	}
}

func (b *MatchTraderBroker) Name() string {
	return "Match-Trader (Trader API REST)"
}

func (b *MatchTraderBroker) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/account-summary", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Match-Trader API error (%d): %s", resp.StatusCode, string(body))
	}

	var sum struct {
		Balance    float64 `json:"balance"`
		Equity     float64 `json:"equity"`
		Margin     float64 `json:"margin"`
		FreeFunds  float64 `json:"freeFunds"`
		MarginLevel float64 `json:"marginLevel"`
		Profit     float64 `json:"profit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sum); err != nil {
		return nil, err
	}

	return &AccountInfo{
		BrokerName:    "Match-Trader Demo",
		AccountNumber: b.login,
		Currency:      "USD",
		Balance:       sum.Balance,
		Equity:        sum.Equity,
		MarginUsed:    sum.Margin,
		FreeMargin:    sum.FreeFunds,
	}, nil
}

func (b *MatchTraderBroker) fetchSymbolsIfNeeded(ctx context.Context) error {
	b.mu.RLock()
	if len(b.specs) > 0 {
		b.mu.RUnlock()
		return nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.specs) > 0 {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/symbols", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Match-Trader symbols error (%d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Symbols []struct {
			Symbol         string  `json:"symbol"`
			Type           string  `json:"type"`
			Multiplier     float64 `json:"multiplier"`
			ContractSize   float64 `json:"contractSize"`
			VolumeStep     float64 `json:"volumeStep"`
			VolumeMin      float64 `json:"volumeMin"`
			VolumeMax      float64 `json:"volumeMax"`
			PricePrecision int     `json:"pricePrecision"`
		} `json:"symbols"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	specs := make(map[string]*SymbolSpec)
	for _, s := range data.Symbols {
		cs := s.ContractSize
		if cs <= 0 {
			cs = 1.0
		}
		pointVal := 1.0
		if s.Multiplier > 0 {
			pointVal = s.Multiplier * cs
		} else {
			pointVal = cs
		}

		step := s.VolumeStep
		if step <= 0 {
			step = 0.01
		}
		vmin := s.VolumeMin
		if vmin <= 0 {
			vmin = 0.01
		}
		vmax := s.VolumeMax
		if vmax <= 0 {
			vmax = 100.0
		}

		spec := &SymbolSpec{
			Symbol:         s.Symbol,
			PointValue:     pointVal,
			VolumeMin:      vmin,
			VolumeStep:     step,
			VolumeMax:      vmax,
			PricePrecision: s.PricePrecision,
		}
		specs[s.Symbol] = spec
	}
	b.specs = specs
	return nil
}

func (b *MatchTraderBroker) GetSymbolSpec(ctx context.Context, symbol string) (*SymbolSpec, error) {
	mtrSymbol := b.MapSymbol(symbol)
	_ = b.fetchSymbolsIfNeeded(ctx)

	b.mu.RLock()
	spec, ok := b.specs[mtrSymbol]
	b.mu.RUnlock()

	if ok && spec != nil {
		return spec, nil
	}

	return FallbackSymbolSpec(mtrSymbol), nil
}

func (b *MatchTraderBroker) MapSymbol(sym string) string {
	s := strings.ToUpper(strings.TrimSpace(sym))
	s = strings.ReplaceAll(s, "/", "")
	switch s {
	case "USTEC", "NAS100", "NDX":
		return "US100"
	case "GOLD":
		return "XAUUSD"
	default:
		return s
	}
}

func (b *MatchTraderBroker) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	mtrSymbol := b.MapSymbol(req.Symbol)
	side := "BUY"
	if strings.ToUpper(req.Direction) == "SELL" {
		side = "SELL"
	}

	// Payload according to Match-Trader Trader API POST /v1/positions
	bodyMap := map[string]any{
		"symbol":  mtrSymbol,
		"side":    side,
		"volume":  req.LotSize,
		"comment": req.Comment,
	}

	if req.StopLoss > 0 {
		digits := 2
		if strings.Contains(mtrSymbol, "EUR") || strings.Contains(mtrSymbol, "GBP") || strings.Contains(mtrSymbol, "AUD") {
			digits = 5
		}
		pow := math.Pow10(digits)
		bodyMap["slPrice"] = math.Round(req.StopLoss*pow) / pow
	}
	if req.TakeProfit > 0 {
		digits := 2
		if strings.Contains(mtrSymbol, "EUR") || strings.Contains(mtrSymbol, "GBP") || strings.Contains(mtrSymbol, "AUD") {
			digits = 5
		}
		pow := math.Pow10(digits)
		bodyMap["tpPrice"] = math.Round(req.TakeProfit*pow) / pow
	}

	jsonBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/positions", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+b.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Match-Trader order error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var resObj struct {
		PositionID string `json:"positionId"`
	}
	_ = json.Unmarshal(bodyBytes, &resObj)

	return &OrderResponse{
		TicketNumber:  resObj.PositionID,
		Symbol:        req.Symbol,
		Direction:     req.Direction,
		ExecutedPrice: req.OpenPrice,
		StopLoss:      req.StopLoss,
		TakeProfit:    req.TakeProfit,
		LotSize:       req.LotSize,
		ExecutedAt:    time.Now().UTC(),
		Status:        "FILLED",
	}, nil
}

func (b *MatchTraderBroker) CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", b.baseURL+"/positions/"+ticket, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Match-Trader close error (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (b *MatchTraderBroker) GetOpenPositions(ctx context.Context) ([]PositionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/positions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Match-Trader get positions error (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Positions []struct {
			ID           string  `json:"id"`
			Symbol       string  `json:"symbol"`
			Side         string  `json:"side"`
			Volume       float64 `json:"volume"`
			OpenPrice    float64 `json:"openPrice"`
			CurrentPrice float64 `json:"currentPrice"`
			SLPrice      float64 `json:"slPrice"`
			TPPrice      float64 `json:"tpPrice"`
			Profit       float64 `json:"profit"`
			OpenTime     int64   `json:"openTime"`
		} `json:"positions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var list []PositionInfo
	for _, p := range res.Positions {
		list = append(list, PositionInfo{
			TicketNumber: p.ID,
			Symbol:       p.Symbol,
			Direction:    p.Side,
			OpenPrice:    p.OpenPrice,
			CurrentPrice: p.CurrentPrice,
			StopLoss:     p.SLPrice,
			TakeProfit:   p.TPPrice,
			LotSize:      p.Volume,
			FloatingPnL:  p.Profit,
			OpenedAt:     time.UnixMilli(p.OpenTime).UTC(),
		})
	}
	return list, nil
}

type MatchTraderClosedPosition struct {
	ID          string  `json:"id"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	Volume      float64 `json:"volume"`
	OpenPrice   float64 `json:"openPrice"`
	ClosePrice  float64 `json:"closePrice"`
	Profit      float64 `json:"profit"`
	CloseReason string  `json:"closeReason"`
	OpenTime    int64   `json:"openTime"`
	CloseTime   int64   `json:"closeTime"`
}

func (b *MatchTraderBroker) GetClosedPositions(ctx context.Context, fromTime, toTime time.Time) ([]MatchTraderClosedPosition, error) {
	nowMs := toTime.UnixMilli()
	fromMs := fromTime.UnixMilli()
	if fromMs <= 0 {
		fromMs = nowMs - (7 * 24 * 3600 * 1000)
	}
	url := fmt.Sprintf("%s/closed-positions?from=%d&to=%d", b.baseURL, fromMs, nowMs)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Match-Trader closed positions error (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		ClosedPositions []MatchTraderClosedPosition `json:"closedPositions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.ClosedPositions, nil
}

// Ensure interface compatibility
var _ Broker = (*MatchTraderBroker)(nil)
var _ = strconv.Itoa
