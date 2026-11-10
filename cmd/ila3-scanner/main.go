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

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ila-engine/internal/datadir"
	"ila-engine/internal/market"
	"ila-engine/internal/model"
	"ila-engine/internal/repository"
	"ila-engine/pkg/broker"
	"ila-engine/pkg/indicators"
	"ila-engine/pkg/marketfeed"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/sequence"
)

const (
	MinRR = 2.0
)

var Pairs = []string{"XAUUSD", "EURUSD", "USTEC"}

func DBPath() string        { return datadir.Berkas("ila3.db") }
func StateFilePath() string { return datadir.Berkas("monitor_state_ila3.json") }

var CorrelationMap = map[string]string{
	"XAUUSD": "XAGUSD",
	"EURUSD": "GBPUSD",
	"USTEC":  "US500",
}

type ILA3PairState struct {
	LastState       string    `json:"last_state"`
	LastSetup       string    `json:"last_setup"`
	LastBias        string    `json:"last_bias"`
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`
}

type MonitorState struct {
	LastSession string                   `json:"last_session"`
	Pairs       map[string]ILA3PairState `json:"pairs"`
}

func loadState() MonitorState {
	s := MonitorState{
		Pairs: make(map[string]ILA3PairState),
	}
	data, err := os.ReadFile(StateFilePath())
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	if s.Pairs == nil {
		s.Pairs = make(map[string]ILA3PairState)
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
		return time.Date(t.Year(), t.Month(), t.Day(), (t.Hour()/4)*4, 0, 0, 0, time.UTC)
	case "D", "D1", "1D":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case "W", "W1", "1W":
		days := (int(t.Weekday()) + 6) % 7
		return time.Date(t.Year(), t.Month(), t.Day()-days, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
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

func calcLot(symbol string, entry, sl, riskDollars float64) float64 {
	dist := math.Abs(entry - sl)
	if dist <= 0.000001 || riskDollars <= 0 {
		return 0.01
	}

	upper := strings.ToUpper(symbol)
	var lot float64
	if strings.Contains(upper, "XAU") || strings.Contains(upper, "GOLD") {
		lot = riskDollars / (dist * 100.0)
	} else if strings.Contains(upper, "NAS") || strings.Contains(upper, "TEC") || strings.Contains(upper, "US100") {
		lot = riskDollars / dist
		lot = math.Floor(lot*10) / 10.0
		if lot < 0.10 {
			lot = 0.10
		}
	} else {
		pips := dist / 0.00010
		if pips <= 0.01 {
			return 0.01
		}
		lot = riskDollars / (pips * 10.0)
	}

	lot = math.Round(lot*100) / 100.0
	if lot < 0.01 {
		lot = 0.01
	}
	if lot > 20.0 {
		lot = 20.0
	}
	return lot
}

func getILA3ActionPlan(state sequence.SequenceState, htf, mtf, ltf, bias string) string {
	switch state {
	case sequence.StateWaitingHTFKeyLevel:
		return fmt.Sprintf("Macro Framework: Menunggu likuiditas makro %s menguji External Range Liquidity (ERL Swing High/Low) atau Fair Value Gap (IRL) untuk menentukan bias arah dan dealing range utama.", htf)
	case sequence.StateWaitingMTFReversal:
		return fmt.Sprintf("Dealing Range Reversal: Level makro %s teruji. Menunggu reaksi penolakan harga (reversal wick) pada MTF %s ERL dan konfirmasi konfluensi Sequential SMT (SSMT Divergence) terhadap correlation pair.", htf, mtf)
	case sequence.StateWaitingMTFFVGRetr:
		return fmt.Sprintf("Second Chance Retracement: Expansion 1 pasca-reversal telah melahirkan FVG baru pada %s. Menunggu harga koreksi (retrace) ke dalam MTF FVG sebelum mencari trigger entri kedua.", mtf)
	case sequence.StateWaitingLTFCISD1:
		return fmt.Sprintf("Trigger Gate (Sniper Mode): MTF %s ERL telah tersapu/teruji. Scanner beralih ke lilin %s (tiap %s). Menunggu Change in State of Delivery (CISD) terkonfirmasi via closed body menembus origin opposing run sebelum titik ekstrim.", mtf, ltf, ltf)
	case sequence.StateWaitingLTFCISD2:
		return fmt.Sprintf("Trigger Gate (Entry 2): Harga berada di dalam MTF FVG %s. Menunggu konfirmasi CISD kedua pada timeframe %s untuk eksekusi continuation.", mtf, ltf)
	case sequence.StateReadyEntry1:
		return fmt.Sprintf("Market Execution (Entry 1): Reversal terkonfirmasi penuh (HTF Reversal + MTF Key Level + LTF CISD Valid). Buka posisi %s di open candle berikutnya dengan target Take Profit Fibo StDev Level -2.0 dan SL di batas ekstrim.", bias)
	case sequence.StateReadyEntry2:
		return fmt.Sprintf("Market Execution (Entry 2): Retracement terkonfirmasi penuh (MTF FVG Mitigation + LTF CISD 2 Valid). Buka posisi %s di open candle berikutnya dengan target ekspansi lanjutan.", bias)
	case sequence.StateExpired:
		return "Session Expiry: Kuartal waktu telah memasuki penutupan sesi Q4 sore (>= 15:00 NY Time). Momentum ekspansi telah habis; sistem menahan eksekusi baru demi menghindari risiko pelebaran spread dan penutupan pasar."
	default:
		return fmt.Sprintf("Memantau pergerakan harga dan struktur likuiditas pada hierarki 3-tier %s ➡️ %s ➡️ %s.", htf, mtf, ltf)
	}
}

func makeILA3RadarBox(stateStr, quarterStr, dolStr, invStr string) string {
	return fmt.Sprintf("```text\n"+
		"┌──────────────┬──────────────────────────────┐\n"+
		"│ Status Siklus│ %s │\n"+
		"│ Kuartal (QT) │ %s │\n"+
		"│ Target (DOL) │ %s │\n"+
		"│ Invalidation │ %s │\n"+
		"└──────────────┴──────────────────────────────┘\n```",
		padRight(stateStr, 28),
		padRight(quarterStr, 28),
		padRight(dolStr, 28),
		padRight(invStr, 28))
}

