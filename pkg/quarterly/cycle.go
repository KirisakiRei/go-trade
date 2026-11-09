package quarterly

import (
	"time"
)

type Calculator struct {
	nyLocation *time.Location
}

func NewCalculator() *Calculator {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback to fixed UTC-5 / UTC-4 approximation
		loc = time.FixedZone("EDT", -4*3600)
	}
	return &Calculator{nyLocation: loc}
}

// Location returns the America/New_York timezone location used for calculations.
func (c *Calculator) Location() *time.Location {
	return c.nyLocation
}

// GetCandleQuarter returns the Daily quarter name ("Q1", "Q2", "Q3", "Q4") for a given timestamp in America/New_York.
func (c *Calculator) GetCandleQuarter(t time.Time) string {
	qInfo := c.GetDailyQuarter(t)
	return string(qInfo.Quarter)
}

// GetCandleQuarter converts a timestamp to America/New_York and returns the Daily quarter string ("Q1", "Q2", "Q3", "Q4").
func GetCandleQuarter(t time.Time) string {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("EDT", -4*3600)
	}
	nyTime := t.In(loc)
	hour := nyTime.Hour()

	switch {
	case hour >= 18 && hour < 24:
		return string(QuarterQ1)
	case hour >= 0 && hour < 6:
		return string(QuarterQ2)
	case hour >= 6 && hour < 12:
		return string(QuarterQ3)
	default:
		return string(QuarterQ4)
	}
}

// GetDailyQuarter determines the current 6-hour Daily quarter based on New York Time
// Q1: 18:00 - 00:00 NY (Asia)
// Q2: 00:00 - 06:00 NY (London)
// Q3: 06:00 - 12:00 NY (NY AM)
// Q4: 12:00 - 18:00 NY (NY PM)
func (c *Calculator) GetDailyQuarter(t time.Time) QuarterInfo {
	nyTime := t.In(c.nyLocation)
	hour := nyTime.Hour()
	minute := nyTime.Minute()

	var q QuarterName
	var desc string

	switch {
	case hour >= 18 && hour < 24:
		q = QuarterQ1
		desc = "Q1: Asia Session (Accumulation / Initial Range)"
	case hour >= 0 && hour < 6:
		q = QuarterQ2
		desc = "Q2: London Session (Manipulation / True Open 00:00 NY)"
	case hour >= 6 && hour < 12:
		q = QuarterQ3
		desc = "Q3: NY AM Session (Primary Expansion / Distribution)"
	default:
		q = QuarterQ4
		desc = "Q4: NY PM Session (Reversal / Rebalance)"
	}

	return QuarterInfo{
		Quarter:     q,
		Cycle:       CycleDaily,
		NYHour:      hour,
		NYMinute:    minute,
		IsQ1:        q == QuarterQ1,
		Description: desc,
	}
}

// Get90MQuarter calculates the 90-minute sub-quarter inside a 6-hour quarter
func (c *Calculator) Get90MQuarter(t time.Time) QuarterInfo {
	nyTime := t.In(c.nyLocation)
	hour := nyTime.Hour()
	minute := nyTime.Minute()

	totalMinutesInDay := hour*60 + minute
	// Daily starts at 18:00 NY (18*60 = 1080 min)
	minutesFrom18 := (totalMinutesInDay - 1080 + 1440) % 1440

	subQuarterIndex := (minutesFrom18 % 360) / 90 // 0 to 3
	var q QuarterName
	switch subQuarterIndex {
	case 0:
		q = QuarterQ1
	case 1:
		q = QuarterQ2
	case 2:
		q = QuarterQ3
	case 3:
		q = QuarterQ4
	}

	return QuarterInfo{
		Quarter:  q,
		Cycle:    Cycle90M,
		NYHour:   hour,
		NYMinute: minute,
		IsQ1:     q == QuarterQ1,
	}
}

// CheckQ1Exception evaluates ILA 3.0 Rule 13:
// If in Q1 AND price has not reached HTF key level -> Eligible to start directly from MTF Key Level (ERL or IRL)
func (c *Calculator) CheckQ1Exception(t time.Time, htfKeyLevelReached bool) bool {
	qInfo := c.GetDailyQuarter(t)
	// Must be Q1 AND HTF key level not reached
	return qInfo.IsQ1 && !htfKeyLevelReached
}
