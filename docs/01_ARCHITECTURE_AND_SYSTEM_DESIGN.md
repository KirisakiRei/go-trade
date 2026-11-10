# 01. ARSITEKTUR & DESAIN SISTEM (SYSTEM DESIGN)

## 1. Ikhtisar Arsitektur
Sistem ini dibangun dengan filosofi **100% Native Golang Clean Architecture**, tanpa ketergantungan runtime eksternal (seperti Python gateway) untuk eksekusi trading real-time. Seluruh proses analisis matematika, penarikan data WebSocket, evaluasi aturan teknikal, manajemen risiko, dan komunikasi broker berjalan dalam satu binary Go yang efisien dan deterministik.

---

## 2. Struktur Direktori Proyek

```text
ila-engine/
├── cmd/
│   ├── cli/            # Tool CLI untuk evaluasi manual & analisis on-demand
│   ├── scanner/        # Worker periodik 15-menit yang dipanggil oleh cronjob
│   └── server/         # Daemon REST API (port 8080) & monitor pasif rekonsiliasi
├── internal/
│   ├── config/         # Parser konfigurasi & inisialisasi SQLite database
│   ├── dto/            # Data Transfer Objects untuk request/response API
│   ├── handler/        # HTTP Handlers (Gin Web Framework)
│   ├── middleware/     # CORS, Recovery, Request Logger
│   ├── model/          # Skema database GORM (Position, TradeJournal, TradeEvaluation)
│   ├── repository/     # Data Access Object (DAO) untuk SQLite
│   ├── router/         # Pendaftaran route REST API
│   └── service/        # Core business logic: ILAService, ExecutionService, EvaluationService
└── pkg/
    ├── apperror/       # Standard error definitions
    ├── broker/         # Adapter Broker: cTrader, Match-Trader, MultiBrokerManager
    ├── httpx/          # HTTP helper utilities
    ├── ila/            # Math Engine ILA 2.0 (FindFVGs, DetectHTFIlaSweep, DetectDynamicCISD)
    ├── indicators/     # Model data Candle, OHLC, dan kalkulator indikator teknikal
    ├── marketfeed/     # Provider data real-time: TradingView WSS (FOREX.com), Binance (kripto)
    └── risk/           # Kalkulator 1.0% Live Equity Risk & Quality Gate R:R
```

---

## 3. Pipeline Data Pasar Real-Time (`pkg/marketfeed`)

Sistem menggunakan **MultiFeedProvider** dengan hierarki prioritas:
1. **Primary Feed: TradingView WebSocket (`tradingview.go`)**
   * Terhubung langsung ke gateway chart TradingView:  
     `wss://data.tradingview.com/socket.io/websocket?from=chart%2F`
   * Menggunakan kuotasi resmi **FOREX.com**:
     * Emas: `FOREXCOM:XAUUSD`
     * Euro: `FOREXCOM:EURUSD`
     * Nasdaq: `FOREXCOM:NAS100`
   * **Keunggulan:** Presisi bentuk candle, jam penutupan (*closing time*), dan wick identik 100% dengan chart di aplikasi TradingView HP pengguna.
2. **Cadangan: Binance REST (`binance.go`)**
   * Hanya untuk instrumen kripto. Forex, logam, dan indeks tidak punya
     padanan di Binance, sehingga permintaan seperti itu dikembalikan sebagai
     galat — bukan diisi candle instrumen lain.

> Yahoo Finance sudah dihapus (2026-11-10). Alasannya: pemetaan timeframe-nya
> tidak lengkap (M5 dan W jatuh ke 15 menit, H4 memakai candle harian) dan
> pemetaan simbolnya bisa mengembalikan instrumen berbeda (mis. emas futures
> untuk XAUUSD). Candle dari instrumen atau resolusi yang salah tetap terlihat
> sah, sehingga lebih berbahaya daripada tidak ada data.

---

## 4. Skema Database SQLite WAL (`internal/model`)

Database disimpan lokal di file `ila.db` dengan mode **Write-Ahead Logging (WAL)** untuk mendukung konkurensi tinggi antara daemon server, scanner cron, dan CLI:

### A. Tabel `positions`
Menyimpan riwayat dan status setiap posisi trading:
* `id` (UUID Primary Key)
* `ticket_number` (String tiket broker gabungan, misal: `Match-Trader (#W...) & cTrader (#692...)`)
* `setup_id` (ID sinyal ILA, format: `ILA-XAUUSD-H4-M15-YYYYMMDDHHMM`)
* `symbol`, `direction`, `timeframe`
* `open_price`, `stop_loss`, `take_profit`, `exit_price`
* `lot_size`, `risk_reward`, `profit_loss`, `outcome_r`
* `status` (`OPEN`, `CLOSED_TP`, `CLOSED_SL`)
* `opened_at`, `closed_at`
* `notes` (Catatan audit broker dan ringkasan setup)

### B. Tabel `trade_evaluations`
Menyimpan audit pasca-trade (*self-learning engine*):
* `ticket_number`, `symbol`, `direction`
* `outcome` (`HIT_TP` atau `HIT_SL`)
* `root_cause` (Klasifikasi akar masalah teknikal)
* `technical_analysis` (Analisa objektif perilaku pergerakan harga)
* `actionable_lesson` (Pelajaran yang wajib dipatuhi sistem di masa depan)

---

## 5. Tiga Mode Operasional Sistem

1. **Daemon Server (`bin/ila-server`):**
   * Berjalan terus-menerus di background pada port **`8080`**.
   * Menjalankan loop rekonsiliasi posisi broker pasif setiap 10 detik (`CheckAndManagePositions`).
   * Melayani request REST API `/api/v1/analysis/ila`, `/api/v1/trade/*`, `/health`.
2. **Scanner Otomatis (`bin/ila-scanner`):**
   * Dijalankan setiap 15 menit oleh cronjob Linux (`scan_propfirm.py`).
   * Melakukan pemindaian 4 matriks timeframe ILA pada seluruh simbol aktif.
   * Mengirim alert transisi langkah ke Telegram dan mengeksekusi order jika ada setup valid.
3. **CLI Evaluator (`bin/ila-cli`):**
   * Tool terminal untuk pengecekan on-demand:  
     `./bin/ila-cli --symbol XAUUSD --htf H4 --ltf M15`
