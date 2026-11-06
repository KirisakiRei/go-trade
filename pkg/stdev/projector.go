package stdev

import "math"

type Projector struct{}

func NewProjector() *Projector {
	return &Projector{}
}

// CalculateProjection computes the ICT Standard Deviation Fibonacci projection levels
func (p *Projector) CalculateProjection(dir Direction, point1, point0 float64) ProjectionLevels {
	dist := math.Abs(point0 - point1)
	res := ProjectionLevels{
		Direction: dir,
		Point1:    point1,
		Point0:    point0,
		Distance:  dist,
	}

	if dir == DirectionBullish {
		res.LevelMinus1 = point0 + (1.0 * dist)
		res.LevelMinus2 = point0 + (2.0 * dist)
		res.LevelMinus2_5 = point0 + (2.5 * dist)
		res.LevelMinus3_5 = point0 + (3.5 * dist)
		res.LevelMinus4 = point0 + (4.0 * dist)
	} else {
		res.LevelMinus1 = point0 - (1.0 * dist)
		res.LevelMinus2 = point0 - (2.0 * dist)
		res.LevelMinus2_5 = point0 - (2.5 * dist)
		res.LevelMinus3_5 = point0 - (3.5 * dist)
		res.LevelMinus4 = point0 - (4.0 * dist)
	}

	return res
}

// CheckZoneStatus evaluates if current price is in the critical warning or final target zones
func (p *Projector) CheckZoneStatus(proj ProjectionLevels, currentPrice float64) ZoneStatus {
	status := ZoneStatus{
		CurrentPrice: currentPrice,
	}

	if proj.Direction == DirectionBullish {
		if currentPrice >= proj.LevelMinus2 && currentPrice <= proj.LevelMinus2_5 {
			status.InCriticalZone = true
			status.WarningMessage = "⚠️ Waspada: Harga memasuki Zona Kritis StDev (-2 s.d. -2.5). Potensi retracement atau reversal dini, amankan partial profit!"
		} else if currentPrice >= proj.LevelMinus3_5 {
			status.InFinalZone = true
			status.WarningMessage = "🎯 Target Final: Harga memasuki Zona Target StDev (-3.5 s.d. -4.0)."
		}
	} else {
		if currentPrice <= proj.LevelMinus2 && currentPrice >= proj.LevelMinus2_5 {
			status.InCriticalZone = true
			status.WarningMessage = "⚠️ Waspada: Harga memasuki Zona Kritis StDev (-2 s.d. -2.5). Potensi retracement atau reversal dini, amankan partial profit!"
		} else if currentPrice <= proj.LevelMinus3_5 {
			status.InFinalZone = true
			status.WarningMessage = "🎯 Target Final: Harga memasuki Zona Target StDev (-3.5 s.d. -4.0)."
		}
	}

	return status
}
