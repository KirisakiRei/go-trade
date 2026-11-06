package sequence

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

func TestSequenceEvaluation(t *testing.T) {
	now := time.Now()

	// HTF: Daily candles showing a swing high at index 2 (1.1260) and current candle reaches it
	htf := []indicators.Candle{
		{Timestamp: now.Add(-120 * time.Hour), High: 1.1170, Low: 1.1120, Close: 1.1150, IsClosed: true},
		{Timestamp: now.Add(-96 * time.Hour), High: 1.1200, Low: 1.1150, Close: 1.1180, IsClosed: true},
		{Timestamp: now.Add(-72 * time.Hour), High: 1.1260, Low: 1.1210, Close: 1.1240, IsClosed: true}, // Swing high 1.1260
		{Timestamp: now.Add(-48 * time.Hour), High: 1.1230, Low: 1.1180, Close: 1.1200, IsClosed: true},
		{Timestamp: now.Add(-24 * time.Hour), High: 1.1240, Low: 1.1190, Close: 1.1220, IsClosed: true},
		{Timestamp: now, High: 1.1265, Low: 1.1220, Close: 1.1230, IsClosed: true}, // Reaches/sweeps 1.1260
	}

	// MTF: 4H candles reaching MTF ERL
	mtf := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), High: 1.1220, Low: 1.1190, Close: 1.1200, IsClosed: true},
		{Timestamp: now.Add(-16 * time.Hour), High: 1.1240, Low: 1.1210, Close: 1.1220, IsClosed: true},
		{Timestamp: now.Add(-12 * time.Hour), High: 1.1255, Low: 1.1230, Close: 1.1250, IsClosed: true}, // MTF Swing high
		{Timestamp: now.Add(-8 * time.Hour), High: 1.1245, Low: 1.1220, Close: 1.1230, IsClosed: true},
		{Timestamp: now.Add(-4 * time.Hour), High: 1.1250, Low: 1.1225, Close: 1.1240, IsClosed: true},
		{Timestamp: now, High: 1.1256, Low: 1.1230, Close: 1.1240, IsClosed: true},
	}

	// LTF: 15m candles showing Bearish CISD confirmed
	ltf := []indicators.Candle{
		{Timestamp: now.Add(-75 * time.Minute), Open: 1.1210, High: 1.1225, Low: 1.1205, Close: 1.1220, IsClosed: true},
		{Timestamp: now.Add(-60 * time.Minute), Open: 1.1220, High: 1.1235, Low: 1.1215, Close: 1.1230, IsClosed: true},
		{Timestamp: now.Add(-45 * time.Minute), Open: 1.1230, High: 1.1240, Low: 1.1218, Close: 1.1235, IsClosed: true}, // Bullish origin Open = 1.1230
		{Timestamp: now.Add(-30 * time.Minute), Open: 1.1235, High: 1.1250, Low: 1.1230, Close: 1.1248, IsClosed: true}, // Peak high 1.1250
		{Timestamp: now.Add(-15 * time.Minute), Open: 1.1248, High: 1.1248, Low: 1.1200, Close: 1.1205, IsClosed: true}, // Bearish candle closes < 1.1210! (CISD)
		{Timestamp: now, Open: 1.1205, High: 1.1208, Low: 1.1200, Close: 1.1202, IsClosed: true},
	}

	eval := NewEvaluator()
	setup := eval.EvaluateSequence(htf, mtf, ltf, nil, "D", "4H", "15m", "EURUSD", "")

	if setup.Bias != "BEARISH" {
		t.Errorf("expected BEARISH bias from HTF swing high, got %s", setup.Bias)
	}
	if setup.State != StateReadyEntry1 {
		t.Errorf("expected StateReadyEntry1, got %s", setup.State)
	}
	if setup.StDevProjections == nil {
		t.Errorf("expected StDev projections to be drawn on CISD confirm")
	} else {
		if setup.StDevProjections.LevelMinus2 >= setup.EntryPrice {
			t.Errorf("in bearish projection, Level -2 (%.5f) should be below entry price (%.5f)",
				setup.StDevProjections.LevelMinus2, setup.EntryPrice)
		}
	}
}
