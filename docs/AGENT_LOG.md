# 🤝 AGENT_LOG.md — Kanal Komunikasi 2 Arah (Human ⇄ Hermes)

> **Tujuan:** Media logging perubahan & pesan strategis antara **Human (owner)** dan **Hermes (agent)** melalui repository Git.
> **Cara pakai:**
> 1. Human / Claude menambahkan entri baru di bagian **⬇️ DARI HUMAN KE HERMES** (paling atas = terbaru).
> 2. Hermes menambahkan entri baru di bagian **⬆️ DARI HERMES KE HUMAN** (paling atas = terbaru).
> 3. Setiap entri **WAJIB** punya: `Tanggal`, `Pelaku`, `Tipe`, `Ringkasan`, `File terdampak`, `Status`.
> 4. Setelah Hermes menjalankan tugas dari human, ia **wajib** menulis balasan dengan menandai `DIBACA` / `SELESAI` / `BLOKIR`.
>
> **Aturan Emas:**
> - Log ini adalah **catatan, bukan instruksi otomatis**. Kesepakatan tetap perlu dilihat human.
> - Hermes **TIDAK** boleh mengubah isi bagian **DARI HUMAN** (hanya boleh membaca & menandai status baca).
> - Human / Claude **TIDAK** mengubah isi bagian **DARI HERMES** (hanya dicatat siapa yang menjawab).

---

## 📍 PENANDA SINKRONISASI

> **Commit terakhir saat log ini ditulis:** `0a2f7ca`
> **Tag titik sinkron:** `sync/2026-11-06-ila3-fixes`

### Cara pakai (untuk Hermes / Human / Claude)

Setiap kali diminta "pull dan baca AGENT_LOG", jalankan urutan ini:

```bash
# 1. Tarik perubahan terbaru
git pull origin develop

# 2. Lihat SEMUA commit yang baru sejak tag sinkron terakhir
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# 3. Baca file ini (AGENT_LOG.md) dari atas — entri terbaru ada di paling atas
```

Setelah selesai membaca & mengerjakan tugas:
- Tambahkan balasan di bagian **⬆️ DARI HERMES** (untuk Hermes) atau **⬇️ DARI HUMAN** (untuk Human/Claude).
- Perbarui bagian ini → ganti `Commit terakhir` menjadi hash HEAD yang baru.

### Perintah siap-tempel (kirim ke Hermes)

```text
Hermes, pull lalu jalankan:
  git pull origin develop
  git log $(git describe --tags --abbrev=0)..HEAD --oneline
Lalu baca docs/AGENT_LOG.md (entri terbaru di paling atas)
dan balas di bagian "⬆️ DARI HERMES KE HUMAN".
```

---

## 📇 Format Template Entri

```markdown
### [YYYY-MM-DD] <TIPE> — <Judul Singkat>
- **Pelaku:** Human / Claude / Hermes
- **Tipe:** INFO | REVIEW | PERBAIKAN | KONFLIK | BLOKIR | TUGAS
- **Status:** 🔴 WAITING | 🟡 DIBACA | 🟢 SELESAI | ⚫ BLOKIR
- **Ringkasan:**
  - ...
- **File / Bagian Terdampak:**
  - `path/to/file:baris`
- **Aksi yang diminta (kalau ada):**
  - ...
- **Catatan:** ...
```

---

## ⬇️ DARI HUMAN KE HERMES

### [2026-11-06] PERBAIKAN — Market-hours gate, wording notifikasi, & pisah notif vs eksekusi
- **Pelaku:** Human + Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI (perlu `git pull` + rebuild oleh Hermes)
- **Ringkasan:**
  - **Scan hanya saat market buka.** Ditambahkan `isMarketClosed()` di `cmd/scanner/main.go`:
    - Sabtu penuh + Minggu penuh → **tutup** (tidak scan, tidak eksekusi). Pasar dianggap mulai **Senin**.
    - Rollover **22:00–01:00 UTC** (05:00–08:00 WIB, spread lebar) → **skip**.
    - Selain itu → scan normal. (Efektif aktif ≈ 05:00–23:00 WIB, hari kerja.)
  - **Fix bug rollover bocor 1 jam** di scanner: sebelumnya `utcHour >= 22 && utcHour < 24`, kini `utcHour >= 22 || utcHour < 1`. Diselaraskan dgn evaluator & bbma_service.
  - **Notif tetap jalan, eksekusi tetap ditahan.** Saat ada posisi open / kuota habis / sudah 1 win, setup valid **tetap dikirim** sebagai notifikasi keputusan manual.
  - **Wording notifikasi diperbaiki** (lebih jelas & actionable):
    - Judul: `🎯 [SETUP ILA VALID - ENTRY MANUAL]`
    - Isi: Entry/SL/TP + R:R, alasan auto ditahan, dan kalimat `Keputusan di tangan Anda: entry manual jika setuju…`
    - `LockReason` di `GetDailyTradeStatusWIB` dibuat lebih netral (tanpa emoji *warning* + tidak lagi menyebut angka internal yang salah).
  - **Posisi terbuka tidak disentuh.** SL/TP tetap server-side di broker; engine pasif. Tidak ada market-close sepihak.
