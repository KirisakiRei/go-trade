package quarterly

import (
	"testing"
	"time"
)

func TestDailyQuarterCalculation(t *testing.T) {
	calc := NewCalculator()

	// 20:00 NY -> Q1 (Asia)
	tQ1 := time.Date(2026, 11, 5, 20, 0, 0, 0, calc.nyLocation)
	qInfo1 := calc.GetDailyQuarter(tQ1)
	if qInfo1.Quarter != QuarterQ1 || !qInfo1.IsQ1 {
		t.Errorf("expected Q1 at 20:00 NY, got %s", qInfo1.Quarter)
	}

	// 02:00 NY -> Q2 (London)
	tQ2 := time.Date(2026, 11, 6, 2, 0, 0, 0, calc.nyLocation)
	qInfo2 := calc.GetDailyQuarter(tQ2)
	if qInfo2.Quarter != QuarterQ2 {
		t.Errorf("expected Q2 at 02:00 NY, got %s", qInfo2.Quarter)
	}

	// 09:30 NY -> Q3 (NY AM)
	tQ3 := time.Date(2026, 11, 6, 9, 30, 0, 0, calc.nyLocation)
	qInfo3 := calc.GetDailyQuarter(tQ3)
	if qInfo3.Quarter != QuarterQ3 {
		t.Errorf("expected Q3 at 09:30 NY, got %s", qInfo3.Quarter)
	}

	// 14:00 NY -> Q4 (NY PM)
	tQ4 := time.Date(2026, 11, 6, 14, 0, 0, 0, calc.nyLocation)
	qInfo4 := calc.GetDailyQuarter(tQ4)
	if qInfo4.Quarter != QuarterQ4 {
		t.Errorf("expected Q4 at 14:00 NY, got %s", qInfo4.Quarter)
	}
}

func TestQ1ExceptionRule(t *testing.T) {
	calc := NewCalculator()

	tQ1 := time.Date(2026, 11, 5, 19, 0, 0, 0, calc.nyLocation)
	// Q1 + HTF not reached -> TRUE
	if !calc.CheckQ1Exception(tQ1, false) {
		t.Errorf("expected Q1 exception to be TRUE when in Q1 and HTF not reached")
	}

	// Q1 + HTF reached -> FALSE (follow normal alur)
	if calc.CheckQ1Exception(tQ1, true) {
		t.Errorf("expected Q1 exception to be FALSE when HTF already reached")
	}

	// Q2 + HTF not reached -> FALSE (not in Q1)
	tQ2 := time.Date(2026, 11, 6, 3, 0, 0, 0, calc.nyLocation)
	if calc.CheckQ1Exception(tQ2, false) {
		t.Errorf("expected Q1 exception to be FALSE in Q2")
	}
}

func TestGetCandleQuarter(t *testing.T) {
	calc := NewCalculator()

	// 20:00 NY -> "Q1"
	tQ1 := time.Date(2026, 11, 5, 20, 0, 0, 0, calc.nyLocation)
	if got := calc.GetCandleQuarter(tQ1); got != "Q1" {
		t.Errorf("calc.GetCandleQuarter(tQ1) = %s, want Q1", got)
	}
	if got := GetCandleQuarter(tQ1); got != "Q1" {
		t.Errorf("GetCandleQuarter(tQ1) = %s, want Q1", got)
	}

	// 03:00 NY -> "Q2"
	tQ2 := time.Date(2026, 11, 6, 3, 0, 0, 0, calc.nyLocation)
	if got := calc.GetCandleQuarter(tQ2); got != "Q2" {
		t.Errorf("calc.GetCandleQuarter(tQ2) = %s, want Q2", got)
	}
	if got := GetCandleQuarter(tQ2); got != "Q2" {
		t.Errorf("GetCandleQuarter(tQ2) = %s, want Q2", got)
	}

	// 08:30 NY -> "Q3"
	tQ3 := time.Date(2026, 11, 6, 8, 30, 0, 0, calc.nyLocation)
	if got := calc.GetCandleQuarter(tQ3); got != "Q3" {
		t.Errorf("calc.GetCandleQuarter(tQ3) = %s, want Q3", got)
	}
	if got := GetCandleQuarter(tQ3); got != "Q3" {
		t.Errorf("GetCandleQuarter(tQ3) = %s, want Q3", got)
	}

	// 16:00 NY -> "Q4"
	tQ4 := time.Date(2026, 11, 6, 16, 0, 0, 0, calc.nyLocation)
	if got := calc.GetCandleQuarter(tQ4); got != "Q4" {
		t.Errorf("calc.GetCandleQuarter(tQ4) = %s, want Q4", got)
	}
	if got := GetCandleQuarter(tQ4); got != "Q4" {
		t.Errorf("GetCandleQuarter(tQ4) = %s, want Q4", got)
	}
}
