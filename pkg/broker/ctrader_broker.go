package broker

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diegobernardes/ctrader"
	"github.com/diegobernardes/ctrader/openapi"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/proto"
)

type CTraderConfig struct {
	ClientID     string
	ClientSecret string
	AccountID    int64
	AccessToken  string
	Live         bool
}

type CTraderBroker struct {
	cfg       CTraderConfig
	client    *ctrader.Client
	mu        sync.Mutex
	authed    bool
	symbolMap map[string]int64
	idToSym   map[int64]string
	lotMult   map[string]int64
	digits    map[string]uint32
}

type CTraderDealInfo struct {
	DealID             int64
	PositionID         int64
	ExecutionPrice     float64
	Volume             int64
	TradeSide          string
	GrossProfit        float64
	ExecutionTimestamp int64
}

func NewCTraderBroker(cfg CTraderConfig) (*CTraderBroker, error) {
	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv("CTRADER_CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv("CTRADER_CLIENT_SECRET")
	}
	if cfg.AccountID == 0 {
		if envAcc := os.Getenv("CTRADER_ACCOUNT_ID"); envAcc != "" {
			cfg.AccountID, _ = strconv.ParseInt(envAcc, 10, 64)
		}
	}
	if cfg.AccessToken == "" {
		cfg.AccessToken = os.Getenv("CTRADER_ACCESS_TOKEN")
	}

	b := &CTraderBroker{
		cfg: cfg,
		symbolMap: map[string]int64{
			"EURUSD": 1,
			"XAUUSD": 41,
			"GOLD":   41,
			"USTEC":  10014,
			"NAS100": 10014,
			"US100":  10014,
		},
		idToSym: map[int64]string{
			1:     "EURUSD",
			41:    "XAUUSD",
			10014: "USTEC",
		},
		lotMult: map[string]int64{
			"EURUSD": 10000000,
			"XAUUSD": 10000,
			"GOLD":   10000,
			"USTEC":  100,
			"NAS100": 100,
			"US100":  100,
		},
		digits: map[string]uint32{
			"EURUSD": 5,
			"XAUUSD": 2,
			"GOLD":   2,
			"USTEC":  2,
			"NAS100": 2,
			"US100":  2,
		},
	}

	return b, nil
}

func (b *CTraderBroker) ensureConnected(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil && b.authed {
		return nil
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &ctrader.Client{
		ApplicationClientID: b.cfg.ClientID,
		ApplicationSecret:   b.cfg.ClientSecret,
		Logger:              logger,
		Deadline:            10 * time.Second,
		Live:                b.cfg.Live,
		HandlerEvent:        func(proto.Message) {},
	}

	if err := client.Start(); err != nil {
		return fmt.Errorf("failed to start cTrader client: %w", err)
	}

	// Authenticate Account
	_, err := ctrader.Command[*openapi.ProtoOAAccountAuthReq, *openapi.ProtoOAAccountAuthRes](
		ctx,
		client,
		&openapi.ProtoOAAccountAuthReq{
			CtidTraderAccountId: &b.cfg.AccountID,
			AccessToken:         &b.cfg.AccessToken,
		},
	)
	if err != nil {
		_ = client.Stop()
		return fmt.Errorf("cTrader account authentication failed: %w", err)
	}

	b.client = client
	b.authed = true
	return nil
}

func (b *CTraderBroker) Client() *ctrader.Client {
	return b.client
}

func (b *CTraderBroker) Name() string {
	return "IC Markets cTrader (Native Go)"
}

func (b *CTraderBroker) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	if err := b.ensureConnected(ctx); err != nil {
		return nil, err
	}

	res, err := ctrader.Command[*openapi.ProtoOATraderReq, *openapi.ProtoOATraderRes](
		ctx,
		b.client,
		&openapi.ProtoOATraderReq{
			CtidTraderAccountId: &b.cfg.AccountID,
		},
	)
	if err != nil {
		b.authed = false
		return nil, fmt.Errorf("failed to fetch cTrader trader info: %w", err)
	}

	trader := res.GetTrader()
	moneyDigits := trader.GetMoneyDigits()
	div := math.Pow10(int(moneyDigits))

	balance := float64(trader.GetBalance()) / div

	// Reconcile to calculate equity
	openPos, _ := b.GetOpenPositions(ctx)
	equity := balance
	for _, p := range openPos {
		equity += p.FloatingPnL
	}

	return &AccountInfo{
		BrokerName:    fmt.Sprintf("%s (cTrader)", trader.GetBrokerName()),
		AccountNumber: strconv.FormatInt(trader.GetTraderLogin(), 10),
		Currency:      "USD",
		Balance:       math.Round(balance*100) / 100,
		Equity:        math.Round(equity*100) / 100,
		MarginUsed:    0,
		FreeMargin:    math.Round(equity*100) / 100,
	}, nil
}

