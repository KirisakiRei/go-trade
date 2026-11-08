package engine

import (
	"time"
)

// UjiEvaluator adalah evaluator tiruan untuk menguji alur sinyal tanpa data
// pasar. Hanya dipakai pada pengujian dan `-demo`.
type UjiEvaluator struct {
	Strategy string
	PairList []Pair
	Hasil    map[string]*Result
}

// NewUjiEvaluator membuat evaluator tiruan dengan satu setup ready.
func NewUjiEvaluator(strategy string, pairs []Pair, hasil map[string]*Result) *UjiEvaluator {
	return &UjiEvaluator{Strategy: strategy, PairList: pairs, Hasil: hasil}
}

func (u *UjiEvaluator) StrategyVersion() string { return u.Strategy }

func (u *UjiEvaluator) Pairs() []Pair { return u.PairList }

func (u *UjiEvaluator) Evaluate(symbol, htf, ltf string) (*Result, error) {
	if r, ok := u.Hasil[htf+"|"+ltf]; ok {
		return r, nil
	}
	return &Result{Status: "WAIT_HTF_SWEEP"}, nil
}

// DemoResult membuat setup ready yang selalu berlaku 5 menit.
func DemoResult() *Result {
	return &Result{
		Status: "READY_TO_ENTRY", Direction: "BUY",
		Entry: 4120.50, StopLoss: 4112.30, TakeProfit: 4149.20, RiskReward: 3.5,
		PassedMinRR: true, Summary: "setup uji dari engine",
		NextAction: "siap entry",
	}
}

var _ = time.Now
