package ila

// Tipe baku New ILA 2.0. Istilah mengikuti "New ILA 2.0/README.md":
//   - candle 1/2/3              : pembentuk FVG (SIBI/BISI)      -> key-level-irl.md
//   - candle kiri/tengah/kanan  : pembentuk swing high/low (ERL) -> key-level-erl.md
//   - C2, C3, C4, ...           : urutan candle HTF dari C2 aktif (tidak dipakai di LTF)

import (
	"time"

	"ila-engine/pkg/indicators"
)

// KeyLevelStatus adalah status key level IRL/ERL. Invalid bersifat permanen.
type KeyLevelStatus string

const (
	StatusValid KeyLevelStatus = "VALID"
	// StatusIFVG: SIBI/BISI invalid karena close body di luar batas zona.
	StatusIFVG KeyLevelStatus = "IFVG"
	// StatusFTM: swing high/low invalid karena close body di luar level swing.
	StatusFTM KeyLevelStatus = "FTM"
	// StatusReplaced: invalid karena terbentuk key level sejenis yang lebih baru (bukan iFVG/FTM).
	StatusReplaced KeyLevelStatus = "REPLACED"
)

// KeyLevelKind membedakan IRL dan ERL.
type KeyLevelKind string

const (
	KeyLevelIRL KeyLevelKind = "IRL"
	KeyLevelERL KeyLevelKind = "ERL"
)

// Arah setup (dipakai sebagai Bias/Direction di output).
const (
	DirBearish = "BEARISH"
	DirBullish = "BULLISH"
	DirNeutral = "NEUTRAL"
)

// Status setup. Nilainya sengaja sama dengan kontrak wire lama
// (internal/engine/status.go, proto engine.v1) agar runner/UI tidak rusak.
const (
	// StatusWaitC2: menunggu C2 closure di key level HTF (di dalam cycle).
	StatusWaitC2 = "WAIT_HTF_SWEEP"
	// StatusC2Running: calon C2 masih berjalan, status belum final.
	StatusC2Running = "WAIT_HTF_CANDLE_CLOSE"
	// StatusWaitCISD: C2 aktif, C3 berjalan di dalam cycle, menunggu CISD di LTF.
	StatusWaitCISD = "WAIT_LTF_CISD"
	// StatusReady: CISD terkonfirmasi selama C3 berjalan.
	StatusReady = "READY_TO_ENTRY"
	// StatusInvalid: C3 close tanpa setup.
	StatusInvalid = "INVALIDATED"
)

// FVGType adalah jenis key level IRL.
type FVGType string

const (
	FVGSIBI FVGType = "SIBI" // Bearish FVG
	FVGBISI FVGType = "BISI" // Bullish FVG
)

// FVG adalah key level IRL (SIBI/BISI) hasil 3 candle berurutan (candle 1, 2, 3).
//
//	SIBI: Top = low(candle 1), Bottom = high(candle 3), acuan awal = high(candle 3)
//	BISI: Top = low(candle 3), Bottom = high(candle 1), acuan awal = low(candle 3)
type FVG struct {
	Type         FVGType   `json:"type"`
	Top          float64   `json:"top"`
	Bottom       float64   `json:"bottom"`
	Candle1Index int       `json:"candle1_index"`
	Candle2Index int       `json:"candle2_index"`
	Candle3Index int       `json:"candle3_index"`
	Candle1Time  time.Time `json:"candle1_time"`
	Candle2Time  time.Time `json:"candle2_time"`
	Candle3Time  time.Time `json:"candle3_time"`
	// Status: VALID, IFVG, atau REPLACED. InvalidIndex = -1 bila masih valid.
	Status       KeyLevelStatus `json:"status"`
	InvalidIndex int            `json:"invalid_index"`
}

// InitialReference adalah level acuan awal C2 closure (c2-closure-irl.md A1/B1).
func (f FVG) InitialReference() float64 {
	if f.Type == FVGSIBI {
		return f.Bottom
	}
	return f.Top
}

// ERLType adalah jenis key level ERL.
type ERLType string

const (
	ERLSwingHigh ERLType = "SWING_HIGH"
	ERLSwingLow  ERLType = "SWING_LOW"
)

// SwingERL adalah key level ERL dari 3 candle (candle kiri, tengah, kanan).
type SwingERL struct {
	Type       ERLType   `json:"type"`
	Level      float64   `json:"level"`
	LeftIndex  int       `json:"left_index"`
	MidIndex   int       `json:"mid_index"`
	RightIndex int       `json:"right_index"`
	MidTime    time.Time `json:"mid_time"`
	// Status: VALID, FTM, atau REPLACED. InvalidIndex = -1 bila masih valid.
	Status       KeyLevelStatus `json:"status"`
	InvalidIndex int            `json:"invalid_index"`
}