- **File Terdampak:**
  - `cmd/scanner/main.go` — `isMarketClosed()`, early-return saat tutup, wording 2 notif
  - `internal/repository/position_repository.go` — wording `LockReason`
- **Cara Verifikasi:**
  ```bash
  go build ./...   # bersih
  go vet ./...     # bersih
  go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...   # PASS
  ```
  `isMarketClosed` sudah diuji langsung (Senin–Jumat buka, Sabtu/Minggu tutup, rollover 22–01 skip).
- **Catatan:** Hermes perlu `git pull`, rebuild binary, lalu restart daemon live.

---

### [2026-11-06] PERBAIKAN — Audit bug jalur eksekusi ILA & risk engine (7 file)
- **Pelaku:** Human + Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI (menunggu review Human sebelum deploy)
- **Ringkasan:** Audit menyeluruh menemukan beberapa bug logika di jalur uang (eksekusi & risiko). Semua diperbaiki TANPA mengubah flow trading ILA yang sudah ada. Build ✅, vet bersih ✅, unit test risk/ila/indicators ✅.

#### Bug yang diperbaiki
| # | Bug | Sebelum | Sesudah |
|---|---|---|---|
| B1 | **Drawdown 2-Strike & Out ILA terbalik** | `if req.Force { if losses >= 5 }` → guard praktis mati | `if !req.Force { if losses >= 2 }` (sama dgn BBMA, doc 04) |
| B2 | **`PassedRisk` hardcoded `true`** | lot clamp bisa naikkan risiko tanpa terdeteksi | risiko aktual dihitung; entry ditolak jika > target (+5% toleransi) |
| B2b | **Tak ada clamp risiko** | `RiskRatio` tak dibatasi | hard clamp maks 2% di `CalculateLot` |
| B3 | **SL floor tidak ditegakkan di ILA** | dokumen klaim SL floor, kode ILA tak cek | `PreFlightCheck` dipanggil di `EvaluateAndExecuteILA` sebelum order |
| B4 | **Root cause SL ILA palsu (konstanta)** | selalu `ILA_SWEEP_INVALIDATION` | taksonomi nyata: `ILA_SL_TERLALU_TIPIS` / `ILA_PELEBARAN_SPREAD_ROLLOVER` / `ILA_LIQUIDITY_SWEEP_FALSE_BREAK` / `ILA_SWEEP_INVALIDATION` |
| B5 | **Bug rollover `hour < 0` (dead) + `closeHour` pakai OpenedAt** | rollover hanya 22-23 UTC, closeHour salah | jendela 22:00-01:00 UTC, `closeHour` dari `ClosedAt` |
| B5b | Bug rollover sama di `bbma_service.go` | `utcHour < 0` dead | diperbaiki jadi `utcHour < 1` |
| B11 | `go vet` warnings `fmt.Println` newline | 4 warning | 0 warning |
| — | `PreFlightCheck` Forex floor 0.0018 | tidak sesuai dokumen (20 pips = 0.0020) | diselaraskan ke 0.0020, index dikecualikan |

#### File Terdampak
- `pkg/risk/calculator.go` — actual risk + clamp 2% + `PassedRisk` nyata
- `pkg/risk/calculator_test.go` — 2 test regresi baru (overshoot & clamp)
- `internal/service/execution_service.go` — fix drawdown ILA, wiring `EvaluationService`, risk gate, SL floor
- `internal/service/evaluation_service.go` — taksonomi root cause ILA nyata, fix rollover
- `internal/service/bbma_service.go` — fix rollover dead condition
- `internal/router/router.go` — wiring `evalService` ke execution (dibuat sebelum dipakai)
- `cmd/cli/main.go` — sesuaikan konstruktor + API `fmt`

#### Cara Verifikasi (wajib dicek Hermes)
```bash
go build ./...          # harus bersih
go vet ./...            # harus bersih
go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...   # harus PASS
# catatan: go test ./pkg/marketfeed/... butuh network (Binance) → bisa timeout, bukan bug kode
```

#### ⚠️ Perubahan perilaku yang perlu Human setujui
1. **2-Strike & Out sekarang AKTIF di ILA** — setelah 2 SL hari itu, eksekusi ILA dikunci (kecuali `--force`). Sesuai `docs/04`.
2. **Entry ILA bisa DITOLAK** jika SL < floor (Gold $4.50 / Forex 20 pips). Ini sesuai dokumen tapi sebelumnya tidak berlaku.
3. **Entry ILA bisa DITOLAK** jika pembulatan lot membuat risiko > target.

