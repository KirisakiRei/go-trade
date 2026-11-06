package quarterly

import (
	"time"
)

type QuarterName string

const (
	QuarterQ1 QuarterName = "Q1" // Asia / Accumulation
	QuarterQ2 QuarterName = "Q2" // London / True Open / Manipulation
	QuarterQ3 QuarterName = "Q3" // NY AM / Expansion
	QuarterQ4 QuarterName = "Q4" // NY PM / Reversal / Rebalance
)

type CycleType string

const (
	CycleYearly  CycleType = "YEARLY"
	CycleMonthly CycleType = "MONTHLY"
	CycleWeekly  CycleType = "WEEKLY"
	CycleDaily   CycleType = "DAILY"
	Cycle90M     CycleType = "90M"
	CycleMicro   CycleType = "MICRO"
)

type QuarterInfo struct {
	Quarter     QuarterName `json:"quarter"`
	Cycle       CycleType   `json:"cycle"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     time.Time   `json:"end_time"`
	NYHour      int         `json:"ny_hour"`
	NYMinute    int         `json:"ny_minute"`
	IsQ1        bool        `json:"is_q1"`
	Description string      `json:"description"`
}

// TimeframePairing matches Key Level TF to Structure/SSMT TF according to Quarterly Theory (gap 2 tingkat)
type TimeframePairing struct {
	Cycle      CycleType `json:"cycle"`
	KeyLevelTF string    `json:"key_level_tf"`
	SSMTTF     string    `json:"ssmt_tf"`
}

var CyclePairings = map[CycleType]TimeframePairing{
	CycleYearly:  {Cycle: CycleYearly, KeyLevelTF: "3M", SSMTTF: "W"},
	CycleMonthly: {Cycle: CycleMonthly, KeyLevelTF: "W", SSMTTF: "4H"},
	CycleWeekly:  {Cycle: CycleWeekly, KeyLevelTF: "D", SSMTTF: "1H"},
	CycleDaily:   {Cycle: CycleDaily, KeyLevelTF: "4H", SSMTTF: "15m"},
	Cycle90M:     {Cycle: Cycle90M, KeyLevelTF: "1H", SSMTTF: "5m"},
	CycleMicro:   {Cycle: CycleMicro, KeyLevelTF: "15m", SSMTTF: "1m"},
}
