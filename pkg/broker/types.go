package broker

import (
	"context"
	"strings"
	"time"
)

type AccountInfo struct {
	BrokerName    string  `json:"broker_name"`
	AccountNumber string  `json:"account_number"`
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
	Equity        float64 `json:"equity"`
	MarginUsed    float64 `json:"margin_used"`
	FreeMargin    float64 `json:"free_margin"`
}

// SymbolSpec represents contract specifications for an instrument on a specific broker.
// PointValue defines how many USD are won/lost per 1.00 full price move per 1.00 lot:
// - Forex Standard: PointValue = 100,000 (1 pip 0.00010 = $10/lot)
// - Gold Standard: PointValue = 100 (1 pip 0.10 = $10/lot, $1 move = $100/lot)
// - Match-Trader US100: Multiplier 20 * ContractSize 1 -> PointValue = 20.0 ($20 per 1.00 index point/lot)
// - cTrader USTEC: 1 CFD contract -> PointValue = 1.0 ($1 per 1.00 index point/lot)
type SymbolSpec struct {
	Symbol         string  `json:"symbol"`
	PointValue     float64 `json:"point_value"`     // USD per 1.00 price move per 1.00 lot
	VolumeMin      float64 `json:"volume_min"`      // Minimum allowed volume
	VolumeStep     float64 `json:"volume_step"`     // Minimum volume increment
	VolumeMax      float64 `json:"volume_max"`      // Maximum allowed volume
	PricePrecision int     `json:"price_precision"` // Digits after decimal
}

// FallbackSymbolSpec provides safe standard specifications if broker dynamic lookup fails.
func FallbackSymbolSpec(symbol string) *SymbolSpec {
	upper := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
	switch {
	case strings.Contains(upper, "XAU") || strings.Contains(upper, "GOLD") || strings.Contains(upper, "PAXG"):
		return &SymbolSpec{
			Symbol:         symbol,
			PointValue:     100.0,
			VolumeMin:      0.01,
			VolumeStep:     0.01,
			VolumeMax:      20.0,
			PricePrecision: 2,
		}
	case strings.Contains(upper, "US100") || strings.Contains(upper, "NAS100") || strings.Contains(upper, "USTEC") || strings.Contains(upper, "NDX"):
		return &SymbolSpec{
			Symbol:         symbol,
			PointValue:     20.0, // Default to Match-Trader 20x multiplier
			VolumeMin:      0.01,
			VolumeStep:     0.01,
			VolumeMax:      20.0,
			PricePrecision: 2,
		}
	case strings.Contains(upper, "US500") || strings.Contains(upper, "SPX"):
		return &SymbolSpec{
			Symbol:         symbol,
			PointValue:     50.0,
			VolumeMin:      0.01,
			VolumeStep:     0.01,
			VolumeMax:      20.0,
			PricePrecision: 2,
		}
	case strings.Contains(upper, "US30") || strings.Contains(upper, "DJ"):
		return &SymbolSpec{
			Symbol:         symbol,
			PointValue:     5.0,
			VolumeMin:      0.01,
			VolumeStep:     0.01,
			VolumeMax:      20.0,
			PricePrecision: 2,
		}
	default:
		// Standard Forex majors
		return &SymbolSpec{
			Symbol:         symbol,
			PointValue:     100000.0,
			VolumeMin:      0.01,
			VolumeStep:     0.01,
			VolumeMax:      20.0,
			PricePrecision: 5,
		}
	}
}

type OrderRequest struct {
	Symbol     string  `json:"symbol"`
	Direction  string  `json:"direction"` // BUY or SELL
	OpenPrice  float64 `json:"open_price"`
	StopLoss   float64 `json:"stop_loss"`
	TakeProfit float64 `json:"take_profit"`
	LotSize    float64 `json:"lot_size"`
	Comment    string  `json:"comment"`
	SetupID    string  `json:"setup_id"`
}

type OrderResponse struct {
	TicketNumber string    `json:"ticket_number"`
	Symbol       string    `json:"symbol"`
	Direction    string    `json:"direction"`
	ExecutedPrice float64  `json:"executed_price"`
	StopLoss     float64   `json:"stop_loss"`
	TakeProfit   float64   `json:"take_profit"`
	LotSize      float64   `json:"lot_size"`
	ExecutedAt   time.Time `json:"executed_at"`
	Status       string    `json:"status"`
}

type PositionInfo struct {
	TicketNumber string    `json:"ticket_number"`
	Symbol       string    `json:"symbol"`
	Direction    string    `json:"direction"`
	OpenPrice    float64   `json:"open_price"`
	CurrentPrice float64   `json:"current_price"`
	StopLoss     float64   `json:"stop_loss"`
	TakeProfit   float64   `json:"take_profit"`
	LotSize      float64   `json:"lot_size"`
	FloatingPnL  float64   `json:"floating_pnl"`
	OpenedAt     time.Time `json:"opened_at"`
}

type Broker interface {
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	GetSymbolSpec(ctx context.Context, symbol string) (*SymbolSpec, error)
	ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error)
	CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error
	GetOpenPositions(ctx context.Context) ([]PositionInfo, error)
	Name() string
}
