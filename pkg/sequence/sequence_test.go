package sequence

import (
	"math"
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

func TestSequenceLateQ4Expiration(t *testing.T) {
	eval := NewEvaluator()
	nyLoc := eval.qtCalc.Location()

	// Buat waktu candle di sesi penutupan Q4 sore (16:00 NY)
	tQ4Late := time.Date(2026, 11, 6, 16, 0, 0, 0, nyLoc)

	htf := []indicators.Candle{
		{Timestamp: tQ4Late.Add(-120 * time.Hour), High: 100, Low: 90, Close: 95, IsClosed: true},
		{Timestamp: tQ4Late.Add(-96 * time.Hour), High: 105, Low: 95, Close: 100, IsClosed: true},
		{Timestamp: tQ4Late.Add(-72 * time.Hour), High: 110, Low: 100, Close: 105, IsClosed: true},
		{Timestamp: tQ4Late.Add(-48 * time.Hour), High: 105, Low: 98, Close: 102, IsClosed: true},
		{Timestamp: tQ4Late.Add(-24 * time.Hour), High: 104, Low: 99, Close: 101, IsClosed: true},
		{Timestamp: tQ4Late, High: 112, Low: 100, Close: 108, IsClosed: true},
	}
	mtf := []indicators.Candle{
		{Timestamp: tQ4Late.Add(-20 * time.Hour), High: 105, Low: 95, Close: 100, IsClosed: true},
		{Timestamp: tQ4Late.Add(-16 * time.Hour), High: 106, Low: 96, Close: 101, IsClosed: true},
		{Timestamp: tQ4Late.Add(-12 * time.Hour), High: 111, Low: 100, Close: 105, IsClosed: true},
		{Timestamp: tQ4Late.Add(-8 * time.Hour), High: 107, Low: 98, Close: 103, IsClosed: true},
		{Timestamp: tQ4Late.Add(-4 * time.Hour), High: 108, Low: 99, Close: 104, IsClosed: true},
		{Timestamp: tQ4Late, High: 112, Low: 100, Close: 107, IsClosed: true},
	}
	ltf := []indicators.Candle{
		{Timestamp: tQ4Late.Add(-45 * time.Minute), Open: 105, High: 106, Low: 104, Close: 105.5, IsClosed: true},
		{Timestamp: tQ4Late.Add(-30 * time.Minute), Open: 105.5, High: 107, Low: 105, Close: 106.8, IsClosed: true},
		{Timestamp: tQ4Late.Add(-15 * time.Minute), Open: 106.8, High: 107, Low: 103, Close: 103.5, IsClosed: true},
		{Timestamp: tQ4Late, Open: 103.5, High: 104, Low: 103, Close: 103.8, IsClosed: true},
		{Timestamp: tQ4Late, Open: 103.8, High: 104, Low: 103.5, Close: 103.6, IsClosed: true},
	}

	setup := eval.EvaluateSequence(htf, mtf, ltf, nil, "D", "4H", "15m", "EURUSD", "")
	if setup.State != StateExpired {
		t.Errorf("expected StateExpired in late Q4 (16:00 NY), got %s", setup.State)
	}
}

func TestGoldSLFloorAndRRClamp(t *testing.T) {
	eval := NewEvaluator()
	nyLoc := eval.qtCalc.Location()

	// Sesi Q3 (09:00 NY)
	tQ3 := time.Date(2026, 11, 6, 9, 0, 0, 0, nyLoc)

	htf := []indicators.Candle{
		{Timestamp: tQ3.Add(-120 * time.Hour), High: 2700, Low: 2680, Close: 2690, IsClosed: true},
		{Timestamp: tQ3.Add(-96 * time.Hour), High: 2710, Low: 2690, Close: 2700, IsClosed: true},
		{Timestamp: tQ3.Add(-72 * time.Hour), High: 2750, Low: 2710, Close: 2740, IsClosed: true}, // Swing High 2750
		{Timestamp: tQ3.Add(-48 * time.Hour), High: 2730, Low: 2700, Close: 2720, IsClosed: true},
		{Timestamp: tQ3.Add(-24 * time.Hour), High: 2735, Low: 2705, Close: 2725, IsClosed: true},
		{Timestamp: tQ3, High: 2755, Low: 2720, Close: 2730, IsClosed: true}, // Reaches/sweeps 2750
	}
	mtf := []indicators.Candle{
		{Timestamp: tQ3.Add(-20 * time.Hour), High: 2720, Low: 2700, Close: 2710, IsClosed: true},
		{Timestamp: tQ3.Add(-16 * time.Hour), High: 2730, Low: 2710, Close: 2720, IsClosed: true},
		{Timestamp: tQ3.Add(-12 * time.Hour), High: 2748, Low: 2725, Close: 2740, IsClosed: true}, // MTF Swing high 2748
		{Timestamp: tQ3.Add(-8 * time.Hour), High: 2735, Low: 2715, Close: 2725, IsClosed: true},
		{Timestamp: tQ3.Add(-4 * time.Hour), High: 2740, Low: 2720, Close: 2730, IsClosed: true},
		{Timestamp: tQ3, High: 2750, Low: 2725, Close: 2735, IsClosed: true}, // Sweeps 2748
	}
	ltf := []indicators.Candle{
		{Timestamp: tQ3.Add(-60 * time.Minute), Open: 2732, High: 2735, Low: 2730, Close: 2734, IsClosed: true},
		{Timestamp: tQ3.Add(-45 * time.Minute), Open: 2734, High: 2742, Low: 2733, Close: 2740, IsClosed: true}, // Bullish origin Open 2734
		{Timestamp: tQ3.Add(-30 * time.Minute), Open: 2740, High: 2750, Low: 2738, Close: 2749, IsClosed: true}, // Peak high 2750
		{Timestamp: tQ3.Add(-15 * time.Minute), Open: 2749, High: 2749, Low: 2730, Close: 2731, IsClosed: true}, // Bearish close < 2734! (CISD)
		{Timestamp: tQ3, Open: 2731, High: 2732, Low: 2729, Close: 2730, IsClosed: true},
	}

	setup := eval.EvaluateSequence(htf, mtf, ltf, nil, "D", "4H", "15m", "XAUUSD", "")
	if setup.State != StateReadyEntry1 {
		t.Fatalf("expected StateReadyEntry1, got %s", setup.State)
	}

	// Cek proteksi SL Floor minimal $4.00 untuk Gold
	slDist := math.Abs(setup.EntryPrice - setup.StopLoss)
	if slDist < 4.00 {
		t.Errorf("expected Gold SL distance >= 4.00, got %.2f (Entry: %.2f, SL: %.2f)",
			slDist, setup.EntryPrice, setup.StopLoss)
	}

	// Cek batasan R:R 1:2.5 s.d. 1:4.5
	if setup.RiskReward < 2.5 || setup.RiskReward > 4.5 {
		t.Errorf("expected R:R within [2.5, 4.5], got %.2f", setup.RiskReward)
	}
}

// TestEnforceLatestActiveFVGOnly memvalidasi bahwa ILA 3.0 strictly hanya menggunakan
// FVG aktif terbaru dan dilarang melompat mundur ke FVG usang (mencegah bug USTEC).
func TestEnforceLatestActiveFVGOnly(t *testing.T) {
	eval := NewEvaluator()
	now := time.Now()

	// HTF dengan 2 FVG:
	// 1. FVG Lama: Bearish FVG di 28000 - 28500 (C1 High=28500, C3 Low=28000)
	// 2. FVG Baru: Bullish FVG di 27200 - 27600
	htf := []indicators.Candle{
		// Bearish FVG lama (index 0, 1, 2)
		{Timestamp: now.Add(-100 * time.Hour), Open: 28600, High: 28700, Low: 28500, Close: 28550, IsClosed: true},
		{Timestamp: now.Add(-96 * time.Hour), Open: 28500, High: 28520, Low: 27900, Close: 27950, IsClosed: true}, // displacement down
		{Timestamp: now.Add(-92 * time.Hour), Open: 27950, High: 28000, Low: 27800, Close: 27850, IsClosed: true}, // Bearish FVG 28000 - 28500

		// Lilin konsolidasi
		{Timestamp: now.Add(-88 * time.Hour), Open: 27850, High: 27900, Low: 27700, Close: 27750, IsClosed: true},
		{Timestamp: now.Add(-84 * time.Hour), Open: 27750, High: 27800, Low: 27100, Close: 27150, IsClosed: true},

		// Bullish FVG terbaru (index 5, 6, 7)
		{Timestamp: now.Add(-80 * time.Hour), Open: 27150, High: 27200, Low: 27100, Close: 27180, IsClosed: true},
		{Timestamp: now.Add(-76 * time.Hour), Open: 27200, High: 27700, Low: 27180, Close: 27680, IsClosed: true}, // displacement up
		{Timestamp: now.Add(-72 * time.Hour), Open: 27680, High: 27800, Low: 27600, Close: 27750, IsClosed: true}, // Bullish FVG 27200 - 27600

		// Lilin terkini: harga di 28100 (berada di dalam range FVG Bearish lama 28000-28500, tapi DI LUAR FVG Bullish baru 27200-27600)
		{Timestamp: now, Open: 27750, High: 28150, Low: 27700, Close: 28100, IsClosed: true},
	}

	mtf := []indicators.Candle{
		{Timestamp: now.Add(-20 * time.Hour), High: 28100, Low: 27900, Close: 28000, IsClosed: true},
		{Timestamp: now.Add(-16 * time.Hour), High: 28120, Low: 27950, Close: 28050, IsClosed: true},
		{Timestamp: now, High: 28150, Low: 28000, Close: 28100, IsClosed: true},
	}

	ltf := []indicators.Candle{
		{Timestamp: now.Add(-30 * time.Minute), Open: 28050, High: 28120, Low: 28040, Close: 28100, IsClosed: true},
		{Timestamp: now, Open: 28100, High: 28150, Low: 28080, Close: 28120, IsClosed: true},
	}

	setup := eval.EvaluateSequence(htf, mtf, ltf, nil, "4H", "1H", "5m", "USTEC", "US500")

	// Setup DILARANG mengambil bias BEARISH dari FVG lama!
	if setup.Bias == "BEARISH" {
		t.Fatalf("FAILED: Evaluator keliru melompat ke FVG Bearish lama padahal ada FVG Bullish aktif terbaru! Got bias: %s", setup.Bias)
	}
}
