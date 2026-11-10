package marketfeed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ila-engine/pkg/indicators"

	"github.com/gorilla/websocket"
)

var (
	tvMsgRegex = regexp.MustCompile(`~m~\d+~m~`)
)

type TradingViewProvider struct {
	wsURL   string
	headers http.Header
}

func NewTradingViewProvider() *TradingViewProvider {
	headers := http.Header{}
	headers.Set("Origin", "https://www.tradingview.com")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	return &TradingViewProvider{
		wsURL:   "wss://data.tradingview.com/socket.io/websocket?from=chart%2F",
		headers: headers,
	}
}

func (p *TradingViewProvider) Name() string {
	return "tradingview_forexcom"
}

func (p *TradingViewProvider) MapSymbol(symbol string) string {
	sym := strings.ToUpper(strings.TrimSpace(symbol))

	// If symbol already has exchange prefix like "FOREXCOM:XAUUSD" or "OANDA:XAUUSD", keep it
	if strings.Contains(sym, ":") {
		return sym
	}

	symClean := strings.ReplaceAll(sym, "/", "")

	switch symClean {
	case "XAUUSD", "GOLD":
		return "FOREXCOM:XAUUSD"
	case "EURUSD":
		return "FOREXCOM:EURUSD"
	case "XAGUSD", "SILVER":
		return "FOREXCOM:XAGUSD"
	case "GBPUSD":
		return "FOREXCOM:GBPUSD"
	case "USDJPY":
		return "FOREXCOM:USDJPY"
	case "AUDUSD":
		return "FOREXCOM:AUDUSD"
	case "USDCAD":
		return "FOREXCOM:USDCAD"
	case "USDCHF":
		return "FOREXCOM:USDCHF"
	case "NZDUSD":
		return "FOREXCOM:NZDUSD"
	case "NAS100", "US100", "USTEC", "NDX":
		return "FOREXCOM:NAS100"
	case "US500", "SPX500", "SPX":
		return "FOREXCOM:SPX500"
	case "BTCUSD", "BTCUSDT":
		return "BINANCE:BTCUSDT"
	case "ETHUSD", "ETHUSDT":
		return "BINANCE:ETHUSDT"
	default:
		// Default to FOREXCOM prefix for currency/metal pairs
		return "FOREXCOM:" + symClean
	}
}

func (p *TradingViewProvider) MapResolution(timeframe string) string {
	tf := strings.ToUpper(strings.TrimSpace(timeframe))
	switch tf {
	case "1M", "M1", "1":
		return "1"
	case "3M", "M3", "3":
		return "3"
	case "5M", "M5", "5":
		return "5"
	case "15M", "M15", "15":
		return "15"
	case "30M", "M30", "30":
		return "30"
	case "1H", "H1", "60M", "60":
		return "60"
	case "2H", "H2", "120M", "120":
		return "120"
	case "4H", "H4", "240M", "240":
		return "240"
	case "1D", "D1", "D":
		return "1D"
	case "1W", "W1", "W":
		return "1W"
	default:
		// Timeframe tak dikenal tidak boleh diam-diam memakai 15 menit:
		// candle dengan resolusi berbeda membuat evaluasi salah tanpa jejak.
		return ""
	}
}

func ParseTimeframeDuration(timeframe string) time.Duration {
	tf := strings.ToUpper(strings.TrimSpace(timeframe))
	switch tf {
	case "1M", "M1", "1":
		return 1 * time.Minute
	case "3M", "M3", "3":
		return 3 * time.Minute
	case "5M", "M5", "5":
		return 5 * time.Minute
	case "15M", "M15", "15":
		return 15 * time.Minute
	case "30M", "M30", "30":
		return 30 * time.Minute
	case "1H", "H1", "60M", "60":
		return 1 * time.Hour
	case "2H", "H2", "120M", "120":
		return 2 * time.Hour
	case "4H", "H4", "240M", "240":
		return 4 * time.Hour
	case "1D", "D1", "D":
		return 24 * time.Hour
	case "1W", "W1", "W":
		return 7 * 24 * time.Hour
	default:
		return 15 * time.Minute
	}
}

