# 📚 PANDUAN URUTAN BACA DOKUMENTASI (AGENT ONBOARDING MANIFEST)

> **Untuk AI Agent / Developer Baru:**  
> Jika Anda adalah AI Agent atau developer yang baru masuk ke repository `go-trade` ini, **BACA DOKUMEN-DOKUMEN BERIKUT SECARA BERURUTAN (01 s.d. 06)**.  
> Seluruh pengetahuan teknikal, rules trading, arsitektur sistem, dan kebijakan operasional telah dibukukan secara lengkap di sini sehingga Anda langsung memiliki pemahaman 100% identik dengan arsitek sistem ini tanpa perlu diajari ulang oleh user.

---

## 🗺️ Urutan Pembacaan Wajib:

```text
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ 1. 01_ARCHITECTURE_AND_SYSTEM_DESIGN.md                                                │
│    └─ Arsitektur Clean Architecture Golang, pipeline data TradingView, skema DB, dll.  │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 2. 02_ILA_2.0_RULES_AND_WORKFLOW.md                                                    │
│    └─ ATURAN BAKU ENGINE LIVE: 4-TF, Candle 4 Sweep, Candle 5 Entry, Dynamic CISD, dll.│
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 3. 03_ILA_3.0_MACRO_FRAMEWORK_SPEC.md                                                  │
│    └─ ATURAN RISET ILA 3.0: ERL vs IRL, 3-Tier, Quarterly Theory, SSMT, StDev Fibo.    │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 4. 04_RISK_MANAGEMENT_AND_POLICIES.md                                                  │
│    └─ Strict 1.0% Equity Risk, 2 Strike & Out, Floor SL, Broker-Managed Policy.        │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 5. 05_BROKER_INTEGRATION_GUIDE.md                                                      │
│    └─ Integrasi Dual-Broker: cTrader (Protobuf) & Match-Trader (REST API).             │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 6. 06_AI_POST_MORTEM_AND_SELF_LEARNING.md                                              │
│    └─ Engine Evaluasi Otomatis Pasca-Trade, Pre-Flight Filter, & Format Telegram.      │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 📌 Ringkasan Cepat Tiap Dokumen:

### 📖 `01_ARCHITECTURE_AND_SYSTEM_DESIGN.md`
* Menjelaskan struktur folder Go (`cmd/`, `internal/`, `pkg/`).
* Bagaimana data market ditarik secara real-time via WebSocket TradingView FOREX.com.
* Skema database SQLite WAL (`positions`, `trade_evaluations`, `trade_journals`).

### 📖 `02_ILA_2.0_RULES_AND_WORKFLOW.md` (Active Live Trading)
* Aturan baku yang saat ini berjalan secara live di cron scanner dan daemon port `8080`.
* 4 Matriks Timeframe: `W ➡️ H4`, `D1 ➡️ H1`, `H4 ➡️ M15`, `H1 ➡️ M5`.
* Aturan 100% closed candle, sumbu bersebelahan (*adjacent sweep*), dan penanganan *consecutive sweeps*.
* Definisi sejati Dynamic CISD (*consecutive opposing leg* murni di dalam Candle 5).
* Titik Stop Loss di batas Low/High Candle 5 dan DOL Breached Rule (kunci TP 1:2.0).

### 📖 `03_ILA_3.0_MACRO_FRAMEWORK_SPEC.md` (Next-Gen Research)
* Kerangka kerja likuiditas makro fraktal ILA 3.0 di `/opt/data/ila3-engine`.
* Dua Key Level: **ERL** (Swing High/Low ➡️ Reversal) dan **IRL** (FVG ➡️ Retracement).
* Hierarki 3-Tier Timeframe: `HTF ➡️ MTF ➡️ LTF` (Formula $i+1, i+2, i+4$).
* Quarterly Theory (Q1 Asia, Q2 London True Open, Q3 NY AM, Q4 NY PM) + **The Q1 Exception Rule**.
* Sequential SMT (SSMT) Correlation Check (EURUSD vs GBPUSD, Gold vs Silver, Nasdaq vs S&P).
* Proyeksi ICT Standard Deviation Fibo (-1, -2/-2.5 Zona Kritis, -3.5/-4 Zona Final).
* Nested Multi-Timeframe Execution (R:R 1:5 hingga 1:10+).

### 📖 `04_RISK_MANAGEMENT_AND_POLICIES.md`
* Rumus kalkulasi lot dinamis 1.0% equity live broker.
* Proteksi Drawdown Harian: **2 Strike & Out** (Kunci trading jika 2x SL tercapai hari itu).
* Batas minimal SL (Gold $4.50, Forex 20 pips) untuk mencegah SL tipis.
* Kebijakan Broker Pasif: Server broker yang mengeksekusi SL/TP, bot dilarang auto-close sepihak.

### 📖 `05_BROKER_INTEGRATION_GUIDE.md`
* Spesifikasi teknis koneksi **IC Markets cTrader** (Open API TCP TLS Protobuf).
* Spesifikasi teknis koneksi **Match-Trader** (REST API Bearer Token JWT, endpoint `/v1/positions` dan `/v1/closed-positions`).
* Mekanisme sinkronisasi dual-broker serentak via `MultiBrokerManager`.
* Kebijakan keamanan kredensial: isolasi di `.env` lokal, dilarang commit secret ke Git.

### 📖 `06_AI_POST_MORTEM_AND_SELF_LEARNING.md`
* Bagaimana sistem mengevaluasi trade yang sudah ditutup (Root Cause Analysis).
* Filter AI Pre-Flight sebelum open posisi.
* Format pelaporan notifikasi Telegram mobile-friendly (tabel ASCII ringkas, highlight `◀`, tanpa LaTeX math).
