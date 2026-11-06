package ssmt

import (
	"fmt"
	"time"

	"ila-engine/pkg/indicators"
)

type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

// EvaluateSSMT evaluates the High/Low sweep comparison between two correlated assets
// across the latest swing points or quarter ranges.
func (d *Detector) EvaluateSSMT(primarySym, corrSym string, primaryCandles, corrCandles []indicators.Candle, direction string) SSMTResult {
	nP := len(primaryCandles)
	nC := len(corrCandles)

	if nP < 2 || nC < 2 {
		return SSMTResult{
			Detected:         false,
			Type:             SMTNone,
			PrimarySymbol:    primarySym,
			CorrelatedSymbol: corrSym,
			Details:          "Data candle tidak cukup untuk evaluasi SMT",
		}
	}

	pCurr := primaryCandles[nP-1]
	pPrev := primaryCandles[nP-2]

	cCurr := corrCandles[nC-1]
	cPrev := corrCandles[nC-2]

	evalTime := pCurr.Timestamp
	if evalTime.IsZero() {
		evalTime = time.Now().UTC()
	}

	if direction == "BULLISH" {
		pSwept := pCurr.Low < pPrev.Low
		cSwept := cCurr.Low < cPrev.Low

		if pSwept && !cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTBullishDivergence,
				ProbabilityGrade: GradeHighProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     true,
				CorrelatedSwept:  false,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("BULLISH SMT DIVERGENCE: %s sweep Low (%.5f < %.5f), tetapi %s gagal sweep / Higher Low (%.5f >= %.5f). Konfluensi Probabilitas Tinggi!",
					primarySym, pCurr.Low, pPrev.Low, corrSym, cCurr.Low, cPrev.Low),
			}
		} else if !pSwept && cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTBullishDivergence,
				ProbabilityGrade: GradeHighProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     false,
				CorrelatedSwept:  true,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("BULLISH SMT DIVERGENCE: %s sweep Low (%.5f < %.5f), sementara %s bertahan kuat / Higher Low (%.5f >= %.5f). Konfluensi Probabilitas Tinggi!",
					corrSym, cCurr.Low, cPrev.Low, primarySym, pCurr.Low, pPrev.Low),
			}
		} else if pSwept && cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTNeutralSweep,
				ProbabilityGrade: GradeStandardProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     true,
				CorrelatedSwept:  true,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("NEUTRAL SWEEP: Kedua instrumen (%s & %s) sama-sama menyapu Low. Setup valid dengan probabilitas standar.",
					primarySym, corrSym),
			}
		}
	} else if direction == "BEARISH" {
		pSwept := pCurr.High > pPrev.High
		cSwept := cCurr.High > cPrev.High

		if pSwept && !cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTBearishDivergence,
				ProbabilityGrade: GradeHighProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     true,
				CorrelatedSwept:  false,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("BEARISH SMT DIVERGENCE: %s sweep High (%.5f > %.5f), tetapi %s gagal sweep / Lower High (%.5f <= %.5f). Konfluensi Probabilitas Tinggi!",
					primarySym, pCurr.High, pPrev.High, corrSym, cCurr.High, cPrev.High),
			}
		} else if !pSwept && cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTBearishDivergence,
				ProbabilityGrade: GradeHighProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     false,
				CorrelatedSwept:  true,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("BEARISH SMT DIVERGENCE: %s sweep High (%.5f > %.5f), sementara %s tertahan / Lower High (%.5f <= %.5f). Konfluensi Probabilitas Tinggi!",
					corrSym, cCurr.High, cPrev.High, primarySym, pCurr.High, pPrev.High),
			}
		} else if pSwept && cSwept {
			return SSMTResult{
				Detected:         true,
				Type:             SMTNeutralSweep,
				ProbabilityGrade: GradeStandardProbability,
				PrimarySymbol:    primarySym,
				CorrelatedSymbol: corrSym,
				PrimarySwept:     true,
				CorrelatedSwept:  true,
				EvalTime:         evalTime,
				Details: fmt.Sprintf("NEUTRAL SWEEP: Kedua instrumen (%s & %s) sama-sama menyapu High. Setup valid dengan probabilitas standar.",
					primarySym, corrSym),
			}
		}
	}

	return SSMTResult{
		Detected:         false,
		Type:             SMTNone,
		PrimarySymbol:    primarySym,
		CorrelatedSymbol: corrSym,
		EvalTime:         evalTime,
		Details:          "Tidak terdeteksi sweep maupun divergence pada instrumen korelasi.",
	}
}