// HTFSweep adalah konteks HTF: key level (ERL/IRL) dengan C2 closure aktif dan C3-nya.
// Nama struct & field JSON dipertahankan untuk kompatibilitas kontrak gRPC.
type HTFSweep struct {
	// Detected: C2 aktif valid di dalam cycle dan C3 sudah dimulai.
	Detected bool `json:"detected"`
	// SweepInProgress: calon C2 masih berjalan (status belum final).
	SweepInProgress bool   `json:"sweep_in_progress"`
	RemainingTime   string `json:"remaining_time,omitempty"`
	Direction       string `json:"direction"`
	// KeyLevelKind: IRL atau ERL. FVG terisi bila IRL, Swing terisi bila ERL.
	KeyLevelKind KeyLevelKind `json:"key_level_kind,omitempty"`
	FVG          *FVG         `json:"fvg"`
	Swing        *SwingERL    `json:"swing,omitempty"`
	Cycle        string       `json:"cycle,omitempty"`
	// SweepCandle = candle C2 aktif. SweptCandle = candle pembentuk key level
	// (candle 3 untuk IRL, candle tengah untuk ERL).
	SweepCandle      indicators.Candle `json:"sweep_candle"`
	SweptCandle      indicators.Candle `json:"swept_candle"`
	SweepCandleIndex int               `json:"sweep_candle_index"`
	SweepTime        time.Time         `json:"sweep_time"`
	C3Start          time.Time         `json:"c3_start"`
	C3End            time.Time         `json:"c3_end"`
	// StopLoss: ekstrem C2 (info konteks). SL posisi final dari CISD LTF.
	StopLoss float64 `json:"stop_loss"`
	// TargetDOL tidak dipakai di New ILA 2.0 (target = RR 1:2). Selalu 0.
	TargetDOL float64  `json:"target_dol"`
	Details   string   `json:"details"`
	C2Status  C2Status `json:"c2_status,omitempty"`
	C2Detail  string   `json:"c2_detail,omitempty"`
}

// CISDCondition adalah kondisi key level ERL LTF (cisd.md A1/B1).
type CISDCondition string

const (
	CISDKondisi1 CISDCondition = "KONDISI_1" // ERL valid + terkena wick
	CISDKondisi2 CISDCondition = "KONDISI_2" // ERL FTM + iFVG terbaru di rangkaian
	CISDKondisi3 CISDCondition = "KONDISI_3" // ERL FTM tanpa iFVG + SWL/SWH FTM
)

// CISD adalah hasil -CISD / +CISD di LTF.
type CISD struct {
	Detected  bool          `json:"detected"`
	Direction string        `json:"direction"`
	Condition CISDCondition `json:"condition,omitempty"`
	// RefCandle = candle pertama rangkaian (sumber level CISD).
	RefCandle indicators.Candle `json:"ref_candle"`
	// BreakCandle = candle konfirmasi (close-nya = entry).
	BreakCandle indicators.Candle `json:"break_candle"`
	BreakTime   time.Time         `json:"break_time"`
	CISDLevel   float64           `json:"cisd_level"`
	// KeySwing = swing ERL LTF terbaru yang dipakai sebagai key level.
	KeySwing *SwingERL `json:"key_swing,omitempty"`
	// ExtremeSL = puncak rangkaian bullish (-CISD) / dasar rangkaian bearish (+CISD).
	ExtremeSL  float64 `json:"extreme_sl"`
	EntryPrice float64 `json:"entry_price"`
	Details    string  `json:"details"`
}

// ILASetup adalah hasil evaluasi lengkap satu pasangan HTF -> LTF.
type ILASetup struct {
	Symbol      string    `json:"symbol"`
	HTF         string    `json:"htf"`
	LTF         string    `json:"ltf"`
	Cycle       string    `json:"cycle,omitempty"`
	Bias        string    `json:"bias"`
	Status      string    `json:"status"`
	HTFSweep    *HTFSweep `json:"htf_sweep,omitempty"`
	CISD        *CISD     `json:"cisd,omitempty"`
	EntryPrice  float64   `json:"entry_price"`
	StopLoss    float64   `json:"stop_loss"`
	TakeProfit  float64   `json:"take_profit"`
	RiskReward  float64   `json:"risk_reward"`
	PassedMinRR bool      `json:"passed_min_rr"`
	NextAction  string    `json:"next_action"`
	Summary     string    `json:"summary"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

type ILAPairResult struct {
	PairName string    `json:"pair_name"`
	HTF      string    `json:"htf"`
	LTF      string    `json:"ltf"`
	Setup    *ILASetup `json:"setup"`
}

type ILAMultiResponse struct {
	Symbol       string          `json:"symbol"`
	EvaluatedAt  time.Time       `json:"evaluated_at"`
	Pairs        []ILAPairResult `json:"pairs"`
	ActiveSetups int             `json:"active_setups"`
	Summary      string          `json:"summary"`
}
