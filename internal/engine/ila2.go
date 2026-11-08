// Adapter ILA 2.0: memanggil evaluator yang sudah ada.
//
// Tidak ada aturan trading di file ini. Semua keputusan entry/SL/TP tetap
// dihitung oleh pkg/ila. Adapter hanya menerjemahkan bentuk hasilnya.
package engine

import (
	"context"
	"fmt"

	"ila-engine/internal/dto"
	"ila-engine/internal/service"
)

// ILA2Pairs adalah pasangan timeframe yang dipakai scanner ILA 2.0.
// Urutan dan isinya sengaja sama dengan cmd/scanner agar hasilnya identik.
var ILA2Pairs = []Pair{
	{HTF: "W", LTF: "H4"},
	{HTF: "D1", LTF: "H1"},
	{HTF: "H4", LTF: "M15"},
	{HTF: "H1", LTF: "M5"},
}

// ila2Evaluator membungkus service ILA 2.0.
type ila2Evaluator struct {
	svc   service.ILA2Service
	pairs []Pair
}

// NewILA2Evaluator merangkai adapter ILA 2.0.
func NewILA2Evaluator(svc service.ILA2Service) Evaluator {
	return &ila2Evaluator{svc: svc, pairs: ILA2Pairs}
}

func (e *ila2Evaluator) StrategyVersion() string { return "ila-2.0" }

func (e *ila2Evaluator) Pairs() []Pair { return e.pairs }

func (e *ila2Evaluator) Evaluate(symbol, htf, ltf string) (*Result, error) {
	setup, err := e.svc.AnalyzeILA2(context.Background(), dto.ILA2AnalysisRequest{
		Symbol: symbol,
		HTF:    htf,
		LTF:    ltf,
		Limit:  100,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluasi ILA 2.0 %s %s->%s: %w", symbol, htf, ltf, err)
	}
	if setup == nil {
		return nil, nil
	}

	return &Result{
		Status:      setup.Status,
		Direction:   directionOf(setup.Bias),
		Entry:       setup.EntryPrice,
		StopLoss:    setup.StopLoss,
		TakeProfit:  setup.TakeProfit,
		RiskReward:  setup.RiskReward,
		PassedMinRR: setup.PassedMinRR,
		Summary:     setup.Summary,
		NextAction:  setup.NextAction,
	}, nil
}

// directionOf menyelaraskan bias dengan arah order. Bias netral tidak
// menghasilkan arah, sehingga sinyalnya tidak akan terbit.
func directionOf(bias string) string {
	switch bias {
	case "BULLISH":
		return "BUY"
	case "BEARISH":
		return "SELL"
	default:
		return ""
	}
}
