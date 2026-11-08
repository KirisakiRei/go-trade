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
	DefaultRiskPercent = 0.50
	MinRR              = 2.0
	MaxRR              = 2.3
)

var Pairs = []string{"XAUUSD", "EURUSD", "USTEC"}

// DBPath dan StateFilePath mengikuti ILA_DATA_DIR agar aman di Docker.
func DBPath() string        { return datadir.Berkas("ila.db") }
func StateFilePath() string { return datadir.Berkas("monitor_state.json") }

type PairState struct {
	LastM15AsOf   string `json:"last_m15_as_of"`
	LastH4Phase   string `json:"last_h4_ph"`
	LastH1Phase   string `json:"last_h1_ph"`
	LastM15Phase  string `json:"last_m15_ph"`
	LastGrade     string `json:"last_grade"`
	LastAlertType string `json:"last_alert_type"`
}

type ILAPairState struct {
	LastBreakTime string `json:"last_break_time"`
	LastStatus    string `json:"last_status"`
	LastHTFStatus string `json:"last_htf_status"`
}

type MonitorState struct {
	LastSession string                  `json:"last_session"`
	Pairs       map[string]PairState    `json:"pairs"`
	ILAPairs    map[string]ILAPairState `json:"ila_pairs"`
}

func loadState() MonitorState {
	var s MonitorState
	s.Pairs = make(map[string]PairState)
	s.ILAPairs = make(map[string]ILAPairState)
	data, err := os.ReadFile(StateFilePath())
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	if s.Pairs == nil {
		s.Pairs = make(map[string]PairState)
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

func statusLabel(st string) string {
	mapping := map[string]string{
		"SETUP_VALID": "Setup Terkonfirmasi",
		"CANDIDATE":   "Kandidat Setup (Terbentuk)",
		"WAIT":        "Menunggu Konfirmasi",
		"INVALIDATED": "Setup Dibatalkan",
	}
	if v, ok := mapping[st]; ok {
		return v
	}
	return strings.ReplaceAll(st, "_", " ")
}

func biasLabel(b string) string {
	mapping := map[string]string{
		"BULLISH":  "Bullish (Naik) 🟢",
		"BEARISH":  "Bearish (Turun) 🔴",
		"NEUTRAL":  "Netral ⚖️",
		"CONFLICT": "Konflik Arah ⚠️",
		"WAIT":     "Wait & See ⏳",
	}
	if v, ok := mapping[b]; ok {
		return v
	}
	return b
}

func patternLabel(p string) string {
	res := strings.ReplaceAll(p, "MOMENTUM", "Momentum")
	res = strings.ReplaceAll(res, "REENTRY", "Re-entry")
	res = strings.ReplaceAll(res, "CSAK", "CSAK")
	res = strings.ReplaceAll(res, "EXTREME", "Extreme")
	res = strings.ReplaceAll(res, "MHV", "MHV")
	res = strings.ReplaceAll(res, "SIDEWAYS", "Sideways")
	res = strings.ReplaceAll(res, "NONE", "Netral")
	return strings.ReplaceAll(res, "->", "➡️")
}

func cleanSessionName(sess string) string {
	s := strings.ReplaceAll(sess, "LONDON_NEWYORK_OVERLAP", "Sesi Overlap London - New York")
	s = strings.ReplaceAll(s, "LONDON_SESSION", "Sesi London")
	s = strings.ReplaceAll(s, "NEWYORK_SESSION", "Sesi New York")
	s = strings.ReplaceAll(s, "ASIAN_SESSION", "Sesi Asia")
	s = strings.ReplaceAll(s, "ROLLOVER_BLACKOUT", "Jeda Rollover")
	s = strings.ReplaceAll(s, "WEEKEND_CLOSED", "Libur Akhir Pekan")
	return s
}

func determineNextAction(grade, bias, m15Ph, m15St string, rr float64) string {
	if rr > 0 && rr < MinRR {
		return fmt.Sprintf("⏳ Tahan Dulu: Rasio R:R (1:%.2f) belum mencapai syarat minimal 1:2. Menunggu harga koreksi lebih dalam mendekati level SL.", rr)
	}
	if grade == "CONFLICT" {
		return "🛑 Wait & See: Arah timeframe besar (H4/H1) dan trigger (M15) masih berlawanan."
	}
	if grade == "INVALID_CSM_AT_TF2" {
		return "🛑 Re-entry Batal: Terbentuk Candle Momentum (CSM) di H1 yang membatalkan setup Re-entry."
	}
	if grade == "INVALID_CSM_AT_TRIGGER" {
		return "🛑 Trigger Batal: Candle M15 menembus BB sebagai CSM berlawanan arah."
	}
	if grade == "GRADE_B_STRONG" {
		return "👀 Mengintai Zona Entry: Tren selaras, menunggu konfirmasi trigger M15 di zona MA."
	}
	if m15Ph == "REENTRY_CONFIRMED" && m15St == "WAIT" {
		return "⏳ Menunggu Pullback: Menunggu candle M15 koreksi masuk ke celah garis MA 5/10."
	}
	if m15Ph == "CS_KUKUH" {
		return "⏳ Menunggu Re-entry: CSAK baru terbentuk, menunggu candle retracement ke MA 5/10."
	}
	if m15Ph == "EXTREME_CONFIRMED" {
		return "⏳ Menunggu Siklus: Extreme baru terbentuk, pantau TP Wajib dan MHV berikutnya."
	}
	if m15St == "SETUP_VALID" && rr >= MinRR {
		return "🎯 Siap Eksekusi: Setup valid dan R:R memenuhi syarat minimal 1:2."
	}
	return "⏳ Menunggu Pembentukan Setup Valid pada Closed Candle M15."
}

func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

type ILARowDisplay struct {
	PairName string
	Bias     string
	Status   string
	Changed  bool
}

func makeILATable(pairs []ILARowDisplay) string {
	rows := []string{}
	for _, p := range pairs {
		biasStr := "⚪ Netr"
		if p.Bias == "BEARISH" {
			biasStr = "🔴 Sell"
		} else if p.Bias == "BULLISH" {
			biasStr = "🟢 Buy "
		}

		mark := ""
		if p.Changed {
			mark = " ◀"
		}
		stFull := padRight(p.Status+mark, 20)

		rows = append(rows, fmt.Sprintf("│ %s │ %s │ %s │",
			padRight(p.PairName, 9), biasStr, stFull))
	}

	return fmt.Sprintf("```text\n"+
		"┌───────────┬─────────┬──────────────────────┐\n"+
		"│ Pasangan  │  Arah   │        Status        │\n"+
		"├───────────┼─────────┼──────────────────────┤\n"+
		"%s\n"+
		"└───────────┴─────────┴──────────────────────┘\n```",
		strings.Join(rows, "\n"),
	)
}

// executeEnabled menandai apakah scanner boleh mengirim order ke broker.
//
// Default true: perilaku lama tidak berubah. Set -execute=false saat Runner
// sudah mengambil alih eksekusi, agar tidak terjadi dua order untuk setup yang
// sama (plans/04 §11). Pemantauan dan notifikasi tetap berjalan.
var executeEnabled = true

func main() {
	flag.BoolVar(&executeEnabled, "execute", true,
		"kirim order ke broker; false = hanya memantau dan memberi notifikasi")
	flag.Parse()

	if !executeEnabled {
		fmt.Fprintln(os.Stderr, "EKSEKUSI DIMATIKAN: hanya memantau, tidak mengirim order")
	}

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
		activePositions, _ := tradeBroker.GetOpenPositions(ctx)
		activeMap := make(map[string]bool)
		for _, ap := range activePositions {
			activeMap[ap.TicketNumber] = true
		}

		for _, pos := range openPositions {
			cPosID, mtrID := broker.ExtractTicketIDs(pos.TicketNumber)
			cPosIDStr := strconv.FormatInt(cPosID, 10)

			// Cek apakah posisi masih aktif di salah satu broker
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

			// Posisi sudah resmi tertutup di server broker! Tarik deal detailnya
			exitPrice := pos.OpenPrice
			actualPnL := 0.0

			// Tarik closed position dari Match-Trader
			if mtrBroker != nil && mtrID != "" {
				closedMtr, _ := mtrBroker.GetClosedPositions(ctx, time.Now().Add(-24*time.Hour), time.Now())
				for _, cp := range closedMtr {
					if cp.ID == mtrID || strings.Contains(pos.TicketNumber, cp.ID) {
						exitPrice = cp.ClosePrice
						actualPnL = cp.Profit
						break
					}
				}
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
	// Jam pasar dipakai dari satu sumber agar ILA 2.0 dan 3.0 tidak berbeda.
	marketClosed := market.IsClosed(utcDay, utcHour)
	currentSession := market.Session(utcDay, utcHour)

	lastSession := state.LastSession
	inActiveSession := market.IsActiveSession(currentSession)

	// Session Transition Briefing
	if inActiveSession && currentSession != lastSession {
		state.LastSession = currentSession
		sessTitle := cleanSessionName(currentSession)
		messages = append(messages, fmt.Sprintf("🔔 **[%s] Sesi Pasar Aktif (Mode ILA)**\nRadar ILA aktif memantau setup di 4 pasangan timeframe (XAUUSD, EURUSD, USTEC).", strings.ToUpper(sessTitle)))
	} else if !inActiveSession {
		state.LastSession = currentSession
	}

	// 7. ILA (Imaginary Liquidity) Multi-Timeframe Scan & Auto-Execution
	// Market closed (weekend / rollover) -> skip scan & execution entirely.
	if marketClosed {
		saveState(state)
		return
	}

	ilaService := service.NewILA2Service(feedProvider)
	allILAPairs := []struct {
		HTF      string
		LTF      string
		PairName string
	}{
		{HTF: "W", LTF: "H4", PairName: "W -> H4"},
		{HTF: "D1", LTF: "H1", PairName: "D1 -> H1"},
		{HTF: "H4", LTF: "M15", PairName: "H4 -> M15"},
		{HTF: "H1", LTF: "M5", PairName: "H1 -> M5"},
	}

	for _, sym := range Pairs {
		symName, symEmoji := getSymbolDisplay(sym)

		var ilaRows []ILARowDisplay
		var changedPairTitle string
		var changedEventDesc string
		var changedNextAction string

		for _, ep := range allILAPairs {
			ilaKey := fmt.Sprintf("%s-%s-%s", sym, ep.HTF, ep.LTF)
			setup, err := ilaService.AnalyzeILA2(ctx, dto.ILA2AnalysisRequest{
				Symbol: sym,
				HTF:    ep.HTF,
				LTF:    ep.LTF,
				Limit:  100,
			})
			if err != nil {
				continue
			}

			prevILA := state.ILAPairs[ilaKey]

			// Scenario A: ILA Auto-Execution (Focus on actionable intraday/swing pairs)
			if (ep.HTF == "H4" || ep.HTF == "D1") && setup.Status == "READY_TO_ENTRY" && setup.PassedMinRR && setup.HTFSweep != nil && !setup.HTFSweep.SweepInProgress && setup.CISD != nil && setup.CISD.Detected {
				breakTimeStr := setup.CISD.BreakTime.Format(time.RFC3339)
				if prevILA.LastBreakTime != breakTimeStr || prevILA.LastStatus != "ENTRY" {
					dir := "BUY"
					if setup.Bias == "BEARISH" {
						dir = "SELL"
					}
					dirEmoji := "🟢 BUY"
					if dir == "SELL" {
						dirEmoji = "🔴 SELL"
					}

					if isTradeLocked {
						if prevILA.LastStatus != "DAILY_TRADE_LOCK" || prevILA.LastBreakTime != breakTimeStr {
							messages = append(messages, fmt.Sprintf("🎯 **[SETUP ILA VALID - ENTRY MANUAL] %s %s • %s**\n• Setup: ILA (%s ➡️ %s) siap entry.\n• Entry: %s | SL: %s | TP: %s (R:R 1:%.2f)\n• **Auto-eksekusi ditahan:** %s\n• **Keputusan di tangan Anda:** entry manual jika setuju dengan setup ini.",
								symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
								formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss), formatPrice(sym, setup.TakeProfit), setup.RiskReward,
								tradeLockReason))
							prevILA.LastBreakTime = breakTimeStr
							prevILA.LastStatus = "DAILY_TRADE_LOCK"
							prevILA.LastHTFStatus = setup.Status
							state.ILAPairs[ilaKey] = prevILA
						}
						continue
					}

					// Cek posisi aktif khusus ILA di timeframe yang sama agar posisi ILA baru tetap diizinkan buka
					var countOpenILA int64
					db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN' AND timeframe = ? AND (setup_id LIKE 'ILA%' OR notes LIKE '%ILA%')", sym, ep.LTF).Count(&countOpenILA)
					if countOpenILA > 0 {
						if prevILA.LastStatus != "OPEN_POS_HOLD" || prevILA.LastBreakTime != breakTimeStr {
							var existingPos model.Position
							db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN' AND timeframe = ? AND (setup_id LIKE 'ILA%' OR notes LIKE '%ILA%')", sym, ep.LTF).First(&existingPos)
							messages = append(messages, fmt.Sprintf("🎯 **[SETUP ILA VALID - ENTRY MANUAL] %s %s • %s**\n• Setup: ILA (%s ➡️ %s) siap entry.\n• Entry: %s | SL: %s | TP: %s (R:R 1:%.2f)\n• **Auto-eksekusi ditahan:** masih ada posisi ILA terbuka (#%s %s %.2f lot).\n• **Keputusan di tangan Anda:** entry manual jika setuju, atau tunggu posisi lama selesai.",
								symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
								formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss), formatPrice(sym, setup.TakeProfit), setup.RiskReward,
								existingPos.TicketNumber, existingPos.Direction, existingPos.LotSize))
							prevILA.LastBreakTime = breakTimeStr
							prevILA.LastStatus = "OPEN_POS_HOLD"
							prevILA.LastHTFStatus = setup.Status
							state.ILAPairs[ilaKey] = prevILA
						}
						continue
					}

					entryPrice := setup.EntryPrice
					sl := setup.StopLoss
					tp := setup.TakeProfit

					riskPercent := 0.50 // Strictly dynamic 0.5% live equity risk
					equity := 10000.0   // Default fallback equity
					acc, err := tradeBroker.GetAccountInfo(ctx)
					if err == nil && acc != nil && acc.Equity > 0 {
						equity = acc.Equity
					} else if acc != nil && acc.Balance > 0 {
						equity = acc.Balance
					}
					riskDollars := equity * (riskPercent / 100.0)
					lot := calcLot(sym, entryPrice, sl, riskDollars)

					setupID := fmt.Sprintf("ILA-%s-%s-%s-%s", sym, ep.HTF, ep.LTF, time.Now().Format("200601021504"))

					if !executeEnabled {
						// Order tidak dikirim: setup tetap dilaporkan sebagai
						// notifikasi manual agar tidak ada sinyal yang hilang.
						if inActiveSession {
							messages = append(messages, fmt.Sprintf("🎯 **[SETUP ILA VALID - ENTRY MANUAL] %s %s • %s**\n• Setup: ILA (%s ➡️ %s) siap entry.\n• Entry: %s | SL: %s | TP: %s (R:R 1:%.2f)\n• **Auto-eksekusi dimatikan (mode pantau).**",
								symEmoji, symName, dirEmoji, ep.HTF, ep.LTF,
								formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp), setup.RiskReward))
						}
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
							Notes:        fmt.Sprintf("Broker: Match-Trader (1089632) | Metode: ILA (%s ➡️ %s) | %s", ep.HTF, ep.LTF, setup.Summary),
						})

						entryAlert := fmt.Sprintf(`🚀 **[ORDER REAL ILA 2.0 - MATCH-TRADER] %s %s • %s**
• **Akun Eksekusi:** Match-Trader Prop (#1089632)
• **Strategi:** ILA 2.0 (%s ➡️ %s) | **Tiket:** `+"`%s`"+`
• **Entry:** `+"`%s`"+` | **SL:** `+"`%s`"+` | **TP:** `+"`%s`"+`
• **R:R:** `+"`1:%.2f`"+` | **Risiko:** 0.50%% Live Equity
⚡ *Posisi aktif di Match-Trader dengan SL/TP otomatis!*`,
							symEmoji, symName, dirEmoji,
							ep.HTF, ep.LTF, orderRes.TicketNumber,
							formatPrice(sym, entryPrice), formatPrice(sym, sl), formatPrice(sym, tp),
							setup.RiskReward,
						)
						messages = append(messages, entryAlert)

						prevILA.LastBreakTime = breakTimeStr
						prevILA.LastStatus = "ENTRY"
						prevILA.LastHTFStatus = setup.Status
					} else {
						messages = append(messages, fmt.Sprintf("⚠️ **Gagal Auto-Eksekusi ILA Match-Trader (%s %s ➡️ %s):** %v", sym, ep.HTF, ep.LTF, err))
					}
				}
			}

			// Determine display status label
			stLabel := "Menunggu"
			if setup.Status == "READY_TO_ENTRY" {
				stLabel = "🎯 Siap Entry"
			} else if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
				stLabel = fmt.Sprintf("⏳ Tunggu Close %s", ep.HTF)
			} else if setup.Status == "WAIT_LTF_CISD" {
				stLabel = "⏳ Tunggu CISD"
			} else if setup.Status == "WAIT_HTF_SWEEP" {
				stLabel = "⏳ Tunggu Sweep"
			} else if setup.Status == "WAIT_HTF_FVG" {
				stLabel = "⏳ Belum Ada FVG"
			}

			isPairChanged := prevILA.LastStatus != "" && prevILA.LastStatus != setup.Status
			if isPairChanged && changedPairTitle == "" {
				changedPairTitle = fmt.Sprintf("%s ➡️ %s", ep.HTF, ep.LTF)
				if setup.Status == "WAIT_HTF_CANDLE_CLOSE" {
					changedEventDesc = fmt.Sprintf("Candle sweep %s sedang terbentuk di dalam FVG (masih running).", ep.HTF)
				} else if setup.Status == "WAIT_LTF_CISD" {
					changedEventDesc = fmt.Sprintf("Candle sweep %s resmi TUTUP di dalam FVG & sweep valid.", ep.HTF)
				} else if setup.Status == "READY_TO_ENTRY" {
					changedEventDesc = fmt.Sprintf("CISD %s terkonfirmasi tembus by body!", ep.LTF)
				} else {
					changedEventDesc = fmt.Sprintf("Status %s ➡️ %s berubah menjadi %s", ep.HTF, ep.LTF, stLabel)
				}
				changedNextAction = setup.NextAction
			}

			ilaRows = append(ilaRows, ILARowDisplay{
				PairName: ep.PairName,
				Bias:     setup.Bias,
				Status:   stLabel,
				Changed:  isPairChanged,
			})

			finalStatus := setup.Status
			if prevILA.LastStatus == "ENTRY" && setup.Status == "READY_TO_ENTRY" {
				finalStatus = "ENTRY"
			}
			state.ILAPairs[ilaKey] = ILAPairState{
				LastBreakTime: prevILA.LastBreakTime,
				LastStatus:    finalStatus,
				LastHTFStatus: setup.Status,
			}
		}

		// Scenario B: Progress Update for ILA (Pilihan 1: Highlight + 4-Pair Table)
		if inActiveSession && changedPairTitle != "" && len(ilaRows) > 0 {
			progMsg := fmt.Sprintf(`🔔 **[%s] Update ILA: %s Maju Step!**
• **Kejadian:** %s
• **Langkah:** %s

📊 **Status ILA 4-Timeframe:**
%s`,
				symName, changedPairTitle,
				changedEventDesc, changedNextAction,
				makeILATable(ilaRows),
			)
			messages = append(messages, progMsg)
		}
	}

	saveState(state)

	if len(messages) > 0 {
		fmt.Println(strings.Join(messages, "\n\n---\n\n"))
	}
}
