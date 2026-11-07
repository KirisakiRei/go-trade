package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"ila-engine/internal/dto"
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
	DBPath              = "/opt/data/ila-engine/ila.db"
	StateFilePath       = "/opt/data/ila-engine/monitor_state.json"
	AccountSize         = 10000.0
	DefaultRiskPercent  = 0.50
	MinRR               = 2.0
	MaxRR               = 2.3
	EnableBBMAMonitor   = false // User instructed to disable BBMA monitoring
	EnableBBMAExecution = false // User instructed to disable BBMA auto-execution
)

var Pairs = []string{"XAUUSD", "EURUSD", "USTEC"}

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
	data, err := os.ReadFile(StateFilePath)
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
	data, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		_ = os.WriteFile(StateFilePath, data, 0644)
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

func phaseLabel(phase string) string {
	mapping := map[string]string{
		"MOMENTUM_CONFIRMED": "Momentum (CSM)",
		"EXTREME_CONFIRMED":  "Extreme",
		"REENTRY_CONFIRMED":  "Re-entry",
		"MHV_CONFIRMED":      "MHV (Market Hilang Volume)",
		"CS_KUKUH":           "CSAK (Arah Kukuh)",
		"CS_AWAL":            "CS Arah Awal",
		"SIDEWAYS":           "Sideways",
		"NEUTRAL":            "Netral",
	}
	if v, ok := mapping[phase]; ok {
		return v
	}
	return strings.ReplaceAll(phase, "_", " ")
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

func gradeLabel(g string) string {
	mapping := map[string]string{
		"GRADE_A_PLUS":           "Grade A+ (Prime Setup)",
		"GRADE_A":                "Grade A (Teknikal Valid)",
		"GRADE_A_PENDING_RR":     "Grade A (Menunggu R:R >= 1:2)",
		"GRADE_A_GOLDEN":         "Grade A (Golden Setup)",
		"GRADE_B_STRONG":         "Grade B (Tren Kuat)",
		"CONFLICT":               "Konflik Arah MTF",
		"INVALID_CSM_AT_TF2":     "Re-entry Cacat (Ada CSM di TF2)",
		"INVALID_CSM_AT_TRIGGER": "Trigger Batal (Ditembus CSM)",
		"ROLLOVER_LOCK":          "Jeda Rollover Subuh",
		"MARKET_CLOSED_WEEKEND":  "Pasar Libur Akhir Pekan",
		"NEUTRAL":                "Netral",
	}
	if v, ok := mapping[g]; ok {
		return v
	}
	return strings.ReplaceAll(g, "_", " ")
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

func isActiveSession(sess string) bool {
	actives := []string{"ASIAN_SESSION", "LONDON_SESSION", "LONDON_NEWYORK_OVERLAP", "NEWYORK_SESSION"}
	for _, a := range actives {
		if strings.Contains(sess, a) {
			return true
		}
	}
	return false
}

// isMarketClosed: weekend (Sabtu/Minggu penuh atau Jumat >= 21:00 UTC) atau rollover 22:00-01:00 UTC.
func isMarketClosed(utcDay time.Weekday, utcHour int) bool {
	if utcDay == time.Saturday || utcDay == time.Sunday {
		return true
	}
	// Jumat >= 21:00 UTC (04:00 WIB Sabtu): margin 1 jam sebelum penutupan akhir pekan,
	// menghindari spread melebar tanpa memotong sesi New York.
	if utcDay == time.Friday && utcHour >= 21 {
		return true
	}
	// Rollover subuh: 22:00 - 01:00 UTC (05:00 - 08:00 WIB).
	if utcHour >= 22 || utcHour < 1 {
		return true
	}
	return false
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

func makeBBMAStepTable(sym string, d *dto.MTFAnalysisResponse, changedTF string, rr float64) string {
	mark := func(tf string) string {
		if changedTF == tf {
			return " ◀"
		}
		return "  "
	}

	h4Bias := "⚪ Netr"
	if d.TF1Analysis.Bias == "BEARISH" {
		h4Bias = "🔴 Sell"
	} else if d.TF1Analysis.Bias == "BULLISH" {
		h4Bias = "🟢 Buy "
	}

	h1Bias := "⚪ Netr"
	if d.TF2Analysis.Bias == "BEARISH" {
		h1Bias = "🔴 Sell"
	} else if d.TF2Analysis.Bias == "BULLISH" {
		h1Bias = "🟢 Buy "
	}

	m15Bias := "⚪ Netr"
	if d.TF3Analysis.Bias == "BEARISH" {
		m15Bias = "🔴 Sell"
	} else if d.TF3Analysis.Bias == "BULLISH" {
		m15Bias = "🟢 Buy "
	}

	h4Phase := padRight(phaseLabel(d.TF1Analysis.MarketPhase), 17)
	h1Phase := padRight(phaseLabel(d.TF2Analysis.MarketPhase), 17)
	m15Phase := padRight(phaseLabel(d.TF3Analysis.MarketPhase), 17)

	gradeStr := padRight(gradeLabel(d.ConfluenceGrade), 32)
	rrStr := "Menunggu Setup M15"
	if rr >= 2.0 {
		rrStr = fmt.Sprintf("1:%.2f (Lolos Quality Gate)", rr)
	} else if rr > 0 {
		rrStr = fmt.Sprintf("1:%.2f (< 1:2 Belum Valid)", rr)
	}
	rrPad := padRight(rrStr, 32)

	return fmt.Sprintf("```text\n"+
		"┌─────┬─────────┬───────────────────┬────────────┐\n"+
		"│ TF  │  Arah   │    Fase Pasar     │   Status   │\n"+
		"├─────┼─────────┼───────────────────┼────────────┤\n"+
		"│ H4  │ %s │ %s │ Tren Utama%s│\n"+
		"│ H1  │ %s │ %s │ Konfirmasi%s│\n"+
		"│ M15 │ %s │ %s │ ⏳ Pantau %s│\n"+
		"├─────┴─────────┴───────────────────┴────────────┤\n"+
		"│ Kualifikasi : %s │\n"+
		"│ R:R Est.    : %s │\n"+
		"└────────────────────────────────────────────────┘\n```",
		h4Bias, h4Phase, mark("H4"),
		h1Bias, h1Phase, mark("H1"),
		m15Bias, m15Phase, mark("M15"),
		gradeStr, rrPad,
	)
}

func makeBBMASessionTable(analyses map[string]*dto.MTFAnalysisResponse, state MonitorState) string {
	rows := []string{}
	for _, sym := range Pairs {
		d, ok := analyses[sym]
		if !ok {
			continue
		}
		sName, _ := getSymbolDisplay(sym)
		biasStr := "⚪ Netr"
		if d.Bias == "BEARISH" {
			biasStr = "🔴 Sell"
		} else if d.Bias == "BULLISH" {
			biasStr = "🟢 Buy "
		}

		stStr := padRight(fmt.Sprintf("M15 %s", phaseLabel(d.TF3Analysis.MarketPhase)), 16)
		gStr := padRight(gradeLabel(d.ConfluenceGrade), 11)

		mark := "  "
		prev := state.Pairs[sym]
		if prev.LastGrade != d.ConfluenceGrade && prev.LastGrade != "" {
			mark = " ◀"
		}

		rows = append(rows, fmt.Sprintf("│ %s │ %s │ %s │ %s%s│",
			padRight(sName, 7), biasStr, stStr, gStr, mark))
	}

	return fmt.Sprintf("```text\n"+
		"┌─────────┬─────────┬──────────────────┬───────────┐\n"+
		"│  Pair   │  Arah   │   Status Setup   │ Kualitas  │\n"+
		"├─────────┼─────────┼──────────────────┼───────────┤\n"+
		"%s\n"+
		"└─────────┴─────────┴──────────────────┴───────────┘\n```",
		strings.Join(rows, "\n"),
	)
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

func main() {
	_ = godotenv.Load("/opt/data/ila-engine/.env")
	var messages []string

	// 1. Connect SQLite
	db, err := gorm.Open(sqlite.Open(DBPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open db: %v\n", err)
		os.Exit(1)
	}

	_ = db.AutoMigrate(&model.SetupRecord{}, &model.TradeJournal{}, &model.Position{}, &model.TradeEvaluation{})
	setupRepo := repository.NewSetupRepository(db)
	_ = setupRepo
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
	marketClosed := isMarketClosed(utcDay, utcHour)
	isRollover := utcHour >= 22 || utcHour < 1     // 05:00 - 08:00 WIB
	isOverlap := (utcHour >= 12 && utcHour < 17)   // 19:00 - 00:00 WIB
	isLondonOnly := (utcHour >= 7 && utcHour < 12) // 14:00 - 19:00 WIB
	isNYOnly := (utcHour >= 17 && utcHour < 22)    // 00:00 - 05:00 WIB

	currentSession := "ASIAN_SESSION"
	if marketClosed {
		currentSession = "WEEKEND_CLOSED"
	} else if isRollover {
		currentSession = "ROLLOVER_BLACKOUT"
	} else if isOverlap {
		currentSession = "LONDON_NEWYORK_OVERLAP"
	} else if isLondonOnly {
		currentSession = "LONDON_SESSION"
	} else if isNYOnly {
		currentSession = "NEWYORK_SESSION"
	}

	lastSession := state.LastSession
	inActiveSession := isActiveSession(currentSession)

	// Session Transition Briefing
	if inActiveSession && currentSession != lastSession {
		state.LastSession = currentSession
		sessTitle := cleanSessionName(currentSession)
		if EnableBBMAMonitor {
			// Will be populated if BBMA is enabled
		} else {
			messages = append(messages, fmt.Sprintf("🔔 **[%s] Sesi Pasar Aktif (Mode ILA)**\nRadar ILA aktif memantau setup di 4 pasangan timeframe (XAUUSD, EURUSD, USTEC).", strings.ToUpper(sessTitle)))
		}
	} else if !inActiveSession {
		state.LastSession = currentSession
	}

	// 7. ILA (Imaginary Liquidity) Multi-Timeframe Scan & Auto-Execution
	// Market closed (weekend / rollover) -> skip scan & execution entirely.
	if marketClosed {
		saveState(state)
		return
	}

	ilaService := service.NewILAService(feedProvider)
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
			setup, err := ilaService.AnalyzeILA(ctx, dto.ILAAnalysisRequest{
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
