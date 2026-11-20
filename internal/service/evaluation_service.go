package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/pkg/marketfeed"

	"github.com/google/uuid"
)

type evaluationService struct {
	evalRepo repository.EvaluationRepository
	posRepo  repository.PositionRepository
	feed     marketfeed.Provider
}

func NewEvaluationService(
	evalRepo repository.EvaluationRepository,
	posRepo repository.PositionRepository,
	feed marketfeed.Provider,
) EvaluationService {
	return &evaluationService{
		evalRepo: evalRepo,
		posRepo:  posRepo,
		feed:     feed,
	}
}

func (s *evaluationService) EvaluateClosedTrade(ctx context.Context, pos *model.Position) (*model.TradeEvaluation, error) {
	if pos == nil {
		return nil, fmt.Errorf("position is nil")
	}

	// 1. Check if already evaluated
	existing, err := s.evalRepo.FindByPositionID(ctx, pos.ID)
	if err == nil && existing != nil {
		return existing, nil
	}

	isGold := strings.Contains(pos.Symbol, "XAU") || strings.Contains(pos.Symbol, "GOLD")
	durationMinutes := 15
	if pos.ClosedAt != nil {
		durationMinutes = int(pos.ClosedAt.Sub(pos.OpenedAt).Minutes())
		if durationMinutes <= 0 {
			durationMinutes = 1
		}
	}

	isProfit := pos.ProfitLoss > 0 || pos.Status == "CLOSED_TP" || pos.OutcomeR > 0
	outcome := "HIT_SL"
	if isProfit {
		outcome = "HIT_TP"
	}

	isILA := strings.Contains(pos.Notes, "ILA") || strings.HasPrefix(pos.SetupID, "ILA")

	// Extract setup type from SetupID (e.g. XAUUSD-M15-2026-11-04...)
	setupType := "REENTRY"
	if isILA {
		setupType = "ILA_SWEEP_CISD"
	} else if strings.Contains(pos.Notes, "MHV") || strings.Contains(pos.SetupID, "MHV") {
		setupType = "MHV"
	} else if strings.Contains(pos.Notes, "EXTREME") || strings.Contains(pos.SetupID, "EXTREME") {
		setupType = "EXTREME"
	} else if strings.Contains(pos.Notes, "CSAK") || strings.Contains(pos.SetupID, "CSAK") {
		setupType = "CSAK"
	}

	confluenceGrade := "GRADE_A"
	if isILA {
		confluenceGrade = "ILA_VALID"
	} else if strings.Contains(pos.Notes, "GRADE_A_PLUS") {
		confluenceGrade = "GRADE_A_PLUS"
	}

	// Analyze root cause & lessons
	var rootCause string
	var technicalAnalysis string
	var actionableLesson string

	slDist := math.Abs(pos.OpenPrice - pos.StopLoss)

	// Jam UTC untuk analisis rollover / false-break.
	openHour := pos.OpenedAt.UTC().Hour()
	closeHour := openHour
	if pos.ClosedAt != nil {
		closeHour = pos.ClosedAt.UTC().Hour()
	}
	// Rollover subuh 22:00-00:00 UTC (termasuk ujung 00:00-01:00).
	isRollover := openHour >= 22 || openHour < 1 || closeHour >= 22 || closeHour < 1

	if !isProfit {
		// Taksonomi root cause SL untuk ILA (bukan konstanta).
		isTightSL := (isGold && slDist < 4.50) || (!isGold && slDist < 0.0018)
		switch {
		case isTightSL:
			rootCause = "ILA_SL_TERLALU_TIPIS"
			technicalAnalysis = fmt.Sprintf("Jarak Stop Loss New ILA 2.0 (%.2f) sangat sempit sehingga noise normal menyapu SL sebelum harga mencapai target RR 1:2.", slDist)
			actionableLesson = "Pastikan SL tepat di swing puncak/dasar rangkaian saat menyapu ERL LTF."
		case isRollover:
			rootCause = "ILA_PELEBARAN_SPREAD_ROLLOVER"
			technicalAnalysis = fmt.Sprintf("Posisi aktif saat rollover subuh (open %02d:xx UTC, close %02d:xx UTC). Spread melebar dan memicu SL sebelum target RR 1:2 tercapai.", openHour, closeHour)
			actionableLesson = "Waspadai pelebaran spread di sekitar pergantian hari trading."
		case durationMinutes <= 20:
			rootCause = "ILA_LIQUIDITY_SWEEP_FALSE_BREAK"
			technicalAnalysis = fmt.Sprintf("Harga berbalik cepat dalam %d menit setelah entry, menembus SL puncak/dasar rangkaian LTF.", durationMinutes)
			actionableLesson = "Pastikan syarat key level ERL LTF (Kondisi 1/2/3) dan close body CISD terkonfirmasi jelas selama C3 berjalan."
		default:
			rootCause = "ILA_SWEEP_INVALIDATION"
			technicalAnalysis = "Harga menembus batas invalidasi (SL puncak/dasar rangkaian LTF) sebelum mencapai target RR 1:2."
			actionableLesson = "Patuhi SL objektif pada swing sapuan ERL; evaluasi ulang validitas C2 dan cycle."
		}
	} else {
		rootCause = "EKSEKUSI_ILA_PRESISI"
		technicalAnalysis = fmt.Sprintf("Setup New ILA 2.0 mencapai target RR 1:2 (+%.2f R). C2 closure HTF dalam cycle dan konfirmasi CISD di LTF selama C3 berjalan memberikan eksekusi presisi.", pos.OutcomeR)
		actionableLesson = "Pertahankan disiplin New ILA 2.0: C2 dan C3 dalam cycle, key level ERL LTF, entry di close konfirmasi CISD, target RR 1:2."
	}

	eval := &model.TradeEvaluation{
		Base: model.Base{
			ID:        uuid.New().String(),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
		PositionID:        pos.ID,
		TicketNumber:      pos.TicketNumber,
		Symbol:            pos.Symbol,
		Direction:         pos.Direction,
		SetupType:         setupType,
		ConfluenceGrade:   confluenceGrade,
		OpenPrice:         pos.OpenPrice,
		ExitPrice:         pos.ExitPrice,
		StopLoss:          pos.StopLoss,
		TakeProfit:        pos.TakeProfit,
		Outcome:           outcome,
		OutcomeR:          pos.OutcomeR,
		ProfitLoss:        pos.ProfitLoss,
		DurationMinutes:   durationMinutes,
		RootCause:         rootCause,
		TechnicalAnalysis: technicalAnalysis,
		ActionableLesson:  actionableLesson,
		NotifiedTelegram:  false,
		EvaluatedAt:       time.Now().UTC(),
	}

	if err := s.evalRepo.Create(ctx, eval); err != nil {
		return nil, fmt.Errorf("failed to save trade evaluation: %w", err)
	}

	return eval, nil
}

func (s *evaluationService) EvaluateAllClosedPositions(ctx context.Context) ([]*model.TradeEvaluation, error) {
	positions, _, err := s.posRepo.List(ctx, "", "", dto.PaginationQuery{Page: 1, PerPage: 100})
	if err != nil {
		return nil, err
	}

	var results []*model.TradeEvaluation
	for i := range positions {
		pos := &positions[i]
		if pos.Status == "OPEN" {
			continue
		}
		// Check if already evaluated
		existing, _ := s.evalRepo.FindByPositionID(ctx, pos.ID)
		if existing != nil {
			results = append(results, existing)
			continue
		}
		eval, err := s.EvaluateClosedTrade(ctx, pos)
		if err == nil && eval != nil {
			results = append(results, eval)
		}
	}

	return results, nil
}

func (s *evaluationService) GetUnnotifiedEvaluations(ctx context.Context) ([]model.TradeEvaluation, error) {
	return s.evalRepo.FindUnnotified(ctx)
}

func (s *evaluationService) MarkNotified(ctx context.Context, id string) error {
	return s.evalRepo.MarkNotified(ctx, id)
}

func (s *evaluationService) GetEvaluationSummary(ctx context.Context) (*dto.EvaluationStats, error) {
	return s.evalRepo.GetStats(ctx)
}

func (s *evaluationService) ListRecentEvaluations(ctx context.Context, limit int) ([]dto.TradeEvaluationDTO, error) {
	evals, err := s.evalRepo.ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.TradeEvaluationDTO, len(evals))
	for i, e := range evals {
		dtos[i] = dto.TradeEvaluationDTO{
			ID:                e.ID,
			TicketNumber:      e.TicketNumber,
			Symbol:            e.Symbol,
			Direction:         e.Direction,
			SetupType:         e.SetupType,
			ConfluenceGrade:   e.ConfluenceGrade,
			OpenPrice:         e.OpenPrice,
			ExitPrice:         e.ExitPrice,
			StopLoss:          e.StopLoss,
			TakeProfit:        e.TakeProfit,
			Outcome:           e.Outcome,
			OutcomeR:          e.OutcomeR,
			ProfitLoss:        e.ProfitLoss,
			DurationMinutes:   e.DurationMinutes,
			RootCause:         e.RootCause,
			TechnicalAnalysis: e.TechnicalAnalysis,
			ActionableLesson:  e.ActionableLesson,
			EvaluatedAt:       e.EvaluatedAt,
		}
	}
	return dtos, nil
}

