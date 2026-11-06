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

	// Timestamps in UTC are needed for BOTH BBMA and ILA root-cause analysis
	// (rollover / false-break detection). Previously only the BBMA branch
	// computed these, and it used OpenedAt for closeHour by mistake.
	openHour := pos.OpenedAt.UTC().Hour()
	closeHour := openHour
	if pos.ClosedAt != nil {
		closeHour = pos.ClosedAt.UTC().Hour()
	}
	// Rollover subuh: 22:00-00:00 UTC (doc 04/06). Hour() is 0..23, so
	// `openHour < 0` was always false (dead condition). Include 00:00-01:00
	// UTC as the tail end of the rollover window.
	isRollover := openHour >= 22 || openHour < 1 || closeHour >= 22 || closeHour < 1

	if !isProfit {
		if isILA {
			// ILA losing-trade taxonomy. Previously this was a hardcoded
			// "ILA_SWEEP_INVALIDATION" for every loss, which made any
			// downstream evaluator (incl. Hermes) always report the same
			// cause. We now derive the real cause using the same signals the
			// BBMA branch uses (thin SL / rollover / fast false break / trend).
			isTightSL := (isGold && slDist < 4.50) || (!isGold && slDist < 0.0018)
			switch {
			case isTightSL:
				rootCause = "ILA_SL_TERLALU_TIPIS"
				technicalAnalysis = fmt.Sprintf("Jarak Stop Loss ILA (%.2f) berada di bawah batas struktural minimal. Retracement/wick normal menyapu level SL sebelum harga bergerak ke arah target DOL.", slDist)
				actionableLesson = "Pastikan SL ILA diletakkan tepat di Low/High Candle 5 dan memenuhi floor minimal (Gold $4.50 / Forex 20 pips) agar tidak tersapu noise."
			case isRollover:
				rootCause = "ILA_PELEBARAN_SPREAD_ROLLOVER"
				technicalAnalysis = fmt.Sprintf("Posisi ILA aktif saat rollover subuh (open %02d:xx UTC, close %02d:xx UTC). Spread melebar tajam dan memicu SL sebelum target DOL tercapai.", openHour, closeHour)
				actionableLesson = "Hindari menahan posisi ILA baru melewati jendela rollover 22:00-00:00 UTC untuk melindungi modal dari lonjakan spread."
			case durationMinutes <= 20:
				rootCause = "ILA_LIQUIDITY_SWEEP_FALSE_BREAK"
				technicalAnalysis = fmt.Sprintf("Harga berbalik cepat dalam %d menit setelah entry ILA, membentuk ekor jarum yang menembus titik invalidasi Candle 5.", durationMinutes)
				actionableLesson = "Tunggu konfirmasi body-break-body CISD yang matang; hindari entry saat transisi candle LTF belum stabil."
			default:
				rootCause = "ILA_SWEEP_INVALIDATION"
				technicalAnalysis = "Pergerakan harga menembus titik batas invalidasi (Low/High) Candle 5 sebelum mencapai target likuiditas DOL, mengindikasikan dorongan timeframe besar mendominasi."
				actionableLesson = "Patuhi batas invalidasi Candle 5. Jika harga menembus High/Low Candle 5, setup batal secara objektif tanpa penyesalan."
			}
		} else {
			// LOSING TRADE POST-MORTEM (BBMA)
			isTightSL := (isGold && slDist < 3.0) || (!isGold && slDist < 0.0018)

			if isTightSL {
				rootCause = "SL_TERLALU_TIPIS_MID_BB"
				technicalAnalysis = fmt.Sprintf("Stop Loss (jarak %.2f) diletakkan terlalu sempit di area Mid BB / dekat entry. Retracement wicks normal candle M15 menyapu level sebelum arah market berkembang.", slDist)
				actionableLesson = "Patuhi kaidah BBMA Oma Ally: Stop Loss WAJIB di luar Low BB (Buy) / Top BB (Sell) dengan batas minimal volatilitas 45 pips (Gold) atau 20 pips (Forex)."
			} else if isRollover {
				rootCause = "PELEBARAN_SPREAD_ROLLOVER"
				technicalAnalysis = fmt.Sprintf("Posisi aktif saat penutupan sesi New York dan rollover subuh (22:00-00:00 UTC / 05:00-07:00 WIB). Spread broker melonjak tajam dan memicu Stop Loss.")
				actionableLesson = "Kunci posisi atau hindari open order menjelang rollover subuh untuk melindungi modal dari lonjakan spread."
			} else if durationMinutes <= 20 {
				rootCause = "LIQUIDITY_SWEEP_FALSE_BREAK"
				technicalAnalysis = fmt.Sprintf("Harga bergerak cepat berlawanan arah dalam %d menit setelah open, membentuk ekor jarum (sweep) yang menembus level invalidasi.", durationMinutes)
				actionableLesson = "Wajib tunggu konfirmasi candle M15 closed resmi dan konfirmasi rejection wick sebelum order pasar dieksekusi."
			} else {
				rootCause = "TEKANAN_TREN_TF_BESAR"
				technicalAnalysis = "Struktur harga berbalik arah karena dorongan timeframe yang lebih tinggi (H4/H1) mendominasi pergerakan pasar."
				actionableLesson = "Pastikan 3 timeframe (H4, H1, M15) memiliki keselarasan arah penuh sebelum entry. Jika ada CSM berlawanan di H1, wajib batalkan Re-entry."
			}
		}
	} else {
		// WINNING TRADE REINFORCEMENT
		if isILA {
			rootCause = "EKSEKUSI_ILA_PRESISI"
			technicalAnalysis = fmt.Sprintf("Setup ILA berhasil mencapai target profit penuh (+%.2f R). HTF Sweep di area FVG tertutup valid dan konfirmasi CISD body-break-body di LTF memberikan pemicu pengantaran harga yang sangat presisi tanpa noise.", pos.OutcomeR)
			actionableLesson = "Pertahankan disiplin kaidah ILA: selalu tunggu HTF sweep candle closed resmi dan konfirmasi CISD body-break-body di candle 5 sebelum entry."
		} else {
			rootCause = "KESELARASAN_MTF_SEMPURNA"
			technicalAnalysis = fmt.Sprintf("Trade berhasil mencapai target profit (+%.2f R). Entry pada zona MA terbukti akurat dan didukung momentum yang sehat.", pos.OutcomeR)
			actionableLesson = "Pertahankan disiplin menunggu keselarasan MTF Grade A/A+ dengan rasio Risk to Reward terukur minimal 1:2."
		}
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
	// Structural SL floor per docs/04: Gold $4.50, Forex 20 pips (0.0020).
	minAllowedSL := 4.50
	if isIndex {
		minAllowedSL = 0 // indices move in whole points; floor not defined by policy
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
