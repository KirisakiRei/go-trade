package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/pkg/broker"
	"ila-engine/pkg/marketfeed"
	"ila-engine/pkg/risk"

	"github.com/google/uuid"
)

type executionService struct {
	bbmaService    BBMAService
	ilaService     ILAService
	evalService    EvaluationService
	positionRepo   repository.PositionRepository
	journalRepo    repository.JournalRepository
	broker         broker.Broker
	feed           marketfeed.Provider
	mu             sync.RWMutex
	pendingSignals map[string]*dto.PendingSignal
}

func NewExecutionService(
	bbmaService BBMAService,
	ilaService ILAService,
	evalService EvaluationService,
	positionRepo repository.PositionRepository,
	journalRepo repository.JournalRepository,
	b broker.Broker,
	feed marketfeed.Provider,
) ExecutionService {
	return &executionService{
		bbmaService:    bbmaService,
		ilaService:     ilaService,
		evalService:    evalService,
		positionRepo:   positionRepo,
		journalRepo:    journalRepo,
		broker:         b,
		feed:           feed,
		pendingSignals: make(map[string]*dto.PendingSignal),
	}
}

func (s *executionService) GetAccount(ctx context.Context) (any, error) {
	return s.broker.GetAccountInfo(ctx)
}

func (s *executionService) EvaluateAndExecute(ctx context.Context, req dto.TradeExecutionRequest) (*dto.TradeExecutionResult, error) {
	if req.RiskRatio <= 0 {
		req.RiskRatio = 1.0 // 1%
	}
	if req.MinRR <= 0 {
		req.MinRR = 2.0 // 1:2 R:R
	}

	result := &dto.TradeExecutionResult{
		Symbol:     req.Symbol,
		Strategy:   "BBMA Oma Ally",
		Timeframe:  "H4 ➡️ H1 ➡️ M15",
		ExecutedAt: time.Now(),
	}

	// 1. Proteksi Drawdown Harian (2 Strike & Out): Maksimal 2x SL per hari
	if !req.Force {
		todayLosses, err := s.positionRepo.CountTodayLosses(ctx)
		if err == nil && todayLosses >= 2 {
			result.Executed = false
			result.RejectionReason = "🚨 PROTEKSI DRAWDOWN HARIAN (2 Strike & Out): Hari ini sudah mengalami 2 kali Stop Loss. Sistem mengunci trading hari ini demi menjaga batas maksimal rugi harian 5% prop firm!"
			return result, nil
		}
	}

	// 2. Check existing open position for this symbol
	openPos, err := s.positionRepo.FindOpenBySymbol(ctx, req.Symbol)
	if err == nil && len(openPos) > 0 {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Ada posisi aktif (%s %s Lot %.2f) yang belum ditutup.", openPos[0].Direction, openPos[0].Symbol, openPos[0].LotSize)
		return result, nil
	}

	// 3. Scan MTF Confluence (H4 -> H1 -> M15)
	mtfResp, err := s.bbmaService.AnalyzeMTF(ctx, dto.MTFAnalysisRequest{
		Symbol: req.Symbol,
		TF1:    "H4",
		TF2:    "H1",
		TF3:    "M15",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate MTF: %w", err)
	}

	// 4. Check Rollover Lock
	if mtfResp.ConfluenceGrade == "ROLLOVER_LOCK" {
		result.Executed = false
		result.RejectionReason = mtfResp.Summary
		return result, nil
	}

	// 5. Strict Confluence Grade Gate: Only GRADE_A_PLUS, GRADE_A, GRADE_A_GOLDEN, or GRADE_B_STRONG allowed
	allowedGrades := map[string]bool{
		"GRADE_A_PLUS":   true,
		"GRADE_A":        true,
		"GRADE_A_GOLDEN": true,
		"GRADE_B_STRONG": true,
	}
	if !allowedGrades[mtfResp.ConfluenceGrade] {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Grade MTF (%s) belum memenuhi standar kualifikasi entry (Wajib GRADE A+ / GRADE A). Alasan: %s", mtfResp.ConfluenceGrade, mtfResp.Summary)
		return result, nil
	}

	if mtfResp.Bias != "BULLISH" && mtfResp.Bias != "BEARISH" {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Arah bias tidak jelas (%s) atau terjadi konflik antar timeframe.", mtfResp.Bias)
		return result, nil
	}

	// 6. Check Trigger Setup on M15
	triggerAnalysis := mtfResp.TF3Analysis
	if triggerAnalysis == nil || triggerAnalysis.EntryPlan == nil {
		result.Executed = false
		result.RejectionReason = "Tidak ditemukan Rencana Entry (Entry Plan) valid pada timeframe eksekusi M15."
		return result, nil
	}

	// Adjust risk: Half risk for GRADE_A (outside London/NY session)
	actualRiskRatio := req.RiskRatio
	if mtfResp.ConfluenceGrade == "GRADE_A" {
		actualRiskRatio = req.RiskRatio * 0.5 // Separuh risiko di sesi Asia
	}

	entryPlan := triggerAnalysis.EntryPlan
	entryPrice := (entryPlan.EntryZone[0] + entryPlan.EntryZone[1]) / 2.0
	sl := entryPlan.StopLoss
	tp := entryPlan.TakeProfit

	setupID := fmt.Sprintf("%s-M15-%s", req.Symbol, triggerAnalysis.AsOf)
	result.SetupID = setupID

	// 5. Hard Gate: Validate Risk & Minimum 1:2 R:R
	accInfo, err := s.broker.GetAccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	// Dynamic 1% risk based on actual account equity
	equity := accInfo.Equity
	if equity <= 0 {
		equity = accInfo.Balance
	}
	if equity <= 0 {
		equity = 100000.0
	}

	riskRes, err := risk.CalculateLot(equity, actualRiskRatio, entryPrice, sl, req.MinRR, tp, req.Symbol)
	if err != nil {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Kalkulasi risiko gagal: %v", err)
		return result, nil
	}

	if !riskRes.PassedRR {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Rasio Risk:Reward (1:%.2f) di bawah batas wajib (Minimal 1:%.2f). Entry dibatalkan demi disiplin trading.", riskRes.RiskReward, req.MinRR)
		return result, nil
	}

	// 6. Execute Order to Broker & Enqueue for MT5 EA
	direction := "BUY"
	if mtfResp.Bias == "BEARISH" {
		direction = "SELL"
	}

	sigID := uuid.New().String()
	sig := &dto.PendingSignal{
		ID:         sigID,
		Symbol:     req.Symbol,
		Direction:  direction,
		Volume:     riskRes.LotSize,
		OpenPrice:  entryPrice,
		StopLoss:   sl,
		TakeProfit: tp,
		Comment:    fmt.Sprintf("BBMA %s", mtfResp.ConfluenceGrade),
		CreatedAt:  time.Now(),
	}
	s.mu.Lock()
	s.pendingSignals[sigID] = sig
	s.mu.Unlock()

	orderRes, err := s.broker.ExecuteOrder(ctx, broker.OrderRequest{
		Symbol:     req.Symbol,
		Direction:  direction,
		OpenPrice:  entryPrice,
		StopLoss:   sl,
		TakeProfit: tp,
		LotSize:    riskRes.LotSize,
		Comment:    fmt.Sprintf("BBMA %s %s", mtfResp.ConfluenceGrade, setupID),
		SetupID:    setupID,
	})
	if err != nil {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Eksekusi broker gagal: %v", err)
		return result, nil
	}

	// 7. Persist Position to Database
	now := time.Now()
	posRecord := &model.Position{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		TicketNumber: orderRes.TicketNumber,
		SetupID:      setupID,
		Symbol:       req.Symbol,
		Timeframe:    "M15",
		Direction:    direction,
		OpenPrice:    entryPrice,
		StopLoss:     sl,
		TakeProfit:   tp,
		LotSize:      riskRes.LotSize,
		RiskReward:   riskRes.RiskReward,
		Status:       "OPEN",
		OpenedAt:     now,
		Notes:        fmt.Sprintf("Pola: %s | Grade: %s", mtfResp.ConfluencePattern, mtfResp.ConfluenceGrade),
	}

	if err := s.positionRepo.Create(ctx, posRecord); err != nil {
		slog.Error("Failed to save position record", "error", err)
	}

	// 8. Record in Trade Journal
	journal := &model.TradeJournal{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		SetupID:     setupID,
		Symbol:      req.Symbol,
		Timeframe:   "M15",
		Action:      "ORDER_FILLED",
		State:       "POSITION_OPEN",
		Price:       entryPrice,
		StopLoss:    sl,
		TakeProfit:  tp,
		Notes:       fmt.Sprintf("Order #%s dibuka: %s %.2f lot @ %.5f (SL: %.5f, TP: %.5f, RR: 1:%.2f)", orderRes.TicketNumber, direction, riskRes.LotSize, entryPrice, sl, tp, riskRes.RiskReward),
		EvaluatedAt: now,
	}
	_ = s.journalRepo.Create(ctx, journal)

	slog.Info("🚀 POSISI TRADING DIEKSEKUSI 🚀",
		"ticket", orderRes.TicketNumber,
		"symbol", req.Symbol,
		"direction", direction,
		"lot", riskRes.LotSize,
		"open", entryPrice,
		"sl", sl,
		"tp", tp,
		"rr", riskRes.RiskReward,
	)

	result.Executed = true
	result.TicketNumber = orderRes.TicketNumber
	result.Direction = direction
	result.LotSize = riskRes.LotSize
	result.OpenPrice = entryPrice
	result.StopLoss = sl
	result.TakeProfit = tp
	result.RiskReward = riskRes.RiskReward
	result.RiskAmount = riskRes.RiskAmount
	return result, nil
}

func (s *executionService) CheckAndManagePositions(ctx context.Context) error {
	openPositions, err := s.positionRepo.FindAllOpen(ctx)
	if err != nil || len(openPositions) == 0 {
		return nil
	}

	// 1. If connected to real brokers (cTrader / Match-Trader / MultiBroker), let brokers execute SL/TP natively.
	// We only reconcile state: if position is closed on broker, update database with REAL broker deal.
	if strings.Contains(s.broker.Name(), "cTrader") || strings.Contains(s.broker.Name(), "Match-Trader") || strings.Contains(s.broker.Name(), "MultiBroker") {
		brokerPositions, err := s.broker.GetOpenPositions(ctx)
		if err != nil {
			slog.Warn("Failed to fetch open positions from brokers for reconciliation", "error", err)
			return nil
		}

		activeBrokerTickets := make(map[string]bool)
		for _, bp := range brokerPositions {
			activeBrokerTickets[bp.TicketNumber] = true
		}

		for _, pos := range openPositions {
			cPosID, mtrID := broker.ExtractTicketIDs(pos.TicketNumber)
			cPosIDStr := strconv.FormatInt(cPosID, 10)

			// If position is still open on any broker, DO NOT TOUCH IT! The broker's server SL/TP is guarding it.
			isOpen := false
			if activeBrokerTickets[pos.TicketNumber] {
				isOpen = true
			}
			if cPosID > 0 && activeBrokerTickets[cPosIDStr] {
				isOpen = true
			}
			if mtrID != "" && activeBrokerTickets[mtrID] {
				isOpen = true
			}
			if isOpen {
				continue
			}

			// Position has been closed on broker! (SL, TP, or manual close in app)
			slog.Info("🔄 Posisi terdeteksi telah ditutup di broker, sinkronisasi deal...", "ticket", pos.TicketNumber)

			exitPrice := pos.OpenPrice
			actualPnL := 0.0
			isDealFound := false

			// Find underlying brokers
			var cb *broker.CTraderBroker
			var mb *broker.MatchTraderBroker
			if multi, ok := s.broker.(*broker.MultiBrokerManager); ok {
				for _, b := range multi.GetBrokers() {
					if c, ok := b.(*broker.CTraderBroker); ok {
						cb = c
					} else if m, ok := b.(*broker.MatchTraderBroker); ok {
						mb = m
					}
				}
			} else if c, ok := s.broker.(*broker.CTraderBroker); ok {
				cb = c
			} else if m, ok := s.broker.(*broker.MatchTraderBroker); ok {
				mb = m
			}

			// 1. Try pulling deals from cTrader
			if cb != nil && cPosID > 0 {
				deals, err := cb.GetDealsByPosition(ctx, cPosID)
				if err == nil && len(deals) > 0 {
					for _, d := range deals {
						if d.GrossProfit != 0 || (pos.Direction == "BUY" && d.TradeSide == "SELL") || (pos.Direction == "SELL" && d.TradeSide == "BUY") {
							exitPrice = d.ExecutionPrice
							actualPnL = d.GrossProfit
							isDealFound = true
							break
						}
					}
				}
			}

			// 2. Try pulling closed position from Match-Trader
			if !isDealFound && mb != nil && mtrID != "" {
				closedMtr, err := mb.GetClosedPositions(ctx, time.Now().Add(-24*time.Hour), time.Now())
				if err == nil {
					for _, cp := range closedMtr {
						if cp.ID == mtrID {
							exitPrice = cp.ClosePrice
							actualPnL = cp.Profit
							isDealFound = true
							break
						}
					}
				}
			}

			isGold := strings.Contains(pos.Symbol, "XAU") || strings.Contains(pos.Symbol, "GOLD")
			tol := 0.50
			if !isGold {
				tol = 0.00050
			}

			hitTP := false
			if pos.Direction == "BUY" {
				if exitPrice >= (pos.TakeProfit - tol) {
					hitTP = true
				}
			} else {
				if exitPrice <= (pos.TakeProfit + tol) {
					hitTP = true
				}
			}
			if !hitTP && actualPnL > 0 {
				hitTP = true
			}

			status := "CLOSED_SL"
			outcomeR := -1.0
			if hitTP {
				status = "CLOSED_TP"
				outcomeR = pos.RiskReward
			}

			_ = s.positionRepo.ClosePosition(ctx, pos.TicketNumber, exitPrice, actualPnL, outcomeR, status, fmt.Sprintf("Ditutup oleh server broker (%s)", status))

			slog.Info("🏁 POSISI BROKER TERKONFIRMASI TUTUP 🏁",
				"ticket", pos.TicketNumber,
				"symbol", pos.Symbol,
				"status", status,
				"exit_price", exitPrice,
				"pnl", actualPnL,
				"outcome_r", outcomeR,
			)
		}
		return nil
	}

	// 2. Simulated Broker (DemoBroker) only
	for _, pos := range openPositions {
		candles, err := s.feed.FetchCandles(ctx, pos.Symbol, "15m", 5)
		if err != nil || len(candles) == 0 {
			continue
		}
		latest := candles[len(candles)-1]

		isBuy := pos.Direction == "BUY"
		closed := false
		var status string
		var exitPrice float64
		var pnl float64
		var outcomeR float64

		if isBuy {
			if latest.High >= pos.TakeProfit {
				closed = true
				status = "CLOSED_TP"
				exitPrice = pos.TakeProfit
				outcomeR = pos.RiskReward
			} else if latest.Low <= pos.StopLoss {
				closed = true
				status = "CLOSED_SL"
				exitPrice = pos.StopLoss
				outcomeR = -1.0
			}
		} else { // SELL
			if latest.Low <= pos.TakeProfit {
				closed = true
				status = "CLOSED_TP"
				exitPrice = pos.TakeProfit
				outcomeR = pos.RiskReward
			} else if latest.High >= pos.StopLoss {
				closed = true
				status = "CLOSED_SL"
				exitPrice = pos.StopLoss
				outcomeR = -1.0
			}
		}

		if closed {
			diff := exitPrice - pos.OpenPrice
			if !isBuy {
				diff = pos.OpenPrice - exitPrice
			}
			isGold := pos.Symbol == "XAUUSD" || pos.Symbol == "GOLD"
			if isGold {
				pnl = diff * 100.0 * pos.LotSize
			} else {
				pnl = diff * 100000.0 * pos.LotSize
			}

			_ = s.broker.CloseOrder(ctx, pos.TicketNumber, exitPrice, status)
			_ = s.positionRepo.ClosePosition(ctx, pos.TicketNumber, exitPrice, pnl, outcomeR, status, fmt.Sprintf("Target tercapai (%s)", status))

			slog.Info("🏁 POSISI DITUTUP (SIMULASI) 🏁",
				"ticket", pos.TicketNumber,
				"symbol", pos.Symbol,
				"status", status,
				"exit_price", exitPrice,
				"pnl", pnl,
				"outcome_r", outcomeR,
			)
		}
	}

	return nil
}

func (s *executionService) GetOpenPositions(ctx context.Context) ([]dto.PositionResponse, error) {
	positions, err := s.positionRepo.FindAllOpen(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]dto.PositionResponse, len(positions))
	for i, p := range positions {
		res[i] = dto.PositionResponse{
			ID:           p.ID,
			TicketNumber: p.TicketNumber,
			Symbol:       p.Symbol,
			Direction:    p.Direction,
			OpenPrice:    p.OpenPrice,
			StopLoss:     p.StopLoss,
			TakeProfit:   p.TakeProfit,
			LotSize:      p.LotSize,
			RiskReward:   p.RiskReward,
			Status:       p.Status,
			ExitPrice:    p.ExitPrice,
			ProfitLoss:   p.ProfitLoss,
			OutcomeR:     p.OutcomeR,
			OpenedAt:     p.OpenedAt.Format(time.RFC3339),
			Notes:        p.Notes,
		}
	}
	return res, nil
}

