package broker

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

type DemoBroker struct {
	mu            sync.RWMutex
	accountNumber string
	balance       float64
	equity        float64
	positions     map[string]*PositionInfo
}

func NewDemoBroker(initialBalance float64) *DemoBroker {
	if initialBalance <= 0 {
		initialBalance = 10000.0 // $10,000 standard demo balance
	}
	return &DemoBroker{
		accountNumber: "DEMO-" + uuid.New().String()[:8],
		balance:       initialBalance,
		equity:        initialBalance,
		positions:     make(map[string]*PositionInfo),
	}
}

func (b *DemoBroker) Name() string {
	return "demo_metatrader_simulator"
}

func (b *DemoBroker) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return &AccountInfo{
		BrokerName:    b.Name(),
		AccountNumber: b.accountNumber,
		Currency:      "USD",
		Balance:       math.Round(b.balance*100) / 100,
		Equity:        math.Round(b.equity*100) / 100,
		MarginUsed:    0,
		FreeMargin:    math.Round(b.equity*100) / 100,
	}, nil
}

func (b *DemoBroker) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ticket := fmt.Sprintf("MT5-%d", time.Now().UnixNano()%1000000000)
	now := time.Now()

	pos := &PositionInfo{
		TicketNumber: ticket,
		Symbol:       req.Symbol,
		Direction:    req.Direction,
		OpenPrice:    req.OpenPrice,
		CurrentPrice: req.OpenPrice,
		StopLoss:     req.StopLoss,
		TakeProfit:   req.TakeProfit,
		LotSize:      req.LotSize,
		FloatingPnL:  0,
		OpenedAt:     now,
	}

	b.positions[ticket] = pos

	return &OrderResponse{
		TicketNumber:  ticket,
		Symbol:        req.Symbol,
		Direction:     req.Direction,
		ExecutedPrice: req.OpenPrice,
		StopLoss:      req.StopLoss,
		TakeProfit:    req.TakeProfit,
		LotSize:       req.LotSize,
		ExecutedAt:    now,
		Status:        "FILLED",
	}, nil
}

func (b *DemoBroker) CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	pos, exists := b.positions[ticket]
	if !exists {
		return fmt.Errorf("position ticket %s not found", ticket)
	}

	// Calculate PnL based on instrument
	// Gold: 1 Lot * $100 per $1.00 move
	// Forex: 1 Lot * $100,000 * deltaPrice
	var pnl float64
	diff := exitPrice - pos.OpenPrice
	if pos.Direction == "SELL" {
		diff = pos.OpenPrice - exitPrice
	}

	isGold := pos.Symbol == "XAUUSD" || pos.Symbol == "GOLD"
	if isGold {
		pnl = diff * 100.0 * pos.LotSize
	} else {
		pnl = diff * 100000.0 * pos.LotSize
	}

	b.balance += pnl
	b.equity = b.balance
	delete(b.positions, ticket)

	return nil
}

func (b *DemoBroker) GetOpenPositions(ctx context.Context) ([]PositionInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	res := make([]PositionInfo, 0, len(b.positions))
	for _, p := range b.positions {
		res = append(res, *p)
	}
	return res, nil
}
