package repository

import (
	"context"
	"errors"

	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/pkg/apperror"

	"gorm.io/gorm"
)

type setupRepository struct {
	db *gorm.DB
}

func NewSetupRepository(db *gorm.DB) SetupRepository {
	return &setupRepository{db: db}
}

func (r *setupRepository) Create(ctx context.Context, setup *model.SetupRecord) error {
	return r.db.WithContext(ctx).Create(setup).Error
}

func (r *setupRepository) FindByID(ctx context.Context, id string) (*model.SetupRecord, error) {
	var setup model.SetupRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&setup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return &setup, nil
}

func (r *setupRepository) FindBySetupID(ctx context.Context, setupID string) (*model.SetupRecord, error) {
	var setup model.SetupRecord
	err := r.db.WithContext(ctx).Where("setup_id = ?", setupID).First(&setup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return &setup, nil
}

func (r *setupRepository) FindLatestBySymbol(ctx context.Context, symbol, timeframe string) (*model.SetupRecord, error) {
	var setup model.SetupRecord
	err := r.db.WithContext(ctx).
		Where("symbol = ? AND timeframe = ?", symbol, timeframe).
		Order("candle_timestamp desc").
		First(&setup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return &setup, nil
}

func (r *setupRepository) ListActive(ctx context.Context, symbol string, p dto.PaginationQuery) ([]model.SetupRecord, int64, error) {
	var setups []model.SetupRecord
	var total int64

	query := r.db.WithContext(ctx).Model(&model.SetupRecord{})
	if symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	query = query.Where("state IN ?", []string{"CANDIDATE", "CONFIRMED", "SETUP_VALID"})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (p.Page - 1) * p.PerPage
	err := query.Order("candle_timestamp desc").
		Offset(offset).
		Limit(p.PerPage).
		Find(&setups).Error

	return setups, total, err
}

func (r *setupRepository) UpdateState(ctx context.Context, setupID, state string) error {
	return r.db.WithContext(ctx).
		Model(&model.SetupRecord{}).
		Where("setup_id = ?", setupID).
		Update("state", state).Error
}