func makeILA3OrderBox(brokerName, setupMode, ticket, entry, sl, tp1, tpExt, rr, riskLot string) string {
	return fmt.Sprintf("```text\n"+
		"┌──────────────┬──────────────────────────────┐\n"+
		"│ Broker/Akun  │ %s │\n"+
		"│ Setup & Mode │ %s │\n"+
		"│ Tiket Order  │ %s │\n"+
		"├──────────────┼──────────────────────────────┤\n"+
		"│ Entry Price  │ %s │\n"+
		"│ Stop Loss    │ %s │\n"+
		"│ Take Profit 1│ %s │\n"+
		"│ Extended DOL │ %s │\n"+
		"│ Risk / Reward│ %s │\n"+
		"│ Risiko & Lot │ %s │\n"+
		"└──────────────┴──────────────────────────────┘\n```",
		padRight(brokerName, 28),
		padRight(setupMode, 28),
		padRight(ticket, 28),
		padRight(entry, 28),
		padRight(sl, 28),
		padRight(tp1, 28),
		padRight(tpExt, 28),
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

	// 1. Connect SQLite database
	db, err := gorm.Open(sqlite.Open(DBPath()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open ila3 db: %v\n", err)
		os.Exit(1)
	}
	_ = db.AutoMigrate(&model.Position{})

	positionRepo := repository.NewPositionRepository(db)

	// 2. Initialize cTrader Broker client exclusively for ILA 3.0
	cTraderBroker, _ := broker.NewCTraderBroker(broker.CTraderConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// 3. Check Closed Positions on cTrader
	var openPositions []model.Position
	db.Where("status = ?", "OPEN").Find(&openPositions)

	if len(openPositions) > 0 && cTraderBroker != nil {
		activePositions, err := cTraderBroker.GetOpenPositions(ctx)
		if err == nil {
			activeMap := make(map[string]bool)
			for _, ap := range activePositions {
				activeMap[ap.TicketNumber] = true
			}

			for _, pos := range openPositions {
				if !activeMap[pos.TicketNumber] {
					cPosID, _ := strconv.ParseInt(pos.TicketNumber, 10, 64)
					exitPrice := pos.OpenPrice
					actualPnL := 0.0
					if cPosID > 0 {
						deals, _ := cTraderBroker.GetDealsByPosition(ctx, cPosID)
						for _, d := range deals {
							if d.GrossProfit != 0 || (pos.Direction == "BUY" && d.TradeSide == "SELL") || (pos.Direction == "SELL" && d.TradeSide == "BUY") {
								exitPrice = d.ExecutionPrice
								actualPnL = d.GrossProfit
								break
							}
						}
					}

					hitTP := false
					if actualPnL > 0 {
						hitTP = true
					} else if actualPnL < 0 {
						hitTP = false
					} else {
						isForex := strings.Contains(pos.Symbol, "EUR") || strings.Contains(pos.Symbol, "GBP") || strings.Contains(pos.Symbol, "AUD")
						tol := 0.50
						if isForex {
							tol = 0.00050
						}
						if pos.Direction == "BUY" {
							hitTP = exitPrice >= (pos.TakeProfit - tol)
						} else {
							hitTP = exitPrice <= (pos.TakeProfit + tol)
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

					rSign := "+"
					if outcomeR < 0 {
						rSign = ""
					}
					pnlSign := "+"
					if actualPnL < 0 {
						pnlSign = ""
					}
					sName, sEmoji := getSymbolDisplay(pos.Symbol)
					statusTitle := "🏁 **[TRADE CLOSED ILA 3.0 - CTRADER] HIT TAKE PROFIT 🎯**"
					if status == "CLOSED_SL" {
						statusTitle = "🛑 **[TRADE CLOSED ILA 3.0 - CTRADER] HIT STOP LOSS**"
					}

					closeMsg := fmt.Sprintf(`%s
• **Tiket:** `+"`#%s`"+` (%s %s %s)
• **Entry:** `+"`%s`"+` ➡️ **Exit:** `+"`%s`"+`
• **Hasil:** `+"`%s%.2f R`"+` (`+"`%s$%.2f`"+`)
• **Broker:** IC Markets cTrader (#10131354)`,
						statusTitle, pos.TicketNumber, sEmoji, sName, pos.Direction,
						formatPrice(pos.Symbol, pos.OpenPrice), formatPrice(pos.Symbol, exitPrice),
						rSign, outcomeR, pnlSign, actualPnL,
					)
					messages = append(messages, closeMsg)
				}
			}
		}
	}

	// 4. Market hours gate (skip weekend and rollover)
	nowUtc := time.Now().UTC()
	utcHour := nowUtc.Hour()
	utcDay := nowUtc.Weekday()
	if market.IsClosed(utcDay, utcHour) {
		if len(messages) > 0 {
			fmt.Println(strings.Join(messages, "\n\n---\n\n"))
		}
		return
	}

	// 5. Evaluate ILA 3.0 on Pairs
	feed := marketfeed.NewMultiFeedProvider()
	eval := sequence.NewEvaluator()
	qtCalc := quarterly.NewCalculator()
	state := loadState()

	presets := []struct {
		ModeName string
		HTF      string
		MTF      string
		LTF      string
	}{
		{ModeName: "New Day", HTF: "4H", MTF: "1H", LTF: "5m"},
		{ModeName: "New Week", HTF: "D", MTF: "4H", LTF: "15m"},
	}

	for _, sym := range Pairs {
		corrSym := CorrelationMap[sym]
		sName, sEmoji := getSymbolDisplay(sym)

		for _, ps := range presets {
			key := fmt.Sprintf("%s-%s", sym, ps.ModeName)
			prevState := state.Pairs[key]

			// Scheduler Berbasis Penutupan Lilin Adaptif ILA 3.0:
			// Fase 3 (Menunggu CISD di LTF): Jadwal mengikuti lilin LTF (5m / 15m)
			// Fase 2 (Menunggu Reversal di MTF): Jadwal mengikuti lilin MTF (1H / 4H)
			// Fase 1 (Menunggu Key Level HTF): Jadwal mengikuti lilin HTF (4H / D)
			var checkTF string
			switch prevState.LastState {
			case "WAIT_LTF_CISD_1", "WAIT_LTF_CISD_2", "READY_ENTRY_1", "READY_ENTRY_2":
				checkTF = ps.LTF
			case "WAIT_MTF_REVERSAL", "WAIT_MTF_FVG_RETRACE":
				checkTF = ps.MTF
			default:
				checkTF = ps.HTF
			}

			if !slotDue(prevState.LastEvaluatedAt, checkTF, nowUtc, 3*time.Second) {
				continue
			}

			hCandles, _ := feed.FetchCandles(ctx, sym, ps.HTF, 40)
			mCandles, _ := feed.FetchCandles(ctx, sym, ps.MTF, 40)
			lCandles, _ := feed.FetchCandles(ctx, sym, ps.LTF, 80) // 80 candles covers full session for anchored CISD
			var corrCandles []indicators.Candle
			if corrSym != "" {
				corrCandles, _ = feed.FetchCandles(ctx, corrSym, ps.MTF, 40)
			}

			setup := eval.EvaluateSequence(hCandles, mCandles, lCandles, corrCandles, ps.HTF, ps.MTF, ps.LTF, sym, corrSym)
			if setup == nil {
				continue
			}

			prevState.LastEvaluatedAt = nowUtc

			dir := "BUY"
			dirEmoji := "🟢 BUY"
			if setup.Bias == "BEARISH" {
				dir = "SELL"
				dirEmoji = "🔴 SELL"
			}

			// Scenario A: Auto-Execution on cTrader
			if setup.SetupType != sequence.SetupNone && setup.EntryPrice > 0 && setup.StopLoss > 0 && setup.TakeProfit > 0 && setup.RiskReward >= MinRR {
				var countOpen int64
				db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN'", sym).Count(&countOpen)

				if countOpen == 0 && cTraderBroker != nil && prevState.LastState != "ENTRY" {
					dailyStatus, _ := positionRepo.GetDailyTradeStatusWIB(ctx, time.Now())
					if dailyStatus != nil && !dailyStatus.CanTrade {
						lockKey := sym + "-LOCK"
						if prevState.LastState != lockKey {
							table := makeILA3OrderBox(
								"IC Markets cTrader (#10131354)",
								fmt.Sprintf("%s (%s)", setup.SetupType, ps.ModeName),
								"ENTRY MANUAL (LOCK)",
								formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss),
								formatPrice(sym, setup.TakeProfit),
								"HTF Macro ERL",
								fmt.Sprintf("1:%.2f", setup.RiskReward),
								"1.00% Live Equity",
							)
							actionPlan := getILA3ActionPlan(setup.State, ps.HTF, ps.MTF, ps.LTF, setup.Bias)
							msg := fmt.Sprintf("🎯 **[SETUP ILA 3.0 VALID - ENTRY MANUAL] %s %s • %s**\n`Mode: %s (%s ➡️ %s ➡️ %s)`\n\n%s\n\n🛑 **AUTO-EKSEKUSI DITAHAN:**\n• %s\n\n👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
								sEmoji, sName, dirEmoji, ps.ModeName, ps.HTF, ps.MTF, ps.LTF, table, dailyStatus.LockReason, actionPlan)
							messages = append(messages, msg)
							prevState.LastState = lockKey
							state.Pairs[key] = prevState
						}
						continue
					}

					if !executeEnabled {
						table := makeILA3OrderBox(
							"IC Markets cTrader (#10131354)",
							fmt.Sprintf("%s (%s)", setup.SetupType, ps.ModeName),
							"MODE PANTAU",
							formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss),
							formatPrice(sym, setup.TakeProfit),
							"HTF Macro ERL",
							fmt.Sprintf("1:%.2f", setup.RiskReward),
							"1.00% Live Equity",
						)
						actionPlan := getILA3ActionPlan(setup.State, ps.HTF, ps.MTF, ps.LTF, setup.Bias)
						msg := fmt.Sprintf("🎯 **[SETUP ILA 3.0 VALID - MODE PANTAU] %s %s • %s**\n`Mode: %s (%s ➡️ %s ➡️ %s)`\n\n%s\n\n👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
							sEmoji, sName, dirEmoji, ps.ModeName, ps.HTF, ps.MTF, ps.LTF, table, actionPlan)
						messages = append(messages, msg)
						prevState.LastState = "ENTRY"
						state.Pairs[key] = prevState
						continue
					}

					acc, accErr := cTraderBroker.GetAccountInfo(ctx)
					var equity float64
					if acc != nil {
						equity = acc.Equity
						if equity <= 0 {
							equity = acc.Balance
						}
					}
					if accErr != nil || equity <= 0 {
						messages = append(messages, fmt.Sprintf("⚠️ **[ILA 3.0] Eksekusi Dibatalkan — %s %s**\nGagal membaca ekuitas akun cTrader. Entry ditolak demi keamanan margin.\n*Error:* `%v`", sEmoji, sName, accErr))
						continue
					}
					riskDollars := equity * 0.01
					lot := calcLot(sym, setup.EntryPrice, setup.StopLoss, riskDollars)

					setupID := fmt.Sprintf("ILA3-%s-%s-%s", sym, ps.ModeName, time.Now().Format("200601021504"))
					orderRes, err := cTraderBroker.ExecuteOrder(ctx, broker.OrderRequest{
						Symbol:     sym,
						Direction:  dir,
						OpenPrice:  setup.EntryPrice,
						StopLoss:   setup.StopLoss,
						TakeProfit: setup.TakeProfit,
						LotSize:    lot,
						Comment:    fmt.Sprintf("ILA 3.0 %s %s", ps.ModeName, setupID),
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
							Timeframe:    ps.LTF,
							Direction:    dir,
							OpenPrice:    setup.EntryPrice,
							StopLoss:     setup.StopLoss,
							TakeProfit:   setup.TakeProfit,
							LotSize:      lot,
							RiskReward:   setup.RiskReward,
							Status:       "OPEN",
							OpenedAt:     nowDt,
							Notes:        fmt.Sprintf("Broker: IC Markets cTrader (#10131354) | ILA 3.0 (%s: %s ➡️ %s ➡️ %s) | %s", ps.ModeName, ps.HTF, ps.MTF, ps.LTF, setup.Summary),
						})

						extDOLStr := "Sesuai Fibo StDev"
						if setup.NestedExecution != nil && setup.NestedExecution.HTFTargetPrice > 0 {
							extDOLStr = formatPrice(sym, setup.NestedExecution.HTFTargetPrice)
						}

						table := makeILA3OrderBox(
							"IC Markets cTrader (#10131354)",
							fmt.Sprintf("%s (%s)", setup.SetupType, ps.ModeName),
							"#"+orderRes.TicketNumber,
							formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss),
							formatPrice(sym, setup.TakeProfit),
							extDOLStr,
							fmt.Sprintf("1:%.2f", setup.RiskReward),
							fmt.Sprintf("1.00%% ($%.2f) | %.2f Lot", riskDollars, lot),
						)

						stdevText := ""
						if setup.StDevProjections != nil {
							stdevText = fmt.Sprintf("\n📐 **ICT STANDARD DEVIATION CHECKPOINTS:**\n• **Level -1.0 (Ekspansi 1):** `%s`\n• **Level -2.0 s.d. -2.5:** `%s – %s` ⚠️ [PARTIAL TP 50%% & SL TO BE]\n• **Level -3.5 s.d. -4.0:** `%s – %s` 🎯 [TARGET EXTENDED FINAL]",
								formatPrice(sym, setup.StDevProjections.LevelMinus1),
								formatPrice(sym, setup.StDevProjections.LevelMinus2), formatPrice(sym, setup.StDevProjections.LevelMinus2_5),
								formatPrice(sym, setup.StDevProjections.LevelMinus3_5), formatPrice(sym, setup.StDevProjections.LevelMinus4),
							)
						}

						actionPlan := getILA3ActionPlan(setup.State, ps.HTF, ps.MTF, ps.LTF, setup.Bias)
						execAlert := fmt.Sprintf("🚀 **[ORDER REAL ILA 3.0] %s %s • %s**\n`Hierarki: %s (HTF) ➡️ %s (MTF) ➡️ %s (LTF)`\n\n%s%s\n\n👉 **TRADING ACTION PLAN & MANAGEMENT:**\n• %s",
							sEmoji, sName, dirEmoji, ps.HTF, ps.MTF, ps.LTF, table, stdevText, actionPlan)
						messages = append(messages, execAlert)

						prevState.LastState = "ENTRY"
						prevState.LastSetup = string(setup.SetupType)
						prevState.LastBias = setup.Bias
						state.Pairs[key] = prevState
					}
				}
				continue
			}

			// Scenario B: Radar Progress Update per Preset (Dedicated Bubble)
			currStateStr := string(setup.State)
			if currStateStr != prevState.LastState && currStateStr != "WAIT_HTF_KEY_LEVEL" {
				if prevState.LastState != "" {
					qInfo := qtCalc.GetDailyQuarter(time.Now().UTC())
					qStr := fmt.Sprintf("%s (%s)", qInfo.Quarter, formatNYTime(nowUtc))

					stateTitle := "⏳ Menunggu"
					switch setup.State {
					case sequence.StateWaitingMTFReversal:
						stateTitle = fmt.Sprintf("⏳ Reversal %s", ps.MTF)
					case sequence.StateWaitingMTFFVGRetr:
						stateTitle = fmt.Sprintf("⏳ Retrace FVG %s", ps.MTF)
					case sequence.StateWaitingLTFCISD1:
						stateTitle = fmt.Sprintf("⏳ Berburu CISD 1 (%s)", ps.LTF)
					case sequence.StateWaitingLTFCISD2:
						stateTitle = fmt.Sprintf("⏳ Berburu CISD 2 (%s)", ps.LTF)
					case sequence.StateReadyEntry1, sequence.StateReadyEntry2:
						stateTitle = "🎯 Siap Eksekusi"
					case sequence.StateExpired:
						stateTitle = "🛑 Sesi Q4 Expired"
					}

					dolStr := "Mencari ERL Makro"
					if setup.TakeProfit > 0 {
						dolStr = formatPrice(sym, setup.TakeProfit)
					}
					invStr := "Batas Ekstrim MTF"
					if setup.StopLoss > 0 {
						invStr = formatPrice(sym, setup.StopLoss)
					}

					table := makeILA3RadarBox(stateTitle, qStr, dolStr, invStr)
					actionPlan := getILA3ActionPlan(setup.State, ps.HTF, ps.MTF, ps.LTF, setup.Bias)

					ssmtText := "• **SSMT Correlation:** Netral (Kedua instrumen seirama)"
					if setup.SSMTResult.Detected {
						ssmtText = fmt.Sprintf("• **SSMT Correlation:** 💎 %s (%s vs %s)", setup.SSMTResult.Type, sym, corrSym)
					}

					htfDesc := setup.Summary
					mtfDesc := fmt.Sprintf("Dealing Range %s aktif. %s", ps.MTF, ssmtText)
					ltfDesc := fmt.Sprintf("Sniper Mode lilin %s aktif. Menunggu konfirmasi penembusan body CISD.", ps.LTF)

					radarMsg := fmt.Sprintf("🔬 **[ILA 3.0 MACRO RADAR] %s %s • %s**\n`Mode: %s (%s ➡️ %s ➡️ %s)`\n\n%s\n\n🕒 **SIKLUS WAKTU & MAKRO (Quarterly Theory):**\n• **Kuartal Waktu:** %s Daily (%s)\n• **Fase IPDA:** Distribusi & Ekspansi Terarah\n\n🏛️ **STRUKTUR HTF (%s):**\n• %s\n\n🧱 **DEALING RANGE MTF (%s):**\n%s\n\n⚡ **TRIGGER LTF (%s):**\n• %s\n\n👉 **ACTION PLAN & TRADING STEPS:**\n• %s",
						sEmoji, sName, dirEmoji, ps.ModeName, ps.HTF, ps.MTF, ps.LTF, table,
						qInfo.Quarter, qInfo.Description,
						ps.HTF, htfDesc,
						ps.MTF, mtfDesc,
						ps.LTF, ltfDesc,
						actionPlan)
					messages = append(messages, radarMsg)
				}

				prevState.LastState = currStateStr
				prevState.LastSetup = string(setup.SetupType)
				prevState.LastBias = setup.Bias
			}
			state.Pairs[key] = prevState
		}
	}

	saveState(state)

	if len(messages) > 0 {
		fmt.Println(strings.Join(messages, "\n\n---\n\n"))
	}
}
