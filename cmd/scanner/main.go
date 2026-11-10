package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"ila-engine/internal/datadir"
	"ila-engine/internal/dto"
	"ila-engine/internal/market"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/internal/service"
	"ila-engine/pkg/broker"
	"ila-engine/pkg/marketfeed"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	AccountSize        = 10000.0
	DefaultRiskPercent = 1.00
	MinRR              = 2.0
	MaxRR              = 2.3
)

var Pairs = []string{"XAUUSD", "EURUSD", "USTEC"}

func DBPath() string        { return datadir.Berkas("ila.db") }
func StateFilePath() string { return datadir.Berkas("monitor_state.json") }

type ILAPairState struct {
	LastBreakTime   string    `json:"last_break_time"`
	LastStatus      string    `json:"last_status"`
	LastHTFStatus   string    `json:"last_htf_status"`
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`
}

type MonitorState struct {
	LastSession string                  `json:"last_session"`
	ILAPairs    map[string]ILAPairState `json:"ila_pairs"`
}

func loadState() MonitorState {
	var s MonitorState
	s.ILAPairs = make(map[string]ILAPairState)
	data, err := os.ReadFile(StateFilePath())
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	if s.ILAPairs == nil {
		s.ILAPairs = make(map[string]ILAPairState)
	}
	return s
}

func saveState(s MonitorState) {
	if err := datadir.TulisJSON(StateFilePath(), s); err == nil {
	}
}

var nyLocation *time.Location

func init() {
	var err error
	nyLocation, err = time.LoadLocation("America/New_York")
	if err != nil {
		nyLocation = time.FixedZone("EST", -5*3600)
	}
}

func formatNYTime(t time.Time) string {
	if nyLocation != nil {
		return t.In(nyLocation).Format("15:04 NY")
	}
	return t.UTC().Add(-5 * time.Hour).Format("15:04 NY")
}

func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

func slotDue(lastEval time.Time, tf string, now time.Time, buffer time.Duration) bool {
	if lastEval.IsZero() {
		return true
	}
	tClose := latestCandleClose(tf, now.Add(-buffer))
	if tClose.IsZero() {
		return true
	}
	tReady := tClose.Add(buffer)
	if now.Before(tReady) {
		return false
	}
	return lastEval.Before(tReady)
}

func latestCandleClose(tf string, t time.Time) time.Time {
	t = t.UTC()
	switch strings.ToUpper(tf) {
	case "M5", "5M", "5":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), (t.Minute()/5)*5, 0, 0, time.UTC)
	case "M15", "15M", "15":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), (t.Minute()/15)*15, 0, 0, time.UTC)
	case "H1", "1H", "60", "60M":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case "H4", "4H", "240", "240M":
		tInNY := t.In(nyLocation)
		nyHour := tInNY.Hour()
		closeHour := nyHour - ((nyHour - 1 + 24) % 4)
		tCloseNY := time.Date(tInNY.Year(), tInNY.Month(), tInNY.Day(), closeHour, 0, 0, 0, nyLocation)
		return tCloseNY.UTC()
	case "D", "D1", "1D":
		tInNY := t.In(nyLocation)
		closeDay := tInNY.Day()
		if tInNY.Hour() < 17 {
			tInNY = tInNY.AddDate(0, 0, -1)
			closeDay = tInNY.Day()
		}
		tCloseNY := time.Date(tInNY.Year(), tInNY.Month(), closeDay, 17, 0, 0, 0, nyLocation)
		return tCloseNY.UTC()
	case "W", "W1", "1W":
		days := (int(t.Weekday()) + 6) % 7
		return time.Date(t.Year(), t.Month(), t.Day()-days, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

func calcLot(symbol string, entry, sl, riskAmount float64) float64 {
	isGold := strings.Contains(symbol, "XAU") || strings.Contains(symbol, "GOLD")
	isIndex := strings.Contains(symbol, "NAS") || strings.Contains(symbol, "TEC") || strings.Contains(symbol, "US100") || strings.Contains(symbol, "US30") || strings.Contains(symbol, "US500") || strings.Contains(symbol, "NDX")
	priceDist := math.Abs(entry - sl)
	if priceDist <= 0 {
		return 0.01
	}

	var pips float64
	var pipVal float64 = 10.0

	if isGold {
		pips = priceDist / 0.10
	} else if isIndex {
		pips = priceDist / 1.0
		pipVal = 1.0
	} else {
		pips = priceDist / 0.00010
	}

	if pips < 1.0 {
		pips = 1.0
	}

	lot := riskAmount / (pips * pipVal)
	lot = math.Round(lot*100) / 100
	minLot := 0.01
	if isIndex {
		minLot = 0.10
		lot = math.Floor(lot*10) / 10.0
	}
	if lot < minLot {
		return minLot
	}
	if lot > 20.0 {
		return 20.0
	}
	return lot
}

func getSymbolDisplay(sym string) (string, string) {
	upper := strings.ToUpper(sym)
	if strings.Contains(upper, "XAU") || strings.Contains(upper, "GOLD") {
		return "XAU/USD", "🟡"
	}
	if strings.Contains(upper, "EUR") {
		return "EUR/USD", "💶"
	}
	if strings.Contains(upper, "USTEC") || strings.Contains(upper, "NAS") || strings.Contains(upper, "US100") {
		return "USTEC (NAS100)", "📈"
	}
	if strings.Contains(upper, "GBP") {
		return "GBP/USD", "💷"
	}
	return sym, "🌐"
}

func formatPrice(symbol string, val float64) string {
	if strings.Contains(symbol, "XAU") || strings.Contains(symbol, "GOLD") || strings.Contains(symbol, "NAS") || strings.Contains(symbol, "TEC") || strings.Contains(symbol, "US100") {
		return fmt.Sprintf("%.2f", val)
	}
	return fmt.Sprintf("%.5f", val)
}

func getILA2ActionPlan(status, htf, ltf, bias string) string {
	switch status {
	case "WAIT_HTF_SWEEP":
		return fmt.Sprintf("Liquidity Hunting: Dealing range %s aktif. Mengawasi aksi manipulasi harga berupa wick sweep (liquidity grab) pada ekor candle acuan di dalam FVG %s untuk memvalidasi pergeseran order flow institusional.", htf, htf)
	case "WAIT_HTF_CANDLE_CLOSE":
		return fmt.Sprintf("Displacement Verification: Candle sweep %s sedang aktif menyerap likuiditas di dalam FVG. Wajib menunggu penutupan lilin (closed candle) resmi untuk memvalidasi rejection wick dan memastikan displacement bertahan tanpa menembus batas FVG.", htf)
	case "WAIT_LTF_CISD":
		return fmt.Sprintf("Trigger Gate (Candle 5): Memasuki jendela eksekusi %s. Mengawasi Change In State of Delivery (CISD) via candle %s closed body menembus origin opposing leg sebelum titik ekstrim untuk konfirmasi entri yang presisi.", ltf, ltf)
	case "READY_TO_ENTRY":
		return fmt.Sprintf("Market Execution: Konfirmasi ganda terverifikasi (HTF Liquidity Sweep + LTF CISD Valid by body). Buka posisi %s di open candle %s berikutnya dengan proteksi Structural SL di titik ekstrim dan target DOL.", bias, ltf)
	case "DAILY_TRADE_LOCK":
		return "Risk Management Gate: Setup terkonfirmasi valid namun auto-eksekusi ditahan demi mematuhi kuota harian prop firm (1 Win = Stop / Max 2 Trades). Keputusan entri manual tetap di tangan Anda jika disetujui."
	case "OPEN_POS_HOLD":
		return "Anti-Overlapping Gate: Setup terkonfirmasi valid namun auto-eksekusi ditahan karena masih ada posisi terbuka yang sedang berjalan di pasar. Menjaga batas risiko eksposur akun."
	case "INVALIDATED":
		return fmt.Sprintf("Structure Invalidation: Setup sweep di %s batal/hangus karena ekor sweep ditembus atau jendela Candle 5 ditutup tanpa CISD. Reset pemantauan ke dealing range FVG berikutnya.", htf)
	default:
		return fmt.Sprintf("Memantau pembentukan formasi FVG dan reaksi likuiditas berikutnya pada timeframe %s.", htf)
	}
}

func makeILA2RadarBox(statusTitle, cycleDesc, dolStr, invStr string) string {
	return fmt.Sprintf("```text\n"+
		"┌──────────────┬──────────────────────────────┐\n"+
		"│ Status Setup │ %s │\n"+
		"│ Siklus Lilin │ %s │\n"+
		"│ Target (DOL) │ %s │\n"+
		"│ Invalidation │ %s │\n"+
		"└──────────────┴──────────────────────────────┘\n```",
		padRight(statusTitle, 28),
		padRight(cycleDesc, 28),
		padRight(dolStr, 28),
		padRight(invStr, 28))
}

