package dto

import "time"

type AnalysisRequest struct {
	Symbol    string `form:"symbol" json:"symbol" binding:"required"`
	Timeframe string `form:"timeframe" json:"timeframe" binding:"required"`
	Limit     int    `form:"limit,default=100" json:"limit" binding:"omitempty,min=50,max=500"`
}

type EvidenceItem struct {
	Condition       string `json:"condition"`
	Passed          bool   `json:"passed"`
	CandleTimestamp string `json:"candle_timestamp"`
	Details         string `json:"details"`
}

type EntryPlan struct {
	EntryZone       [2]float64 `json:"entry_zone"`
	StopLoss        float64    `json:"stop_loss"`
	TakeProfit      float64    `json:"take_profit"`
	RiskRewardRatio float64    `json:"risk_reward_ratio"`
}

type RiskCheck struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

type IndicatorSnapshot struct {
	BBUpper   float64 `json:"bb_upper"`
	BBMiddle  float64 `json:"bb_middle"`
	BBLower   float64 `json:"bb_lower"`
	WMA5High  float64 `json:"wma5_high"`
	WMA10High float64 `json:"wma10_high"`
	WMA5Low   float64 `json:"wma5_low"`
	WMA10Low  float64 `json:"wma10_low"`
	EMA50     float64 `json:"ema50"`
	LastClose float64 `json:"last_close"`
}

// AnalysisResponse adheres directly to Section 12 JSON contract.
type AnalysisResponse struct {
	Symbol                 string             `json:"symbol"`
	Timeframe              string             `json:"timeframe"`
	AsOf                   string             `json:"as_of"`
	DataQuality            string             `json:"data_quality"` // VALID, INSUFFICIENT_DATA, CORRUPTED
	MarketPhase            string             `json:"market_phase"`
	Bias                   string             `json:"bias"` // BULLISH, BEARISH, NEUTRAL, UNCONFIRMED
	SetupType              string             `json:"setup_type"`
	SetupStatus            string             `json:"setup_status"` // WAIT, CANDIDATE, SETUP_VALID, PLAN_VALIDATED, INVALIDATED, EXPIRED
	Evidence               []EvidenceItem     `json:"evidence"`
	MissingConditions      []string           `json:"missing_conditions"`
	InvalidationConditions []string           `json:"invalidation_conditions"`
	EntryPlan              *EntryPlan         `json:"entry_plan"`
	RiskCheck              RiskCheck          `json:"risk_check"`
	NextAction             string             `json:"next_action"`
	RulebookVersion        string             `json:"rulebook_version"`
	IndicatorConfigVersion string             `json:"indicator_config_version"`
	IndicatorsSnapshot     *IndicatorSnapshot `json:"indicators_snapshot,omitempty"`
}

type MonitorStatusResponse struct {
	IsRunning       bool      `json:"is_running"`
	MonitoredSymbol string    `json:"monitored_symbol"`
	ContextTF       string    `json:"context_tf"`
	EntryTF         string    `json:"entry_tf"`
	LastCheckedAt   time.Time `json:"last_checked_at"`
	LastSetupFound  string    `json:"last_setup_found"`
	ActiveAlerts    int       `json:"active_alerts"`
}

type MTFAnalysisRequest struct {
	Symbol string `form:"symbol" json:"symbol" binding:"required"`
	TF1    string `form:"tf1,default=H4" json:"tf1"`
	TF2    string `form:"tf2,default=H1" json:"tf2"`
	TF3    string `form:"tf3,default=M15" json:"tf3"`
}

type MTFAnalysisResponse struct {
	Symbol               string            `json:"symbol"`
	TF1                  string            `json:"tf1"`
	TF2                  string            `json:"tf2"`
	TF3                  string            `json:"tf3"`
	AsOf                 string            `json:"as_of"`
	ConfluenceGrade      string            `json:"confluence_grade"`
	ConfluencePattern    string            `json:"confluence_pattern"`
	Bias                 string            `json:"bias"`
	ActionRecommendation string            `json:"action_recommendation"`
	Summary              string            `json:"summary"`
	MarketSession        string            `json:"market_session"`
	IsHighLiquidity      bool              `json:"is_high_liquidity"`
	RecommendedRisk      string            `json:"recommended_risk"`
	TF1Analysis          *AnalysisResponse `json:"tf1_analysis"`
	TF2Analysis          *AnalysisResponse `json:"tf2_analysis"`
	TF3Analysis          *AnalysisResponse `json:"tf3_analysis"`
	EntryPlan            *EntryPlan        `json:"entry_plan,omitempty"`
}

type ILA2AnalysisRequest struct {
	Symbol string `form:"symbol" json:"symbol" binding:"required"`
	HTF    string `form:"htf,default=H4" json:"htf"`
	LTF    string `form:"ltf,default=M15" json:"ltf"`
	Limit  int    `form:"limit,default=100" json:"limit"`
}
