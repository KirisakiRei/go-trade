// Adapter ILA 3.0: memanggil evaluator yang sudah ada.
//
// Tidak ada aturan trading di file ini. Cara memuat candle dan pasangan
// timeframe disamakan persis dengan cmd/ila3-scanner agar hasil evaluasinya
// identik. Semua keputusan entry/SL/TP tetap milik pkg/sequence.
package engine

import (
	"context"
	"fmt"

	"ila-engine/pkg/indicators"
	"ila-engine/pkg/marketfeed"
	"ila-engine/pkg/sequence"
)

// ila3Preset adalah pasangan timeframe ILA 3.0, sama dengan cmd/ila3-scanner.
type ila3Preset struct {
	Mode string
	HTF  string
	MTF  string
	LTF  string
}

var ila3Presets = []ila3Preset{
	{Mode: "New Day", HTF: "4H", MTF: "1H", LTF: "5m"},
	{Mode: "New Week", HTF: "D", MTF: "4H", LTF: "15m"},
}

// CorrelationMap memasangkan simbol dengan pembanding SMT-nya.
// Salinan nilai dari cmd/ila3-scanner; pemetaan ini bukan aturan entry.
var correlationMap = map[string]string{
	"XAUUSD": "XAGUSD",
	"EURUSD": "GBPUSD",
	"USTEC":  "US500",
}

// ila3Evaluator membungkus evaluator sequence ILA 3.0.
type ila3Evaluator struct {
	feed     marketfeed.Provider
	eval     *sequence.Evaluator
	pairs    []Pair
	presetOf map[string]ila3Preset
}

// NewILA3Evaluator merangkai adapter ILA 3.0.
func NewILA3Evaluator(feed marketfeed.Provider) Evaluator {
	e := &ila3Evaluator{
		feed:     feed,
		eval:     sequence.NewEvaluator(),
		presetOf: make(map[string]ila3Preset, len(ila3Presets)),
	}
	// Slot dikunci per (htf, ltf) agar bisa dipetakan balik ke preset saat evaluasi.
	seen := make(map[string]ila3Preset, len(ila3Presets))
	for _, p := range ila3Presets {
		seen[p.HTF+"|"+p.LTF] = p
	}
	e.presetOf = seen
	for _, p := range ila3Presets {
		e.pairs = append(e.pairs, Pair{HTF: p.HTF, LTF: p.LTF})
	}
	return e
}

func (e *ila3Evaluator) StrategyVersion() string { return "ila-3.0" }

func (e *ila3Evaluator) Pairs() []Pair { return e.pairs }

func (e *ila3Evaluator) Evaluate(symbol, htf, ltf string) (*Result, error) {
	preset, ok := e.presetOf[htf+"|"+ltf]
	if !ok {
		return nil, fmt.Errorf("pasangan timeframe ILA 3.0 tidak dikenal: %s->%s", htf, ltf)
	}

	ctx := context.Background()
	hCandles, err := e.feed.FetchCandles(ctx, symbol, preset.HTF, 40)
	if err != nil {
		return nil, fmt.Errorf("candle HTF %s: %w", preset.HTF, err)
	}
	mCandles, err := e.feed.FetchCandles(ctx, symbol, preset.MTF, 40)
	if err != nil {
		return nil, fmt.Errorf("candle MTF %s: %w", preset.MTF, err)
	}
	lCandles, err := e.feed.FetchCandles(ctx, symbol, preset.LTF, 40)
	if err != nil {
		return nil, fmt.Errorf("candle LTF %s: %w", preset.LTF, err)
	}

	corrSym := correlationMap[symbol]
	var corrCandles []indicators.Candle
	if corrSym != "" {
		// Pembanding SMT boleh gagal: evaluator menanganinya sebagai tanpa SMT.
		corrCandles, _ = e.feed.FetchCandles(ctx, corrSym, preset.MTF, 40)
	}

	setup := e.eval.EvaluateSequence(hCandles, mCandles, lCandles, corrCandles,
		preset.HTF, preset.MTF, preset.LTF, symbol, corrSym)
	if setup == nil {
		return nil, nil
	}

	res := &Result{
		Status:        string(setup.State),
		Direction:     directionOf(setup.Bias),
		Entry:         setup.EntryPrice,
		StopLoss:      setup.StopLoss,
		TakeProfit:    setup.TakeProfit,
		RiskReward:    setup.RiskReward,
		Summary:       setup.Summary,
		NextAction:    setup.NextAction,
		SequenceState: string(setup.State),
		SSMTResult:    setup.SSMTResult.Details,
		QuarterInfo:   setup.QuarterInfo.Description,
	}
	if setup.SetupType != sequence.SetupNone {
		res.SetupType = string(setup.SetupType)
	}
	if setup.NestedExecution != nil {
		res.CriticalZoneTP = setup.NestedExecution.CriticalZoneTP
		res.FinalZoneTP = setup.NestedExecution.FinalZoneTP
	}
	return res, nil
}