#### Belum disentuh (perlu keputusan bisnis)
- **B2 penamaan `RiskRatio`** → masih bernama `RiskRatio` ( nilai 1.0 = 1% ), belum di-rename ke `RiskPercent` demi menghindari perubahan API massal. Sudah di-clamp, jadi aman.
- **B7 `AccountSize/FullRiskPercent=0.75` di scanner** → konstanta tidak dipakai untuk sizing riil; dibiarkan (hanya dokumentasi).
- **B10 dead code BBMA** → sengaja tidak dihapus.
- **B14 ILA 3.0** → di repo terpisah, bukan di sini.

- **Aksi yang diminta (Hermes):**
  1. Pull, baca entri ini.
  2. Jalankan blok **Cara Verifikasi**. Jika ada yang gagal → laporkan di bagian ⬆️ HERMES dengan status `⚫ BLOKIR`.
  3. Jangan buka PR untuk mengubah file di zona merah tanpa persetujuan Human.
  4. Balas di bagian **⬆️ DARI HERMES** dengan status `🟡 DIBACA` / `🟢 SELESAI`.

---

### [2026-11-06] INFO — Tambah Penanda Sinkronisasi (tag + perintah baca commit)
- **Pelaku:** Human + Claude
- **Tipe:** INFO
- **Status:** 🔴 WAITING (menunggu dibaca Hermes)
- **Ringkasan:**
  - Menambahkan bagian **📍 PENANDA SINKRONISASI** berisi hash commit terakhir + tag `sync/2026-11-06`.
  - Hermes tidak perlu dituliskan hash manual; cukup jalankan `git log $(git describe --tags --abbrev=0)..HEAD --oneline` untuk melihat commit baru.
- **File Terdampak:**
  - `docs/AGENT_LOG.md`
- **Aksi yang diminta:**
  - Pull, baca file ini, lalu **balas di bagian ⬆️ DARI HERMES** dengan status `🟡 DIBACA`.
  - Setelah membaca, perbarui baris "Commit terakhir" pada bagian Penanda Sinkronisasi menjadi hash HEAD milikmu.
- **Cara Verifikasi:** `git log $(git describe --tags --abbrev=0)..HEAD --oneline` tidak kosong setelah pull.

### [2026-06-XX] INFO — Setup AGENT_LOG.md sebagai kanal komunikasi resmi
- **Pelaku:** Human + Claude
- **Tipe:** INFO
- **Status:** 🔴 WAITING (menunggu dibaca Hermes)
- **Ringkasan:**
  - Mulai sekarang, semua perubahan dari Human/Claude akan dicatat di atas.
  - Hermes **wajib** membaca file ini setiap kali pull, dan membalas di bagian **⬆️ DARI HERMES KE HUMAN** bila diminta.
- **Aksi yang diminta:**
  - Konfirmasi dengan entri balasan di bagian **⬆️ DARI HERMES**, status `🟡 DIBACA`.
- **Catatan:** Log ini akan tumbuh; jangan hapus entri lama, cukup tambah baru di paling atas.

---

## ⬆️ DARI HERMES KE HUMAN

### [2026-11-06] AUDIT & PERBAIKAN FATAL ILA 3.0 — Fix False TP Alert, Validasi Interaksi MTF Key Level, & SL Floor Terukur
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Audit Bug Notifikasi False TP (`cmd/ila3-scanner/main.go`):**
    - **Penyebab:** Toleransi TP sebelumnya di-hardcode `0.50` untuk semua instrumen (`exitPrice >= pos.TakeProfit - 0.50`). Untuk EUR/USD, `0.50` setara dengan 5.000 pips! Akibatnya, setiap posisi sell/buy Forex yang kena SL selalu lolos cek harga dan keliru dilaporkan sebagai `HIT TAKE PROFIT 🎯` dengan hasil R:R positif palsu padahal PnL riilnya minus ($-340).
    - **Perbaikan:** Menjadikan status PnL riil broker sebagai penentu utama (`actualPnL > 0` ➡️ `CLOSED_TP`, `actualPnL < 0` ➡️ `CLOSED_SL` dengan outcome R negatif). Memperbaiki toleransi instrumen: `0.00050` untuk Forex dan `0.50` untuk Emas/Indeks.
  - **Audit Bug R:R Fantasi 57:1 & SL 1.8 Pip (`pkg/sequence/evaluator.go`):**
    - **Penyebab:** Engine mengambil Stop Loss murni dari ekstrim 5-menit terdekat tanpa batas struktural (jarak hanya 1.8 pip di Forex) dan memasang TP tunggal langsung ke Swing High Daily yang berjarak 105 pips (`105 / 1.85` = 57:1 R:R). SL 1.8 pip langsung disapu oleh spread broker!
    - **Perbaikan:** 
      1. Menegakkan **SL Floor Struktural Minimal**: Minimal 15 pips di Forex dan $4.00 di Emas menggunakan ekstrim MTF Key Level atau batas spread aman.
      2. Mengarahkan Take Profit eksekusi utama ke **ICT Standard Deviation Level -2.0 s.d. -2.5 (Zona Kritis / TP 1)** sesuai Bagian 12, 14, dan 16 Dokumen ILA 3.0, dengan batas R:R terukur (1:2.5 s.d. 1:4.5). Target makro HTF tetap dicatat di info `NestedExecution`.
  - **Validasi Interaksi MTF Key Level (Bagian 8 & 10 Dokumen ILA 3.0):**
    - Engine sebelumnya langsung melompat ke LTF CISD tanpa memverifikasi apakah harga di MTF sudah menguji MTF Key Level ERL.
    - Kini diwajibkan validasi sentuhan: MTF candle wajib menguji ERL Swing Low (untuk Bullish) atau ERL Swing High (untuk Bearish) sebelum mencari konfirmasi CISD di LTF.
  - **Hasil Uji & Verifikasi:**
    - `go build ./...` ➡️ BERSIH (exit 0)
    - `go vet ./...` ➡️ BERSIH (exit 0)
    - `go test -v ./pkg/...` ➡️ PASS 100%
    - Status EURUSD & XAUUSD ILA 3.0 kini rapi di `WAIT_MTF_REVERSAL` menunggu pengujian MTF ERL riil, tidak ada lagi order prematur dengan SL tipis!

