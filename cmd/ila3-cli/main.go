package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
	"ila-engine/pkg/marketfeed"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/sequence"
	"ila-engine/pkg/ssmt"
)

func main() {
	symbol := flag.String("symbol", "EURUSD", "Primary market symbol to analyze")
	corrSym := flag.String("corr", "", "Correlated symbol for SSMT (default: auto-detected)")
	mode := flag.String("mode", "new-day", "Preset mode: new-week (D->4H->15m) or new-day (4H->1H->5m)")
	htfFlag := flag.String("htf", "", "Custom HTF (e.g. D, 4H)")
	mtfFlag := flag.String("mtf", "", "Custom MTF (e.g. 4H, 1H)")
	ltfFlag := flag.String("ltf", "", "Custom LTF (e.g. 15m, 5m)")
	format := flag.String("format", "text", "Output format: text or json")
	flag.Parse()

	symClean := strings.ToUpper(strings.ReplaceAll(*symbol, "/", ""))
	corrClean := *corrSym
	if corrClean == "" {
		corrClean = ssmt.DefaultCorrelationPairs[symClean]
	}

	htf := "4H"
	mtf := "1H"
	ltf := "5m"

	if *mode == "new-week" {
		htf = "D"
		mtf = "4H"
		ltf = "15m"
	} else if *mode == "monthly" {
		htf = "1M"
		mtf = "D"
		ltf = "1H"
	} else if *mode == "intraday" {
		htf = "4H"
		mtf = "15m"
		ltf = "1m"
	}
	if *htfFlag != "" {
		htf = *htfFlag
	}
	if *mtfFlag != "" {
		mtf = *mtfFlag
	}
	if *ltfFlag != "" {
		ltf = *ltfFlag
	}

	feed := marketfeed.NewMultiFeedProvider()
	eval := sequence.NewEvaluator()
	qtCalc := quarterly.NewCalculator()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch candles in parallel
	type candleFetchResult struct {
		htf  []indicators.Candle
		mtf  []indicators.Candle
		ltf  []indicators.Candle
		corr []indicators.Candle
	}

	resCh := make(chan candleFetchResult, 1)
	go func() {
		h, _ := feed.FetchCandles(ctx, symClean, htf, 40)
		m, _ := feed.FetchCandles(ctx, symClean, mtf, 40)
		l, _ := feed.FetchCandles(ctx, symClean, ltf, 40)
		var c []indicators.Candle
		if corrClean != "" {
			c, _ = feed.FetchCandles(ctx, corrClean, mtf, 40)
		}
		resCh <- candleFetchResult{htf: h, mtf: m, ltf: l, corr: c}
	}()

	var fetched candleFetchResult
	select {
	case fetched = <-resCh:
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "Error: Timeout fetching market feed\n")
		os.Exit(1)
	}

	setup := eval.EvaluateSequence(
		fetched.htf, fetched.mtf, fetched.ltf, fetched.corr,
		htf, mtf, ltf, symClean, corrClean,
	)

	if *format == "json" {
		out, _ := json.MarshalIndent(setup, "", "  ")
		fmt.Println(string(out))
		return
	}

	now := time.Now().UTC()
	qInfo := qtCalc.GetDailyQuarter(now)
	subQ := qtCalc.Get90MQuarter(now)

	fmt.Println("==================================================")
	fmt.Println("🔬 ILA 3.0 RESEARCH & MACRO LIQUIDITY ENGINE")
	fmt.Println("==================================================")
	fmt.Printf("• Simbol Utama  : %s\n", symClean)
	if corrClean != "" {
		fmt.Printf("• Pasangan SSMT : %s\n", corrClean)
	}
	fmt.Printf("• Kombinasi TF  : %s (HTF) ➡️ %s (MTF) ➡️ %s (LTF)\n", htf, mtf, ltf)
	fmt.Printf("• Siklus Waktu  : %s Daily (%s) | Sub-Kuartal: %s (90m)\n",
		qInfo.Quarter, qInfo.Description, subQ.Quarter)
	if setup.IsQ1Bypass {
		fmt.Println("⭐ Q1 EXCEPTION : AKTIF (Boleh langsung mulai dari MTF Key Level ERL/IRL!)")
	}
	fmt.Printf("• Bias Utama    : %s\n", setup.Bias)
	if setup.CrossTFValid {
		fmt.Printf("• Validasi Cross-TF: 🟢 VALID (Key Level HTF memiliki jejak swing di MTF %s)\n", mtf)
	}
	if setup.WaypointFVG != nil {
		fmt.Printf("⚠️ WAYPOINT FVG : Terdeteksi FVG MTF (%.5f - %.5f) di dalam zona HTF (Hanya waypoint, DILARANG ENTRY!)\n",
			setup.WaypointFVG.Bottom, setup.WaypointFVG.Top)
	}
	fmt.Printf("• Status Siklus : %s (%s)\n\n", setup.State, setup.SetupType)

	if setup.SSMTResult.Detected {
		fmt.Printf("📊 KONFLUENSI SSMT (%s vs %s):\n", symClean, corrClean)
		fmt.Printf("  └ Status : %s (%s)\n", setup.SSMTResult.Type, setup.SSMTResult.ProbabilityGrade)
		fmt.Printf("  └ Detail : %s\n\n", setup.SSMTResult.Details)
	}

	if setup.SetupType != sequence.SetupNone {
		fmt.Printf("🚀 RENCANA EKSEKUSI ILA 3.0 (%s):\n", setup.SetupType)
		fmt.Printf("  • Harga Entry : %.5f\n", setup.EntryPrice)
		fmt.Printf("  • Stop Loss   : %.5f\n", setup.StopLoss)
		fmt.Printf("  • Take Profit : %.5f\n", setup.TakeProfit)
		fmt.Printf("  • Rasio R:R   : 1:%.2f\n\n", setup.RiskReward)

		if setup.StDevProjections != nil {
			st := setup.StDevProjections
			fmt.Println("📐 PROYEKSI ICT STANDARD DEVIATION (FIBO):")
			fmt.Printf("  • Poin 1 (Anchor Extreme) : %.5f\n", st.Point1)
			fmt.Printf("  • Poin 0 (MSS / CISD)     : %.5f\n", st.Point0)
			fmt.Printf("  • Level -1.0 (Extension 1): %.5f\n", st.LevelMinus1)
			fmt.Printf("  • Level -2.0 s.d. -2.5    : %.5f - %.5f ⚠️ [ZONA KRITIS / PARTIAL TP]\n", st.LevelMinus2, st.LevelMinus2_5)
			fmt.Printf("  • Level -3.5 s.d. -4.0    : %.5f - %.5f 🎯 [ZONA TARGET FINAL]\n\n", st.LevelMinus3_5, st.LevelMinus4)
		}

		if setup.NestedExecution != nil {
			nx := setup.NestedExecution
			fmt.Println("🔗 NESTED MULTI-TIMEFRAME EXECUTION (HIGH R:R):")
			fmt.Printf("  • Target HTF (%s) : %.5f (Jarak: %.5f)\n", nx.HTFTargetName, nx.HTFTargetPrice, nx.RewardDistance)
			fmt.Printf("  • Stop Loss LTF (%s) : %.5f (Jarak: %.5f)\n", ltf, nx.LTFSLPrice, nx.RiskDistance)
			fmt.Printf("  • Rasio R:R Nested  : 1:%.2f 💎 (Ultra-High Reward)\n", nx.NestedRR)
			fmt.Printf("  • Checkpoint TP 1   : %.5f (Level -2.0 StDev Warning Zone)\n", nx.CriticalZoneTP)
			fmt.Printf("  • Checkpoint TP 2   : %.5f (Level -3.5 StDev Final Target)\n\n", nx.FinalZoneTP)
		}
	}

	fmt.Printf("📝 Ringkasan : %s\n", setup.Summary)
	fmt.Printf("👉 Langkah   : %s\n", setup.NextAction)
	fmt.Println("==================================================")
}
