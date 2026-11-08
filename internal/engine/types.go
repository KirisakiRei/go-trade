package engine

import (
	"time"

	"ila-engine/internal/signal"
)

// Signal adalah bentuk sinyal yang dikirim engine ke Runner.
//
// Field harga diisi apa adanya oleh evaluator. Engine tidak menghitung lot,
// tidak membaca ekuitas, dan tidak menyentuh akun mana pun.
type Signal struct {
	ID          string
	Symbol      string
	HTF         string
	LTF         string
	Status      signal.Status
	Action      signal.Action
	Direction   string
	Entry       float64
	StopLoss    float64
	TakeProfit  float64
	RiskReward  float64
	PassedMinRR bool
	Summary     string
	NextAction  string

	PublishedAt time.Time
	ExpiresAt   time.Time

	// ILA 3.0. Kosong untuk ILA 2.0.
	SetupType      string
	SequenceState  string
	CriticalZoneTP float64
	FinalZoneTP    float64
	QuarterInfo    string
	SSMTResult     string

	SupersededBy string
	Reason       string
}

// Version menyimpan identitas engine dan strategi pada sinyal.
type Version struct {
	Engine   string
	Strategy string
}

// Evaluator menghasilkan setup untuk satu simbol pada satu pasangan timeframe.
//
// Implementasi nyata memanggil pkg/ila (ILA 2.0) atau pkg/sequence (ILA 3.0).
// Antarmuka ini yang membuat engine bisa menampung dua strategi tanpa
// menyalin aturan tradingnya.
type Evaluator interface {
	// StrategyVersion mengembalikan mis. "ila-2.0" atau "ila-3.0".
	StrategyVersion() string
	// Pairs mengembalikan pasangan timeframe yang dievaluasi per simbol.
	Pairs() []Pair
	// Evaluate menjalankan evaluasi untuk satu pasangan timeframe.
	Evaluate(symbol, htf, ltf string) (*Result, error)
}

// Pair adalah pasangan timeframe, mis. H4 -> M15.
type Pair struct {
	HTF string
	LTF string
}

// Result adalah keluaran evaluator dalam bentuk netral.
type Result struct {
	// Status memakai nilai internal evaluator. Pemetaan ke status sinyal
	// dilakukan oleh masing-masing adapter.
	Status      string
	Direction   string
	Entry       float64
	StopLoss    float64
	TakeProfit  float64
	RiskReward  float64
	PassedMinRR bool
	Summary     string
	NextAction  string

	SetupType      string
	SequenceState  string
	CriticalZoneTP float64
	FinalZoneTP    float64
	QuarterInfo    string
	SSMTResult     string
}