func makeILA2OrderBox(brokerName, tfStr, ticket, entry, sl, tp, rr, riskLot string) string {
	return fmt.Sprintf("```text\n"+
		"┌──────────────┬──────────────────────────────┐\n"+
		"│ Broker/Akun  │ %s │\n"+
		"│ Setup & TF   │ %s │\n"+
		"│ Tiket Order  │ %s │\n"+
		"├──────────────┼──────────────────────────────┤\n"+
		"│ Entry Price  │ %s │\n"+
		"│ Stop Loss    │ %s │\n"+
		"│ Take Profit  │ %s │\n"+
		"│ Risk / Reward│ %s │\n"+
		"│ Risiko & Lot │ %s │\n"+
		"└──────────────┴──────────────────────────────┘\n```",
		padRight(brokerName, 28),
		padRight(tfStr, 28),
		padRight(ticket, 28),
		padRight(entry, 28),
		padRight(sl, 28),
		padRight(tp, 28),
		padRight(rr, 28),
		padRight(riskLot, 28))
}

var executeEnabled = true

func main() {
	flag.BoolVar(&executeEnabled, "execute", true,
		"kirim order ke broker; false = hanya memantau dan memberi notifikasi")
	flag.Parse()

	_ = godotenv.Load("/opt/data/ila-engine/.env")
	var messages []string

	// 1. Connect SQLite
	db, err := gorm.Open(sqlite.Open(DBPath()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open db: %v\n", err)
		os.Exit(1)
	}

	_ = db.AutoMigrate(&model.TradeJournal{}, &model.Position{}, &model.TradeEvaluation{})
	positionRepo := repository.NewPositionRepository(db)
	evalRepo := repository.NewEvaluationRepository(db)
	feedProvider := marketfeed.NewMultiFeedProvider()
	evalService := service.NewEvaluationService(evalRepo, positionRepo, feedProvider)

	// Inisialisasi Broker: Match-Trader (#1089632) khusus ILA 2.0
	mtrBroker := broker.NewMatchTraderBroker("")
	var tradeBroker broker.Broker = mtrBroker

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// 2. Check Closed Positions (TP / SL / Exit)
	var openPositions []model.Position
	db.Where("status = ?", "OPEN").Find(&openPositions)

	if len(openPositions) > 0 {
		activePositions, err := tradeBroker.GetOpenPositions(ctx)
		if err == nil {
			activeMap := make(map[string]bool)
			for _, ap := range activePositions {
				activeMap[ap.TicketNumber] = true
			}

			for _, pos := range openPositions {
				cPosID, mtrID := broker.ExtractTicketIDs(pos.TicketNumber)
				cPosIDStr := strconv.FormatInt(cPosID, 10)

				// Cek apakah posisi masih aktif di broker
				isOpen := false
				if activeMap[pos.TicketNumber] {
					isOpen = true
				}
				if cPosID > 0 && activeMap[cPosIDStr] {
					isOpen = true
				}
				if mtrID != "" && activeMap[mtrID] {
					isOpen = true
				}
				if isOpen {
					continue
				}

				// Posisi tidak ada di daftar open. Cek konfirmasi closed position dari Match-Trader
				exitPrice := pos.OpenPrice
				actualPnL := 0.0
				isDealFound := false

				if mtrBroker != nil && mtrID != "" {
					closedMtr, _ := mtrBroker.GetClosedPositions(ctx, time.Now().Add(-48*time.Hour), time.Now())
					for _, cp := range closedMtr {
						if cp.ID == mtrID || strings.Contains(pos.TicketNumber, cp.ID) {
							exitPrice = cp.ClosePrice
							actualPnL = cp.Profit
							isDealFound = true
							break
						}
					}
				}

				// Jika deal penutup belum ditemukan dari broker, jangan tutup prematur di database!
				if !isDealFound {
					continue
				}

				isGold := strings.Contains(pos.Symbol, "XAU") || strings.Contains(pos.Symbol, "GOLD")
				tol := 0.50
				if !isGold {
					tol = 0.00050
				}

				hitTP := false
				hitSL := false

				if pos.Direction == "BUY" {
					if exitPrice >= (pos.TakeProfit - tol) {
						hitTP = true
					} else if exitPrice <= (pos.StopLoss + tol) {
						hitSL = true
					}
				} else {
					if exitPrice <= (pos.TakeProfit + tol) {
						hitTP = true
					} else if exitPrice >= (pos.StopLoss - tol) {
						hitSL = true
					}
				}

				if !hitTP && !hitSL {
					if actualPnL > 0 {
						hitTP = true
					} else {
						hitSL = true
					}
				}

				status := "CLOSED_SL"
				outcomeR := -1.0
				if hitTP {
					status = "CLOSED_TP"
					outcomeR = pos.RiskReward
				}

				now := time.Now().UTC()
				db.Model(&model.Position{}).Where("id = ?", pos.ID).Updates(map[string]interface{}{
					"status":      status,
					"exit_price":  exitPrice,
					"profit_loss": actualPnL,
					"outcome_r":   outcomeR,
					"closed_at":   now,
				})

				pos.Status = status
				pos.ExitPrice = exitPrice
				pos.ProfitLoss = actualPnL
				pos.OutcomeR = outcomeR
				pos.ClosedAt = &now

				// Run post-trade self learning evaluation
				_, _ = evalService.EvaluateClosedTrade(ctx, &pos)
			}
		}
	}

	// 2.1 Ensure all closed positions have evaluations created
	_, _ = evalService.EvaluateAllClosedPositions(ctx)

	// 2.2 Process any unnotified evaluations and send to Telegram!
	unnotified, _ := evalService.GetUnnotifiedEvaluations(ctx)
	if len(unnotified) > 0 {
		acc, _ := mtrBroker.GetAccountInfo(ctx)
		currBal := 10000.0
		if acc != nil && acc.Balance > 0 {
			currBal = acc.Balance
		}

		for _, ev := range unnotified {
			statusTitle := "🏁 **[TRADE CLOSED] HIT TAKE PROFIT 🎯**"
			if ev.Outcome == "HIT_SL" {
				statusTitle = "🛑 **[TRADE CLOSED] HIT STOP LOSS**"
			}
			rSign := "+"
			if ev.OutcomeR < 0 {
				rSign = ""
			}
			pnlSign := "+"
			if ev.ProfitLoss < 0 {
				pnlSign = ""
			}
			sName, sEmoji := getSymbolDisplay(ev.Symbol)
			pnlPercent := (ev.ProfitLoss / currBal) * 100.0

			closeMsg := fmt.Sprintf(`%s
• **Tiket:** `+"`#%s`"+` (%s %s %s)
• **Entry:** `+"`%s`"+` ➡️ **Exit:** `+"`%s`"+`
• **Hasil:** `+"`%s%.2f R`"+` (`+"`%s$%.2f`"+` / %s%.2f%%)
• **Saldo:** `+"`$%.2f`"+`
🧠 **Evaluasi & Pelajaran AI:**
• **Faktor:** %s
• **Analisa:** %s
• **Pelajaran:** %s`,
				statusTitle, ev.TicketNumber, sEmoji, sName, ev.Direction,
				formatPrice(ev.Symbol, ev.OpenPrice), formatPrice(ev.Symbol, ev.ExitPrice),
				rSign, ev.OutcomeR, pnlSign, ev.ProfitLoss, pnlSign, pnlPercent,
				currBal, ev.RootCause, ev.TechnicalAnalysis, ev.ActionableLesson,
			)
			messages = append(messages, closeMsg)
			_ = evalService.MarkNotified(ctx, ev.ID)
		}
	}

	// 3. Trade Management Harian (WIB / Jam Jakarta): 1 Win = Stop, Max 2 Trades, 0.5% Risk
	dailyStatus, _ := positionRepo.GetDailyTradeStatusWIB(ctx, time.Now())
	isTradeLocked := dailyStatus != nil && !dailyStatus.CanTrade
	tradeLockReason := ""
	if isTradeLocked {
		tradeLockReason = dailyStatus.LockReason
	}

	// 4. Load State & Analyze Pairs
	state := loadState()

	nowUtc := time.Now().UTC()
	utcHour := nowUtc.Hour()
	utcDay := nowUtc.Weekday()
	marketClosed := market.IsClosed(utcDay, utcHour)
	currentSession := market.Session(utcDay, utcHour)

	lastSession := state.LastSession
	inActiveSession := market.IsActiveSession(currentSession)

	// Session Transition Briefing
	if inActiveSession && currentSession != lastSession {
		state.LastSession = currentSession
		sessTitle := strings.ReplaceAll(currentSession, "_", " ")
		messages = append(messages, fmt.Sprintf("🔔 **[%s] Sesi Pasar Aktif (Mode ILA 2.0)**\nRadar ILA aktif memantau setup adaptif berbasis penutupan lilin.", strings.ToUpper(sessTitle)))
	} else if !inActiveSession {
		state.LastSession = currentSession
	}

	// 5. ILA 2.0 Multi-Timeframe Scan & Auto-Execution
	// Market closed (weekend / rollover) -> skip scan & execution entirely.
	if marketClosed {
		saveState(state)
		if len(messages) > 0 {
			fmt.Println(strings.Join(messages, "\n\n---\n\n"))
		}
		return
	}

	ilaService := service.NewILA2Service(feedProvider)
	allILAPairs := []struct {
		HTF      string
		LTF      string
		PairName string
	}{
		{HTF: "H4", LTF: "M15", PairName: "H4 -> M15"},
		{HTF: "H1", LTF: "M5", PairName: "H1 -> M5"},
	}

	for _, sym := range Pairs {
		symName, symEmoji := getSymbolDisplay(sym)

		for _, ep := range allILAPairs {
			ilaKey := fmt.Sprintf("%s-%s-%s", sym, ep.HTF, ep.LTF)
			prevILA := state.ILAPairs[ilaKey]

			// Scheduler Berbasis Penutupan Lilin (Candle-Close Scheduling):
			// Jika status menunggu CISD di LTF, jadwal mengikuti penutupan lilin LTF.
			// Selain itu, jadwal mengikuti penutupan lilin HTF.
			checkTF := ep.HTF
			if prevILA.LastStatus == "WAIT_LTF_CISD" || prevILA.LastStatus == "READY_TO_ENTRY" {
				checkTF = ep.LTF
			}

			if !slotDue(prevILA.LastEvaluatedAt, checkTF, nowUtc, 3*time.Second) {
				continue
			}

			setup, err := ilaService.AnalyzeILA2(ctx, dto.ILA2AnalysisRequest{
				Symbol: sym,
				HTF:    ep.HTF,
				LTF:    ep.LTF,
				Limit:  100,
			})
			if err != nil {
				continue
			}

			prevILA.LastEvaluatedAt = nowUtc

			dir := "BUY"
			if setup.Bias == "BEARISH" {
				dir = "SELL"
			}
			dirEmoji := "🟢 BUY"
			if dir == "SELL" {
				dirEmoji = "🔴 SELL"
			}

			// Scenario A: Auto-Execution / Entry Ready Alert (H4 ➡️ M15 & H1 ➡️ M5)
			if (ep.HTF == "H4" || ep.HTF == "H1") && setup.Status == "READY_TO_ENTRY" && setup.PassedMinRR && setup.HTFSweep != nil && !setup.HTFSweep.SweepInProgress && setup.CISD != nil && setup.CISD.Detected {
				breakTimeStr := setup.CISD.BreakTime.Format(time.RFC3339)
				if prevILA.LastBreakTime != breakTimeStr || prevILA.LastStatus != "ENTRY" {
					entryPrice := setup.EntryPrice
					sl := setup.StopLoss
					tp := setup.TakeProfit

					riskPercent := DefaultRiskPercent
					equity := AccountSize
					acc, err := tradeBroker.GetAccountInfo(ctx)
					if err == nil && acc != nil && acc.Equity > 0 {
						equity = acc.Equity
					} else if acc != nil && acc.Balance > 0 {
						equity = acc.Balance
					}
					riskDollars := equity * (riskPercent / 100.0)
					lot := calcLot(sym, entryPrice, sl, riskDollars)

					setupID := fmt.Sprintf("ILA-%s-%s-%s-%s", sym, ep.HTF, ep.LTF, time.Now().Format("200601021504"))

					if isTradeLocked {
						if prevILA.LastStatus != "DAILY_TRADE_LOCK" || prevILA.LastBreakTime != breakTimeStr {
							table := makeILA2OrderBox(
								"Match-Trader Prop (#1089632)",
								fmt.Sprintf("ILA 2.0 (%s ➡️ %s)", ep.HTF, ep.LTF),
								"ENTRY MANUAL (LOCK)",
								formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp),
								fmt.Sprintf("1:%.2f", setup.RiskReward),
								fmt.Sprintf("%.2f%% ($%.2f)", riskPercent, riskDollars),
							)
							actionPlan := getILA2ActionPlan("DAILY_TRADE_LOCK", ep.HTF, ep.LTF, setup.Bias)
							msg := fmt.Sprintf("🎯 **[SETUP ILA VALID - ENTRY MANUAL] %s %s • %s**\n`Timeframe: %s ➡️ %s`\n\n"+
								"🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n"+
								"⚡ **TRIGGER LTF (%s):**\n• %s\n\n"+
								"%s\n\n"+
								"🛑 **AUTO-EKSEKUSI DITAHAN:**\n• %s\n\n"+
								"👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
								symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
								ep.HTF, setup.HTFSweep.Details,
								ep.LTF, setup.CISD.Details,
								table, tradeLockReason, actionPlan)
							messages = append(messages, msg)
							prevILA.LastBreakTime = breakTimeStr
							prevILA.LastStatus = "DAILY_TRADE_LOCK"
							prevILA.LastHTFStatus = setup.Status
							state.ILAPairs[ilaKey] = prevILA
						}
						continue
					}

					var countOpenILA int64
					db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN' AND timeframe = ? AND (setup_id LIKE 'ILA%' OR notes LIKE '%ILA%')", sym, ep.LTF).Count(&countOpenILA)
					if countOpenILA > 0 {
						if prevILA.LastStatus != "OPEN_POS_HOLD" || prevILA.LastBreakTime != breakTimeStr {
							var existingPos model.Position
							db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN' AND timeframe = ? AND (setup_id LIKE 'ILA%' OR notes LIKE '%ILA%')", sym, ep.LTF).First(&existingPos)
							table := makeILA2OrderBox(
								"Match-Trader Prop (#1089632)",
								fmt.Sprintf("ILA 2.0 (%s ➡️ %s)", ep.HTF, ep.LTF),
								"ENTRY MANUAL (HOLD)",
								formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp),
								fmt.Sprintf("1:%.2f", setup.RiskReward),
								fmt.Sprintf("%.2f%% ($%.2f)", riskPercent, riskDollars),
							)
							actionPlan := getILA2ActionPlan("OPEN_POS_HOLD", ep.HTF, ep.LTF, setup.Bias)
							msg := fmt.Sprintf("⏳ **[SETUP ILA VALID - ENTRY MANUAL] %s %s • %s**\n`Timeframe: %s ➡️ %s`\n\n"+
								"🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n"+
								"⚡ **TRIGGER LTF (%s):**\n• %s\n\n"+
								"%s\n\n"+
								"⚠️ **AUTO-EKSEKUSI DITAHAN:**\n• Masih ada posisi aktif ILA (#%s %s %.2f lot).\n\n"+
								"👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
								symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
								ep.HTF, setup.HTFSweep.Details,
								ep.LTF, setup.CISD.Details,
								table, existingPos.TicketNumber, existingPos.Direction, existingPos.LotSize, actionPlan)
							messages = append(messages, msg)
							prevILA.LastBreakTime = breakTimeStr
							prevILA.LastStatus = "OPEN_POS_HOLD"
							prevILA.LastHTFStatus = setup.Status
							state.ILAPairs[ilaKey] = prevILA
						}
						continue
					}

					if !executeEnabled {
						table := makeILA2OrderBox(
							"Match-Trader Prop (#1089632)",
							fmt.Sprintf("ILA 2.0 (%s ➡️ %s)", ep.HTF, ep.LTF),
							"MODE PANTAU",
							formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp),
							fmt.Sprintf("1:%.2f", setup.RiskReward),
							fmt.Sprintf("%.2f%% ($%.2f)", riskPercent, riskDollars),
						)
						actionPlan := getILA2ActionPlan("READY_TO_ENTRY", ep.HTF, ep.LTF, setup.Bias)
						msg := fmt.Sprintf("🎯 **[SETUP ILA VALID - MODE PANTAU] %s %s • %s**\n`Timeframe: %s ➡️ %s`\n\n"+
							"🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n"+
							"⚡ **TRIGGER LTF (%s):**\n• %s\n\n"+
							"%s\n\n"+
							"👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
							symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
							ep.HTF, setup.HTFSweep.Details,
							ep.LTF, setup.CISD.Details,
							table, actionPlan)
						messages = append(messages, msg)
						prevILA.LastStatus = "READY_TO_ENTRY"
						state.ILAPairs[ilaKey] = prevILA
						continue
					}

					orderRes, err := tradeBroker.ExecuteOrder(ctx, broker.OrderRequest{
						Symbol:     sym,
						Direction:  dir,
						OpenPrice:  entryPrice,
						StopLoss:   sl,
						TakeProfit: tp,
						LotSize:    lot,
						Comment:    fmt.Sprintf("ILA %s->%s %s", ep.HTF, ep.LTF, setupID),
						SetupID:    setupID,
					})

					if err == nil {
						nowDt := time.Now()
						db.Create(&model.Position{
							Base: model.Base{
								ID:        uuid.New().String(),
								CreatedAt: nowDt,
								UpdatedAt: nowDt,
							},
							TicketNumber: orderRes.TicketNumber,
							SetupID:      setupID,
							Symbol:       sym,
							Timeframe:    ep.LTF,
							Direction:    dir,
							OpenPrice:    entryPrice,
							StopLoss:     sl,
							TakeProfit:   tp,
							LotSize:      lot,
							RiskReward:   setup.RiskReward,
							Status:       "OPEN",
							OpenedAt:     nowDt,
							Notes:        fmt.Sprintf("Broker: Match-Trader (1089632) | Metode: ILA 2.0 (%s ➡️ %s) | %s", ep.HTF, ep.LTF, setup.Summary),
						})

						table := makeILA2OrderBox(
							"Match-Trader Prop (#1089632)",
							fmt.Sprintf("ILA 2.0 (%s ➡️ %s)", ep.HTF, ep.LTF),
							"#"+orderRes.TicketNumber,
							formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp),
							fmt.Sprintf("1:%.2f", setup.RiskReward),
							fmt.Sprintf("%.2f%% ($%.2f) | %.2f Lot", riskPercent, riskDollars, lot),
						)
						actionPlan := getILA2ActionPlan("READY_TO_ENTRY", ep.HTF, ep.LTF, setup.Bias)
						entryAlert := fmt.Sprintf("🚀 **[ORDER REAL ILA 2.0] %s %s • %s**\n`Timeframe: %s ➡️ %s`\n\n"+
							"🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n"+
							"⚡ **TRIGGER LTF (%s):**\n• %s\n\n"+
							"%s\n\n"+
							"👉 **TRADING ACTION PLAN & MANAGEMENT:**\n• %s",
							symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
							ep.HTF, setup.HTFSweep.Details,
							ep.LTF, setup.CISD.Details,
							table, actionPlan)
						messages = append(messages, entryAlert)

						prevILA.LastBreakTime = breakTimeStr
						prevILA.LastStatus = "ENTRY"
						prevILA.LastHTFStatus = setup.Status
						state.ILAPairs[ilaKey] = prevILA
					} else {
						messages = append(messages, fmt.Sprintf("⚠️ **Gagal Auto-Eksekusi ILA Match-Trader (%s %s ➡️ %s):** %v", sym, ep.HTF, ep.LTF, err))
					}
					continue
				}
			}

			// Scenario B: Progress Radar Alert per Timeframe Slot (Single-Slot Dedicated Bubble)
			if prevILA.LastStatus != setup.Status {
				if inActiveSession && prevILA.LastStatus != "" {
					stLabel := "⏳ Menunggu"
					switch setup.Status {
					case "WAIT_HTF_SWEEP":
						stLabel = "⏳ Menunggu Sweep FVG"
					case "WAIT_HTF_CANDLE_CLOSE":
						stLabel = fmt.Sprintf("⏳ Menunggu Close %s", ep.HTF)
					case "WAIT_LTF_CISD":
						stLabel = "⏳ Berburu Trigger CISD"
					case "READY_TO_ENTRY":
						stLabel = "🎯 Siap Eksekusi"
					case "INVALIDATED":
						stLabel = "🛑 Setup Expired/Batal"
					}

					cycleDesc := fmt.Sprintf("Lilin %s (%s)", ep.HTF, formatNYTime(nowUtc))
					dolStr := "Mencari Eksternal DOL"
					if setup.TakeProfit > 0 {
						dolStr = formatPrice(sym, setup.TakeProfit)
					}
					invStr := "Batas Ekor HTF"
					if setup.StopLoss > 0 {
						invStr = formatPrice(sym, setup.StopLoss)
					}

					table := makeILA2RadarBox(stLabel, cycleDesc, dolStr, invStr)
					actionPlan := getILA2ActionPlan(setup.Status, ep.HTF, ep.LTF, setup.Bias)

					htfDetail := setup.Summary
					if setup.HTFSweep != nil && setup.HTFSweep.Details != "" {
						htfDetail = setup.HTFSweep.Details
					}
					ltfDetail := "Memasuki jendela Candle 5. Menunggu validasi CISD."
					if setup.CISD != nil && setup.CISD.Details != "" {
						ltfDetail = setup.CISD.Details
					}

					radarMsg := fmt.Sprintf("📊 **[ILA 2.0 RADAR] %s %s • %s**\n`Timeframe Slot: %s ➡️ %s`\n\n%s\n\n🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n⚡ **TRIGGER LTF (%s):**\n• %s\n\n👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
						symEmoji, symName, dirEmoji, ep.HTF, ep.LTF, table, ep.HTF, htfDetail, ep.LTF, ltfDetail, actionPlan)
					messages = append(messages, radarMsg)
				}

				finalStatus := setup.Status
				if prevILA.LastStatus == "ENTRY" && setup.Status == "READY_TO_ENTRY" {
					finalStatus = "ENTRY"
				}
				prevILA.LastStatus = finalStatus
				prevILA.LastHTFStatus = setup.Status
			}
			state.ILAPairs[ilaKey] = prevILA
		}
	}

	saveState(state)

	if len(messages) > 0 {
		fmt.Println(strings.Join(messages, "\n\n---\n\n"))
	}
}
