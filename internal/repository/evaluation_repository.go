package repository

import (
	"context"
	"math"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"

	"gorm.io/gorm"
)

type evaluationRepository struct {
	db *gorm.DB
}

func NewEvaluationRepository(db *gorm.DB) EvaluationRepository {
	return &evaluationRepository{db: db}
}

func (r *evaluationRepository) Create(ctx context.Context, eval *model.TradeEvaluation) error {
	return r.db.WithContext(ctx).Create(eval).Error
}

func (r *evaluationRepository) FindByPositionID(ctx context.Context, posID string) (*model.TradeEvaluation, error) {
	var eval model.TradeEvaluation
	err := r.db.WithContext(ctx).Where("position_id = ?", posID).First(&eval).Error
	if err != nil {
		return nil, err
	}
	return &eval, nil
}

func (r *evaluationRepository) FindByTicket(ctx context.Context, ticket string) (*model.TradeEvaluation, error) {
	var eval model.TradeEvaluation
	err := r.db.WithContext(ctx).Where("ticket_number = ?", ticket).First(&eval).Error
	if err != nil {
		return nil, err
	}
	return &eval, nil
}

func (r *evaluationRepository) FindUnnotified(ctx context.Context) ([]model.TradeEvaluation, error) {
	var evals []model.TradeEvaluation
	err := r.db.WithContext(ctx).Where("notified_telegram = ?", false).Order("evaluated_at ASC").Find(&evals).Error
	return evals, err
}

func (r *evaluationRepository) MarkNotified(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&model.TradeEvaluation{}).Where("id = ?", id).Update("notified_telegram", true).Error
}

func (r *evaluationRepository) ListRecent(ctx context.Context, limit int) ([]model.TradeEvaluation, error) {
	if limit <= 0 {
		limit = 10
	}
	var evals []model.TradeEvaluation
	err := r.db.WithContext(ctx).Order("evaluated_at DESC").Limit(limit).Find(&evals).Error
	return evals, err
}

func (r *evaluationRepository) GetStats(ctx context.Context) (*dto.EvaluationStats, error) {
	var evals []model.TradeEvaluation
	if err := r.db.WithContext(ctx).Order("evaluated_at DESC").Find(&evals).Error; err != nil {
		return nil, err
	}

	stats := &dto.EvaluationStats{
		TotalTrades: len(evals),
	}

	if len(evals) == 0 {
		return stats, nil
	}

	var totalProfit float64
	var totalLoss float64
	mistakeCounts := make(map[string]int)
	lessonSet := make(map[string]bool)

	for _, e := range evals {
		stats.TotalPnL += e.ProfitLoss
		stats.TotalR += e.OutcomeR

		if e.Outcome == "HIT_TP" || e.ProfitLoss > 0 {
			stats.WinningTrades++
			totalProfit += e.ProfitLoss
		} else if e.Outcome == "HIT_SL" || e.ProfitLoss < 0 {
			stats.LosingTrades++
			totalLoss += math.Abs(e.ProfitLoss)
			if e.RootCause != "" {
				mistakeCounts[e.RootCause]++
			}
		}

		if e.ActionableLesson != "" && !lessonSet[e.ActionableLesson] {
			lessonSet[e.ActionableLesson] = true
			stats.LearnedLessons = append(stats.LearnedLessons, e.ActionableLesson)
		}
	}

	if stats.TotalTrades > 0 {
		stats.WinRate = math.Round((float64(stats.WinningTrades)/float64(stats.TotalTrades))*1000) / 10
		stats.AverageR = math.Round((stats.TotalR/float64(stats.TotalTrades))*100) / 100
	}

	if totalLoss > 0 {
		stats.ProfitFactor = math.Round((totalProfit/totalLoss)*100) / 100
	} else if totalProfit > 0 {
		stats.ProfitFactor = 99.0
	}

	for m := range mistakeCounts {
		stats.TopMistakes = append(stats.TopMistakes, m)
	}

	return stats, nil
}
