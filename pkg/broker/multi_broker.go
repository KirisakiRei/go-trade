package broker

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type MultiBrokerManager struct {
	brokers []Broker
}

func NewMultiBrokerManager(brokers ...Broker) *MultiBrokerManager {
	var valid []Broker
	for _, b := range brokers {
		if b != nil {
			valid = append(valid, b)
		}
	}
	return &MultiBrokerManager{brokers: valid}
}

func (m *MultiBrokerManager) Name() string {
	var names []string
	for _, b := range m.brokers {
		names = append(names, b.Name())
	}
	return fmt.Sprintf("MultiBrokerManager [%s]", strings.Join(names, " + "))
}

func (m *MultiBrokerManager) GetBrokers() []Broker {
	return m.brokers
}

func (m *MultiBrokerManager) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	if len(m.brokers) == 0 {
		return nil, fmt.Errorf("no brokers configured")
	}
	// Return the first broker (primary) account info
	return m.brokers[0].GetAccountInfo(ctx)
}

func (m *MultiBrokerManager) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	if len(m.brokers) == 0 {
		return nil, fmt.Errorf("no brokers configured for multi-execution")
	}

	var tickets []string
	var firstResp *OrderResponse
	var errs []string

	for _, b := range m.brokers {
		// Calculate dynamic 1% live equity risk specifically for THIS broker
		acc, err := b.GetAccountInfo(ctx)
		lot := req.LotSize
		if err == nil && acc != nil && acc.Equity > 0 {
			riskDollars := acc.Equity * 0.01 // strict 1.0%
			slDist := req.OpenPrice - req.StopLoss
			if slDist < 0 {
				slDist = -slDist
			}
			if slDist > 0 {
				sym := strings.ToUpper(strings.ReplaceAll(req.Symbol, "/", ""))
				switch {
				case strings.Contains(sym, "XAU") || strings.Contains(sym, "GOLD"):
					lot = riskDollars / (slDist * 100.0)
				case strings.Contains(sym, "EUR") || strings.Contains(sym, "GBP") || strings.Contains(sym, "AUD"):
					lot = riskDollars / (slDist * 100000.0)
				case strings.Contains(sym, "USTEC") || strings.Contains(sym, "NAS100") || strings.Contains(sym, "US100"):
					lot = riskDollars / (slDist * 1.0)
				}
				// Clamp
				if lot < 0.01 {
					lot = 0.01
				}
				if lot > 20.0 {
					lot = 20.0
				}
				// Round
				if strings.Contains(sym, "USTEC") || strings.Contains(sym, "NAS100") || strings.Contains(sym, "US100") {
					lot = float64(int(lot*10)) / 10.0 // 0.10 step
					if lot < 0.10 {
						lot = 0.10
					}
				} else {
					lot = float64(int(lot*100)) / 100.0 // 0.01 step
				}
			}
		}

		brokerReq := req
		brokerReq.LotSize = lot

		resp, err := b.ExecuteOrder(ctx, brokerReq)
		if err != nil {
			errs = append(errs, fmt.Sprintf("[%s]: %v", b.Name(), err))
		} else {
			tickets = append(tickets, fmt.Sprintf("%s(#%s, %.2f lot)", b.Name(), resp.TicketNumber, resp.LotSize))
			if firstResp == nil {
				firstResp = resp
			}
		}
	}

	if firstResp == nil {
		return nil, fmt.Errorf("all brokers failed execution: %s", strings.Join(errs, " | "))
	}

	firstResp.TicketNumber = strings.Join(tickets, " & ")
	return firstResp, nil
}

func (m *MultiBrokerManager) CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error {
	for _, b := range m.brokers {
		_ = b.CloseOrder(ctx, ticket, exitPrice, reason)
	}
	return nil
}

func (m *MultiBrokerManager) GetOpenPositions(ctx context.Context) ([]PositionInfo, error) {
	var allPos []PositionInfo
	for _, b := range m.brokers {
		pos, err := b.GetOpenPositions(ctx)
		if err == nil {
			allPos = append(allPos, pos...)
		}
	}
	return allPos, nil
}

var _ Broker = (*MultiBrokerManager)(nil)
var _ = time.Now
