package stdev

type Direction string

const (
	DirectionBullish Direction = "BULLISH"
	DirectionBearish Direction = "BEARISH"
)

type ProjectionLevels struct {
	Direction Direction `json:"direction"`
	Point1    float64   `json:"point_1"`    // Anchor 1: Structure extreme before MSS
	Point0    float64   `json:"point_0"`    // Anchor 0: Price at MSS confirm
	Distance  float64   `json:"distance"`   // Absolute range between Point 1 and Point 0

	LevelMinus1   float64 `json:"level_minus_1"`   // -1.0 extension
	LevelMinus2   float64 `json:"level_minus_2"`   // -2.0 critical warning zone start
	LevelMinus2_5 float64 `json:"level_minus_2_5"` // -2.5 critical warning zone end
	LevelMinus3_5 float64 `json:"level_minus_3_5"` // -3.5 final target zone start
	LevelMinus4   float64 `json:"level_minus_4"`   // -4.0 final target zone end
}

// ZoneStatus indicates where the current price is relative to the StDev zones
type ZoneStatus struct {
	InCriticalZone bool    `json:"in_critical_zone"` // Inside -2 to -2.5
	InFinalZone    bool    `json:"in_final_zone"`    // Inside -3.5 to -4.0
	CurrentPrice   float64 `json:"current_price"`
	WarningMessage string  `json:"warning_message"`
}