func (s *executionService) ListPositions(ctx context.Context, symbol, status string, p dto.PaginationQuery) ([]dto.PositionResponse, dto.PaginationMeta, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 || p.PerPage > 100 {
		p.PerPage = 10
	}

	positions, total, err := s.positionRepo.List(ctx, symbol, status, p)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}

	res := make([]dto.PositionResponse, len(positions))
	for i, pos := range positions {
		var closedAtStr *string
		if pos.ClosedAt != nil {
			c := pos.ClosedAt.Format(time.RFC3339)
			closedAtStr = &c
		}
		res[i] = dto.PositionResponse{
			ID:           pos.ID,
			TicketNumber: pos.TicketNumber,
			Symbol:       pos.Symbol,
			Direction:    pos.Direction,
			OpenPrice:    pos.OpenPrice,
			StopLoss:     pos.StopLoss,
			TakeProfit:   pos.TakeProfit,
			LotSize:      pos.LotSize,
			RiskReward:   pos.RiskReward,
			Status:       pos.Status,
			ExitPrice:    pos.ExitPrice,
			ProfitLoss:   pos.ProfitLoss,
			OutcomeR:     pos.OutcomeR,
			OpenedAt:     pos.OpenedAt.Format(time.RFC3339),
			ClosedAt:     closedAtStr,
			Notes:        pos.Notes,
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(p.PerPage)))
	meta := dto.PaginationMeta{
		CurrentPage: p.Page,
		PerPage:     p.PerPage,
		TotalItems:  total,
		TotalPages:  totalPages,
	}

	return res, meta, nil
}

