package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"

	"github.com/google/uuid"
)

type monitorService struct {
	bbmaService BBMAService
	journalRepo repository.JournalRepository
	intervalSec int

	mu              sync.RWMutex
	isRunning       bool
	monitoredSymbol string
	contextTF       string
	entryTF         string
	lastCheckedAt   time.Time
	lastSetupFound  string
	activeAlerts    int
	lastCandleTime  map[string]time.Time

	cancelFunc    context.CancelFunc
	alertCallback func(setup *dto.AnalysisResponse)
}

func NewMonitorService(bbmaService BBMAService, journalRepo repository.JournalRepository, intervalSec int) MonitorService {
	return &monitorService{
		bbmaService:    bbmaService,
		journalRepo:    journalRepo,
		intervalSec:    intervalSec,
		lastCandleTime: make(map[string]time.Time),
	}
}

func (m *monitorService) SetAlertCallback(cb func(setup *dto.AnalysisResponse)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertCallback = cb
}

func (m *monitorService) StartMonitoring(parentCtx context.Context, symbol, contextTF, entryTF string) error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("monitor is already running for %s", m.monitoredSymbol)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	m.cancelFunc = cancel
	m.isRunning = true
	m.monitoredSymbol = symbol
	m.contextTF = contextTF
	m.entryTF = entryTF
	m.mu.Unlock()

	slog.Info("Starting BBMA monitor", "symbol", symbol, "context_tf", contextTF, "entry_tf", entryTF, "interval_sec", m.intervalSec)

	go m.monitorLoop(ctx)
	return nil
}

func (m *monitorService) StopMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	m.isRunning = false
	slog.Info("Stopped BBMA monitor", "symbol", m.monitoredSymbol)
}

func (m *monitorService) GetStatus() dto.MonitorStatusResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return dto.MonitorStatusResponse{
		IsRunning:       m.isRunning,
		MonitoredSymbol: m.monitoredSymbol,
		ContextTF:       m.contextTF,
		EntryTF:         m.entryTF,
		LastCheckedAt:   m.lastCheckedAt,
		LastSetupFound:  m.lastSetupFound,
		ActiveAlerts:    m.activeAlerts,
	}
}

func (m *monitorService) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(m.intervalSec) * time.Second)
	defer ticker.Stop()

	// Initial check immediately
	m.runCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runCheck(ctx)
		}
	}
}

func (m *monitorService) runCheck(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in monitor runCheck", "panic", r)
		}
	}()

	m.mu.RLock()
	sym := m.monitoredSymbol
	cTF := m.contextTF
	eTF := m.entryTF
	cb := m.alertCallback
	m.mu.RUnlock()

	now := time.Now()

	// 1. Analyze Context Timeframe (H1)
	ctxResp, err := m.bbmaService.Analyze(ctx, dto.AnalysisRequest{
		Symbol:    sym,
		Timeframe: cTF,
		Limit:     100,
	})
	if err != nil || ctxResp == nil {
		slog.Error("Monitor failed to analyze context timeframe", "error", err, "tf", cTF)
		return
	}

	// 2. Analyze Entry Timeframe (M15)
	entryResp, err := m.bbmaService.Analyze(ctx, dto.AnalysisRequest{
		Symbol:    sym,
		Timeframe: eTF,
		Limit:     100,
	})
	if err != nil {
		slog.Error("Monitor failed to analyze entry timeframe", "error", err, "tf", eTF)
		return
	}

	m.mu.Lock()
	m.lastCheckedAt = now
	m.mu.Unlock()

	// Check if this candle was already evaluated to prevent duplicate alerts (Rule 7.6)
	asOfTime, _ := time.Parse(time.RFC3339, entryResp.AsOf)
	m.mu.RLock()
	lastProcessed := m.lastCandleTime[eTF]
	m.mu.RUnlock()

	if !asOfTime.After(lastProcessed) {
		// No new closed candle yet
		return
	}

	m.mu.Lock()
	m.lastCandleTime[eTF] = asOfTime
	m.mu.Unlock()

	slog.Info("New closed candle evaluated",
		"symbol", sym,
		"timeframe", eTF,
		"as_of", entryResp.AsOf,
		"phase", entryResp.MarketPhase,
		"status", entryResp.SetupStatus,
		"bias", entryResp.Bias,
	)

	// If a valid setup or actionable phase is found
	if entryResp.SetupStatus == "SETUP_VALID" || entryResp.SetupStatus == "CANDIDATE" {
		m.mu.Lock()
		m.lastSetupFound = fmt.Sprintf("%s %s (%s)", entryResp.SetupType, entryResp.Bias, eTF)
		m.activeAlerts++
		m.mu.Unlock()

		// Record in trade journal
		var entryPrice, sl, tp float64
		if entryResp.EntryPlan != nil {
			entryPrice = (entryResp.EntryPlan.EntryZone[0] + entryResp.EntryPlan.EntryZone[1]) / 2.0
			sl = entryResp.EntryPlan.StopLoss
			tp = entryResp.EntryPlan.TakeProfit
		}

		ctxBias := "UNKNOWN"
		if ctxResp != nil {
			ctxBias = ctxResp.Bias
		}

		journal := &model.TradeJournal{
			Base: model.Base{
				ID:        uuid.New().String(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			SetupID:     fmt.Sprintf("%s-%s-%d", sym, eTF, asOfTime.Unix()),
			Symbol:      sym,
			Timeframe:   eTF,
			Action:      "SETUP_DETECTED",
			State:       entryResp.SetupStatus,
			Price:       entryPrice,
			StopLoss:    sl,
			TakeProfit:  tp,
			Notes:       fmt.Sprintf("Context: %s %s. Entry: %s %s.", cTF, ctxBias, eTF, entryResp.SetupType),
			EvaluatedAt: asOfTime,
		}
		_ = m.journalRepo.Create(ctx, journal)

		if cb != nil {
			cb(entryResp)
		}
	}
}
