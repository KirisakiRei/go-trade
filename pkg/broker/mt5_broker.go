package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type MT5GatewayBroker struct {
	baseURL string
	client  *http.Client
}

func NewMT5GatewayBroker(baseURL string) *MT5GatewayBroker {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:5050"
	}
	return &MT5GatewayBroker{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (b *MT5GatewayBroker) Name() string {
	return "MetaQuotes-Demo (Live MT5)"
}

func (b *MT5GatewayBroker) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	url := b.baseURL + "/account"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach MT5 gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mt5 gateway returned status %d", resp.StatusCode)
	}

	var acc AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&acc); err != nil {
		return nil, err
	}

	return &acc, nil
}

func (b *MT5GatewayBroker) GetSymbolSpec(ctx context.Context, symbol string) (*SymbolSpec, error) {
	return FallbackSymbolSpec(symbol), nil
}

func (b *MT5GatewayBroker) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	url := b.baseURL + "/order"
	payload := map[string]any{
		"symbol":    req.Symbol,
		"direction": req.Direction,
		"volume":    req.LotSize,
		"sl":        req.StopLoss,
		"tp":        req.TakeProfit,
		"comment":   req.Comment,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send order to MT5 gateway: %w", err)
	}
	defer resp.Body.Close()

	var res struct {
		Success       bool    `json:"success"`
		Ticket        string  `json:"ticket"`
		Symbol        string  `json:"symbol"`
		Direction     string  `json:"direction"`
		ExecutedPrice float64 `json:"executed_price"`
		SL            float64 `json:"sl"`
		TP            float64 `json:"tp"`
		Volume        float64 `json:"volume"`
		Error         string  `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success {
		return nil, fmt.Errorf("order rejected by MT5 server: %s", res.Error)
	}

	return &OrderResponse{
		TicketNumber:  res.Ticket,
		Symbol:        res.Symbol,
		Direction:     res.Direction,
		ExecutedPrice: res.ExecutedPrice,
		StopLoss:      res.SL,
		TakeProfit:    res.TP,
		LotSize:       res.Volume,
		ExecutedAt:    time.Now(),
		Status:        "FILLED",
	}, nil
}

func (b *MT5GatewayBroker) CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error {
	// Not mandatory to implement immediately for webclient unless needed
	return nil
}

func (b *MT5GatewayBroker) GetOpenPositions(ctx context.Context) ([]PositionInfo, error) {
	url := b.baseURL + "/positions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw []struct {
		Ticket       string  `json:"ticket"`
		Symbol       string  `json:"symbol"`
		Direction    string  `json:"direction"`
		OpenPrice    float64 `json:"open_price"`
		CurrentPrice float64 `json:"current_price"`
		SL           float64 `json:"sl"`
		TP           float64 `json:"tp"`
		Volume       float64 `json:"volume"`
		Profit       float64 `json:"profit"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	positions := make([]PositionInfo, len(raw))
	for i, r := range raw {
		positions[i] = PositionInfo{
			TicketNumber: r.Ticket,
			Symbol:       r.Symbol,
			Direction:    r.Direction,
			OpenPrice:    r.OpenPrice,
			CurrentPrice: r.CurrentPrice,
			StopLoss:     r.SL,
			TakeProfit:   r.TP,
			LotSize:      r.Volume,
			FloatingPnL:  r.Profit,
			OpenedAt:     time.Now(),
		}
	}

	return positions, nil
}