func (s *executionService) GetPendingSignals() []*dto.PendingSignal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*dto.PendingSignal, 0, len(s.pendingSignals))
	for _, sig := range s.pendingSignals {
		res = append(res, sig)
	}
	return res
}

func (s *executionService) ConfirmSignal(ctx context.Context, req dto.ConfirmSignalRequest) error {
	s.mu.Lock()
	sig, exists := s.pendingSignals[req.SignalID]
	if exists {
		delete(s.pendingSignals, req.SignalID)
	}
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("signal %s not found or already executed", req.SignalID)
	}

	now := time.Now()
	posRecord := &model.Position{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		TicketNumber: req.TicketNumber,
		SetupID:      fmt.Sprintf("%s-M15-%s", sig.Symbol, sig.ID),
		Symbol:       sig.Symbol,
		Timeframe:    "M15",
		Direction:    sig.Direction,
		OpenPrice:    req.ExecutedPrice,
		StopLoss:     sig.StopLoss,
		TakeProfit:   sig.TakeProfit,
		LotSize:      sig.Volume,
		RiskReward:   math.Round(math.Abs(sig.TakeProfit-req.ExecutedPrice)/math.Abs(req.ExecutedPrice-sig.StopLoss)*100) / 100,
		Status:       "OPEN",
		OpenedAt:     now,
		Notes:        fmt.Sprintf("Confirmed by MT5 EA. Ticket: %s", req.TicketNumber),
	}

	return s.positionRepo.Create(ctx, posRecord)
}

