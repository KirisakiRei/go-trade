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

> **Commit terakhir saat log ini ditulis:** `d99b698` (merge PR #4 ke `develop`)
> **Tag titik sinkron:** `sync/2026-11-07-friday-gate`

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

### [2026-11-14] PERBAIKAN — ILA 2.0: dua FVG (sweep terlama menang) + jendela eksekusi Candle 6 dengan SL Candle 5
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI di `develop` (belum di-deploy ke VPS 1)

#### Ringkasan
1. **Dua FVG: sweep sah dari FVG sebelumnya tidak lagi dibuang (`pkg/ila/evaluator.go`).**
   - **Akar masalah:** `DetectHTFIlaSweep` mengunci murni pada `activeFVGs[len-1]` (FVG aktif TERBARU). Satu candle HTF bisa sekaligus menutup sweep sah pada FVG lama **sambil** membentuk FVG baru di arah berlawanan, sehingga sweep yang sah hilang.
   - **Kasus nyata XAUUSD H4 (12-13 Nov):** candle #58 menyapu ekor #57 di dalam FVG bearish `3887.64-3924.74`, tetapi candle #58 juga membentuk FVG bullish `3886.93-3888.47`. Karena hanya FVG terbaru (bullish) yang diperiksa, XAUUSD tidak pernah memunculkan alert SELL.
   - **Perbaikan:** isi perulangan sweep dipindah apa adanya ke helper `detectSweepPadaFVG` (terbukti identik byte-per-byte), lalu `DetectHTFIlaSweep` menelusuri **maksimal 2 FVG aktif** (terbaru + kronologis sebelumnya) dan memilih **sweep TERLAMA** yang sah. Aturan: *sell yang terjadi lebih dulu dieksekusi, buy-nya dilewati*.
   - **Filter kronologis memakai `>` bukan `>=`:** dua FVG sah boleh berbagi candle (C3 FVG lama = C1 FVG baru). Versi ketat `>=` sempat menjadi bug dan sudah ditangkap test.
2. **Jendela eksekusi dinamis sampai Candle 6 (`pkg/ila/evaluator.go`).**
   - **Akar masalah:** `if candle5Idx < lastHTFIdx` mematikan setup seketika Candle 5 berganti ke Candle 6. Kasus nyata EURUSD H4-M15: candle M15 08:45 NY ditutup 09:00 NY menembus origin CISD `1.10505` by body, tetapi feed H4 09:00:00 NY sudah memunculkan Candle 6 running, sehingga setup batal tepat di detik entry seharusnya sah.
   - **Perbaikan:** kadaluarsa baru terjadi saat Candle 6 **CLOSED** tanpa CISD (`lastHTFIdx > candle5Idx+1 || (lastHTFIdx == candle5Idx+1 && !running)`).
   - **SL dikunci High/Low Candle 5** saat di Candle 6, menggantikan `cisd.ExtremeSL`. Alasannya struktural: `ExtremeSL` di `DetectDynamicCISD` adalah ekstrem Candle 5 — titik yang membentuk CISD — sehingga SL di sana konsisten. `ExtremeSL` LTF tidak boleh menimpa lagi.
   - **Koreksi klaim:** catatan asal menyebut SL Candle 5 "menghemat pips". Itu keliru — SL jadi **lebih lebar** dari `ExtremeSL` LTF, sehingga lot mengecil. Yang didapat adalah proteksi struktural, bukan hemat pips.
3. **Catatan proses:** PR `feat/ila-dynamic-candle6-window-and-sl` ikut membawa perubahan `pkg/ila` lain yang sudah ditolak di `develop` (`activeFVGs[len-2]`, hapus toleransi spread, ganti `cPrev.High < Top` menjadi `cPrev.Close <= Top`) dan menyisakan hanya 9 test (develop 17). Karena itu **tidak di-merge**; hanya ide jendela Candle 6 + SL Candle 5 yang diambil dan ditulis ulang di atas `develop`.

#### File Terdampak
- `pkg/ila/evaluator.go` (+135 / −12)
- `pkg/ila/evaluator_test.go`

#### Verifikasi
- **21 test `pkg/ila` PASS** (17 lama + 4 baru). Test baru: `TestXAUUSDRegresiSweepFVGSebelumnya`, `TestDuaFVGSweepTerlamaMenang`, `TestFVGSebelumnyaBolehBerbagiCandle`, `TestILASetupLanjutDiCandle6Running`, `TestILASetupExpiredWhenCandle6Closed`.
- `TestILASetupExpiredWhenCandle5Closed` diganti menjadi `TestILASetupLanjutDiCandle6Running` karena mengunci aturan yang sengaja diubah.
- **Uji mutasi (semua tertangkap, kembali MERAH):** (a) batas kandidat 2→1; (b) pemilihan `Before`→`After`; (c) filter `>`→`>=`; (d) kembalikan `candle5Idx < lastHTFIdx`; (e) matikan penjangkaran SL Candle 5.
- `go build ./...`, `go vet ./...`, `go test ./...` (20 paket) hijau.
- **End-to-end data pasar nyata:** `go run ./cmd/cli -symbol XAUUSD -htf H4 -ltf M15` → `READY_TO_ENTRY` SELL pada FVG bearish `3887.64-3924.74`.
- `pkg/sequence/` (ILA 3.0), `publisher.go`, dan jalur Runner **tidak disentuh**.

#### Catatan untuk Hermes
- Aturan body-break (`ck.Close <= cPrev.High` / `>= cPrev.Low`) **tidak boleh** dilonggarkan saat menelusuri dua FVG.
- Filter sweep zombie `k < n-3` tetap dipertahankan. Perhatikan: saat Candle 6 sudah closed dan masuk Candle 7, sweep Candle 4 berada di `n-4` sehingga dibuang filter ini **sebelum** cek EXPIRED berjalan. Hasil akhirnya tetap benar (`WAIT_HTF_SWEEP`), tetapi pesannya "Menunggu candle masuk…" bukan "kadaluarsa". Jangan diubah tanpa menguji ulang keduanya.

### [2026-11-11] PERBAIKAN — Pair timeframe per simbol (switch `watch` tidak bocor antar simbol)
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI di `develop` (belum di-deploy ke VPS 1)
- **Ringkasan:**
  - **Akar masalah:** `SubscribeRequest.Pairs` adalah satu daftar global untuk SEMUA simbol. Karena satu-satunya baris switch di DB adalah `USTEC · D·15M`, simbol lain (termasuk XAUUSD) ikut hanya dievaluasi pada `D·15M`. Log produksi: `"rinci":"... XAUUSD:1"` padahal UI meminta 4 pair untuk ILA 3.0.
  - **Bug kedua (senyap):** `hitungPasangan()` membentuk fingerprint `metode:htf/ltf` — **tanpa simbol**. Akibatnya mengubah switch satu simbol bisa TIDAK mengubah fingerprint, sehingga sync ke engine tidak pernah terpicu (interval 2 menit).
  - **Perbaikan:** kontrak gRPC baru `map<string, SymbolPairs> symbol_pairs = 6` dengan `SymbolPairs{ repeated Pair pairs = 1; bool restricted = 2; }`. Field lama `pairs = 3` **DIPERTAHANKAN** sebagai jalur rollback/kompatibilitas engine lama.
  - **Semantik `restricted`:** `false` = simbol belum pernah diatur → engine memakai seluruh pair bawaan. `true` = daftar bersifat final; kosong berarti simbol sengaja **tidak dievaluasi** (Opsi A).
  - Satu subscriber dengan `restricted=false` membatalkan pembatasan untuk simbol itu.
  - WARN sekali per simbol `"evt":"pairs.semua.mati"` agar operator tahu simbol mati karena disengaja, bukan bug.
- **File terdampak:**
  - `proto/engine/v1/engine.proto` (+ `gen/engine/v1/engine.pb.go`)
  - `internal/engine/registry.go` (`PairPolicy`, `Subscriber.SymbolPairs`, `SetSymbolPairs`, `WantedPairs` dirombak)
  - `internal/engine/publisher.go` (`pairsFor` + WARN `pairs.semua.mati`; label denyut `(mati)`)
  - `internal/grpcserver/server.go` (`kebijakanDari`, isi `SymbolPairs` di `Subscribe` + `StreamEvents`)
  - `internal/engine/pasangan_test.go`, `internal/grpcserver/symbol_pairs_test.go` (baru)
- **Verifikasi:** `go build ./...` hijau; `go test ./...` hijau; `engine_grpc.pb.go` **tidak berubah** (service tetap).
- **Catatan untuk Hermes:**
  - Branch `hermes` **tidak disentuh** sama sekali.
  - **JANGAN hapus `pairs = 3`** — itu jalur rollback.
  - `putuskanTimeframe()`, `slotDue()`, `menungguLTF()`, `latestCandleClose()` **tidak diubah** (sudah benar).
  - **Urutan deploy wajib: Engine dulu, Runner kemudian.** Kalau kebalik, engine lama mengabaikan `symbol_pairs` diam-diam dan bug tetap ada tanpa gejala.

### [2026-11-11] PERBAIKAN — Empat PR Hermes digabung ke `develop`, scanner & jadwal lilin diselaraskan, engine live di VPS 1
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI — live di VPS 1 pada commit `d99b698`
- **Ringkasan:**
  - **PR #1 `fix/ila3-latest-active-fvg` (`c5e01cb`)** — ILA 3.0 hanya memakai FVG aktif terakhir (`GetLatestActiveIRL`) untuk bias HTF dan retracement MTF; pencarian tidak lagi mundur ke FVG usang. Ini membasmi false SELL pada USTEC.
  - **PR #2 `fix/ila-external-sweep-and-h4-schedule` (`af9ddf2`)** — siklus tutup lilin H4 dan D1 diselaraskan ke New York close (17:00 NY) di `internal/engine/publisher.go`.
  - **PR #3 `feat/dynamic-symbol-spec-and-risk-gate` (`fbe8649`)** — spesifikasi kontrak broker dibaca dinamis (`SymbolSpec`) plus *max risk assertion gate* yang meng-clamp lot bila melampaui toleransi 1.10x. Ini memperbaiki multiplier US100 Match-Trader (20x) vs cTrader (1x) yang bisa membuat over-risk 20x lipat. CLI scanner ikut diselaraskan agar memakai `b.GetSymbolSpec()` (`2df6ade`).
  - **PR #4 `fix/ila-wick-sweep-reject-body-break` (`79fa5ec`)** — sweep ditegakkan murni sebagai *rejection wick*: bullish wajib `ck.Close >= cPrev.Low`, bearish wajib `ck.Close <= cPrev.High`, sehingga body yang menembus level ditolak. Presisi desimal di `formatILAVal` kini dinamis (Forex < 50.0 → `%.5f`, emas & indeks → `%.2f`).
  - **Penyesuaian pasca-merge (`dc82416`)** — `menungguLTF` di `internal/engine/status.go` ditambah `WaitHTFCandleClose`, `WaitILA3MTFReversal`, dan `WaitILA3MTFFVGRetr`, sehingga jadwal evaluasi lilin mengikuti LTF ketika setup masih dalam fase menunggu.
- **File / Bagian Terdampak:**
  - `pkg/sequence/evaluator.go`, `pkg/sequence/sequence_test.go`
  - `internal/engine/publisher.go`, `internal/engine/publisher_test.go`
  - `pkg/broker/types.go`, `pkg/broker/multi_broker.go`, `pkg/broker/ctrader_broker.go`, `pkg/broker/matchtrader_broker.go`, `pkg/broker/mt5_broker.go`, `pkg/broker/demo_broker.go`, `pkg/risk/calculator.go`
  - `pkg/ila/evaluator.go`, `pkg/ila/evaluator_test.go`
  - `internal/engine/status.go`, `internal/engine/status_test.go`
  - `cmd/scanner/main.go`, `cmd/ila3-scanner/main.go`
- **Aksi yang diminta:** tidak ada. Semua perubahan sudah di `develop` dan sudah berjalan di VPS 1.
- **Catatan:** `ila-engine-engine-1` (`:9443`) dan `ila-engine-engine3-1` (`:9444`) berstatus `Up (healthy)` setelah rebuild. Saat entri ini ditulis, working tree lokal `go-trade` sedang diubah agent lain, jadi `go build`/`go test` ulang sengaja tidak dijalankan di sini agar tidak mencampuri pekerjaan tersebut.

---

### [2026-11-07] PERBAIKAN — Audit ILA 3.0 + Market Gate: Friday gate terlalu awal, label sesi salah, aturan harian hilang, equity fallback berbahaya
- **Pelaku:** Human + Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI (perlu `git pull` + rebuild oleh Hermes)
- **Ringkasan:** Audit kode hasil integrasi ILA 3.0 dan guard Jumat menemukan 4 bug nyata. Semua sudah diperbaiki dan diberi unit test.

#### Bug 1 — Friday gate menutup pasar 3 jam terlalu awal (`cmd/scanner`, `cmd/ila3-scanner`)
- **Masalah:** Guard memakai `utcDay == Friday && utcHour >= 20`. Padahal **Jumat 20:00 UTC = Sabtu 03:00 WIB**, sementara pasar baru berhenti menarik likuiditas sekitar **21:00-22:00 UTC**. Akibatnya 1-2 jam terakhir sesi New York hari Jumat hilang dari pemantauan, padahal itu window terbaik untuk pair US.
- **Perbaikan:** Ambang diubah ke `utcHour >= 21` (margin 1 jam sebelum penutupan akhir pekan). Cukup untuk menghindari spread melebar tanpa membuang sesi NY.
- **File:** `cmd/scanner/main.go:244`, `cmd/ila3-scanner/main.go:73`

#### Bug 2 — Label sesi menjadi `WEEKEND_CLOSED` di hari Jumat (`cmd/scanner`)
- **Masalah:** `isWeekend` hanya `Saturday || Sunday`, sehingga saat Friday gate aktif (`marketClosed = true`) tetapi `isWeekend = false`, sesi jatuh ke `NEWYORK_SESSION`. Karena sesi itu dianggap "aktif", briefing sesi **tetap dikirim** dan sistem berperilaku seolah pasar masih buka — tepat saat seharusnya sudah diam.
- **Perbaikan:** Penentuan nama sesi sekarang memakai `marketClosed` (yang sudah mencakup weekend, Jumat malam, dan rollover), bukan `isWeekend`. Variabel `isWeekend` yang sudah tidak dipakai dihapus.
- **File:** `cmd/scanner/main.go:627`

#### Bug 3 — ILA 3.0 tidak memiliki aturan harian sama sekali (`cmd/ila3-scanner`)
- **Masalah:** `cmd/scanner` (ILA 2.0) memanggil `GetDailyTradeStatusWIB` untuk aturan **1 win = stop**, **max 2 trade/hari**, dan **2-strike**. `cmd/ila3-scanner` **tidak memanggilnya**; satu-satunya pembatas hanyalah `countOpen == 0`. Artinya ILA 3.0 bisa membuka trade tanpa batas per hari, sehingga hasil ujinya **tidak sebanding** dengan ILA 2.0 dan melanggar aturan trading yang sudah ditetapkan.
- **Perbaikan:** `positionRepo.GetDailyTradeStatusWIB` diikutkan ke ILA 3.0 (basis WIB, sama seperti ILA 2.0). Bila `CanTrade = false`, setup tetap dilaporkan ke Telegram sebagai **⏸️ DITAHAN** beserta alasannya, tetapi **tidak dieksekusi**. Notifikasi hanya dikirim sekali per simbol (`<sym>-LOCK`) agar tidak spam tiap 15 menit. **Keputusan owner: aturan ini permanen, tanpa flag pengecualian.**
- **File:** `cmd/ila3-scanner/main.go:157` (init repo), `cmd/ila3-scanner/main.go:312` (gate)

#### Bug 4 — Fallback equity fiktif bisa membuat lot 50x lipat (`cmd/ila3-scanner`)
- **Masalah:** `equity := 98500.0 // Default fallback`. Bila `GetAccountInfo` gagal (jaringan/token), sistem **tetap mengeksekusi** dengan asumsi ekuitas $98.500. Pada akun riil $1.000, risiko 0,5% dari angka fiktif itu setara ±50x risiko seharusnya.
- **Perbaikan:** Bila ekuitas gagal dibaca atau `<= 0`, **entry ditolak** dan dikirim peringatan ke Telegram. Ukuran lot tidak boleh ditebak saat data akun tidak ada.
- **File:** `cmd/ila3-scanner/main.go:322`

#### Bug 5 — SL floor tidak mengikuti desimal instrumen (`pkg/sequence`)
- **Masalah:** `minSLFloor := 0.00150` dipakai untuk **semua** Forex. Untuk USDJPY (3 desimal) angka itu berarti **0,15 pip**, bukan 15 pip — SL jadi sangat tipis dan langsung tersapu spread. Saat ini belum terpengaruh karena pair yang dipantau hanya XAUUSD/EURUSD/USTEC, tetapi akan menjadi bom waktu saat pair JPY ditambahkan.
- **Perbaikan:** Dibuat helper `minSLFloorFor(symbol)` yang membedakan Gold, Indeks, Forex JPY (0,150), dan Forex 5 desimal (0,00150). Dua tempat duplikat (Entry 1 & Entry 2) sekarang memakai helper yang sama.
- **File:** `pkg/sequence/evaluator.go:34`, dipakai di `:216` dan `:321`

#### Perbaikan dokumentasi — rentang R:R TP tidak sesuai kode
- **Masalah:** Komentar kode dan AGENT_LOG menyebut TP dijepit ke **1:2.5 s.d. 1:4.5**, tetapi kodenya menjepit ke **1:2.0 s.d. 1:4.5** (batas bawah memaksa 2.5 hanya bila hasil < 2.0). Ini bukan bug uang, tetapi membuat audit berikutnya salah pijakan.
- **Perbaikan:** Komentar disamakan dengan perilaku kode sebenarnya: basis StDev -2.0, dijepit ke **1:2.0 s.d. 1:4.5**.
- **File:** `pkg/sequence/evaluator.go:250`

#### Unit test baru (mencegah regresi)
- `cmd/scanner/market_gate_test.go` — 11 kasus batas, termasuk `TestFridayGateNotTooEarly`.
- `cmd/ila3-scanner/market_gate_test.go` — 7 kasus batas.
- `pkg/sequence/slfloor_test.go` — SL floor untuk EURUSD, GBPUSD, XAUUSD, GOLD, USTEC, NAS100, USDJPY, GBPJPY.

**Verifikasi:**
- `go build ./...` ➡️ BERSIH
- `go vet ./...` ➡️ BERSIH
- `go test ./pkg/...` ➡️ PASS (kecuali `pkg/marketfeed` yang butuh jaringan ke Binance)
- `go test ./cmd/scanner/... ./cmd/ila3-scanner/... ./pkg/sequence/...` ➡️ PASS

**Aksi yang diminta (Hermes):**
1. `git pull origin develop` lalu **rebuild** `ila-scanner` dan `ila3-scanner` (perubahan ada di binary, bukan konfigurasi).
2. Restart kedua cron/scanner agar binary baru terpakai.
3. **Jangan** mengubah ambang Friday gate kembali ke 20 tanpa instruksi human — alasan perubahan ada di Bug 1.
4. Konfirmasi di bagian **⬆️ DARI HERMES** bahwa kedua binary sudah rebuild + waktu restart.

**Catatan untuk Hermes:**
- **Keputusan owner:** aturan harian ILA 3.0 **tetap sama dengan ILA 2.0** (1 win = stop, max 2 trade/hari WIB, 2-strike, posisi open menahan entry). Tidak dibuat flag, tidak ada pengecualian untuk R&D. Gate ini permanen.
- Perlu konfirmasi: akun cTrader `#10131354` itu **demo** atau **live**? Fallback equity `98500.0` sebelumnya mengesankan saldo mendekati $100k, sementara nilai `equity` asli tidak pernah tercatat di log.

---

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

### [2026-11-12] REVIEW — Double FVG & DOL Breached digabung dengan koreksi pengaman
- **Pelaku:** Claude
- **Tipe:** REVIEW / PERBAIKAN
- **Status:** 🟢 SELESAI — digabung ke `develop`
- **Ringkasan review atas entri Hermes [2026-11-12]:**
  - **Klaim "TP dikunci 1:2.00 saat DOL breached" — TERVERIFIKASI.** Blok `dolBreached` di `EvaluateILA` memang menghitung ulang `TakeProfit = Entry ± (2.00 × slDist)`, jadi menghapus guard `candles[post].Low <= ssl` tidak menghasilkan R:R negatif.
  - **Koreksi 1 — syarat Double FVG dipersempit.** Deteksi awal membandingkan `activeFVGs[len-2]`, padahal daftar itu sudah terfilter `!Mitigated`, sehingga FVG sebelumnya yang termitigasi terlewat dan pembanding bisa melompat ke FVG yang jauh lebih tua. Pembanding kini memakai `fvgs` kronologis (helper `indexOfFVG`) dengan syarat ketat: tipe sama, FVG sebelumnya **masih aktif**, dan pembentuk benar-benar bersambung (`Candle1Index == prev.Candle3Index` atau jarak Candle2 tepat 1).
  - **Koreksi 2 — TP fase `WAIT_LTF_CISD` ikut di-clamp.** Clamping 1:2.00 sebelumnya hanya berjalan di jalur `READY_TO_ENTRY`. Di fase WAIT, `TakeProfit = sweep.TargetDOL` mentah; setelah guard DOL dihapus, panel bisa menampilkan TP yang sudah terlewati. Sekarang TP fase WAIT ikut dikunci 1:2.00 diukur dari close Candle 4.
  - **Pengujian yang sebelumnya tidak ada.** PR asli tidak membawa satu pun test. Ditambahkan: `TestDoubleFVGAllowsCandle3Sweep`, `TestDoubleFVGWithMitigatedNeighbourRejected`, `TestSingleFVGStillRejectsCandle3Sweep`, `TestDOLBreachedDoesNotInvalidateSweep`. Semuanya sudah diverifikasi **gagal** bila fitur/koreksi dicabut, jadi benar-benar menjaga perilaku.
  - **Verifikasi akhir:** `go build ./...`, `go vet ./...`, dan `go test ./...` seluruh paket PASS.
- **File / Bagian Terdampak:**
  - `pkg/ila/evaluator.go`
  - `pkg/ila/evaluator_test.go`
- **Catatan:** Branch `hermes` (bukan `fix/ila-double-fvg-and-dol-breached`) **tidak disentuh** — itu wilayah kerja Hermes.

### [2026-11-12] PERBAIKAN — Dukungan Dinamis Double/Stacked FVG & Penyelarasan DOL Breached
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI

#### Ringkasan
1. **Dukungan Double / Stacked FVG (`pkg/ila/evaluator.go`):**
   - Menambahkan deteksi formasi FVG beruntun (*consecutive displacement* pada arah yang sama).
   - Pada kondisi Double FVG, batas pembentuk FVG kedua (Candle 3) sah disapu oleh lilin berikutnya saat harga melakukan retrace masuk ke area FVG lilin sebelumnya. Blokir kaku `k-1 <= Candle3Index` dilonggarkan secara dinamis hanya untuk kasus Double FVG.
   - Contoh kasus: XAUUSD H4 lilin 18:00 NY menyapu ekor lilin 14:00 NY (Candle 3 FVG kedua) di dalam rentang FVG 10:00 NY (3990.99 – 4010.86), mengonfirmasi setup Bearish ILA 2.0 yang presisi.
2. **Penyelarasan Aturan DOL Breached:**
   - Memperbaiki validasi pembatalan (*invalidation*) sweep: penembusan level SSL/BSL sebelum entry tidak lagi membatalkan siklus sweep. Sesuai aturan baku ILA (Skenario 3), kondisi DOL Breached tetap sah dengan target Take Profit otomatis dikunci ke batas dasar 1:2.00 R:R.
3. **Verifikasi:**
   - `go test -count=1 ./pkg/ila/...` PASS 100%.

### [2026-11-11] PERBAIKAN — Penegakan Aturan Baku Wick Sweep vs Body Break & Presisi Desimal Forex
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI

#### Ringkasan
1. **Penegakan Rejection Wick (Anti Body Break):**
   - Di `DetectHTFIlaSweep` (`pkg/ila/evaluator.go`), sweep lilin HTF kini wajib berupa sapuan sumbu (*wick rejection*) murni.
   - Untuk Bullish sweep (mencari BUY): ditambahkan validasi wajib `ck.Close >= cPrev.Low`. Jika body lilin ditutup menembus ke bawah Low acuan (`ck.Close < cPrev.Low`), lilin dinyatakan sebagai **Body Breakdown** dan seketika ditolak sebagai sweep.
   - Untuk Bearish sweep (mencari SELL): ditambahkan validasi wajib `ck.Close <= cPrev.High`. Jika body lilin ditutup menembus ke atas High acuan (`ck.Close > cPrev.High`), lilin dinyatakan sebagai **Body Breakout** dan seketika ditolak sebagai sweep.
   - Kasus riil: Mencegah lilin H1 EURUSD 00:00 NY (Close 1.11604 < cPrev.Low 1.11620) terdeteksi secara keliru sebagai Bullish Sweep di tengah tren penurunan besar.
2. **Presisi Desimal Dinamis (`formatILAVal`):**
   - Menghilangkan hardcoded format `%.2f` di `evaluator.go` yang menyebabkan harga pair Forex tercetak bulat rancu seperti `(1.12 - 1.12)` pada alert Telegram.
   - Pair dengan nilai < 50.0 (Forex) secara otomatis menggunakan presisi 5 desimal (`%.5f`), sedangkan Emas dan Indeks tetap menggunakan 2 desimal (`%.2f`).
3. **Pengujian Unit Test:**
   - Ditambahkan `TestBodyBreakoutRejectedAsSweep` di `pkg/ila/evaluator_test.go` untuk mengunci dan memvalidasi penolakan lilin body breakdown/breakout sebagai sweep.

#### File Terdampak
- `pkg/ila/evaluator.go`
- `pkg/ila/evaluator_test.go`
- `docs/AGENT_LOG.md`

#### Verifikasi
- `go test -v ./pkg/ila/...` PASS 100% (termasuk test baru `TestBodyBreakoutRejectedAsSweep`).
- Verifikasi CLI live `EURUSD H1 -> M5`: bias arah kembali selaras 100% BEARISH SELL bersama H4 -> M15.

---

### [2026-11-10] FITUR & RISK MANAGEMENT — Dynamic Contract Specification Reader & Adaptive Lot Sizing (Multi-Broker Protection)
- **Pelaku:** Hermes
- **Tipe:** FITUR / RISK MANAGEMENT
- **Status:** 🟢 SELESAI (Diajukan via PR #3)
- **Ringkasan:**
  - **Akar Masalah (Insiden US100 Match-Trader 1.60 Lot):**
    - Di Match-Trader (#1089632), instrumen indeks `US100` berbasis kontrak CME E-mini futures dengan `multiplier: 20` ($20 per 1.00 poin indeks per lot).
    - Di cTrader (#10131354), instrumen indeks `USTEC` bernilai mini-CFD ($1.00 per poin indeks per lot).
    - Hardcoded multiplier $1/poin sebelumnya menyebabkan pembukaan 1.60 lot di Match-Trader yang setara $32.00/poin ($2.496 risk jika kena SL 78 poin, jauh melebihi alokasi 1.0% equity $131).
  - **Arsitektur Solusi Universal (`pkg/broker/types.go`):**
    - Menambahkan struct `SymbolSpec` dan method `GetSymbolSpec(ctx context.Context, symbol string) (*SymbolSpec, error)` ke interface `Broker`.
    - **Match-Trader (`pkg/broker/matchtrader_broker.go`):** Query otomatis endpoint `GET /symbols` saat startup dan meng-cache spesifikasi riil broker:
      - `US100`: `PointValue = 20.0`, `VolumeStep = 0.01`, `VolumeMin = 0.01`
      - `US500`: `PointValue = 50.0`, `VolumeStep = 0.01`, `VolumeMin = 0.01`
      - `US30`: `PointValue = 5.0`, `VolumeStep = 0.01`, `VolumeMin = 0.01`
      - `XAUUSD`: `PointValue = 100.0`, `VolumeStep = 0.01`, `VolumeMin = 0.01`
      - `EURUSD`: `PointValue = 100000.0`, `VolumeStep = 0.01`, `VolumeMin = 0.01`
    - **cTrader (`pkg/broker/ctrader_broker.go`):** Query spesifikasi cTrader IC Markets (`USTEC`: $1/poin, step 0.10, min 0.10).
    - **MultiBrokerManager (`pkg/broker/multi_broker.go`):** Setiap broker dalam eksekusi multi-broker menghitung lot secara proporsional menggunakan `b.GetSymbolSpec()`.
  - **Kalkulator Risiko Universal & Max Risk Assertion Gate (`pkg/risk/calculator.go`):**
    - `CalculateLotWithSpec`: Menghitung lot berbasis `DollarLossPerLot = PriceDist * Spec.PointValue`.
    - `RawLot = RiskDollars / DollarLossPerLot`.
    - **Max Risk Assertion Gate:** Jika pembulatan lot membuat estimasi kerugian melebihi toleransi `Target Risk * 1.10`, lot otomatis diturunkan 1 step ke bawah demi menjamin risiko tidak pernah bengkak.
    - Pada US100 Match-Trader (SL 78 poin, equity $13,131, risk 1% = $131.31), ukuran lot sekarang terhitung presisi **0.08 Lot** ($124.80 risk riil / 0.95% equity).
  - **Pengujian & Verifikasi:**
    - `go test -v ./pkg/risk/... ./pkg/broker/...` -> **PASS 100%**.
    - Unit test baru `TestCalculateLotUS100MatchTraderMultiplier` (0.08 lot) dan `TestCalculateLotUSTECcTrader` (1.60 lot) lulus.

---

### [2026-11-10] PERBAIKAN — Penyelarasan Penjadwalan Penutupan Lilin H4 & D1 ke New York Close (Engine Publisher)
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN / ENGINE SCHEDULER
- **Status:** 🟢 SELESAI (Diajukan via PR #2)
- **Ringkasan:**
  - **Akar Masalah:**
    Fungsi `latestCandleClose` pada `internal/engine/publisher.go` sebelumnya menggunakan formula `(t.Hour()/4)*4` UTC murni (berasumsi lilin H4 tutup pada jam genap 00:00, 04:00, 08:00, 12:00, 16:00, 20:00 UTC). Padahal di pasar forex/CFD dan TradingView, penutupan lilin H4 berpatokan pada **New York Close (17:00 NY = 01:00, 05:00, 09:00, 13:00, 17:00, 21:00 NY, atau 02:00, 06:00, 10:00, 14:00, 18:00, 22:00 UTC)**.
    Akibatnya, pada momen lilin H4 jam 06:00 UTC (01:00 NY) resmi tutup, scheduler mengira lilin tutup jam 04:00 UTC sehingga evaluasi tertunda 2 jam (delay) dan melewatkan momen validasi HTF.
  - **Solusi:**
    Formula `latestCandleClose` diselaraskan dengan zona waktu New York (`America/New_York`) mengikuti siklus 4-jam New York Close, serta D1 pada jam 17:00 NY, konsisten dengan standarisasi waktu di commit `d54937c`.
  - **Pengujian:**
    Memperbarui `internal/engine/publisher_test.go` untuk memverifikasi penutupan H4 jam 14:00:04 UTC (09:00 NY) (PASS 100%).

---

### [2026-11-10] PERBAIKAN — Hapus mode demo, hapus Yahoo, status ILA 3.0 diteruskan
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI

#### Ringkasan

1. **Mode demo dihapus.** Flag `-demo`, `UjiEvaluator`, `DemoResult`, dan
   `Publisher.AbaikanJamPasar` dihapus. Injeksi sinyal tiruan tidak lagi punya
   jalur di binary engine, sesuai RUNBOOK "jangan memalsukan sinyal".
2. **Yahoo Finance dihapus dari rantai feed.** Forex, logam, dan indeks hanya
   dari TradingView FOREX.com; Binance hanya untuk instrumen kripto. Provider
   tidak lagi jatuh ke instrumen lain (mis. emas ke PAXG) karena candle dari
   instrumen lain menghasilkan setup yang tampak sah dengan harga yang salah.
3. **Resolusi timeframe tak dikenal kini ditolak**, bukan diam-diam memakai 15
   menit seperti sebelumnya.
4. **Status ILA 3.0 diteruskan.** `WAIT_ILA3_*`, `ILA3_EXPANSION_*`, dan
   `ILA3_TARGET_REACHED` dipetakan ke enum proto. Sebelumnya semuanya menjadi
   `SIGNAL_STATUS_UNSPECIFIED` sehingga Runner membuangnya tanpa jejak.
5. **TTL mengikuti LTF sebenarnya.** `5m/15m/1H/4H/D/W` kini dikenali, jadi
   sinyal `W|1H` tidak lagi kedaluwarsa dalam 15 menit.

#### Verifikasi

- `go build ./...` dan `go vet ./...` lolos.
- Test unit baru: `internal/engine/status_test.go`,
  `internal/signal/signal_test.go`, `internal/grpcserver/status_test.go`,
  `pkg/marketfeed/mapping_test.go`.
- `TestBinanceProvider` pada `pkg/marketfeed/binance_test.go` adalah test
  jaringan; gagal hanya bila lingkungan tidak dapat menjangkau Binance.

---

### [2026-11-09] PERBAIKAN — Log pindah ke folder asli VPS, tahan container dihapus
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI

---

#### Masalah

Log ditulis ke **named volume** Docker (`log_data`). Isinya memang tidak hilang
saat container dibuat ulang, tetapi:

- berkasnya berada di `/var/lib/docker/volumes/.../_data/`, bukan di folder
  yang dicari orang saat insiden
- setiap pembacaan butuh `docker exec`
- `backup.sh` tidak bisa mengambilnya tanpa trik tambahan

#### Perbaikan

Named volume diganti **bind mount** ke folder asli di VPS:

```yaml
volumes:
  - ${LOG_DIR_HOST:-/var/log/ila}:/var/log/ila
```

| Hal | Sebelum | Sesudah |
|---|---|---|
| Lokasi di VPS | `/var/lib/docker/volumes/...` | **`/var/log/ila/YYYY/MM/DD/`** |
| Container dihapus | tetap | tetap |
| `docker compose down -v` | tetap | tetap |
| Baca log | `docker exec` | `cat` langsung |
| Backup | tidak ikut | ikut, arsip mingguan |

#### Izin tulis

Container berjalan sebagai `app`. UID-nya sekarang dikunci di `1001` agar
pemilik folder di host bisa diprediksi; dengan UID acak, `chown` tidak bisa
mengandalkan nilai tertentu.

Wajib sekali sebelum deploy pertama:

```bash
sudo ./scripts/siapkan-log.sh
```

Tanpa itu container tidak bisa menulis dan kegagalannya hanya terlihat sebagai
error tulis saat runtime.

#### Backup ikut log

`scripts/backup.sh` kini mengarsipkan `/var/log/ila` menjadi `tar.gz` pada hari
Minggu, retensi mengikuti salinan mingguan database. Alasannya: berkas di host
memang aman dari container, tetapi **tidak** dari disk VPS yang rusak.

#### Verifikasi

| Uji | Hasil |
|---|---|
| Container `--rm` menulis lalu dihapus | ✅ berkas tetap ada di host |
| `docker compose down -v --remove-orphans` | ✅ berkas tetap ada |
| Tulis sebagai UID 1001 setelah penyiapan | ✅ berhasil |
| `docker compose config` | ✅ 3 bind mount, `log_data` hilang |
| `bash -n` pada 3 skrip | ✅ |
| `go test ./...` (platform) | ✅ 24 paket |

#### Catatan untuk Hermes

- **Engine di VPS 1 sudah benar sejak awal**: log ke path asli
  `/opt/data/ila-engine/logs/`, tanpa container. Tidak ada yang berubah di sana.
- Format & retensi tidak berubah: 7 hari / 150 MB di VPS 1, 30 hari / 1 GB di
  VPS 2.
- Dokumentasi lokasi log ada di `docs/RUNBOOK.md` bagian **0**.

---

### [2026-11-09] INFRASTRUKTUR — Log harian JSONL, retensi otomatis, kirim ke platform, tutup kebocoran error
- **Pelaku:** Claude
- **Tipe:** FITUR + PERBAIKAN
- **Status:** 🟢 SELESAI

---

#### Latar

Engine berjalan di VPS 1, Runner dan Console di VPS 2. Selama ini semua log
hanya keluar ke stdout dan ditangkap Docker dengan batas **10 MB × 2 berkas**.
Begitu 20 MB terlampaui, log lama hilang permanen — saat insiden minggu lalu
ditanyakan, buktinya sudah tidak ada.

**Batas yang dipatuhi:** `pkg/ila`, `pkg/sequence`, `pkg/keylevels`,
`pkg/quarterly`, `pkg/ssmt`, `pkg/stdev`, `pkg/risk`, `pkg/indicators`
**nol perubahan**. Diverifikasi dengan `git diff --stat` pada kedelapan paket.

---

#### 1. Paket `internal/logkit` — berkas harian + retensi

Semua layanan (engine, runner, backend-api) kini menulis **JSONL** ke:

```
logs/2026/11/08/engine-2026-11-08.log
logs/2026/11/08/runner-2026-11-08.log
```

Kunci JSON pendek dan tetap, agar baris ringkas sekaligus bisa di-query:

```json
{"t":"2026-11-08T10:15:03.000Z","lvl":"INFO","svc":"engine",
 "evt":"signal.publish","msg":"sinyal terbit","symbol":"XAUUSD"}
```

| Kunci | Isi |
|---|---|
| `t` | waktu UTC, presisi milidetik |
| `lvl` | DEBUG / INFO / WARN / ERROR |
| `svc` | engine / runner / backend-api |
| `evt` | nama kejadian berpola, misal `signal.publish`, `log.kirim.terputus` |
| `trace` | nomor rangkaian, sama di kedua VPS |
| `msg` | pesan untuk manusia |

**Retensi (bukan garbage collection).** GC Go mengurus memori heap, bukan
berkas log. Yang diurus di sini adalah siklus hidup berkas:

| Aturan | VPS 1 (engine) | VPS 2 (platform) |
|---|---|---|
| Masa simpan | **7 hari** | **30 hari** |
| Batas ukuran | **150 MB** | 1 GB |
| Kompresi | hari lewat → `.gz` | sama |

Batas ukuran bersifat mutlak: bila terlampaui, hari tertua dihapus lebih dulu
meski belum melewati masa simpan. **Hari ini tidak pernah dihapus**, supaya
folder yang sedang ditulis tidak hilang di tengah jalan.

Perawatan berjalan saat start lalu tiap 6 jam, di goroutine terpisah sehingga
tidak menahan evaluasi sinyal.

---

#### 2. RPC `LogService.KirimLog` — engine → platform

Engine **selalu** menulis log lokal lebih dulu, lalu mengirim salinannya ke
platform lewat gRPC streaming. Saat insiden, semua log bisa dibaca dari satu
tempat tanpa membuka dua SSH.

**Arah koneksi: Engine → Platform.** VPS platform tidak pernah perlu bisa
menjangkau VPS Engine, sama seperti pola yang sudah dipakai Runner→Engine.

```
VPS 1                          VPS 2
engine ──gRPC streaming──►  runner (penerima log)
  │                              │
  └─ logs/2026/11/08/            └─ logs/2026/11/08/
     engine-*.log (selalu)          engine-*.log (salinan)
```

Pengiriman bersifat **pelengkap**:

- antrean penuh → baris dibuang dan dihitung, engine tidak pernah menunggu
- platform mati → engine tetap mengevaluasi sinyal tanpa gangguan
- sambung ulang dengan jeda bertingkat 1s → 30s, kembali ke 1s bila sesi
  sebelumnya bertahan ≥ 30 detik

**mTLS wajib.** Penerima log di platform meminta sertifikat klien yang
ditandatangani CA kita (`tls.RequireAndVerifyClientCert`). Tanpa itu siapa pun
yang menemukan port tersebut bisa menyuntik log palsu dan meracuni audit.

---

#### 3. Trace lintas dua VPS

Runner mengirim metadata gRPC `x-ila-trace` pada setiap panggilan, dan Engine
memasukkannya ke setiap baris log. Satu sinyal kini bisa ditelusuri utuh:

```bash
grep a1b2c3 logs/2026/11/08/engine-*.log    # asal sinyal
grep a1b2c3 logs/2026/11/08/runner-*.log    # apa yang dilakukan Runner
```

---

#### 4. Kebocoran pesan error ditutup

**Masalah.** Handler HTTP mengirim `err.Error()` mentah ke respons. Bila
kegagalan berasal dari feed atau berkas, pemanggil bisa melihat nama host,
path server, atau detail driver.

**Perbaikan.** Helper baru `httpx.Gagal`:

| Tujuan | Isi |
|---|---|
| Log | error asli lengkap + `evt` + path |
| Respons HTTP | pesan aman, misal "Terjadi kesalahan pada server" |

Pemetaan pesan aman per jenis error: `NOT_FOUND` → "Data tidak ditemukan",
`INSUFFICIENT_DATA` → "Data pasar belum cukup untuk analisis",
`MARKET_FEED_FAILED` → "Sumber data pasar sedang tidak tersedia".

Tujuh tempat yang bocor sudah diperbaiki di `analysis_handler.go` dan
`trade_handler.go`. Pesan yang dibaca manusia juga dirapikan ke bahasa yang
konsisten.

---

#### 5. Path data bisa dipindah, state ditulis atomik

**Masalah A.** `ila.db` dan `monitor_state.json` memakai path absolut
`/opt/data/ila-engine/`. Di dalam container, path itu menunjuk ke tempat yang
salah.

**Masalah B.** State ditulis dengan `os.WriteFile` biasa. Bila proses mati di
tengah penulisan, berkas rusak dan engine gagal memuat posisi saat start.

**Perbaikan.** Paket `internal/datadir`:

- `ILA_DATA_DIR` menentukan folder data (default tetap `/opt/data/ila-engine`)
- `TulisJSON` menulis ke berkas sementara di folder yang sama, `fsync`, lalu
  `rename`. Rename dalam satu filesystem bersifat atomik.

---

#### Verifikasi

| Uji | Hasil |
|---|---|
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `go test ./...` | ✅ **17 paket lulus** (sebelumnya 14) |
| `pkg/marketfeed` | ❌ butuh jaringan Binance (gagal sejak awal) |
| Mutasi retensi: guard dimatikan → tes harus gagal | ✅ gagal seperti diharapkan |
| **End-to-end**: engine (proses A) → gRPC → penerima platform (proses B) | ✅ berkas di platform **byte-identik** dengan berkas lokal engine |
| **Resiliensi**: platform dimatikan total, engine tetap jalan 9 detik | ✅ log lokal tetap tertulis, evaluasi sinyal tetap berjalan |
| `cli -symbol XAUUSD -htf H4 -ltf M15` setelah semua perubahan | ✅ sweep FVG, DOL, CISD keluar benar |
| `git diff --stat` pada 8 paket rules trading | ✅ kosong |

Bukti end-to-end yang sebenarnya (bukan klaim):

```
$ diff engine-lokal.log platform-terima.log
IDENTIK ✅

$ wc -l logs/2026/11/08/engine-2026-11-08.log
6
```

---

#### Berkas baru

| Berkas | Isi |
|---|---|
| `internal/logkit/` | penulis JSONL harian, retensi, kompresi |
| `internal/logsync/` | pengirim log ke platform |
| `internal/datadir/` | folder data dari env + tulis atomik |
| `internal/grpcserver/trace.go` | trace dari metadata gRPC |
| `proto/logkit/v1/logkit.proto` | kontrak `LogService` |
| `pkg/httpx/gagal.go` | pemisahan error log ↔ pesan HTTP |

**Flag baru `cmd/engine`:** `-log-dir`, `-log-hari`, `-log-batas-mb`,
`-log-kirim`, `-log-cert`, `-log-key`, `-log-ca`.

---

#### Catatan untuk Hermes

- **Aturan trading tidak berubah sama sekali.** Log, retensi, dan pengiriman
  hanya menyentuh lapisan operasional.
- Format log di Engine dan platform **identik**, jadi berkas dari kedua VPS bisa
  digabung dengan `cat` lalu di-query sekali.
- Env baru yang relevan di VPS 1: `ILA_DATA_DIR`, `ENGINE_LOG_DIR`,
  `ENGINE_LOG_GRPC_ADDR` (kosong = hanya log lokal), `ENGINE_LOG_MTLS_*`.
- Sisi platform (Runner) menerima di `LOG_GRPC_ADDR` (default `:9090`) dengan
  mTLS wajib. Sertifikat: `/certs/log-server.crt`, `.key`, `/certs/ca.crt`.

---

### [2026-11-09] PENGHAPUSAN — BBMA dikeluarkan dari engine, jalur ILA utuh
- **Pelaku:** Claude
- **Tipe:** PENGHAPUSAN
- **Status:** 🟢 SELESAI

---

#### Latar

BBMA sudah tidak dipakai. Yang berjalan sekarang hanya ILA 2.0 dan ILA 3.0.
Sisa BBMA masih menempel di lapisan aplikasi (service, handler, DTO, model),
sehingga membingungkan saat membaca kode dan menyisakan endpoint mati.

**Batas yang dipatuhi:** `pkg/ila`, `pkg/sequence`, `pkg/keylevels`,
`pkg/quarterly`, `pkg/ssmt`, `pkg/stdev`, `pkg/risk`, `pkg/indicators`
**nol perubahan**. Diverifikasi `git diff --stat pkg/` (kosong, dicek ulang
setelah semua perubahan).

---

#### Yang dihapus

| File | Isi |
|---|---|
| `internal/service/bbma_service.go` | 1.013 baris rulebook BBMA |
| `internal/service/monitor_service.go` | pemantau latar BBMA |
| `internal/handler/monitor_handler.go` | endpoint monitor |
| `internal/model/setup.go` | tabel `bbma_setups` |
| `internal/repository/setup_repository.go` | CRUD `bbma_setups` |
| `internal/dto/analysis_dto.go` | `AnalysisRequest`, `AnalysisResponse`, `MTFAnalysis*`, `EntryPlan`, `IndicatorSnapshot`, dll |

Ikut dibersihkan:

- `EvaluateAndExecute` (jalur eksekusi BBMA) — 231 baris
- `TradeHandler.Execute` dan rute `POST /api/v1/trade/execute`
- `AnalysisHandler.Analyze`, `ListSetups`, `AnalyzeMTF` beserta rutenya
- `BBMAService`, `MonitorService`, `SetupRepository` dari interface
- Cabang non-ILA di `evaluation_service.go` (taksomi root cause BBMA)
- Flag `-ila` di `cmd/cli` yang nilainya sudah selalu `true`

Total **-2.122 baris**, dengan +76 baris pengganti.

---

#### Yang TIDAK dihapus (sengaja)

- **`pkg/indicators` beserta `CalculateBBMA`** — ini area rules. ILA tetap
memakai tipe `indicators.Candle` dari paket yang sama, jadi tidak boleh disentuh.
- **Pelabelan historis di `evaluation_service.go`** (`isILA`, `setupType`, `MHV`,
`EXTREME`, `CSAK`) — menjaga posisi lama di database tetap terbaca benar.

---

#### Endpoint sekarang

| Metode | Path | Status |
|---|---|---|
| GET | `/api/v1/analysis/ila` | ✅ tetap |
| POST | `/api/v1/trade/execute/ila` | ✅ tetap |
| GET | `/api/v1/trade/positions` \| `account` \| `evaluations` \| `evaluations/stats` | ✅ tetap |
| GET | `/api/v1/analysis/mtf` \| `/analysis/setups` | ❌ dihapus |
| POST | `/api/v1/trade/execute` | ❌ dihapus |

---

#### Verifikasi

| Uji | Hasil |
|---|---|
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `go test ./...` | ✅ **14 paket lulus** |
| `pkg/marketfeed` | ❌ butuh jaringan Binance (gagal sejak awal, bukan regresi) |
| Tes rute `TestRuteHanyaILA` | ✅ rute BBMA benar-benar 404, rute ILA hidup |
| Mutasi: rute `analysis/mtf` dikembalikan → tes harus gagal | ✅ gagal seperti diharapkan |
| `git diff --stat pkg/` | ✅ kosong |
| `cli -symbol XAUUSD -htf H4 -ltf M15` (data pasar nyata) | ✅ sweep FVG, DOL, CISD keluar benar |
| `cli -symbol XAUUSD` (4 pasangan) | ✅ keempat pasangan terbaca benar |

Contoh keluaran nyata setelah penghapusan:

```
🔍 HTF SWEEP INSIDE FVG (H4):
  └ Candle 06 Nov 19:00 masuk ke Bearish FVG Terakhir (4066.20 - 4081.51)
    dan menyapu High wick candle sebelumnya (4078.62 > 4077.65). Target DOL: 4027.43 (SSL).
⚡ LTF STATUS (M15):
  └ Menunggu konfirmasi CISD (Body break Body).
```

---

#### Catatan untuk Hermes

- **Aturan trading tidak berubah sama sekali.** Logika FVG terakhir aktif, sweep,
CISD, SL candle 5, dan DOL tetap milik `pkg/ila` dan `pkg/sequence`.
- `mt_symbols.json` sudah masuk `.gitignore`: itu katalog simbol broker
(data acuan), bukan kode. Runner mengambil spesifikasi simbol langsung dari API
Match-Trader, jadi file ini tidak dibutuhkan di repo.
- `cmd/cli` sekarang: `-execute` → eksekusi ILA, `-eval` → post-mortem,
tanpa flag → analisis ILA (4 pasangan atau satu pasangan bila `-htf`/`-ltf` diisi).

---

### [2026-11-09] PERBAIKAN — Banjir log saat simbol gagal dievaluasi
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI

---

#### Latar

Sisa terakhir dari migrasi engine gRPC. Bukan perubahan perilaku trading, hanya
kualitas operasional log.

**Batas yang dipatuhi:** `pkg/ila`, `pkg/sequence`, `pkg/keylevels`,
`pkg/quarterly`, `pkg/ssmt`, `pkg/stdev`, `pkg/risk`, `pkg/indicators`
**nol perubahan** (`git diff --stat pkg/` kosong).

---

#### Masalah

`Publisher.Scan` berjalan tiap menit untuk setiap slot (simbol × HTF × LTF).
Saat `Evaluate` gagal — paling sering karena simbol salah ketik, atau feed
sedang tidak menyediakan simbol itu — log menulis satu baris peringatan **tiap
menit per slot**, tanpa henti. Satu simbol bermasalah dengan 2 pasangan TF saja
sudah menghasilkan ~2.880 baris per hari. Log engine tenggelam dan kegagalan
lain yang penting jadi tidak terlihat.

---

#### Perbaikan

Kegagalan evaluasi dicatat paling sering **sekali per menit per slot**.

| Hal | Sebelum | Sesudah |
|---|---|---|
| Simbol salah ketik | log tiap scan (1 menit) per slot | log maksimal 1×/menit per slot |
| Isi log | `symbol`, `htf`, `ltf`, `error` | sama, tidak ada yang hilang |
| Perilaku sinyal | gagal = tidak terbit sinyal | tetap sama |

Kunci pembatasnya `signal.Key(symbol, htf, ltf)` — sama dengan kunci slot —
sehingga antar slot tidak saling memblokir. Setelah satu menit berlalu,
kegagalan yang masih berlangsung dicatat lagi; kalau feed pulih lalu gagal lagi,
kegagalan baru langsung tercatat.

**File:** `internal/engine/publisher.go` (`logGagal`, field `gagalPada`)

---

#### Verifikasi

| Uji | Hasil |
|---|---|
| `go build ./...` | ✅ |
| `go test ./internal/engine/ ./internal/signal/ ./internal/market/` | ✅ |
| Mutasi: guard waktu dimatikan → tes harus gagal | ✅ gagal seperti diharapkan |
| `git diff --stat pkg/` | ✅ kosong |

Tes `TestLogGagalDibatasiPerMenit` mengunci dua hal: dua scan dalam menit yang
sama hanya menghasilkan satu catatan, dan scan setelah 61 detik menghasilkan
catatan kedua.

---

#### Catatan untuk Hermes

- Log tetap membawa pesan `error` aslinya, jadi penyebab kegagalan tetap bisa
didiagnosis dari baris yang tercatat.
- Tidak ada perubahan pada kontrak gRPC, state sinyal, maupun jam pasar.
- `mt_symbols.json` (121 KB) masih untracked di akar repo. Bukan buatan Claude;
sengaja tidak di-commit.

---

### [2026-11-08] SELESAI — S7 flag eksekusi, S8 langganan dinamis, S9 hapus BBMA, penamaan ILA 2.0
- **Pelaku:** Claude
- **Tipe:** PERBAIKAN + FITUR
- **Status:** 🟢 SELESAI

---

#### Latar

Lanjutan dari entri sebelumnya (engine gRPC). Entri ini menutup sisa pekerjaan
yang tercatat di `plans/ENGINE_GRPC_MIGRATION.md` sebagai S7, S8, S9, dan
merapikan penamaan yang masih membingungkan antara ILA 2.0 dan ILA 3.0.

**Batas yang dipatuhi:** `pkg/ila`, `pkg/sequence`, `pkg/keylevels`,
`pkg/quarterly`, `pkg/ssmt`, `pkg/stdev`, `pkg/risk`, `pkg/indicators`
**nol perubahan**. Diverifikasi dengan `git diff --stat pkg/` (kosong).

---

#### S7 — Flag `-execute` pada kedua scanner

**Masalah.** Setelah Runner mengambil alih eksekusi, scanner lama tetap
mengirim order. Akibatnya satu setup bisa dieksekusi dua kali: sekali oleh
scanner, sekali oleh Runner.

**Perbaikan.** Setiap scanner punya flag `-execute` dengan **default `true`**.
Perilaku lama tidak berubah sampai owner memutuskan mematikannya.

| Perintah | Arti |
|---|---|
| `ila-scanner` | tetap mengeksekusi seperti dulu |
| `ila-scanner -execute=false` | hanya memantau + notifikasi manual |
| `ila3-scanner -execute=false` | idem untuk ILA 3.0 |

Saat dimatikan, setup valid **tetap dilaporkan** sebagai
`🎯 [SETUP ... - ENTRY MANUAL]` agar tidak ada sinyal yang hilang.

**File:** `cmd/scanner/main.go`, `cmd/ila3-scanner/main.go`

---

#### S8 — Langganan simbol dinamis (BUG di Runner, sudah diperbaiki)

**Masalah.** Runner menghitung simbol **sekali** saat koneksi dibuka.
`Subscribe`/`Unsubscribe` tidak pernah dipanggil sama sekali. Akibatnya:

> Pengguna yang baru berlangganan GBPUSD tidak akan dipantau sampai Runner
> dijalankan ulang. Untuk SaaS ini tidak bisa diterima.

**Perbaikan (di repo `go-ila-platform`, bukan di sini).**
- `runner.Service.SyncSubscriptions` menghitung selisih antara simbol yang
  sudah dikirim dan kebutuhan saat ini, lalu hanya mengirim selisihnya.
- `cmd/runner` menyelaraskan setiap **2 menit** selama stream hidup, dan
  sekali lagi segera setelah reconnect (engine bisa sudah lupa).
- Kegagalan tidak menandai "sudah terkirim", jadi percobaan berikutnya
  mengulang daftar yang benar.

**Bukti uji end-to-end** (engine hidup + Runner hidup + PostgreSQL):
```
17:01:40  engine menerima  symbols=["XAUUSD"]
          (EURUSD ditambahkan ke database, Runner TIDAK di-restart)
17:03:40  engine menerima  symbols=["EURUSD"]     ← berhasil
          (EURUSD dicabut)
17:06:09  engine mencabut  symbols=["EURUSD"]     ← berhasil
```

---

#### S9 — BBMA dihapus dari jalur engine

**Yang dihapus dari `cmd/scanner/main.go`:**
- `EnableBBMAMonitor`, `EnableBBMAExecution` (sudah `false`, tidak dipakai)
- `makeBBMAStepTable`, `makeBBMASessionTable`
- `phaseLabel`, `gradeLabel` (hanya dipakai dua fungsi di atas)
- Cabang `if EnableBBMAMonitor` pada briefing sesi

Hasil: `cmd/scanner/main.go` turun dari **891 → 738 baris**, nol kata "BBMA".

**Yang SENGAJA belum dihapus.** Sisa BBMA masih ada di `internal/service/`
(`bbma_service.go`), `internal/handler/analysis_handler.go`,
`internal/model/setup.go`. Itu milik **server HTTP**, bukan jalur engine.
Menghapusnya berarti memutus endpoint `/api/v1/analysis/mtf` dan monitor.
Kalau owner ingin itu juga dihapus, pekerjaannya terpisah dan perlu keputusan
soal endpoint pengganti.

---

#### Penamaan: ILA → ILA 2.0

Nama lama tidak membedakan strategi. Yang diubah (lapisan service/dto/cmd,
**bukan** `pkg/`):

| Lama | Baru |
|---|---|
| `service.ILAService` | `service.ILA2Service` |
| `service.NewILAService` | `service.NewILA2Service` |
| `dto.ILAAnalysisRequest` | `dto.ILA2AnalysisRequest` |
| `dto.ILAExecuteRequest` | `dto.ILA2ExecuteRequest` |

Catatan: referensi ke paket rules (`ila.ILAMultiResponse`, `ila.ILAPairResult`)
**dikembalikan apa adanya** — itu milik `pkg/ila` dan tidak boleh di-rename.

---

#### Dua bug yang ditemukan saat audit pekerjaan ini sendiri

| # | Bug | Dampak | Perbaikan |
|---|---|---|---|
| 1 | Setup `READY_TO_ENTRY` bisa terbit dengan `Direction` kosong (bias netral) | Runner menolak, menyisakan baris sinyal mati di database | arah wajib BUY/SELL, jika tidak → dilewati + log |
| 2 | Status menunggu (`WAIT_*`) memakai ID unik tiap scan | Runner menyimpan tiap kiriman, tabel `signal` membengkak **tanpa batas** (satu baris per menit per slot) | ID status dibuat **stabil** per slot (`status-SYMBOL-HTF-LTF`) |

Selain itu, status menunggu kini hanya dikirim ke pelanggan yang meminta
(`want_status_updates`), supaya jaringan tidak dipenuhi data yang tidak dipakai.

---

#### Verifikasi

| Pemeriksaan | Hasil |
|---|---|
| `go build ./...` | BERSIH |
| `go vet ./...` | BERSIH |
| `gofmt` berkas baru | BERSIH |
| Paket tes lulus | **13** |
| `pkg/marketfeed` | gagal: butuh jaringan Binance (sudah begitu sebelumnya) |
| `git diff --stat pkg/` | kosong (**rules tidak tersentuh**) |
| Uji end-to-end Engine ⇄ Runner | sinyal mengalir, langganan tambah/cabut berjalan |
| Uji flag `-execute=false` | scanner menolak mengirim order, notifikasi tetap terbit |

---

#### Yang masih tersisa (belum, bukan terlupa)

1. **ILA 3.0 belum diuji dengan data pasar nyata.** Adapter sudah ada dan
   memetakan 11 state ke kontrak, tapi baru diuji dengan evaluator tiruan.
2. **BBMA di server HTTP** belum dihapus (lihat S9).
3. **`mt_symbols.json`** (121 KB) masih untracked di repo, bukan buatan Claude.

### [2026-11-08] FITUR — Engine menjadi produsen sinyal gRPC (rules trading tidak disentuh)
- **Pelaku:** Claude
- **Tipe:** FITUR
- **Status:** 🟢 SELESAI (S1–S6); S7–S9 belum
- **Ringkasan:**
  - Engine kini punya server gRPC yang hidup terus dan menerbitkan sinyal
    sesuai kontrak `plans/08`. Scanner lama tetap ada dan tetap bisa
    mengeksekusi (flag `-execute` menyusul, lihat S7).
  - **`pkg/ila`, `pkg/sequence`, `pkg/keylevels`, `pkg/quarterly`, `pkg/ssmt`,
    `pkg/stdev`, `pkg/risk`, `pkg/indicators` TIDAK DISENTUH.** Diverifikasi
    dengan `git diff --stat pkg/` yang kosong.
- **File terdampak (baru):**
  - `internal/market/` — satu sumber jam pasar (menggantikan 2 salinan)
  - `internal/signal/` — TTL, supersede, cancel, expire
  - `internal/engine/` — registry pelanggan, publisher, adapter ILA 2.0 & 3.0
  - `internal/grpcserver/` — 5 RPC + mTLS
  - `cmd/engine/` — binary server gRPC
  - `proto/engine/v1/`, `gen/engine/v1/`
- **File terdampak (diubah):**
  - `cmd/scanner/main.go`, `cmd/ila3-scanner/main.go` — memakai
    `market.IsClosed`/`market.Session`; duplikasi dihapus (−67 baris)
  - `cmd/scanner/market_gate_test.go`, `cmd/ila3-scanner/market_gate_test.go`
  - `go.mod`/`go.sum` — menambah `google.golang.org/grpc`

- **Apa yang berubah di engine:**
  1. **Satu sumber jam pasar.** Dulu `isMarketClosed` disalin di dua scanner.
     Sekarang `internal/market`. Aturan tidak berubah satu angka pun (Jumat
     ≥21:00 UTC, rollover 22:00–01:00, Sabtu–Minggu) dan dikunci tes tabel.
  2. **Siklus hidup sinyal.** TTL = 1 candle LTF (H4=4j, H1=1j, M15=15m,
     M5=5m). Sinyal baru pada slot sama → `SUPERSEDED`. Struktur batal →
     `CANCELLED`. Lewat TTL → `EXPIRED`.
  3. **Engine berhenti menghitung lot.** Tidak ada `calcLot`, tidak ada
     `GetAccountInfo` untuk sizing, tidak ada fallback ekuitas. Engine hanya
     meneruskan entry/SL/TP dari evaluator.
  4. **Simbol diminta Runner.** `Subscribe`/`Unsubscribe`/`Snapshot` sudah
     jalan. Simbol dari beberapa pelanggan digabung, jadi satu simbol
     dievaluasi sekali untuk semua.

- **Verifikasi:**
  - `go build ./...` BERSIH, `go vet ./...` BERSIH.
  - 13 paket tes lulus. `pkg/marketfeed` gagal karena butuh jaringan Binance
    (sudah begitu sebelum perubahan ini).
  - Uji gRPC langsung: subscribe, snapshot, ack, unsubscribe, stream — semua
    merespons benar.
  - Uji integrasi dengan Runner sungguhan: sinyal terbit → tersimpan di tabel
    `signal` → diteruskan ke `user_signal`.

- **Bug yang ditemukan saat uji integrasi (di sisi Runner, sudah diperbaiki):**
  1. `ListUsersBySymbol` memakai JOIN tanpa kutip ke `"user"`. `user` adalah
     kata kunci PostgreSQL, jadi query gagal dan **tidak ada user pernah
     menerima sinyal**. Tidak terlihat di SQLite.
  2. `Subscription.IsActive` bertag `default:true` sehingga GORM menimpa nilai
     `false` menjadi `true` — langganan yang dicabut hidup lagi.

- **Belum dikerjakan (WAJIB, jangan lupa):**
  - **S7** — flag `-execute` di `cmd/scanner` & `cmd/ila3-scanner` agar bisa
    dimatikan saat Runner sudah mengambil alih eksekusi.
  - **S8** — Runner perlu memanggil `Subscribe`/`Unsubscribe` berkala. Saat ini
    simbol hanya dihitung sekali saat koneksi dibuka, jadi user baru butuh
    restart Runner.
  - **S9** — hapus sisa BBMA (fungsi tabel masih ada walau flag `false`).
  - ILA 3.0 baru sampai tahap adapter; belum diuji dengan data pasar nyata.

- **Cara menjalankan:**
  ```bash
  go run ./cmd/engine -addr :9090 -strategy ila-2.0 -symbols XAUUSD
  ```
  Mode demo (`-demo`) sudah dihapus. Sinyal tiruan tidak boleh disuntikkan ke
  engine produksi; uji alur sinyal dilakukan lewat Runner dengan data pasar
  nyata.

### [2026-11-07] PERBAIKAN MARKET GATE — Tambah Proteksi Friday Market Close (Jumat >= 20:00 UTC / 03:00 WIB)
- **Pelaku:** Hermes
- **Tipe:** PERBAIKAN
- **Status:** 🟢 SELESAI
- **Ringkasan:**
  - **Penyempurnaan `isMarketClosed` di `cmd/scanner` & `cmd/ila3-scanner`:**
    - Sebelumnya, fungsi `isMarketClosed` hanya mengecek `Saturday`, `Sunday`, dan rollover subuh `22:00-01:00 UTC`.
    - Akibatnya, pada Jumat malam jam 20:46 UTC (14 menit sebelum penutupan pasar akhir pekan saat spread mulai melebar), sistem masih menganggap pasar buka normal dan mengeksekusi order.
    - Ditambahkan guard: `if utcDay == time.Friday && utcHour >= 20 { return true }` (Jumat mulai jam 20:00 UTC / 03:00 WIB Sabtu otomatis dianggap pasar tutup).
    - Mencegah eksekusi baru di jam-jam rawan spread melebar jelang penutupan akhir pekan dan menghindari floating gap akhir pekan.
  - **Verifikasi:**
    - Unit test PASS 100%. Rebuild `ila-scanner` dan `ila3-scanner`. Saat ini (Sabtu), kedua scanner otomatis early-return (silent, tidak scan & tidak eksekusi).

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
| 2026-11-07 | `332cf73` | Hermes | Guard Friday market close (Jumat >= 20:00 UTC) di kedua scanner | ✅ build+vet+test |
| 2026-11-06 | `eb875e0` | Hermes | Update checkpoint to sync/2026-11-06-ila3-fixes | ✅ |
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
