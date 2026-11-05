package repository

import (
	"context"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"

	"gorm.io/gorm"
)

type journalRepository struct {
	db *gorm.DB
}

func NewJournalRepository(db *gorm.DB) JournalRepository {
	return &journalRepository{db: db}
}

func (r *journalRepository) Create(ctx context.Context, journal *model.TradeJournal) error {
	return r.db.WithContext(ctx).Create(journal).Error
}

func (r *journalRepository) List(ctx context.Context, symbol string, p dto.PaginationQuery) ([]model.TradeJournal, int64, error) {
	var journals []model.TradeJournal
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TradeJournal{})
	if symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (p.Page - 1) * p.PerPage
	err := query.Order("evaluated_at desc").
		Offset(offset).
		Limit(p.PerPage).
		Find(&journals).Error

	return journals, total, err
}