func (s *executionService) EvaluateAndExecuteILA(ctx context.Context, req dto.ILAExecuteRequest) (*dto.TradeExecutionResult, error) {
	if req.RiskRatio <= 0 {
		req.RiskRatio = 0.5 // Default 0.5% (konservatif & aman prop firm)
	}
	if req.MinRR <= 0 {
		req.MinRR = 2.0 // 1:2 R:R
	}
	if req.HTF == "" {
		req.HTF = "H4"
	}
	if req.LTF == "" {
		req.LTF = "M15"
	}

	result := &dto.TradeExecutionResult{
		Symbol:     req.Symbol,
		Strategy:   "ILA (Imaginary Liquidity)",
		Timeframe:  fmt.Sprintf("%s ➡️ %s", req.HTF, req.LTF),
		ExecutedAt: time.Now(),
	}

	// 1. Trade Management Harian (WIB / Jam Jakarta): 1 Win = Stop, Max 2 Trades, 0.5% Risk.
	// `--force` tetap bisa bypass untuk override manual.
	if !req.Force {
		dailyStatus, err := s.positionRepo.GetDailyTradeStatusWIB(ctx, time.Now())
		if err == nil && dailyStatus != nil && !dailyStatus.CanTrade {
			result.Executed = false
			result.RejectionReason = dailyStatus.LockReason
			return result, nil
		}
	}

	// 2. Check existing open position specifically for this ILA pair
	openPos, err := s.positionRepo.FindOpenBySymbol(ctx, req.Symbol)
	if err == nil && len(openPos) > 0 {
		for _, p := range openPos {
			if p.Timeframe == req.LTF && (strings.HasPrefix(p.SetupID, "ILA") || strings.Contains(p.Notes, "ILA")) {
				result.Executed = false
				result.RejectionReason = fmt.Sprintf("Ada posisi ILA aktif (%s %s Lot %.2f) yang belum ditutup.", p.Direction, p.Symbol, p.LotSize)
				return result, nil
			}
		}
	}

	// 3. Evaluate ILA Setup
	setup, err := s.ilaService.AnalyzeILA(ctx, dto.ILAAnalysisRequest{
		Symbol: req.Symbol,
		HTF:    req.HTF,
		LTF:    req.LTF,
		Limit:  100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate ILA: %w", err)
	}

	if setup.Status != "READY_TO_ENTRY" {
		result.Executed = false
		result.RejectionReason = setup.Summary
		return result, nil
	}

	if !setup.PassedMinRR || setup.RiskReward < req.MinRR {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Rasio Risk:Reward ILA (1:%.2f) di bawah batas wajib (Minimal 1:%.2f). Entry dibatalkan demi disiplin.", setup.RiskReward, req.MinRR)
		return result, nil
	}

	entryPrice := setup.EntryPrice
	sl := setup.StopLoss
	tp := setup.TakeProfit
	direction := "BUY"
	if setup.Bias == "BEARISH" {
		direction = "SELL"
	}

	setupID := fmt.Sprintf("ILA-%s-%s-%s-%s", req.Symbol, req.HTF, req.LTF, time.Now().Format("200601021504"))
	result.SetupID = setupID

	// 4. Calculate Lot Size with dynamic 1% risk based on actual account equity
	accInfo, err := s.broker.GetAccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	equity := accInfo.Equity
	if equity <= 0 {
		equity = accInfo.Balance
	}
	if equity <= 0 {
		equity = 100000.0
	}

	riskRes, err := risk.CalculateLot(equity, req.RiskRatio, entryPrice, sl, req.MinRR, tp, req.Symbol)
	if err != nil {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Kalkulasi risiko gagal: %v", err)
		return result, nil
	}

	// 4b. Tolak jika pembulatan lot membuat risiko aktual melebihi target.
	if !riskRes.PassedRisk {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Risiko aktual (%.3f%%) melebihi target setelah pembulatan lot broker. Entry dibatalkan demi disiplin risiko.", riskRes.RiskPercentage)
		return result, nil
	}

	// 4c. Structural SL floor (docs/04): tolak SL terlalu tipis.
	if s.evalService != nil {
		pf, pfErr := s.evalService.PreFlightCheck(ctx, req.Symbol, direction, "ILA_SWEEP_CISD", math.Abs(entryPrice-sl))
		if pfErr == nil && pf != nil && !pf.CanExecute {
			result.Executed = false
			result.RejectionReason = pf.BlockingReason
			return result, nil
		}
	}

	// 5. Execute Order on cTrader Broker
	orderRes, err := s.broker.ExecuteOrder(ctx, broker.OrderRequest{
		Symbol:     req.Symbol,
		Direction:  direction,
		OpenPrice:  entryPrice,
		StopLoss:   sl,
		TakeProfit: tp,
		LotSize:    riskRes.LotSize,
		Comment:    fmt.Sprintf("ILA %s->%s %s", req.HTF, req.LTF, setupID),
		SetupID:    setupID,
	})
	if err != nil {
		result.Executed = false
		result.RejectionReason = fmt.Sprintf("Eksekusi broker cTrader gagal: %v", err)
		return result, nil
	}

	// 6. Persist to Database
	now := time.Now()
	posRecord := &model.Position{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		TicketNumber: orderRes.TicketNumber,
		SetupID:      setupID,
		Symbol:       req.Symbol,
		Timeframe:    req.LTF,
		Direction:    direction,
		OpenPrice:    entryPrice,
		StopLoss:     sl,
		TakeProfit:   tp,
		LotSize:      riskRes.LotSize,
		RiskReward:   setup.RiskReward,
		Status:       "OPEN",
		OpenedAt:     now,
		Notes:        fmt.Sprintf("Metode: ILA (%s ➡️ %s) | %s", req.HTF, req.LTF, setup.Summary),
	}
	if err := s.positionRepo.Create(ctx, posRecord); err != nil {
		slog.Error("Failed to save ILA position record", "error", err)
	}

	// 7. Record in Trade Journal
	journal := &model.TradeJournal{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		SetupID:     setupID,
		Symbol:      req.Symbol,
		Timeframe:   req.LTF,
		Action:      "ORDER_FILLED",
		State:       "POSITION_OPEN",
		Price:       entryPrice,
		StopLoss:    sl,
		TakeProfit:  tp,
		Notes:       fmt.Sprintf("ILA Order #%s dibuka: %s %.2f lot @ %.5f (SL: %.5f, TP: %.5f, RR: 1:%.2f)", orderRes.TicketNumber, direction, riskRes.LotSize, entryPrice, sl, tp, setup.RiskReward),
		EvaluatedAt: now,
	}
	_ = s.journalRepo.Create(ctx, journal)

	result.Executed = true
	result.TicketNumber = orderRes.TicketNumber
	result.Direction = direction
	result.LotSize = riskRes.LotSize
	result.OpenPrice = entryPrice
	result.StopLoss = sl
	result.TakeProfit = tp
	result.RiskReward = setup.RiskReward
	result.RiskAmount = riskRes.RiskAmount

	return result, nil
}
