package repository

import (
	"context"
	"errors"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/pkg/apperror"

	"gorm.io/gorm"
)

type positionRepository struct {
	db *gorm.DB
}

func NewPositionRepository(db *gorm.DB) PositionRepository {
	return &positionRepository{db: db}
}

func (r *positionRepository) Create(ctx context.Context, pos *model.Position) error {
	return r.db.WithContext(ctx).Create(pos).Error
}

func (r *positionRepository) FindByTicket(ctx context.Context, ticket string) (*model.Position, error) {
	var pos model.Position
	err := r.db.WithContext(ctx).Where("ticket_number = ?", ticket).First(&pos).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return &pos, nil
}

func (r *positionRepository) FindOpenBySymbol(ctx context.Context, symbol string) ([]model.Position, error) {
	var positions []model.Position
	err := r.db.WithContext(ctx).
		Where("symbol = ? AND status = ?", symbol, "OPEN").
		Find(&positions).Error
	return positions, err
}

func (r *positionRepository) FindAllOpen(ctx context.Context) ([]model.Position, error) {
	var positions []model.Position
	err := r.db.WithContext(ctx).
		Where("status = ?", "OPEN").
		Find(&positions).Error
	return positions, err
}

func (r *positionRepository) List(ctx context.Context, symbol, status string, p dto.PaginationQuery) ([]model.Position, int64, error) {
	var positions []model.Position
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Position{})
	if symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (p.Page - 1) * p.PerPage
	err := query.Order("opened_at desc").
		Offset(offset).
		Limit(p.PerPage).
		Find(&positions).Error

	return positions, total, err
}

func (r *positionRepository) ClosePosition(ctx context.Context, ticket string, exitPrice, pnl, outcomeR float64, status, notes string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.Position{}).
		Where("ticket_number = ?", ticket).
		Updates(map[string]any{
			"status":       status,
			"exit_price":   exitPrice,
			"profit_loss":  pnl,
			"outcome_r":    outcomeR,
			"closed_at":    &now,
			"notes":        notes,
			"updated_at":   now,
		}).Error
}

func (r *positionRepository) CountTodayLosses(ctx context.Context) (int64, error) {
	var count int64
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	err := r.db.WithContext(ctx).Model(&model.Position{}).
		Where("closed_at >= ? AND (status = ? OR profit_loss < 0)", todayStart, "CLOSED_SL").
		Count(&count).Error
	return count, err
}
