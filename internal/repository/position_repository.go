package repository

import (
	"context"
	"errors"
	"fmt"
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
			"status":      status,
			"exit_price":  exitPrice,
			"profit_loss": pnl,
			"outcome_r":   outcomeR,
			"closed_at":   &now,
			"notes":       notes,
			"updated_at":  now,
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

// GetDailyTradeStatusWIB evaluates daily trade quota based on Jakarta Time (WIB / UTC+7):
// Rules:
// 1. Max 2 trades per calendar day (WIB).
// 2. If Trade 1 is WIN (CLOSED_TP or profit_loss > 0) -> STOP trading for the day.
// 3. If Trade 1 is LOSS -> Allow Trade 2.
// 4. If Trade 2 is completed (WIN or LOSS) -> STOP trading (quota 2 trades reached).
// 5. If any position is currently OPEN -> Lock new trades.
func (r *positionRepository) GetDailyTradeStatusWIB(ctx context.Context, now time.Time) (*DailyTradeStatus, error) {
	loc := time.FixedZone("WIB", 7*3600)
	nowWIB := now.In(loc)
	startWIB := time.Date(nowWIB.Year(), nowWIB.Month(), nowWIB.Day(), 0, 0, 0, 0, loc).UTC()
	endWIB := startWIB.Add(24 * time.Hour)
	dateStr := nowWIB.Format("2006-01-02")

	// 1. Cek semua posisi terbuka saat ini di database
	var allOpen []model.Position
	if err := r.db.WithContext(ctx).Model(&model.Position{}).Where("status = ?", "OPEN").Find(&allOpen).Error; err != nil {
		return nil, err
	}
	if len(allOpen) > 0 {
		return &DailyTradeStatus{
			DateWIB:    dateStr,
			OpenTrades: len(allOpen),
			CanTrade:   false,
			LockReason: fmt.Sprintf("Masih ada posisi terbuka (%s %s) yang belum ditutup broker.", allOpen[0].Symbol, allOpen[0].Direction),
		}, nil
	}

	// 2. Ambil semua posisi yang dibuka pada hari ini (WIB window)
	var todayPositions []model.Position
	if err := r.db.WithContext(ctx).Model(&model.Position{}).
		Where("opened_at >= ? AND opened_at < ?", startWIB, endWIB).
		Find(&todayPositions).Error; err != nil {
		return nil, err
	}

	totalTrades := len(todayPositions)
	winTrades := 0
	lossTrades := 0

	for _, p := range todayPositions {
		if p.Status == "CLOSED_TP" || p.ProfitLoss > 0 {
			winTrades++
		} else if p.Status == "CLOSED_SL" || p.ProfitLoss < 0 {
			lossTrades++
		}
	}

	res := &DailyTradeStatus{
		DateWIB:       dateStr,
		TotalTrades:   totalTrades,
		WinningTrades: winTrades,
		LosingTrades:  lossTrades,
		OpenTrades:    0,
	}

	// Evaluasi aturan harian:
	// Rule 1: Jika sudah 1x WIN -> Selesai / Kunci eksekusi otomatis
	if winTrades >= 1 {
		res.CanTrade = false
		res.LockReason = fmt.Sprintf("Sudah ada 1 WIN hari ini (%s WIB). Auto-eksekusi ditutup untuk mengamankan profit harian.", dateStr)
		return res, nil
	}

	// Rule 2: Jika sudah 2 trade (apapun hasilnya) -> Jatah habis
	if totalTrades >= 2 {
		res.CanTrade = false
		res.LockReason = fmt.Sprintf("Kuota 2 trade harian (%s WIB) sudah terpakai. Auto-eksekusi ditutup demi menjaga batas drawdown prop firm.", dateStr)
		return res, nil
	}

	// Rule 3: Jika trade 1 kalah -> Buka kesempatan trade ke-2
	if totalTrades == 1 && lossTrades == 1 {
		res.CanTrade = true
		res.LockReason = "" // Kesempatan trade ke-2 diizinkan
		return res, nil
	}

	// Rule 4: Belum ada trade hari ini -> Buka kesempatan trade ke-1
	res.CanTrade = true
	res.LockReason = ""
	return res, nil
}
