package keylevels

import (
	"ila-engine/pkg/indicators"
)

// Detector handles extraction and mitigation tracking of ERL and IRL key levels
type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

// FindERLs extracts all Swing Highs and Swing Lows (External Range Liquidity)
func (d *Detector) FindERLs(candles []indicators.Candle) []ERL {
	n := len(candles)
	if n < 3 {
		return nil
	}

	var erls []ERL

	for i := 1; i < n-1; i++ {
		cPrev := candles[i-1]
		cCurr := candles[i]
		cNext := candles[i+1]

		if !cCurr.IsClosed {
			continue
		}

		// Swing High
		if cCurr.High > cPrev.High && cCurr.High > cNext.High {
			erl := ERL{
				Type:        TypeERLSwingHigh,
				Price:       cCurr.High,
				CandleTime:  cCurr.Timestamp,
				CandleIndex: i,
			}
			// Check mitigation/displacement in future candles
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close > erl.Price {
					erl.Displaced = true
					break
				} else if ck.High >= erl.Price {
					erl.Mitigated = true
				}
			}
			erls = append(erls, erl)
		}

		// Swing Low
		if cCurr.Low < cPrev.Low && cCurr.Low < cNext.Low {
			erl := ERL{
				Type:        TypeERLSwingLow,
				Price:       cCurr.Low,
				CandleTime:  cCurr.Timestamp,
				CandleIndex: i,
			}
			// Check mitigation/displacement in future candles
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close < erl.Price {
					erl.Displaced = true
					break
				} else if ck.Low <= erl.Price {
					erl.Mitigated = true
				}
			}
			erls = append(erls, erl)
		}
	}

	return erls
}

// FindIRLs extracts all Fair Value Gaps (Internal Range Liquidity)
func (d *Detector) FindIRLs(candles []indicators.Candle) []IRL {
	n := len(candles)
	if n < 3 {
		return nil
	}

	var irls []IRL

	for i := 2; i < n; i++ {
		c1 := candles[i-2]
		c2 := candles[i-1]
		c3 := candles[i]

		if !c1.IsClosed || !c2.IsClosed || !c3.IsClosed {
			continue
		}

		// Bearish FVG: Low(C1) > High(C3) and C2 is Bearish
		if c1.Low > c3.High && c2.Close < c2.Open {
			irl := IRL{
				Type:        TypeIRLBearish,
				Top:          c1.Low,
				Bottom:       c3.High,
				Candle1Time:  c1.Timestamp,
				Candle2Time:  c2.Timestamp,
				Candle3Time:  c3.Timestamp,
				CandleIndex: i,
			}
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close > irl.Top {
					irl.Invalidated = true
					break
				}
				if ck.High >= irl.Top {
					irl.FullyFilled = true
					irl.Mitigated = true
					break
				} else if ck.High >= irl.Bottom {
					irl.Mitigated = true
				}
			}
			irls = append(irls, irl)
		}

		// Bullish FVG: High(C1) < Low(C3) and C2 is Bullish
		if c1.High < c3.Low && c2.Close > c2.Open {
			irl := IRL{
				Type:        TypeIRLBullish,
				Top:          c3.Low,
				Bottom:       c1.High,
				Candle1Time:  c1.Timestamp,
				Candle2Time:  c2.Timestamp,
				Candle3Time:  c3.Timestamp,
				CandleIndex: i,
			}
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close < irl.Bottom {
					irl.Invalidated = true
					break
				}
				if ck.Low <= irl.Bottom {
					irl.FullyFilled = true
					irl.Mitigated = true
					break
				} else if ck.Low <= irl.Top {
					irl.Mitigated = true
				}
			}
			irls = append(irls, irl)
		}
	}

	return irls
}

// GetLatestActiveIRL returns the latest FVG that is neither fully filled nor invalidated
func (d *Detector) GetLatestActiveIRL(candles []indicators.Candle) *IRL {
	irls := d.FindIRLs(candles)
	for i := len(irls) - 1; i >= 0; i-- {
		if !irls[i].FullyFilled && !irls[i].Invalidated {
			return &irls[i]
		}
	}
	return nil
}

// GetLatestActiveERL returns the latest Swing High/Low that has not been displaced
func (d *Detector) GetLatestActiveERL(candles []indicators.Candle, erlType LevelType) *ERL {
	erls := d.FindERLs(candles)
	for i := len(erls) - 1; i >= 0; i-- {
		if erls[i].Type == erlType && !erls[i].Displaced {
			return &erls[i]
		}
	}
	return nil
}
