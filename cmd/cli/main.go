package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ila-engine/internal/config"
	"ila-engine/internal/dto"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/internal/service"
	"ila-engine/pkg/broker"
	"ila-engine/pkg/marketfeed"
)

func main() {
	symbol := flag.String("symbol", "XAUUSD", "Market symbol to analyze")
	limit := flag.Int("limit", 100, "Number of candles to evaluate")
	format := flag.String("format", "text", "Output format: text or json")
	execute := flag.Bool("execute", false, "Evaluate quality gate and execute order on demo account")
	eval := flag.Bool("eval", false, "Display post-mortem trade evaluations and self-learning summary")
	ila := flag.Bool("ila", true, "Run Imaginary Liquidity (ILA) evaluation on paired timeframes (default: true)")
	htf := flag.String("htf", "", "High Timeframe for ILA (leave empty to scan all 4 paired timeframes)")
	ltf := flag.String("ltf", "", "Low Timeframe for ILA (leave empty to scan all 4 paired timeframes)")
	force := flag.Bool("force", false, "Force execution bypassing daily loss lock")
	flag.Parse()

	feed := marketfeed.NewMultiFeedProvider()
	ilaService := service.NewILAService(feed)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if *execute {
		cfg := config.LoadConfig()
		db, err := config.InitDatabase(cfg.DBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening db: %v\n", err)
			os.Exit(1)
		}
		_ = db.AutoMigrate(&model.SetupRecord{}, &model.TradeJournal{}, &model.Position{})
		setupRepo := repository.NewSetupRepository(db)
		_ = setupRepo
		journalRepo := repository.NewJournalRepository(db)
		positionRepo := repository.NewPositionRepository(db)
		cTraderBroker, _ := broker.NewCTraderBroker(broker.CTraderConfig{})
		mtrBroker := broker.NewMatchTraderBroker("")
		var tradeBroker broker.Broker = broker.NewMultiBrokerManager(mtrBroker, cTraderBroker)
		execService := service.NewExecutionService(nil, ilaService, positionRepo, journalRepo, tradeBroker, feed)

		var res *dto.TradeExecutionResult
		if *ila {
			targetHTF := *htf
			targetLTF := *ltf
			if targetHTF == "" {
				targetHTF = "H4"
			}
			if targetLTF == "" {
				targetLTF = "M15"
			}
			res, err = execService.EvaluateAndExecuteILA(ctx, dto.ILAExecuteRequest{
				Symbol:    *symbol,
				HTF:       targetHTF,
				LTF:       targetLTF,
				RiskRatio: 1.0,
				MinRR:     2.0,
				Force:     *force,
			})
		} else {
			res, err = execService.EvaluateAndExecute(ctx, dto.TradeExecutionRequest{
				Symbol:    *symbol,
				RiskRatio: 1.0,
				MinRR:     2.0,
				Force:     *force,
			})
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			out, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(out))
			return
		}

		fmt.Println("🛡️ HASIL EVALUASI QUALITY GATE & EKSEKUSI")
		fmt.Printf("Simbol: %s\n", res.Symbol)
		if res.Strategy != "" {
			fmt.Printf("• Strategi   : %s (%s)\n", res.Strategy, res.Timeframe)
		}
		if !res.Executed {
			fmt.Println("❌ STATUS: DITOLAK (ORDER TIDAK DIBUKA)")
			fmt.Printf("Alasan: %s\n", res.RejectionReason)
		} else {
			fmt.Println("✅ STATUS: LOLOS & BERHASIL DIBUKA")
			fmt.Printf("• Tiket Order: %s\n", res.TicketNumber)
			fmt.Printf("• Arah Posisi: %s\n", res.Direction)
			fmt.Printf("• Ukuran Lot : %.2f Lot (Risiko: $%.2f / 1%%)\n", res.LotSize, res.RiskAmount)
			fmt.Printf("• Harga Open : %.5f\n", res.OpenPrice)
			fmt.Printf("• Stop Loss  : %.5f\n", res.StopLoss)
			fmt.Printf("• Take Profit: %.5f\n", res.TakeProfit)
			fmt.Printf("• Rasio R:R  : 1:%.2f (Memenuhi syarat minimal 1:2)\n", res.RiskReward)
		}
		return
	}

	if *ila {
		ilaService := service.NewILAService(feed)

		if *htf == "" && *ltf == "" {
			multi, err := ilaService.AnalyzeAllPairs(ctx, *symbol)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error analyzing all ILA pairs: %v\n", err)
				os.Exit(1)
			}

			if *format == "json" {
				out, _ := json.MarshalIndent(multi, "", "  ")
				fmt.Println(string(out))
				return
			}

			fmt.Println("🎯 IMAGINARY LIQUIDITY (ILA) - 4 PAIRED TIMEFRAMES")
			fmt.Printf("Simbol: %s | Sumber: FOREX.com (TradingView)\n\n", multi.Symbol)

			fmt.Println("┌───────────┬─────────┬──────────────────────┐")
			fmt.Println("│ Pasangan  │  Arah   │        Status        │")
			fmt.Println("├───────────┼─────────┼──────────────────────┤")
			for _, p := range multi.Pairs {
				setup := p.Setup
				dirBadge := "⚪ Netr"
				if setup.Bias == "BULLISH" {
					dirBadge = "🟢 Buy "
				} else if setup.Bias == "BEARISH" {
					dirBadge = "🔴 Sell"
				}

				stBadge := "⏳ Menunggu"
				if setup.Status == "READY_TO_ENTRY" {
					stBadge = "🎯 Siap Entry"
				} else if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
					stBadge = fmt.Sprintf("⏳ Tunggu Close %s", p.HTF)
				} else if setup.Status == "WAIT_LTF_CISD" {
					stBadge = "⏳ Tunggu CISD"
				} else if setup.Status == "WAIT_HTF_SWEEP" {
					stBadge = "⏳ Tunggu Sweep"
				} else if setup.Status == "INVALIDATED" {
					stBadge = "🛑 Batal (Invalid)"
				}

				pNamePad := p.PairName + strings.Repeat(" ", 9-len([]rune(p.PairName)))
				stPad := stBadge + strings.Repeat(" ", 20-len([]rune(stBadge)))
				fmt.Printf("│ %s │ %s │ %s │\n", pNamePad, dirBadge, stPad)
			}
			fmt.Println("└───────────┴─────────┴──────────────────────┘")
			fmt.Println()

			for i, p := range multi.Pairs {
				setup := p.Setup
				statusBadge := "⏳ WAIT HTF SWEEP"
				if setup.Status == "READY_TO_ENTRY" {
					statusBadge = "🎯 READY TO ENTRY"
				} else if setup.Status == "WAIT_LTF_CISD" {
					statusBadge = "⏳ WAIT LTF CISD"
				} else if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
					statusBadge = "⏳ WAIT HTF CLOSE (Belum Ganti Candle)"
				}

				dirBadge := "⚪ Netral"
				if setup.Bias == "BULLISH" {
					dirBadge = "🟢 Bullish"
				} else if setup.Bias == "BEARISH" {
					dirBadge = "🔴 Bearish"
				}

				fmt.Printf("[%d] %s (%s) ➡️ %s\n", i+1, p.PairName, dirBadge, statusBadge)

				if setup.HTFSweep != nil {
					fmt.Printf("    • HTF (%s): %s\n", p.HTF, setup.HTFSweep.Details)
				}
				if setup.CISD != nil && setup.CISD.Detected {
					fmt.Printf("    • LTF (%s): %s\n", p.LTF, setup.CISD.Details)
				} else if setup.CISD != nil && setup.CISD.CISDLevel > 0 {
					fmt.Printf("    • LTF (%s): %s\n", p.LTF, setup.CISD.Details)
				} else if setup.Status == "WAIT_LTF_CISD" {
					fmt.Printf("    • LTF (%s): Menunggu pembentukan CISD (Body break Body)\n", p.LTF)
				}

				if setup.Status == "READY_TO_ENTRY" {
					fmt.Printf("    • Rencana: Entry=%.5f | SL=%.5f | TP=%.5f | R:R=1:%.2f\n",
						setup.EntryPrice, setup.StopLoss, setup.TakeProfit, setup.RiskReward)
				}
				fmt.Println()
			}

			fmt.Println("==================================================")
			fmt.Printf("📋 Ringkasan: %s\n", multi.Summary)
			return
		}

		setup, err := ilaService.AnalyzeILA(ctx, dto.ILAAnalysisRequest{
			Symbol: *symbol,
			HTF:    *htf,
			LTF:    *ltf,
			Limit:  *limit,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error analyzing ILA: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			out, _ := json.MarshalIndent(setup, "", "  ")
			fmt.Println(string(out))
			return
		}

		fmt.Println("🎯 IMAGINARY LIQUIDITY (ILA) ANALYSIS")
		fmt.Println("==================================================")
		fmt.Printf("• Pasangan Timeframe : %s (HTF) ➡️ %s (LTF)\n", setup.HTF, setup.LTF)
		fmt.Printf("• Simbol Instrumen   : %s\n", setup.Symbol)
		fmt.Printf("• Bias Arah          : %s\n", setup.Bias)
		fmt.Printf("• Status Evaluasi    : %s\n\n", setup.Status)

		if setup.HTFSweep != nil && setup.HTFSweep.Detected {
			fmt.Printf("🔍 HTF SWEEP INSIDE FVG (%s):\n", setup.HTF)
			fmt.Printf("  └ %s\n", setup.HTFSweep.Details)
			fmt.Printf("  └ Level Stop Loss : %.5f (Puncak Sweep)\n", setup.HTFSweep.StopLoss)
			fmt.Printf("  └ Target DOL      : %.5f (Eksternal Liquidity / SSL)\n\n", setup.HTFSweep.TargetDOL)
		} else if setup.HTFSweep != nil {
			fmt.Printf("🔍 HTF STATUS (%s):\n", setup.HTF)
			if setup.HTFSweep.FVG != nil {
				fmt.Printf("  └ Area FVG Terakhir: %.2f - %.2f (%s)\n", setup.HTFSweep.FVG.Bottom, setup.HTFSweep.FVG.Top, setup.HTFSweep.FVG.Type)
			}
			fmt.Printf("  └ Detail           : %s\n\n", setup.HTFSweep.Details)
		} else {
			fmt.Printf("🔍 HTF STATUS (%s):\n", setup.HTF)
			fmt.Println("  └ Belum terdeteksi sweep wick di dalam FVG HTF.\n")
		}

		if setup.CISD != nil && setup.CISD.Detected {
			fmt.Printf("⚡ LTF CISD CONFIRMATION (%s):\n", setup.LTF)
			fmt.Printf("  └ %s\n", setup.CISD.Details)
			fmt.Printf("  └ Waktu Tembus    : %s\n\n", setup.CISD.BreakTime.Format("2006-01-02 15:04"))
		} else if setup.CISD != nil && setup.CISD.CISDLevel > 0 {
			fmt.Printf("⚡ LTF STATUS (%s):\n", setup.LTF)
			fmt.Printf("  └ Level CISD Acuan: %.5f (Origin deretan opposing leg)\n", setup.CISD.CISDLevel)
			fmt.Printf("  └ Level SL Terkini: %.5f (Extreme Candle 5)\n", setup.CISD.ExtremeSL)
			fmt.Printf("  └ Status          : %s\n\n", setup.CISD.Details)
		} else {
			fmt.Printf("⚡ LTF STATUS (%s):\n", setup.LTF)
			fmt.Println("  └ Menunggu konfirmasi CISD (Body break Body).\n")
		}

		if setup.Status == "READY_TO_ENTRY" {
			fmt.Println("🚀 RENCANA EKSEKUSI (READY TO ENTRY):")
			fmt.Printf("• Harga Entry : %.5f (Next candle open)\n", setup.EntryPrice)
			fmt.Printf("• Stop Loss   : %.5f\n", setup.StopLoss)
			fmt.Printf("• Take Profit : %.5f (DOL)\n", setup.TakeProfit)
			fmt.Printf("• Rasio R:R   : 1:%.2f\n", setup.RiskReward)
		}

		fmt.Printf("\n👉 Langkah Berikutnya: %s\n", setup.NextAction)
		return
	}

	if *eval {
		cfg := config.LoadConfig()
		db, err := config.InitDatabase(cfg.DBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening db: %v\n", err)
			os.Exit(1)
		}
		_ = db.AutoMigrate(&model.SetupRecord{}, &model.TradeJournal{}, &model.Position{}, &model.TradeEvaluation{})
		positionRepo := repository.NewPositionRepository(db)
		evalRepo := repository.NewEvaluationRepository(db)
		evalService := service.NewEvaluationService(evalRepo, positionRepo, feed)

		// 1. Ensure all closed trades are evaluated
		_, _ = evalService.EvaluateAllClosedPositions(ctx)

		// 2. Fetch stats & list
		stats, err := evalService.GetEvaluationSummary(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching evaluation stats: %v\n", err)
			os.Exit(1)
		}

		list, err := evalService.ListRecentEvaluations(ctx, 20)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching evaluations: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			out, _ := json.MarshalIndent(map[string]any{"stats": stats, "evaluations": list}, "", "  ")
			fmt.Println(string(out))
			return
		}

		fmt.Println("🧠 ILA AI SELF-LEARNING & TRADE EVALUATION")
		fmt.Println("==================================================")
		fmt.Printf("• Total Trade Selesai : %d Trade\n", stats.TotalTrades)
		fmt.Printf("• Menang / Kalah      : %d Menang (%.1f%%) | %d Kalah\n", stats.WinningTrades, stats.WinRate, stats.LosingTrades)
		pnlSign := "+"
		if stats.TotalPnL < 0 {
			pnlSign = ""
		}
		fmt.Printf("• Akumulasi Realisasi : %s%.2f R (%s$%.2f)\n", pnlSign, stats.TotalR, pnlSign, stats.TotalPnL)
		fmt.Printf("• Profit Factor       : %.2f\n\n", stats.ProfitFactor)

		if len(stats.LearnedLessons) > 0 {
			fmt.Println("💡 PELAJARAN UTAMA (FEEDBACK LOOP):")
			for i, l := range stats.LearnedLessons {
				if i >= 3 {
					break
				}
				fmt.Printf("  %d. %s\n", i+1, l)
			}
			fmt.Println()
		}

		fmt.Println("📜 RIWAYAT EVALUASI TIAP POSISI:")
		for i, e := range list {
			dirEmoji := "🟢 BUY"
			if e.Direction == "SELL" {
				dirEmoji = "🔴 SELL"
			}
			outcomeBadge := "🎯 HIT TAKE PROFIT"
			if e.Outcome == "HIT_SL" {
				outcomeBadge = "🛑 HIT STOP LOSS"
			}
			rSign := "+"
			if e.OutcomeR < 0 {
				rSign = ""
			}
			fmt.Printf("[%d] %s %s (Tiket #%s) ➡️ %s (%s%.2f R)\n", i+1, e.Symbol, dirEmoji, e.TicketNumber, outcomeBadge, rSign, e.OutcomeR)
			fmt.Printf("    • Entry: %.5f ➡️ Exit: %.5f | PnL: %s$%.2f\n", e.OpenPrice, e.ExitPrice, rSign, e.ProfitLoss)
			fmt.Printf("    • Akar Masalah / Faktor : %s\n", e.RootCause)
			fmt.Printf("    • Analisa Teknis        : %s\n", e.TechnicalAnalysis)
			fmt.Printf("    • Tindakan Perbaikan    : %s\n\n", e.ActionableLesson)
		}
		return
	}

	// Default action: Evaluasi ILA (4-Paired atau spesifik pair)
	if *htf == "" && *ltf == "" {
		multi, err := ilaService.AnalyzeAllPairs(ctx, *symbol)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error analyzing all ILA pairs: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			out, _ := json.MarshalIndent(multi, "", "  ")
			fmt.Println(string(out))
			return
		}

		fmt.Println("🎯 IMAGINARY LIQUIDITY (ILA) - 4 PAIRED TIMEFRAMES")
		fmt.Printf("Simbol: %s | Sumber: FOREX.com (TradingView)\n\n", multi.Symbol)

		fmt.Println("┌───────────┬─────────┬──────────────────────┐")
		fmt.Println("│ Pasangan  │  Arah   │        Status        │")
		fmt.Println("├───────────┼─────────┼──────────────────────┤")
		for _, p := range multi.Pairs {
			setup := p.Setup
			dirBadge := "⚪ Netr"
			if setup.Bias == "BULLISH" {
				dirBadge = "🟢 Buy "
			} else if setup.Bias == "BEARISH" {
				dirBadge = "🔴 Sell"
			}

			stBadge := "⏳ Menunggu"
			if setup.Status == "READY_TO_ENTRY" {
				stBadge = "🎯 Siap Entry"
			} else if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
				stBadge = fmt.Sprintf("⏳ Tunggu Close %s", p.HTF)
			} else if setup.Status == "WAIT_LTF_CISD" {
				stBadge = "⏳ Tunggu CISD"
			} else if setup.Status == "WAIT_HTF_SWEEP" {
				stBadge = "⏳ Tunggu Sweep"
			} else if setup.Status == "INVALIDATED" {
				stBadge = "🛑 Batal (Invalid)"
			}

			pNamePad := p.PairName + strings.Repeat(" ", 9-len([]rune(p.PairName)))
			stPad := stBadge + strings.Repeat(" ", 20-len([]rune(stBadge)))
			fmt.Printf("│ %s │ %s │ %s │\n", pNamePad, dirBadge, stPad)
		}
		fmt.Println("└───────────┴─────────┴──────────────────────┘")
		fmt.Println()

		for i, p := range multi.Pairs {
			setup := p.Setup
			statusBadge := "⏳ WAIT HTF SWEEP"
			if setup.Status == "READY_TO_ENTRY" {
				statusBadge = "🎯 READY TO ENTRY"
			} else if setup.Status == "WAIT_LTF_CISD" {
				statusBadge = "⏳ WAIT LTF CISD"
			} else if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
				statusBadge = "⏳ WAIT HTF CLOSE (Belum Ganti Candle)"
			}

			dirBadge := "⚪ Netral"
			if setup.Bias == "BULLISH" {
				dirBadge = "🟢 Bullish"
			} else if setup.Bias == "BEARISH" {
				dirBadge = "🔴 Bearish"
			}

			fmt.Printf("[%d] %s (%s) ➡️ %s\n", i+1, p.PairName, dirBadge, statusBadge)

			if setup.HTFSweep != nil {
				fmt.Printf("    • HTF (%s): %s\n", p.HTF, setup.HTFSweep.Details)
			}
			if setup.CISD != nil && setup.CISD.Detected {
				fmt.Printf("    • LTF (%s): %s\n", p.LTF, setup.CISD.Details)
				fmt.Printf("    • Rencana: Entry=%.5f | SL=%.5f | TP=%.5f | R:R=1:%.2f\n",
					setup.EntryPrice, setup.StopLoss, setup.TakeProfit, setup.RiskReward)
			}
			fmt.Println()
		}

		fmt.Println("==================================================")
		fmt.Printf("📋 Ringkasan: %s\n", multi.Summary)
		return
	}

	setup, err := ilaService.AnalyzeILA(ctx, dto.ILAAnalysisRequest{
		Symbol: *symbol,
		HTF:    *htf,
		LTF:    *ltf,
		Limit:  *limit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing ILA: %v\n", err)
		os.Exit(1)
	}

	if *format == "json" {
		out, _ := json.MarshalIndent(setup, "", "  ")
		fmt.Println(string(out))
		return
	}

	fmt.Println("🎯 IMAGINARY LIQUIDITY (ILA) ANALYSIS")
	fmt.Println("==================================================")
	fmt.Printf("• Pasangan Timeframe : %s (HTF) ➡️ %s (LTF)\n", setup.HTF, setup.LTF)
	fmt.Printf("• Simbol Instrumen   : %s\n", setup.Symbol)
	fmt.Printf("• Bias Arah          : %s\n", setup.Bias)
	fmt.Printf("• Status Evaluasi    : %s\n\n", setup.Status)

	if setup.HTFSweep != nil && setup.HTFSweep.Detected {
		fmt.Printf("🔍 HTF SWEEP INSIDE FVG (%s):\n", setup.HTF)
		fmt.Printf("  └ %s\n", setup.HTFSweep.Details)
		fmt.Printf("  └ Level Stop Loss : %.5f (Puncak Sweep)\n", setup.HTFSweep.StopLoss)
		fmt.Printf("  └ Target DOL      : %.5f (Eksternal Liquidity / SSL)\n\n", setup.HTFSweep.TargetDOL)
	} else if setup.HTFSweep != nil {
		fmt.Printf("🔍 HTF STATUS (%s):\n", setup.HTF)
		if setup.HTFSweep.FVG != nil {
			fmt.Printf("  └ Area FVG Terakhir: %.2f - %.2f (%s)\n", setup.HTFSweep.FVG.Bottom, setup.HTFSweep.FVG.Top, setup.HTFSweep.FVG.Type)
		}
		fmt.Printf("  └ Detail           : %s\n\n", setup.HTFSweep.Details)
	} else {
		fmt.Printf("🔍 HTF STATUS (%s):\n", setup.HTF)
		fmt.Println("  └ Belum terdeteksi sweep wick di dalam FVG HTF.\n")
	}

	if setup.CISD != nil && setup.CISD.Detected {
		fmt.Printf("⚡ LTF CISD CONFIRMATION (%s):\n", setup.LTF)
		fmt.Printf("  └ %s\n", setup.CISD.Details)
		fmt.Printf("  └ Waktu Tembus    : %s\n\n", setup.CISD.BreakTime.Format("2006-01-02 15:04"))
	} else if setup.CISD != nil && setup.CISD.CISDLevel > 0 {
		fmt.Printf("⚡ LTF STATUS (%s):\n", setup.LTF)
		fmt.Printf("  └ Level CISD Acuan: %.5f (Origin deretan opposing leg)\n", setup.CISD.CISDLevel)
		fmt.Printf("  └ Level SL Terkini: %.5f (Extreme Candle 5)\n", setup.CISD.ExtremeSL)
		fmt.Printf("  └ Status          : %s\n\n", setup.CISD.Details)
	} else {
		fmt.Printf("⚡ LTF STATUS (%s):\n", setup.LTF)
		fmt.Println("  └ Menunggu konfirmasi CISD (Body break Body).\n")
	}

	if setup.Status == "READY_TO_ENTRY" {
		fmt.Println("🚀 RENCANA EKSEKUSI (READY TO ENTRY):")
		fmt.Printf("• Harga Entry : %.5f (Next candle open)\n", setup.EntryPrice)
		fmt.Printf("• Stop Loss   : %.5f\n", setup.StopLoss)
		fmt.Printf("• Take Profit : %.5f (DOL)\n", setup.TakeProfit)
		fmt.Printf("• Rasio R:R   : 1:%.2f\n", setup.RiskReward)
	}

	fmt.Printf("\n👉 Langkah Berikutnya: %s\n", setup.NextAction)
}

