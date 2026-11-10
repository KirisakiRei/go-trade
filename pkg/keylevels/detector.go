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
// Menggunakan standar fraktal multi-bar ICT 5-bar (2 candle kiri dan 2 candle kanan).
func (d *Detector) FindERLs(candles []indicators.Candle) []ERL {
	n := len(candles)
	if n < 5 {
		return nil
	}

	var erls []ERL

	for i := 2; i < n-2; i++ {
		cCurr := candles[i]

		if !cCurr.IsClosed {
			continue
		}

		// Swing High (Fraktal 5-Bar: 2 bar kiri dan 2 bar kanan lebih rendah)
		if cCurr.High > candles[i-2].High &&
			cCurr.High > candles[i-1].High &&
			cCurr.High > candles[i+1].High &&
			cCurr.High > candles[i+2].High {

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

		// Swing Low (Fraktal 5-Bar: 2 bar kiri dan 2 bar kanan lebih tinggi)
		if cCurr.Low < candles[i-2].Low &&
			cCurr.Low < candles[i-1].Low &&
			cCurr.Low < candles[i+1].Low &&
			cCurr.Low < candles[i+2].Low {

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
// Aturan baku ILA 3.0:
// - Bullish FVG: Lantai = High Candle 1, Atap = Low Candle 3, Candle 2 wajib displacement.
// - Bearish FVG: Atap = Low Candle 1, Lantai = High Candle 3, Candle 2 wajib displacement.
// - FVG baru gugur (Mitigated / Fully Filled) HANYA jika harga telah 100% fully filled melewati batas seberang FVG.
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

		// Bearish FVG: Low(C1) > High(C3) and C2 is Bearish displacement
		if c1.Low > c3.High && c2.Close < c2.Open {
			irl := IRL{
				Type:        TypeIRLBearish,
				Top:         c1.Low,  // Atap FVG
				Bottom:      c3.High, // Lantai FVG
				Candle1Time: c1.Timestamp,
				Candle2Time: c2.Timestamp,
				Candle3Time: c3.Timestamp,
				CandleIndex: i,
			}
			// FVG baru gugur jika tertutup penuh (100% fully filled) melewati atap (Top)
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close > irl.Top {
					irl.Invalidated = true
					irl.FullyFilled = true
					irl.Mitigated = true
					break
				}
				if ck.High >= irl.Top {
					irl.FullyFilled = true
					irl.Mitigated = true
					break
				}
			}
			irls = append(irls, irl)
		}

		// Bullish FVG: High(C1) < Low(C3) and C2 is Bullish displacement
		if c1.High < c3.Low && c2.Close > c2.Open {
			irl := IRL{
				Type:        TypeIRLBullish,
				Top:         c3.Low,  // Atap FVG
				Bottom:      c1.High, // Lantai FVG
				Candle1Time: c1.Timestamp,
				Candle2Time: c2.Timestamp,
				Candle3Time: c3.Timestamp,
				CandleIndex: i,
			}
			// FVG baru gugur jika tertutup penuh (100% fully filled) melewati lantai (Bottom)
			for k := i + 1; k < n; k++ {
				ck := candles[k]
				if ck.Close < irl.Bottom {
					irl.Invalidated = true
					irl.FullyFilled = true
					irl.Mitigated = true
					break
				}
				if ck.Low <= irl.Bottom {
					irl.FullyFilled = true
					irl.Mitigated = true
					break
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
