package ssmt

import "time"

type SMTType string

const (
	SMTBullishDivergence SMTType = "BULLISH_SMT_DIVERGENCE"
	SMTBearishDivergence SMTType = "BEARISH_SMT_DIVERGENCE"
	SMTNeutralSweep      SMTType = "NEUTRAL_SWEEP"
	SMTNone              SMTType = "NO_DIVERGENCE"
)

type ProbabilityGrade string

const (
	GradeHighProbability     ProbabilityGrade = "HIGH_PROBABILITY"     // SMT Divergence confirmed
	GradeStandardProbability ProbabilityGrade = "STANDARD_PROBABILITY" // Neutral Sweep (both swept)
)

type SSMTResult struct {
	Detected         bool             `json:"detected"`
	Type             SMTType          `json:"type"`
	ProbabilityGrade ProbabilityGrade `json:"probability_grade"`
	PrimarySymbol    string           `json:"primary_symbol"`
	CorrelatedSymbol string           `json:"correlated_symbol"`
	PrimarySwept     bool             `json:"primary_swept"`
	CorrelatedSwept  bool             `json:"correlated_swept"`
	EvalTime         time.Time        `json:"eval_time"`
	Details          string           `json:"details"`
}

// DefaultCorrelationPairs maps default pairs for SMT comparison
var DefaultCorrelationPairs = map[string]string{
	"EURUSD": "GBPUSD",
	"GBPUSD": "EURUSD",
	"XAUUSD": "XAGUSD",
	"XAGUSD": "XAUUSD",
	"USTEC":  "US500",
	"NAS100": "US500",
	"US100":  "US500",
}
