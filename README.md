# 🚀 GO-TRADE: Dual-Engine Automated Trading Ecosystem (ILA 2.0 & ILA 3.0)

Repositori resmi sistem otomasi trading kuantitatif deterministik berbasis metode **Imaginary Liquidity Analysis (ILA)** dengan arsitektur 100% native Golang.

Sistem mengoperasikan integrasi pasar real-time via WebSocket TradingView FOREX.com, kalkulasi risiko live equity 1.0%, proteksi prop firm harian, serta auto-eksekusi sinkron ke dua platform broker (**Match-Trader** & **IC Markets cTrader**).

---

## 📚 Panduan Onboarding AI Agent & Dokumentasi Lengkap (`docs/`)

Bagi AI Agent atau developer baru yang mengakses repository ini, silakan baca dokumentasi berikut secara berurutan:

1. **[00. Manifest & Panduan Urutan Baca](docs/00_READING_ORDER_MANIFEST.md)** ⬅️ *Mulai dari sini*
2. **[01. Arsitektur & Desain Sistem](docs/01_ARCHITECTURE_AND_SYSTEM_DESIGN.md)** — Clean architecture Go, pipeline data, dan skema database SQLite WAL.
3. **[02. Aturan Baku & Workflow ILA 2.0 (Active Live Engine)](docs/02_ILA_2.0_RULES_AND_WORKFLOW.md)** — Core engine live: 4-TF Matrix, Candle 4 Sweep, Candle 5 Entry, Dynamic CISD, dan DOL Breached rule.
4. **[03. Spesifikasi Framework Riset ILA 3.0](docs/03_ILA_3.0_MACRO_FRAMEWORK_SPEC.md)** — ERL vs IRL, Formula 3-Tier ($i+1, i+2, i+4$), Quarterly Theory (Q1-Q4 NY), SSMT Divergence, StDev Fibo, dan Nested Execution (R:R 1:5 s/d 1:10+).
5. **[04. Kebijakan Manajemen Risiko & Protokol Eksekusi](docs/04_RISK_MANAGEMENT_AND_POLICIES.md)** — Strict 1.0% Equity Risk, Quality Gate R:R $\ge$ 1:2.0, 2 Strike & Out, dan Floor SL anti-ketipisan.
6. **[05. Panduan Integrasi Multi-Broker](docs/05_BROKER_INTEGRATION_GUIDE.md)** — Native cTrader Protobuf Open API & Match-Trader REST Client API.
7. **[06. Sistem Evaluasi AI Pasca-Trade & Self-Learning](docs/06_AI_POST_MORTEM_AND_SELF_LEARNING.md)** — Klasifikasi Root Cause, Pre-Flight Safety Gate, dan format notifikasi Telegram mobile-friendly.

---

## 🏗️ Dua Pilar Engine dalam Repositori

| Parameter | ILA 2.0 (`ila-engine`) | ILA 3.0 (`ila3-engine`) |
|---|---|---|
| **Peran** | **Live Execution Engine (Aktif)** | **Macro Research & Testing (Riset)** |
| **Port Server** | `8080` (Daemon Utama) | `8082` (Daemon Riset) |
| **Database** | `ila.db` (SQLite WAL) | `ila3.db` (SQLite WAL) |
| **Fokus Strategi**| Retracement FVG HTF ➡️ CISD LTF | ERL 🔄 IRL Macro Cycle + StDev Fibo |
| **Timeframe** | 2-Tier Berpasangan (H4 ➡️ M15) | 3-Tier Fraktal (HTF ➡️ MTF ➡️ LTF) |
| **Filter Waktu** | Sesi Pasar Utama | Quarterly Theory (Q1-Q4 New York Time) |
| **Konfluensi** | Single Symbol Multi-Timeframe | Multi-Asset SSMT Divergence Check |
| **Eksekusi Broker**| Auto Dual-Broker (Live Active) | Simulasi Riset & Nested Execution |

---

## 🛠️ Instalasi & Kompilasi Cepat

### Prasyarat:
* Go 1.23+
* Linux OS (Headless VPS didukung penuh)

### Clone & Build:
```bash
# Clone repository
git clone https://github.com/KirisakiRei/go-trade.git
cd go-trade
git checkout develop

# Salin environment file
cp .env.example .env
# Masukkan token Match-Trader dan kredensial cTrader ke .env

# Kompilasi Binary ILA 2.0 (Live Engine):
go build -o bin/ila-cli ./cmd/cli
go build -o bin/ila-scanner ./cmd/scanner
go build -o bin/ila-server ./cmd/server
```

---

## 💻 Penggunaan CLI

```bash
# Evaluasi radar 4-timeframe ILA 2.0 untuk XAU/USD
./bin/ila-cli --symbol XAUUSD

# Evaluasi pasangan spesifik (H4 -> M15)
./bin/ila-cli --symbol XAUUSD --htf H4 --ltf M15

# Menjalankan daemon background (port 8080)
./bin/ila-server
```

---

## 🛡️ Kebijakan Keamanan Kredensial
Seluruh kredensial akun broker, token API, dan data rahasia disimpan murni dalam file `.env` lokal yang dilindungi oleh `.gitignore`. Dilarang keras melakukan commit atau menyebarkan kunci rahasia ke repositori publik.

## 📄 Lisensi
MIT License - KirisakiRei / Ryn