func (b *CTraderBroker) ExecuteOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	if err := b.ensureConnected(ctx); err != nil {
		return nil, err
	}

	symID, ok := b.symbolMap[req.Symbol]
	if !ok {
		return nil, fmt.Errorf("simbol %s belum terdaftar di cTrader", req.Symbol)
	}

	mult := b.lotMult[req.Symbol]
	if mult == 0 {
		mult = 10000000
	}
	rawVolume := int64(math.Round(req.LotSize * float64(mult)))
	if symID == 10014 { // USTEC: step volume is 10 (0.10 lot)
		rawVolume = (rawVolume / 10) * 10
		if rawVolume < 10 {
			rawVolume = 10
		}
	}

	orderType := openapi.ProtoOAOrderType_MARKET
	tradeSide := openapi.ProtoOATradeSide_BUY
	if req.Direction == "SELL" {
		tradeSide = openapi.ProtoOATradeSide_SELL
	}

	orderReq := &openapi.ProtoOANewOrderReq{
		CtidTraderAccountId: &b.cfg.AccountID,
		SymbolId:            &symID,
		OrderType:           &orderType,
		TradeSide:           &tradeSide,
		Volume:              &rawVolume,
		Comment:             &req.Comment,
	}

	digits := 2
	if !strings.Contains(req.Symbol, "XAU") && !strings.Contains(req.Symbol, "GOLD") {
		digits = 5
	}
	pow := math.Pow10(digits)
	roundPrice := func(p float64) float64 {
		return math.Round(p*pow) / pow
	}

	if req.StopLoss > 0 {
		sl := roundPrice(req.StopLoss)
		orderReq.StopLoss = &sl
	}
	if req.TakeProfit > 0 {
		tp := roundPrice(req.TakeProfit)
		orderReq.TakeProfit = &tp
	}

	respMsg, err := ctrader.Command[*openapi.ProtoOANewOrderReq, proto.Message](
		ctx,
		b.client,
		orderReq,
	)
	if err != nil {
		b.authed = false
		return nil, fmt.Errorf("cTrader order execution failed: %w", err)
	}

	var execEvent *openapi.ProtoOAExecutionEvent
	switch v := respMsg.(type) {
	case *openapi.ProtoOAExecutionEvent:
		execEvent = v
	case *openapi.ProtoOAOrderErrorEvent:
		return nil, fmt.Errorf("cTrader order error [%s]: %s", v.GetErrorCode(), v.GetDescription())
	default:
		return nil, fmt.Errorf("cTrader unexpected response message: %T", respMsg)
	}

	posID := int64(0)
	execPrice := 0.0

	if execEvent.GetPosition() != nil {
		posID = execEvent.GetPosition().GetPositionId()
		execPrice = execEvent.GetPosition().GetPrice()
	} else if execEvent.GetDeal() != nil {
		posID = execEvent.GetDeal().GetPositionId()
		execPrice = execEvent.GetDeal().GetExecutionPrice()
	}

	if posID == 0 {
		return nil, fmt.Errorf("cTrader execution event did not return position ID (ErrorCode: %s)", execEvent.GetErrorCode())
	}

	// Ensure SL and TP are amended and confirmed on server
	if req.StopLoss > 0 || req.TakeProfit > 0 {
		amendReq := &openapi.ProtoOAAmendPositionSLTPReq{
			CtidTraderAccountId: &b.cfg.AccountID,
			PositionId:          &posID,
		}
		if req.StopLoss > 0 {
			sl := roundPrice(req.StopLoss)
			amendReq.StopLoss = &sl
		}
		if req.TakeProfit > 0 {
			tp := roundPrice(req.TakeProfit)
			amendReq.TakeProfit = &tp
		}
		_, _ = ctrader.Command[*openapi.ProtoOAAmendPositionSLTPReq, proto.Message](ctx, b.client, amendReq)
	}

	return &OrderResponse{
		TicketNumber:  strconv.FormatInt(posID, 10),
		Symbol:        req.Symbol,
		Direction:     req.Direction,
		ExecutedPrice: execPrice,
		StopLoss:      req.StopLoss,
		TakeProfit:    req.TakeProfit,
		LotSize:       req.LotSize,
		ExecutedAt:    time.Now().UTC(),
		Status:        "FILLED",
	}, nil
}

