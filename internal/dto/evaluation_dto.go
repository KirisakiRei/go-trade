package dto

import "time"

type TradeEvaluationDTO struct {
	ID                string    `json:"id"`
	TicketNumber      string    `json:"ticket_number"`
	Symbol            string    `json:"symbol"`
	Direction         string    `json:"direction"`
	SetupType         string    `json:"setup_type"`
	ConfluenceGrade   string    `json:"confluence_grade"`
	OpenPrice         float64   `json:"open_price"`
	ExitPrice         float64   `json:"exit_price"`
	StopLoss          float64   `json:"stop_loss"`
	TakeProfit        float64   `json:"take_profit"`
	Outcome           string    `json:"outcome"`
	OutcomeR          float64   `json:"outcome_r"`
	ProfitLoss        float64   `json:"profit_loss"`
	DurationMinutes   int       `json:"duration_minutes"`
	RootCause         string    `json:"root_cause"`
	TechnicalAnalysis string    `json:"technical_analysis"`
	ActionableLesson  string    `json:"actionable_lesson"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type EvaluationStats struct {
	TotalTrades     int       `json:"total_trades"`
	WinningTrades   int       `json:"winning_trades"`
	LosingTrades    int       `json:"losing_trades"`
	WinRate         float64   `json:"win_rate"`
	TotalPnL        float64   `json:"total_pnl"`
	TotalR          float64   `json:"total_r"`
	AverageR        float64   `json:"average_r"`
	ProfitFactor    float64   `json:"profit_factor"`
	TopMistakes     []string  `json:"top_mistakes"`
	LearnedLessons  []string  `json:"learned_lessons"`
}

type PreFlightCheckResult struct {
	Passed          bool     `json:"passed"`
	CanExecute      bool     `json:"can_execute"`
	WarningMessages []string `json:"warnings"`
	BlockingReason  string   `json:"blocking_reason,omitempty"`
	MemoryInsights  []string `json:"memory_insights"`
}