func randomTVSessionID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func formatTVMsg(funcName string, params []any) string {
	payload, _ := json.Marshal(map[string]any{
		"m": funcName,
		"p": params,
	})
	return fmt.Sprintf("~m~%d~m~%s", len(payload), payload)
}

func (p *TradingViewProvider) FetchCandles(ctx context.Context, symbol string, timeframe string, limit int) ([]indicators.Candle, error) {
	tvSymbol := p.MapSymbol(symbol)
	resolution := p.MapResolution(timeframe)
	if resolution == "" {
		return nil, fmt.Errorf("timeframe TradingView tidak dikenal: %q", timeframe)
	}

	if limit <= 0 {
		limit = 100
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, p.wsURL, p.headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to TradingView websocket: %w", err)
	}
	defer conn.Close()

	chartSession := randomTVSessionID("cs_")

	// 1. Auth token
	_ = conn.WriteMessage(websocket.TextMessage, []byte(formatTVMsg("set_auth_token", []any{"unauthorized_user_token"})))
	// 2. Chart session
	_ = conn.WriteMessage(websocket.TextMessage, []byte(formatTVMsg("chart_create_session", []any{chartSession, ""})))

	// 3. Resolve symbol
	symSpec, _ := json.Marshal(map[string]string{
		"symbol":     tvSymbol,
		"adjustment": "splits",
	})
	_ = conn.WriteMessage(websocket.TextMessage, []byte(formatTVMsg("resolve_symbol", []any{chartSession, "sds_sym_1", "=" + string(symSpec)})))

	// 4. Create series
	_ = conn.WriteMessage(websocket.TextMessage, []byte(formatTVMsg("create_series", []any{chartSession, "sds_1", "s1", "sds_sym_1", resolution, limit, ""})))

	readDeadline := time.Now().Add(8 * time.Second)
	conn.SetReadDeadline(readDeadline)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("tradingview websocket read error (%s): %w", tvSymbol, err)
		}

		rawStr := string(msgBytes)
		splits := tvMsgRegex.Split(rawStr, -1)
		for _, part := range splits {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			if strings.HasPrefix(part, "~h~") {
				// Heartbeat reply
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("~m~%d~m~%s", len(part), part)))
				continue
			}

			var rawMap map[string]any
			if err := json.Unmarshal([]byte(part), &rawMap); err != nil {
				continue
			}

			method, _ := rawMap["m"].(string)
			if method == "timescale_update" || method == "du" {
				pSlice, ok := rawMap["p"].([]any)
				if !ok || len(pSlice) < 2 {
					continue
				}
				dataObj, ok := pSlice[1].(map[string]any)
				if !ok {
					continue
				}
				sdsObj, ok := dataObj["sds_1"].(map[string]any)
				if !ok {
					continue
				}
				barsList, ok := sdsObj["s"].([]any)
				if !ok || len(barsList) == 0 {
					continue
				}

				now := time.Now().UTC()
				candles := make([]indicators.Candle, 0, len(barsList))
				for _, b := range barsList {
					barMap, ok := b.(map[string]any)
					if !ok {
						continue
					}
					vList, ok := barMap["v"].([]any)
					if !ok || len(vList) < 5 {
						continue
					}

					tsSec, _ := vList[0].(float64)
					openPrice, _ := vList[1].(float64)
					highPrice, _ := vList[2].(float64)
					lowPrice, _ := vList[3].(float64)
					closePrice, _ := vList[4].(float64)
					vol := 0.0
					if len(vList) >= 6 {
						vol, _ = vList[5].(float64)
					}

					candleTime := time.Unix(int64(tsSec), 0).UTC()
					tfDuration := ParseTimeframeDuration(timeframe)
					candleCloseTime := candleTime.Add(tfDuration)
					isClosed := !now.Before(candleCloseTime)

					candles = append(candles, indicators.Candle{
						Timestamp: candleTime,
						Open:      openPrice,
						High:      highPrice,
						Low:       lowPrice,
						Close:     closePrice,
						Volume:    vol,
						IsClosed:  isClosed,
					})
				}

				return ValidateCandles(candles)
			}
		}
	}
}
