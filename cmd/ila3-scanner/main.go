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

// DBPath dan StateFilePath mengikuti ILA_DATA_DIR agar aman di Docker.
func DBPath() string        { return datadir.Berkas("ila3.db") }
func StateFilePath() string { return datadir.Berkas("monitor_state_ila3.json") }

var CorrelationMap = map[string]string{
	"XAUUSD": "XAGUSD",
	"EURUSD": "GBPUSD",
	"USTEC":  "US500",
}

type ILA3PairState struct {
	LastState string `json:"last_state"`
	LastSetup string `json:"last_setup"`
	LastBias  string `json:"last_bias"`
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

func calcLot(ctx context.Context, b broker.Broker, symbol string, entry, sl, riskDollars float64) (float64, *broker.SymbolSpec) {
	var spec *broker.SymbolSpec
	if b != nil {
		spec, _ = b.GetSymbolSpec(ctx, symbol)
	}
	if spec == nil || spec.PointValue <= 0 {
		spec = broker.FallbackSymbolSpec(symbol)
	}

	dist := math.Abs(entry - sl)
	if dist <= 0.000001 || riskDollars <= 0 {
		return spec.VolumeMin, spec
	}

	dollarLossPerLot := dist * spec.PointValue
	rawLot := (riskDollars / dollarLossPerLot) + 1e-8

	step := spec.VolumeStep
	if step <= 0 {
		step = 0.01
	}
	lot := math.Floor(rawLot/step) * step
	if spec.VolumeMin > 0 && lot < spec.VolumeMin {
		lot = spec.VolumeMin
	}
	if spec.VolumeMax > 0 && lot > spec.VolumeMax {
		lot = spec.VolumeMax
	}

	// Max Risk Assertion Gate: if rounded lot overshoots risk budget by > 10%, clamp down by step
	if lot > spec.VolumeMin && (lot*dollarLossPerLot) > (riskDollars*1.10) {
		lot = math.Floor((riskDollars/dollarLossPerLot)/step) * step
	}

	return math.Round(lot*100) / 100.0, spec
}

// executeEnabled menandai apakah scanner boleh mengirim order ke broker.
//
// Default true: perilaku lama tidak berubah. Set -execute=false saat Runner
// sudah mengambil alih eksekusi (plans/04 §11).
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

	// 1. Connect SQLite database
	db, err := gorm.Open(sqlite.Open(DBPath()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open ila3 db: %v\n", err)
		os.Exit(1)
	}
	_ = db.AutoMigrate(&model.Position{})

	// Aturan harian (1 win = stop, max 2 trade/hari, 2-strike) berbasis WIB —
	// sama seperti ILA 2.0 agar hasil uji kedua engine bisa dibandingkan.
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
					// Posisi sudah closed di cTrader!
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

			hCandles, _ := feed.FetchCandles(ctx, sym, ps.HTF, 40)
			mCandles, _ := feed.FetchCandles(ctx, sym, ps.MTF, 40)
			lCandles, _ := feed.FetchCandles(ctx, sym, ps.LTF, 40)
			var corrCandles []indicators.Candle
			if corrSym != "" {
				corrCandles, _ = feed.FetchCandles(ctx, corrSym, ps.MTF, 40)
			}

			setup := eval.EvaluateSequence(hCandles, mCandles, lCandles, corrCandles, ps.HTF, ps.MTF, ps.LTF, sym, corrSym)
			if setup == nil {
				continue
			}

			// Scenario A: Auto-Execution on cTrader
			if setup.SetupType != sequence.SetupNone && setup.EntryPrice > 0 && setup.StopLoss > 0 && setup.TakeProfit > 0 && setup.RiskReward >= MinRR {
				// Cek posisi aktif di cTrader untuk simbol ini
				var countOpen int64
				db.Model(&model.Position{}).Where("symbol = ? AND status = 'OPEN'", sym).Count(&countOpen)

				if countOpen == 0 && cTraderBroker != nil && prevState.LastState != "ENTRY" {
					// Aturan harian WIB: 1 win = stop, kuota 2 trade, 2-strike & posisi open.
					dailyStatus, _ := positionRepo.GetDailyTradeStatusWIB(ctx, time.Now())
					if dailyStatus != nil && !dailyStatus.CanTrade {
						lockKey := sym + "-LOCK"
						if prevState.LastState != lockKey {
							messages = append(messages, fmt.Sprintf("⏸️ **[ILA 3.0 DITAHAN] %s %s**\nSetup %s terdeteksi namun tidak dieksekusi.\n*Alasan:* %s", sEmoji, sName, setup.SetupType, dailyStatus.LockReason))
							prevState.LastState = lockKey
							state.Pairs[key] = prevState
						}
						continue
					}

					dir := "BUY"
					dirEmoji := "🟢 BUY"
					if setup.Bias == "BEARISH" {
						dir = "SELL"
						dirEmoji = "🔴 SELL"
					}

					if !executeEnabled {
						messages = append(messages, fmt.Sprintf(
							"🎯 **[SETUP ILA 3.0 VALID - ENTRY MANUAL] %s %s • %s**\n• Setup: %s (%s)\n• Entry: %s | SL: %s | TP: %s (R:R 1:%.2f)\n• **Auto-eksekusi dimatikan (mode pantau).**",
							sEmoji, sName, dirEmoji, setup.SetupType, ps.ModeName,
							formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss),
							formatPrice(sym, setup.TakeProfit), setup.RiskReward))
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
					// Tanpa ekuitas terverifikasi, ukuran lot tidak bisa dihitung aman: tolak entry.
					if accErr != nil || equity <= 0 {
						messages = append(messages, fmt.Sprintf("⚠️ **[ILA 3.0] Eksekusi Dibatalkan — %s %s**\nGagal membaca ekuitas akun cTrader. Entry ditolak agar ukuran lot tidak salah.\n*Error:* `%v`", sEmoji, sName, accErr))
						continue
					}
					riskDollars := equity * 0.005 // strict 0.5% live equity risk
					lot, _ := calcLot(ctx, cTraderBroker, sym, setup.EntryPrice, setup.StopLoss, riskDollars)

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

						qInfo := qtCalc.GetDailyQuarter(time.Now().UTC())
						ssmtDesc := "Netral (Kedua instrumen seirama)"
						if setup.SSMTResult.Detected {
							ssmtDesc = fmt.Sprintf("💎 %s (%s)", setup.SSMTResult.Type, setup.SSMTResult.ProbabilityGrade)
						}

						stdevText := ""
						if setup.StDevProjections != nil {
							stdevText = fmt.Sprintf("\n📐 **ICT STANDARD DEVIATION CHECKPOINTS:**\n• Level -1.0 (Extension 1): `%s`\n• Level -2.0 s.d. -2.5: `%s - %s` ⚠️ [PARTIAL TP 50%%]\n• Level -3.5 s.d. -4.0: `%s - %s` 🎯 [TARGET FINAL]",
								formatPrice(sym, setup.StDevProjections.LevelMinus1),
								formatPrice(sym, setup.StDevProjections.LevelMinus2), formatPrice(sym, setup.StDevProjections.LevelMinus2_5),
								formatPrice(sym, setup.StDevProjections.LevelMinus3_5), formatPrice(sym, setup.StDevProjections.LevelMinus4),
							)
						}

						execAlert := fmt.Sprintf(`🚀 **[ORDER REAL ILA 3.0 - CTRADER] %s %s • %s**

🏦 **Broker:** IC Markets cTrader (#10131354)
💎 **Tipe Setup:** %s (%s)
📐 **Formula 3-TF:** %s (HTF) ➡️ %s (MTF) ➡️ %s (LTF)

🕒 **SIKLUS WAKTU & MAKRO (Quarterly Theory):**
• **Kuartal:** %s Daily (%s)
• **Key Level HTF (%s):** %s
• **Validasi Cross-TF:** 🟢 VALID (Terkonfirmasi di MTF %s)
• **Waypoint Filter:** 🟢 CLEAR

📊 **KONFLUENSI KORELASI (SSMT):**
• **Pasangan:** %s vs %s
• **Status:** %s

🎯 **PARAMETER EKSEKUSI (NESTED EXECUTION):**
• **Tiket cTrader:** `+"`#%s`"+`
• **Entry:** `+"`%s`"+` | **SL:** `+"`%s`"+` | **TP:** `+"`%s`"+`
• **R:R:** `+"`1:%.2f`"+` | **Risiko:** 0.50%% ($%.2f) | **Lot:** %.2f Lot%s

⚡ *Posisi aktif otomatis di cTrader dengan SL & Trailing Checkpoint server-side!*`,
							sEmoji, sName, dirEmoji,
							setup.SetupType, ps.ModeName,
							ps.HTF, ps.MTF, ps.LTF,
							qInfo.Quarter, qInfo.Description,
							ps.HTF, setup.Summary,
							ps.MTF,
							sym, corrSym,
							ssmtDesc,
							orderRes.TicketNumber,
							formatPrice(sym, setup.EntryPrice), formatPrice(sym, setup.StopLoss), formatPrice(sym, setup.TakeProfit),
							setup.RiskReward, riskDollars, lot,
							stdevText,
						)
						messages = append(messages, execAlert)
						prevState.LastState = "ENTRY"
						prevState.LastSetup = string(setup.SetupType)
						prevState.LastBias = setup.Bias
						state.Pairs[key] = prevState
					}
				}
				continue
			}

			// Scenario B: Radar Progress Update (State Transition)
			currStateStr := string(setup.State)
			if currStateStr != prevState.LastState && currStateStr != "WAIT_HTF_KEY_LEVEL" {
				qInfo := qtCalc.GetDailyQuarter(time.Now().UTC())
				dirEmoji := "🟢 BUY"
				if setup.Bias == "BEARISH" {
					dirEmoji = "🔴 SELL"
				}

				ssmtText := ""
				if setup.SSMTResult.Detected {
					ssmtText = fmt.Sprintf("\n• **SSMT:** 💎 %s (%s vs %s)", setup.SSMTResult.Type, sym, corrSym)
				}

				progAlert := fmt.Sprintf(`🔔 **[ILA 3.0 RADAR] %s %s • %s**
• **Mode:** %s (%s ➡️ %s ➡️ %s)
• **Status Siklus:** ⏳ `+"`%s`"+`
• **Kuartal:** %s Daily (%s)
• **Struktur:** %s%s
• **Langkah Selanjutnya:** %s`,
					sEmoji, sName, dirEmoji,
					ps.ModeName, ps.HTF, ps.MTF, ps.LTF,
					currStateStr,
					qInfo.Quarter, qInfo.Description,
					setup.Summary, ssmtText,
					setup.NextAction,
				)
				messages = append(messages, progAlert)
				prevState.LastState = currStateStr
				prevState.LastSetup = string(setup.SetupType)
				prevState.LastBias = setup.Bias
				state.Pairs[key] = prevState
			}
		}
	}

	saveState(state)

	if len(messages) > 0 {
		fmt.Println(strings.Join(messages, "\n\n---\n\n"))
	}
}
