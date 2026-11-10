package broker

import (
	"context"
	"fmt"
	"strconv"
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
		return &AccountInfo{
			BrokerName: "Default Multi-Broker",
			Balance:    10000.0,
			Equity:     10000.0,
			FreeMargin: 10000.0,
		}, nil
	}
	var lastErr error
	for _, b := range m.brokers {
		acc, err := b.GetAccountInfo(ctx)
		if err == nil && acc != nil && acc.Equity > 0 {
			return acc, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	// Fallback safe dummy info rather than nil panic
	return &AccountInfo{
		BrokerName: "Fallback Account",
		Balance:    10000.0,
		Equity:     10000.0,
		FreeMargin: 10000.0,
	}, lastErr
}

func (m *MultiBrokerManager) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	if len(m.brokers) == 0 {
		return nil, fmt.Errorf("no brokers configured for multi-execution")
	}

	var tickets []string
	var firstResp *OrderResponse
	var errs []string

	for _, b := range m.brokers {
		// Calculate dynamic 1.0% live equity risk specifically for THIS broker
		acc, err := b.GetAccountInfo(ctx)
		lot := req.LotSize
		if err == nil && acc != nil && acc.Equity > 0 {
			riskDollars := acc.Equity * 0.01 // strict 1.0% live equity risk
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

// ExtractTicketIDs parses composite multi-broker ticket string into cTrader numeric ID and Match-Trader string ID
func ExtractTicketIDs(ticket string) (int64, string) {
	var cPosID int64
	var mtrID string

	// Extract cTrader ID (numeric after cTrader ... #\d+)
	if cIdx := strings.Index(ticket, "cTrader"); cIdx != -1 {
		sub := ticket[cIdx:]
		if hashIdx := strings.Index(sub, "#"); hashIdx != -1 {
			rest := sub[hashIdx+1:]
			end := strings.IndexAny(rest, ", )")
			if end != -1 {
				rest = rest[:end]
			}
			cPosID, _ = strconv.ParseInt(rest, 10, 64)
		}
	} else if hashIdx := strings.Index(ticket, "#"); hashIdx != -1 {
		rest := ticket[hashIdx+1:]
		end := strings.IndexAny(rest, ", )")
		if end != -1 {
			rest = rest[:end]
		}
		cPosID, _ = strconv.ParseInt(rest, 10, 64)
	}

	// Extract Match-Trader ID (alphanumeric after Match-Trader ... #W\w+)
	if mIdx := strings.Index(ticket, "Match-Trader"); mIdx != -1 {
		sub := ticket[mIdx:]
		if hashIdx := strings.Index(sub, "#"); hashIdx != -1 {
			rest := sub[hashIdx+1:]
			end := strings.IndexAny(rest, ", )")
			if end != -1 {
				rest = rest[:end]
			}
			mtrID = rest
		}
	}

	if mtrID == "" && (strings.HasPrefix(ticket, "W") || strings.HasPrefix(ticket, "w")) {
		mtrID = ticket
	}

	if cPosID == 0 {
		cPosID, _ = strconv.ParseInt(ticket, 10, 64)
	}
	return cPosID, mtrID
}

var _ Broker = (*MultiBrokerManager)(nil)
var _ = time.Now
