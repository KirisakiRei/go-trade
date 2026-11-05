package service

import (
	"context"
	"fmt"
	"time"

	"ila-engine/internal/model"
	"ila-engine/pkg/apperror"
	"ila-engine/pkg/marketfeed"

	"github.com/google/uuid"
)

type marketService struct {
	feed marketfeed.Provider
}

func NewMarketService(feed marketfeed.Provider) MarketService {
	return &marketService{feed: feed}
}

func (s *marketService) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]model.MarketCandle, error) {
	rawCandles, err := s.feed.FetchCandles(ctx, symbol, timeframe, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", apperror.ErrMarketFeedFailed, err)
	}

	result := make([]model.MarketCandle, len(rawCandles))
	for i, c := range rawCandles {
		result[i] = model.MarketCandle{
			Base: model.Base{
				ID:        uuid.New().String(),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Symbol:    symbol,
			Timeframe: timeframe,
			Timestamp: c.Timestamp,
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
			IsClosed:  c.IsClosed,
		}
	}

	return result, nil
}
