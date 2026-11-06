package sequence

import (
	"time"

	"ila-engine/pkg/keylevels"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/ssmt"
	"ila-engine/pkg/stdev"
)

type SetupType string

const (
	SetupEntry1Reversal SetupType = "ENTRY_1_REVERSAL"
	SetupEntry2Retrace  SetupType = "ENTRY_2_RETRACE"
	SetupNone           SetupType = "NONE"
)

type SequenceState string

const (
	StateWaitingHTFKeyLevel SequenceState = "WAIT_HTF_KEY_LEVEL"
	StateWaitingMTFReversal SequenceState = "WAIT_MTF_REVERSAL"
	StateWaitingLTFCISD1    SequenceState = "WAIT_LTF_CISD_1"
	StateReadyEntry1        SequenceState = "READY_ENTRY_1"
	StateExpansion1Active   SequenceState = "EXPANSION_1_ACTIVE"
	StateWaitingMTFFVGRetr  SequenceState = "WAIT_MTF_FVG_RETRACE"
	StateWaitingLTFCISD2    SequenceState = "WAIT_LTF_CISD_2"
	StateReadyEntry2        SequenceState = "READY_ENTRY_2"
	StateExpansion2Active   SequenceState = "EXPANSION_2_ACTIVE"
	StateTargetReached      SequenceState = "TARGET_REACHED"
	StateInvalidated        SequenceState = "INVALIDATED"
)

type NestedExecutionInfo struct {
	HTFTargetPrice float64 `json:"htf_target_price"`
	HTFTargetName  string  `json:"htf_target_name"`
	LTFSLPrice     float64 `json:"ltf_sl_price"`
	RiskDistance   float64 `json:"risk_distance"`
	RewardDistance float64 `json:"reward_distance"`
	NestedRR       float64 `json:"nested_rr"`
	CriticalZoneTP float64 `json:"critical_zone_tp"` // -2.0 StDev
	FinalZoneTP    float64 `json:"final_zone_tp"`    // -3.5 StDev
}

type ILA3SequenceSetup struct {
	Symbol           string                    `json:"symbol"`
	HTF              string                    `json:"htf"`
	MTF              string                    `json:"mtf"`
	LTF              string                    `json:"ltf"`
	Bias             string                    `json:"bias"` // BULLISH or BEARISH
	State            SequenceState             `json:"state"`
	SetupType        SetupType                 `json:"setup_type"`
	QuarterInfo      quarterly.QuarterInfo     `json:"quarter_info"`
	IsQ1Bypass       bool                      `json:"is_q1_bypass"`
	SSMTResult       ssmt.SSMTResult           `json:"ssmt_result"`
	HTFStartLevel    *keylevels.ERL            `json:"htf_start_level,omitempty"`
	HTFTargetLevel   *keylevels.ERL            `json:"htf_target_level,omitempty"`
	MTFKeyLevel      *keylevels.ERL            `json:"mtf_key_level,omitempty"`
	MTFFVGRetrace    *keylevels.IRL            `json:"mtf_fvg_retrace,omitempty"`
	EntryPrice       float64                   `json:"entry_price"`
	StopLoss         float64                   `json:"stop_loss"`
	TakeProfit       float64                   `json:"take_profit"`
	RiskReward       float64                   `json:"risk_reward"`
	NestedExecution  *NestedExecutionInfo      `json:"nested_execution,omitempty"`
	CrossTFValid     bool                      `json:"cross_tf_valid"`
	WaypointFVG      *keylevels.IRL            `json:"waypoint_fvg,omitempty"`
	StDevProjections *stdev.ProjectionLevels   `json:"stdev_projections,omitempty"`
	EvaluatedAt      time.Time                 `json:"evaluated_at"`
	Summary          string                    `json:"summary"`
	NextAction       string                    `json:"next_action"`
}