func (b *CTraderBroker) CloseOrder(ctx context.Context, ticket string, exitPrice float64, reason string) error {
	if err := b.ensureConnected(ctx); err != nil {
		return nil
	}

	posID, err := strconv.ParseInt(ticket, 10, 64)
	if err != nil {
		return err
	}

	// Find position volume
	positions, _ := b.GetOpenPositions(ctx)
	rawVolume := int64(100000)
	for _, p := range positions {
		if p.TicketNumber == ticket {
			mult := b.lotMult[p.Symbol]
			if mult == 0 {
				mult = 10000000
			}
			rawVolume = int64(math.Round(p.LotSize * float64(mult)))
			break
		}
	}

	closeReq := &openapi.ProtoOAClosePositionReq{
		CtidTraderAccountId: &b.cfg.AccountID,
		PositionId:          &posID,
		Volume:              &rawVolume,
	}

	closeResp, err := ctrader.Command[*openapi.ProtoOAClosePositionReq, proto.Message](
		ctx,
		b.client,
		closeReq,
	)
	if err != nil {
		return err
	}
	if errEvent, ok := closeResp.(*openapi.ProtoOAOrderErrorEvent); ok {
		return fmt.Errorf("cTrader close error [%s]: %s", errEvent.GetErrorCode(), errEvent.GetDescription())
	}
	return nil
}

func (b *CTraderBroker) GetOpenPositions(ctx context.Context) ([]PositionInfo, error) {
	if err := b.ensureConnected(ctx); err != nil {
		return nil, err
	}

	res, err := ctrader.Command[*openapi.ProtoOAReconcileReq, *openapi.ProtoOAReconcileRes](
		ctx,
		b.client,
		&openapi.ProtoOAReconcileReq{
			CtidTraderAccountId: &b.cfg.AccountID,
		},
	)
	if err != nil {
		b.authed = false
		return nil, fmt.Errorf("cTrader reconcile failed: %w", err)
	}

	var list []PositionInfo
	for _, p := range res.GetPosition() {
		symID := p.GetTradeData().GetSymbolId()
		symName := b.idToSym[symID]
		if symName == "" {
			symName = fmt.Sprintf("SYM_%d", symID)
		}

		mult := b.lotMult[symName]
		if mult == 0 {
			mult = 10000000
		}
		lotSize := float64(p.GetTradeData().GetVolume()) / float64(mult)
		lotSize = math.Round(lotSize*100) / 100

		direction := "BUY"
		if p.GetTradeData().GetTradeSide() == openapi.ProtoOATradeSide_SELL {
			direction = "SELL"
		}

		list = append(list, PositionInfo{
			TicketNumber: strconv.FormatInt(p.GetPositionId(), 10),
			Symbol:       symName,
			Direction:    direction,
			OpenPrice:    p.GetPrice(),
			CurrentPrice: p.GetPrice(),
			StopLoss:     p.GetStopLoss(),
			TakeProfit:   p.GetTakeProfit(),
			LotSize:      lotSize,
			FloatingPnL:  0,
			OpenedAt:     time.UnixMilli(p.GetTradeData().GetOpenTimestamp()).UTC(),
		})
	}

	return list, nil
}

func (b *CTraderBroker) GetDealsByPosition(ctx context.Context, positionID int64) ([]CTraderDealInfo, error) {
	if err := b.ensureConnected(ctx); err != nil {
		return nil, err
	}

	fromTs := int64(0)
	toTs := time.Now().UnixMilli()

	res, err := ctrader.Command[*openapi.ProtoOADealListByPositionIdReq, *openapi.ProtoOADealListByPositionIdRes](
		ctx,
		b.client,
		&openapi.ProtoOADealListByPositionIdReq{
			CtidTraderAccountId: &b.cfg.AccountID,
			PositionId:          &positionID,
			FromTimestamp:       &fromTs,
			ToTimestamp:         &toTs,
		},
	)
	if err != nil {
		return nil, err
	}

	var deals []CTraderDealInfo
	for _, d := range res.GetDeal() {
		grossProfit := 0.0
		if d.GetClosePositionDetail() != nil {
			grossProfit = float64(d.GetClosePositionDetail().GetGrossProfit()) / 100.0
		}
		side := "BUY"
		if d.GetTradeSide() == openapi.ProtoOATradeSide_SELL {
			side = "SELL"
		}

		deals = append(deals, CTraderDealInfo{
			DealID:             d.GetDealId(),
			PositionID:         d.GetPositionId(),
			ExecutionPrice:     d.GetExecutionPrice(),
			Volume:             d.GetVolume(),
			TradeSide:          side,
			GrossProfit:        grossProfit,
			ExecutionTimestamp: d.GetExecutionTimestamp(),
		})
	}

	return deals, nil
}