func (s *evaluationService) PreFlightCheck(ctx context.Context, symbol, direction, setupType string, slDist float64) (*dto.PreFlightCheckResult, error) {
	res := &dto.PreFlightCheckResult{
		Passed:     true,
		CanExecute: true,
	}

	isGold := strings.Contains(symbol, "XAU") || strings.Contains(symbol, "GOLD")
	isIndex := strings.Contains(symbol, "NAS") || strings.Contains(symbol, "TEC") || strings.Contains(symbol, "US100") || strings.Contains(symbol, "US30") || strings.Contains(symbol, "US500") || strings.Contains(symbol, "SPX") || strings.Contains(symbol, "NDX")
	// Structural SL floor (docs/04): Gold $4.50, Forex 20 pips (0.0020).
	minAllowedSL := 4.50
	if isIndex {
		minAllowedSL = 0 // indeks bergerak per poin, tanpa floor
	} else if !isGold {
		minAllowedSL = 0.0020
	}

	if slDist > 0 && slDist < minAllowedSL {
		res.Passed = false
		res.CanExecute = false
		res.BlockingReason = fmt.Sprintf("Stop Loss (%.2f) terlalu tipis dari batas aman (minimal %.2f). Evaluasi sistem mencatat SL tipis memiliki tingkat kegagalan tinggi.", slDist, minAllowedSL)
		return res, nil
	}

	// Consult memory from past trade evaluations
	stats, err := s.evalRepo.GetStats(ctx)
	if err == nil && stats != nil {
		if len(stats.TopMistakes) > 0 {
			res.MemoryInsights = append(res.MemoryInsights, fmt.Sprintf("Perhatian Evaluasi Masa Lalu: Hindari %s", strings.Join(stats.TopMistakes, ", ")))
		}
		if len(stats.LearnedLessons) > 0 {
			res.MemoryInsights = append(res.MemoryInsights, fmt.Sprintf("Pelajaran Utama: %s", stats.LearnedLessons[0]))
		}
	}

	return res, nil
}
