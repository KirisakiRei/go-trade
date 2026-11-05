# ILA Engine (Imaginary Liquidity Analysis)

Sistem otomasi trading multi-broker (Match-Trader & cTrader) berbasis metode deterministik **Imaginary Liquidity Analysis (ILA)** dengan arsitektur 100% native Golang.

## 🚀 Fitur Utama

- **Analisis Multi-Timeframe Berpasangan:**
  - `Weekly ➡️ H4`: Swing Bias & Macro Draw on Liquidity
  - `Daily ➡️ H1`: Daily Liquidity & Medium Swing
  - `H4 ➡️ M15`: Intraday Primary Setup
  - `H1 ➡️ M5`: Scalping Trigger
- **Kaidah Teknis Deterministik ILA:**
  - **Strict Latest FVG:** Deteksi otomatis Fair Value Gap 3-candle closed yang belum termitigasi penuh.
  - **Adjacent Candle Sweep:** Deteksi sapuan ekor pada candle persis bersebelahan di dalam area FVG.
  - **Consecutive Sweeps Handling:** Reset otomatis status sweep jika candle HTF berikutnya melakukan sweep berulang tanpa menembus batas mitigasi FVG.
  - **Dynamic CISD Tracing:** Penelusuran origin deretan candle lawan murni di dalam rentang waktu candle entry.
  - **Validasi Body-Break-Body:** Konfirmasi pembalikan arah melalui candle LTF yang ditutup resmi menembus garis CISD.
- **Eksekusi Dual-Broker Serentak:**
  - **Match-Trader:** Terhubung langsung melalui REST Client Trader API.
  - **cTrader:** Terhubung langsung melalui native TCP TLS Protobuf Open API.
  - **Dynamic 1% Live Equity Risk:** Menghitung ukuran lot secara proporsional sesuai saldo riil tiap broker dengan proteksi SL/TP server-side.

## 📁 Struktur Direktori

```text
ila-engine/
├── cmd/
│   ├── cli/       # CLI evaluasi sinyal ILA & eksekusi manual
│   ├── scanner/   # Scanner periodik otomatis untuk cronjob
│   └── server/    # REST API daemon & sinkronisasi pasif broker
├── internal/
│   ├── config/    # Konfigurasi aplikasi & database
│   ├── dto/       # Data transfer objects
│   ├── handler/   # HTTP handlers (REST API)
│   ├── model/     # Database models (GORM SQLite)
│   ├── repository/# Database access layer
│   ├── router/    # Routing engine (Gin)
│   └── service/   # Business logic (ILA, Execution, Post-Trade Evaluation)
└── pkg/
    ├── apperror/  # Error definitions
    ├── broker/    # Broker connectors (Match-Trader, cTrader, MultiBrokerManager)
    ├── httpx/     # HTTP utilities
    ├── ila/       # Core math & rule engine ILA (FVG, Sweep, Dynamic CISD)
    ├── indicators/# Data struktur candle & teknikal
    ├── marketfeed/# Real-time market feed (TradingView FOREX.com WSS, Binance, Yahoo)
    └── risk/      # 1% Live equity risk calculator & Quality Gate
```

## 🛠️ Instalasi & Build

```bash
# Clone repository
git clone https://github.com/KirisakiRei/go-trade.git
cd go-trade
git checkout develop

# Copy file environment & masukkan kredensial akun
cp .env.example .env

# Build binary
go build -o bin/ila-cli ./cmd/cli
go build -o bin/ila-scanner ./cmd/scanner
go build -o bin/ila-server ./cmd/server
```

## 💻 Penggunaan

```bash
# Scan 4 pasangan timeframe serentak untuk simbol tertentu
./bin/ila-cli --symbol XAUUSD

# Scan pasangan timeframe spesifik
./bin/ila-cli --symbol XAUUSD --htf H4 --ltf M15

# Jalankan daemon server
./bin/ila-server
```

## 🛡️ Lisensi
MIT License
