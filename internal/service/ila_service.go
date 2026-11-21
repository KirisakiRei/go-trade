package service

import (
	"context"
	"fmt"
	"time"

	"ila-engine/internal/dto"
	"ila-engine/pkg/ila"
	"ila-engine/pkg/marketfeed"
)

type ilaService struct {
	feed      marketfeed.Provider
	evaluator *ila.Evaluator
}

func NewILA2Service(feed marketfeed.Provider) ILA2Service {
	return &ilaService{
		feed:      feed,
		evaluator: ila.NewEvaluator(),
	}
}

// MapDefaultLTF returns default LTF for a given HTF per New ILA 2.0 time-cycle table.
func (s *ilaService) MapDefaultLTF(htf string) string {
	if c, ok := ila.CycleForHTF(htf); ok {
		return ila.CycleSpec[c][1]
	}
	return "M15"
}

func (s *ilaService) AnalyzeILA2(ctx context.Context, req dto.ILA2AnalysisRequest) (*ila.ILASetup, error) {
	if req.Symbol == "" {
		req.Symbol = "XAUUSD"
	}
	if req.HTF == "" {
		req.HTF = "H4"
	}
	if req.LTF == "" {
		req.LTF = s.MapDefaultLTF(req.HTF)
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}

	// 1. Fetch HTF Candles
	htfCandles, err := s.feed.FetchCandles(ctx, req.Symbol, req.HTF, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HTF (%s) candles: %w", req.HTF, err)
	}
	if len(htfCandles) < 10 {
		return nil, fmt.Errorf("insufficient HTF candle data (got %d, need >= 10)", len(htfCandles))
	}

	// 2. Fetch LTF Candles
	ltfCandles, err := s.feed.FetchCandles(ctx, req.Symbol, req.LTF, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch LTF (%s) candles: %w", req.LTF, err)
	}
	if len(ltfCandles) < 10 {
		return nil, fmt.Errorf("insufficient LTF candle data (got %d, need >= 10)", len(ltfCandles))
	}

	// 3. Run ILA Evaluation
	setup := s.evaluator.EvaluateILA(htfCandles, ltfCandles, req.HTF, req.LTF, req.Symbol)
	return setup, nil
}

func (s *ilaService) AnalyzeAllPairs(ctx context.Context, symbol string) (*ila.ILAMultiResponse, error) {
	if symbol == "" {
		symbol = "XAUUSD"
	}

	pairs := []struct {
		name string
		htf  string
		ltf  string
	}{
		{"W ➡️ H4", "W", "H4"},
		{"D1 ➡️ H1", "D1", "H1"},
		{"H4 ➡️ M15", "H4", "M15"},
		{"H1 ➡️ M5", "H1", "M5"},
		{"M15 ➡️ M1", "M15", "M1"},
	}

	resp := &ila.ILAMultiResponse{
		Symbol:      symbol,
		EvaluatedAt: time.Now().UTC(),
		Pairs:       make([]ila.ILAPairResult, len(pairs)),
	}

	activeCount := 0
	for i, p := range pairs {
		setup, err := s.AnalyzeILA2(ctx, dto.ILA2AnalysisRequest{
			Symbol: symbol,
			HTF:    p.htf,
			LTF:    p.ltf,
			Limit:  100,
		})
		if err != nil {
			setup = &ila.ILASetup{
				Symbol:      symbol,
				HTF:         p.htf,
				LTF:         p.ltf,
				Status:      "ERROR",
				NextAction:  fmt.Sprintf("Gagal ambil data: %v", err),
				EvaluatedAt: time.Now().UTC(),
			}
		} else if setup.Status == "READY_TO_ENTRY" {
			activeCount++
		}
		resp.Pairs[i] = ila.ILAPairResult{
			PairName: p.name,
			HTF:      p.htf,
			LTF:      p.ltf,
			Setup:    setup,
		}
	}

	resp.ActiveSetups = activeCount
	if activeCount > 0 {
		resp.Summary = fmt.Sprintf("Ditemukan %d setup ILA siap eksekusi (READY TO ENTRY)!", activeCount)
	} else {
		resp.Summary = fmt.Sprintf("Semua %d pasangan timeframe dalam status pemantauan (WAIT).", len(pairs))
	}

	return resp, nil
}