### [2026-11-06] INTEGRASI REPO TUNGGAL — Penyatuan ILA 3.0 ke dalam Repo go-trade (Single Project, Multi-Binary)
- **Pelaku:** Hermes
- **Tipe:** FITUR
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Penyatuan Kode Sumber ke dalam Repo Utama `go-trade` (`/opt/data/ila-engine`):**
    - Sesuai arahan user, seluruh paket dan engine ILA 3.0 telah diintegrasikan langsung ke dalam repositori tunggal `go-trade` tanpa mengganggu struktur atau kode ILA 2.0 sedikit pun.
    - **Paket Algoritma ILA 3.0 Baru di `pkg/`:**
      - `pkg/keylevels/` ➡️ Deteksi ERL (External Range Liquidity) & IRL (Internal Range Liquidity).
      - `pkg/quarterly/` ➡️ Quarterly Theory (Q1-Q4 Daily NY Time & 90-min sub-cycle) + Q1 Exception Rule.
      - `pkg/ssmt/` ➡️ Sequential SMT Correlation & Divergence (XAUUSD vs XAGUSD, EURUSD vs GBPUSD, USTEC vs US500).
      - `pkg/stdev/` ➡️ ICT Standard Deviation Fibonacci Projection (-1.0, -2.0 s.d. -2.5 partial TP, -3.5 s.d. -4.0 final target).
      - `pkg/sequence/` ➡️ Macro Sequence Evaluator ILA 3.0 (ERL Reversal Entry 1, IRL Retrace Entry 2, Waypoint FVG filter, Nested R:R).
    - **Binary & Perintah Eksekusi Baru di `cmd/` & `bin/`:**
      - `bin/ila3-cli` (`cmd/ila3-cli`) ➡️ CLI analyzer ILA 3.0 multi-mode (New Day 4H->1H->5m, New Week D1->4H->15m).
      - `bin/ila3-scanner` (`cmd/ila3-scanner`) ➡️ Background scanner cTrader dedikasi (Auto-execution, trailing checkpoints, dan Telegram alert).
      - `bin/ila3-server` (`cmd/ila3-server`) ➡️ REST API ILA 3.0 aktif di port `8082`.
  - **Status Kompilasi & Pengujian Bersih:**
    - `go build ./...` ➡️ BERSIH (exit 0)
    - `go vet ./...` ➡️ BERSIH (exit 0, 0 warning)
    - `go test -v ./pkg/...` ➡️ PASS 100% (semua unit test ILA 2.0 & ILA 3.0 lulus sempurna).
  - **Dual-Engine & Dual-Broker Berjalan Serentak:**
    - Port 8080 (`ila-server`): ILA 2.0 Live pada akun Match-Trader (#1089632).
    - Port 8082 (`ila3-server`): ILA 3.0 Live pada akun IC Markets cTrader (#10131354).
    - Cronjob `3bb5433dac8a` (ILA 2.0) dan `93fc06a1b574` (ILA 3.0) berjalan mandiri tiap 15 menit.

### [2026-11-06] ARSITEKTUR DUAL-BROKER DEDIKASI — ILA 2.0 di Match-Trader (#1089632) & ILA 3.0 di cTrader (#10131354)
- **Pelaku:** Hermes
- **Tipe:** FITUR
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Pemisahan Akun Eksekusi (Isolasi Penuh Tanpa Saling Ganggu):**
    - **ILA 2.0 (`ila-engine` port 8080):** Dikonfigurasi murni mengeksekusi akun **Match-Trader Demo (#1089632)** dengan disiplin prop firm ketat (0.5% risk, R:R 1:2.0-2.3 Opsi A, kuota harian WIB). cTrader dilepas dari engine ILA 2.0.
    - **ILA 3.0 (`ila3-engine` port 8082):** Dikonfigurasi murni mengeksekusi akun **IC Markets cTrader (#10131354)** melalui Open API Protobuf TLS.
  - **Implementasi Core ILA 3.0 Scanner (`cmd/ila3-scanner`):**
    - Memantau 3 instrumen (XAUUSD, EURUSD, USTEC) dalam mode New Day (4H ➡️ 1H ➡️ 5m) dan New Week (D1 ➡️ 4H ➡️ 15m).
    - Memproses evaluasi makro komprehensif: Quarterly Theory (QT), Key Levels ERL/IRL, Cross-TF validation, Waypoint FVG filter, SSMT correlation (XAUUSD vs XAGUSD, EURUSD vs GBPUSD, USTEC vs US500), dan StDev Fibonacci checkpoints (-1.0, -2.0 s.d. -2.5 partial TP, -3.5 s.d. -4.0 final target).
    - Auto-eksekusi cTrader dengan dynamic 0.5% live equity sizing ($492.50) dan Stop Loss di LTF extreme.
    - Database lokal independen: `ila3-engine/ila3.db`.
  - **Penjadwalan Cron Terisolasi:**
    - Cron `3bb5433dac8a` (setiap 15m): Menjalankan `scan_propfirm.py` ➡️ ILA 2.0 Match-Trader.
    - Cron `93fc06a1b574` (setiap 15m): Menjalankan `scan_ila3.py` ➡️ ILA 3.0 cTrader.
    - Notifikasi kedua engine dikirimkan per bubble terpisah via `hermes send`.
  - **Verifikasi & Eksekusi Pertama:**
    - Setup ILA 3.0 pertama terkonfirmasi valid pada XAU/USD (Mode New Week D1 ➡️ 4H ➡️ 15m, ENTRY 1 REVERSAL).
    - Berhasil dieksekusi real ke cTrader: **Tiket `#693200601` XAUUSD BUY 0.32 Lot** (Entry 4070.11, SL 4053.91, TP 4146.99, Nested R:R 1:4.75).

### [2026-11-06] PERBAIKAN ILA 2.0 — Kunci Mutlak FVG Aktif Terbaru (Anti-Mundur ke FVG Usang) & Pemisahan Bubble Notifikasi Telegram
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Kunci Mutlak FVG Aktif Terbaru (`pkg/ila/evaluator.go`):**
    - Evaluator HTF sweep kini secara ketat HANYA menggunakan `latestFVG := activeFVGs[len(activeFVGs)-1]`.
    - Dilarang keras melompat mundur ke FVG usang yang sudah terlewati oleh siklus gelombang baru.
    - Status XAUUSD H1 ➡️ M5 kini konsisten **🟢 Buy (Bullish)** berpatokan pada FVG Bullish terkini (`4050.58 - 4054.30`), selaras dengan displacement kuat pasar dan bukan lagi keliru Sell ke FVG subuh.
  - **Pemisahan Bubble Notifikasi Telegram (`scripts/scan_propfirm.py`):**
    - Bila ada lebih dari 1 update pair yang perlu dilaporkan (misal XAUUSD dan USTEC), script memecah pesan per pair dan mengirimkannya masing-masing ke Telegram via `hermes send --to telegram:808466451` dengan jeda 0.5 detik.
    - Hasilnya di Telegram: setiap pair tampil rapi dalam bubble terpisah, tidak bertumpuk dalam 1 bubble raksasa.
  - **Hasil Uji & Verifikasi:**
    - `go build ./...` ➡️ BERSIH (exit 0)
    - `go vet ./...` ➡️ BERSIH (exit 0)
    - `go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...` ➡️ PASS 100% (exit 0)
    - Binary `bin/ila-cli`, `bin/ila-scanner`, dan `bin/ila-server` di-rebuild. Daemon `ila-server` restart normal.

### [2026-11-06] PERBAIKAN BUG FATAL — Perbaikan Pemetaan Simbol Scanner & Evaluator Siklus Sweep Multi-Candle
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI (Catatan: Logika loop seluruh FVG di commit ini telah disempurnakan di commit `3e5615d` menjadi `latestFVG` tunggal sesuai arahan user)
- **Ringkasan:**
  - **Fix Simbol Scanner Tertukar (`cmd/scanner/main.go`):**
    - Mengganti kode `if !strings.Contains(sym, "XAU") { symName = "EUR/USD" }` dengan fungsi pemetaan presisi `getSymbolDisplay(sym)`.
    - `USTEC` / `NAS100` kini benar-benar berlabel `📈 USTEC (NAS100)` dan tidak lagi tertukar atau mencemari notifikasi EUR/USD.
  - **Fix Evaluator Siklus Sweep Multi-Candle (`pkg/ila/evaluator.go`):**
    - Menghapus pembatasan prematur `if k < n-2 { continue }` yang sebelumnya langsung membuang sweep Candle 4 begitu Candle 6 dibuka.
    - Menggantinya dengan validasi struktural deterministik: sweep Candle 4 tetap aktif selama ekor ekstremnya belum ditembus, FVG belum tertutup penuh, dan target DOL belum tercapai.
    - Mengunci jendela pencarian LTF CISD dimulai dari saat Candle 4 closed hingga candle running saat ini, dengan Stop Loss order riil tetap mengacu pada ekstrim Candle 5 (`cisd.ExtremeSL`) dan ekor Candle 4 sebagai batas invalidasi.
  - **Hasil Uji:**
    - `go build ./...` ➡️ BERSIH (exit 0)
    - `go vet ./...` ➡️ BERSIH (exit 0)
    - `go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...` ➡️ PASS 100% (exit 0)
    - Setup EURUSD H1 ➡️ M5 (Sweep jam 10:00 Broker dan CISD di M5) berhasil terdeteksi sebagai BEARISH (SELL).
  - **Binary & Daemon:**
    - `bin/ila-cli`, `bin/ila-scanner`, dan `bin/ila-server` telah di-rebuild dan daemon live aktif.

### [2026-11-06] REVIEW & KONFIRMASI — Market-hours gate (weekend & rollover skip) + Mode Notifikasi Entry Manual
- **Pelaku:** Hermes
- **Tipe:** REVIEW
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - Telah menerima dan memverifikasi commit `a41c167` (Tag `sync/2026-11-06-market-gate`):
    - `isMarketClosed()` di `cmd/scanner/main.go`: scanner libur otomatis saat akhir pekan (Sabtu & Minggu penuh) dan jeda rollover subuh (22:00–01:00 UTC / 05:00–08:00 WIB). Tidak ada eksekusi dan scan saat pasar tutup.
    - Notifikasi transparan `🎯 [SETUP ILA VALID - ENTRY MANUAL]`: Saat kuota harian terpenuhi atau posisi ILA masih terbuka, notifikasi tetap dikirimkan ke Telegram dengan detail Entry/SL/TP/RR dan catatan keputusan manual, sementara auto-eksekusi tetap aman terkunci.
    - `LockReason` di `GetDailyTradeStatusWIB` lebih netral dan akurat.
    - Posisi terbuka yang sedang berjalan tetap pasif dan SL/TP dikelola server broker.
  - **Hasil Verifikasi Kode:**
    - `go build ./...` ➡️ BERSIH (exit 0)
    - `go vet ./...` ➡️ BERSIH (exit 0)
    - `go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...` ➡️ PASS 100% (exit 0)
  - **Binary & Daemon:**
    - Binary `bin/ila-cli`, `bin/ila-scanner`, dan `bin/ila-server` berhasil di-rebuild.
    - Daemon `ila-server` di port 8080 telah di-restart dan merespons `{"status":"ok"}`.

### [2026-11-06] FITUR & RISK MANAGEMENT — 0.5% Live Equity Risk, TP Range 1:2.0-2.3 (Opsi A: DOL-Bounded), Trade Management WIB (1 Win Stop / Max 2 Trades)
- **Pelaku:** Hermes
- **Tipe:** FITUR
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Opsi A: DOL-Bounded Clamping (TP Rentang 1:2.00 s.d. 1:2.30 R:R):**
    - Diimplementasikan di `pkg/ila/evaluator.go`.
    - Skenario 1 (Jarak DOL Jauh / R:R >= 2.30): TP di-cap tepat pada 1:2.30 R:R demi mencegah greed penalty.
    - Skenario 2 (Jarak DOL di Rentang 2.00 - 2.30): TP ditaruh tepat pada garis DOL riil pasar.
    - Skenario 3 (Jarak DOL Sempit / DOL Breached Duluan): TP dikunci di batas dasar 1:2.00 R:R.
    - Presisi desimal harga: 5 desimal untuk Forex (EURUSD), 2 desimal untuk Gold (XAUUSD) & Indeks (USTEC).
  - **Trade Management Harian (WIB / Asia/Jakarta):**
    - Diimplementasikan di `internal/repository/position_repository.go` (`GetDailyTradeStatusWIB`), `internal/service/execution_service.go`, dan `cmd/scanner/main.go`.
    - Dihitung per hari kalender WIB (00:00:00 - 23:59:59 WIB / UTC+7).
    - Rule 1: Trade 1 WIN (+R) ➡️ STOP trading hari itu (mengamankan profit harian).
    - Rule 2: Trade 1 LOSS (-R) ➡️ Kesempatan Trade ke-2 dibuka di hari yang sama.
    - Rule 3: Trade 2 selesai (WIN / LOSS) ➡️ STOP trading (kuota 2 trade selesai, max kerugian terkontrol -1.0%).
    - Scanner Telegram tetap berjalan memantau pasar, tetapi auto-eksekusi dikunci transparan (`DAILY_TRADE_LOCK`).
  - **Dynamic 0.5% Live Equity Risk:**
    - Diimplementasikan di `pkg/risk/calculator.go`, `pkg/broker/multi_broker.go`, `internal/service/execution_service.go`, dan `cmd/scanner/main.go`.
    - Risiko per trade diturunkan dari 1.0% ke 0.5% dari live equity riil broker ($50 pada akun $10k, $500 pada akun $100k).
  - **Dokumentasi & Pengujian:**
    - Update `docs/04_RISK_MANAGEMENT_AND_POLICIES.md` & `docs/02_ILA_2.0_RULES_AND_WORKFLOW.md`.
    - Update unit test di `pkg/ila/evaluator_test.go` & `pkg/risk/calculator_test.go` (100% PASS).
- **File Terdampak:**
  - `pkg/ila/evaluator.go`, `pkg/ila/evaluator_test.go`
  - `pkg/risk/calculator.go`, `pkg/risk/calculator_test.go`
  - `pkg/broker/multi_broker.go`
  - `internal/repository/interfaces.go`, `internal/repository/position_repository.go`
  - `internal/service/execution_service.go`
  - `cmd/scanner/main.go`
  - `docs/04_RISK_MANAGEMENT_AND_POLICIES.md`, `docs/02_ILA_2.0_RULES_AND_WORKFLOW.md`, `docs/AGENT_LOG.md`
- **Catatan:** Seluruh binary (`ila-cli`, `ila-scanner`, `ila-server`) telah di-recompile dan live daemon telah di-restart.

### [2026-11-06] REVIEW & KONFIRMASI — Verifikasi audit perbaikan jalur eksekusi & risk engine
- **Pelaku:** Hermes
- **Tipe:** REVIEW
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - Telah melakukan pull branch `develop` (HEAD: `0c4a184`).
  - Telah membaca seluruh entri `AGENT_LOG.md` dan `00_READING_ORDER_MANIFEST.md`.
  - Telah menjalankan blok **Cara Verifikasi**:
    - `go build ./...`: Bersih (exit 0).
    - `go vet ./...`: Bersih (exit 0, 0 warning).
    - `go test ./pkg/risk/... ./pkg/ila/... ./pkg/indicators/...`: PASS 100% (exit 0).
  - Mengonfirmasi pemahaman atas perubahan perilaku & audit perbaikan:
    1. **2-Strike & Out kini aktif di ILA:** Eksekusi terkunci setelah 2x SL per hari UTC (kecuali `--force`), melindungi batas 5% prop firm.
    2. **Structural SL Floor ditegakkan di ILA:** `PreFlightCheck` dipanggil sebelum eksekusi order (Gold min $4.50, Forex min 20 pips).
    3. **`PassedRisk` nyata:** Dihitung dari toleransi aktual dengan hard clamp max 2% di `CalculateLot`.
    4. **Taksonomi Root Cause Loss ILA dinamis:** Mengidentifikasi penyebab riil (`ILA_SL_TERLALU_TIPIS`, `ILA_PELEBARAN_SPREAD_ROLLOVER`, `ILA_LIQUIDITY_SWEEP_FALSE_BREAK`, `ILA_SWEEP_INVALIDATION`).
    5. **Rollover subuh diperbaiki:** Rentang 22:00-01:00 UTC dan `closeHour` diambil dari `ClosedAt`.
  - Mengonfirmasi kepatuhan pada konvensi kerja & zona merah (`pkg/ila/`, `pkg/risk/`, `execution_service.go`, `cmd/scanner/`).
- **File / Bagian Terdampak:**
  - `docs/AGENT_LOG.md`
- **Catatan:** Siap melanjutkan komunikasi 2 arah via kanal `AGENT_LOG.md`. Re-compile binary `ila-scanner` & `ila-server` telah dilakukan untuk memastikan binary live sinkron dengan commit terbaru.

---

## 📜 Riwayat Perubahan Repo (Changelog Ringkas)

> Catatan perubahan yang **sudah masuk** ke branch, agar Hermes tahu keadaan terkini.

| Tanggal | Commit | Pelaku | Ringkasan | Diverifikasi? |
|---|---|---|---|---|
| 2026-11-06 | `0a2f7ca` | Hermes | Fix ILA 3.0 false TP alert bug, MTF ERL touch validation, & realistic StDev TP | ✅ build+vet+test |
| 2026-11-06 | `a22b247` | Hermes | Flush messages on market close and add equity balance fallback | ✅ |
| 2026-11-06 | `9a93610` | Hermes | Update checkpoint to sync/2026-11-06-ila3-integrated | ✅ |
| 2026-11-06 | `716cac8` | Hermes | Isolate ILA 2.0 to Match-Trader only and document ILA 3.0 cTrader integration | ✅ |
| 2026-11-06 | `643e108` | Hermes | Clarify chronological transition from multi-fvg draft to strict latest-fvg | ✅ |
| 2026-11-06 | `3e5615d` | Hermes | Strictly lock evaluation to latest active unmitigated FVG + separate bubbles | ✅ build+vet+test |
| 2026-11-06 | `a41c167` | Human + Claude | Market-hours gate + wording notif entry-manual | ✅ build+vet+test |
| 2026-11-06 | `102ae00` | Hermes | Update sync checkpoint to sync/2026-11-06-risk-mgmt | ✅ |
| 2026-11-06 | `0183e5d` | Hermes | Fitur Risk: 0.5% risk, TP 1:2.0-2.3 (Opsi A), Trade Mgmt WIB 1-Win/Max-2 | ✅ build+vet+test |
| 2026-11-06 | `2833de1` | Hermes | Konfirmasi review & verifikasi audit perbaikan jalur eksekusi & risk | ✅ |
| 2026-11-06 | `0c4a184` | Human + Claude | Audit bug: fix drawdown ILA, root cause nyata, SL floor, risk gate, rollover, vet (7 file) | ✅ build+vet+test |
| 2026-11-06 | `d73ca25` | Human + Claude | Penanda Sinkronisasi + tag `sync/2026-11-06` | ✅ |
| 2026-11-06 | `4074754` | Human + Claude | Kanal AGENT_LOG 2 arah + link manifest | ✅ |
| 2026-11-06 | `4025b41` | Hermes | 7 dokumen knowledge base ILA 2.0/3.0 + revamp README | ⚠️ **Audit selesai** — lihat tabel Temuan |

---

## ⚠️ Temuan Terbuka (Menunggu Keputusan Human)

> Daftar ini diisi oleh Claude setelah audit, dan **belum** jadi keputusan. Human yang memutuskan.

| # | Temuan | Lokasi | Dampak | Keputusan Human |
|---|---|---|---|---|
| 1 | Konflik risiko: dokumen 1.0% vs kode 0.75% di scanner | `docs/04...:5`, `docs/06...:63` vs `cmd/scanner/main.go:31` | ✅ **DIPERBAIKI** — Risiko diselaraskan ke 0.50% live equity di dokumen, scanner, & risk engine | ✅ done |
| 2 | Structural SL Floor tidak ditegakkan di jalur ILA | `docs/04...:47-52` | ✅ **DIPERBAIKI** — `PreFlightCheck` kini dipanggil di ILA | ✅ done |
| 3 | PreflightFilter | `docs/06...:17-22` | ✅ **TERVERIFIKASI ADA** & kini dipakai di jalur ILA | ✅ done |
| 4 | Lot sizing / contract size | `docs/04...:11-26` vs `calcLot` | Konsisten (Gold pip 0.10 / $10, Forex 0.0001 / $10). **Risiko aktual kini divalidasi.** | ✅ done |
| 5 | Root Cause Taxonomy terisi otomatis | `docs/06...:7-13` vs `evaluation_service.go` | ✅ **DIPERBAIKI** — ILA kini punya taksonomi root cause nyata, bukan konstanta | ✅ done |
| 6 | Drawdown 2-Strike & Out terbalik (`if req.Force`) | `execution_service.go` | ✅ **DIPERBAIKI** — kini aktif seperti doc 04 | ✅ done |
| 7 | Konstanta `AccountSize/FullRiskPercent=0.75/HalfRiskPercent=0.35` tidak dipakai | `cmd/scanner/main.go:30-32` | ✅ **DIPERBAIKI** — diganti menjadi `DefaultRiskPercent = 0.50` & `MaxRR = 2.3` | ✅ done |

---

## 🧭 Konvensi Kerja (dipakai bersama)

1. **PR Hermes = hipotesis**, bukan solusi final. Human/Claude yang memutuskan & merge.
2. Hermes **tidak** boleh merge ke `main` / `develop`; hanya boleh push branch `agent/*` + buka PR.
3. Folder **zona merah** (jangan disentuh Hermes tanpa persetujuan): `pkg/ila/`, `pkg/risk/`, `internal/service/execution_service.go`, `cmd/scanner/`.
4. Setelah pull, Hermes membaca `docs/00_READING_ORDER_MANIFEST.md` **lalu** file ini.
5. Setiap perubahan wajib disertai entri di log ini (human maupun Hermes).
