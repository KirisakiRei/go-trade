# 06. SISTEM EVALUASI AI PASCA-TRADE & SELF-LEARNING (POST-MORTEM)

## 1. Engine Evaluasi Pasca-Trade Otomatis (`evaluation_service.go`)

Setiap kali ada posisi yang ditutup oleh server broker (baik menyentuh Take Profit maupun Stop Loss), sistem secara otomatis menjalankan audit teknis pasca-trade (*post-mortem analysis*) dan mencatat hasilnya ke dalam database SQLite:

### Klasifikasi Akar Masalah (*Root Cause Taxonomy*):
1. **Posisi Menang (`HIT_TP`):**
   * **`EKSEKUSI_ILA_PRESISI`:** Konfirmasi sweep FVG di HTF dan CISD body-break-body di LTF valid sempurna, mengantar ekspansi harga menyentuh target DOL.
2. **Posisi Kalah (`HIT_SL`):**
   * **`ILA_SWEEP_INVALIDATION`:** Pergerakan harga menembus titik batas invalidasi (Low/High) Candle 5 sebelum mencapai target likuiditas DOL.
   * **`PELEBARAN_SPREAD_ROLLOVER`:** Posisi aktif saat pergantian sesi New York ke rollover subuh (22:00-00:00 UTC / 05:00-07:00 WIB), di mana pelebaran spread memicu SL secara prematur.
   * **`LIQUIDITY_SWEEP_FALSE_BREAK`:** Harga berbalik cepat sesaat setelah open order, menandakan candle LTF belum cukup matang saat transisi.

---

## 2. Filter AI Pre-Flight (Pre-Execution Safety Gate)

Sebelum membuka order baru, engine eksekusi memanggil `PreflightFilter`:
* Mengecek data statistik historis simbol dan setup terkini.
* Memblokir eksekusi jika simbol sedang berada dalam jam *rollover blackout* subuh atau jika pasar sedang libur akhir pekan (*weekend lock*).
* Menjaga performa prop firm agar tidak terjadi *overtrading* pada kondisi pasar yang tidak menguntungkan.

---

## 3. Protokol Notifikasi Telegram Transparan

### A. Format Bebas LaTeX Math
* **DILARANG:** Sintaks LaTeX math (seperti `$rightarrow$`, `\ge`, atau math block) karena auto-convert di Telegram akan error atau tidak terbaca di HP.
* **WAJIB:** Menggunakan emoji resmi Telegram:
  * ➡️ untuk panah alur timeframe (contoh: `H4 ➡️ M15`)
  * 🎯 untuk rencana entry & target TP
  * 🛑 untuk Stop Loss & status batal
  * ⏳ untuk status menunggu / in-progress
  * 🔴 untuk Bearish (Sell) / 🟢 untuk Bullish (Buy) / ⚪ untuk Netral

### B. Format Tabel ASCII Ringkas & Highlight (`◀`)
Tabel status matriks 4-timeframe didesain ringkas (~40 karakter) agar pas di layar smartphone:

```text
📊 Status ILA 4-Timeframe:
┌───────────┬─────────┬──────────────────────┐
│ Pasangan  │  Arah   │        Status        │
├───────────┼─────────┼──────────────────────┤
│ W -> H4   │ 🔴 Sell │ ⏳ Tunggu Sweep       │
│ D1 -> H1  │ 🔴 Sell │ ⏳ Tunggu Sweep       │
│ H4 -> M15 │ 🟢 Buy  │ 🎯 Siap Entry ◀       │
│ H1 -> M5  │ 🟢 Buy  │ ⏳ Tunggu Sweep       │
└───────────┴─────────┴──────────────────────┘
```
Penanda `◀` secara otomatis disematkan pada baris pasangan timeframe yang baru saja mengalami perubahan status (*state transition*).

### C. Kartu Transparansi Eksekusi Order Real
Ketika order berhasil dibuka serentak ke broker, bot wajib mengirimkan kartu detail lengkap:

```text
🚀 [ORDER REAL ILA - DUAL BROKER] 🟡 XAU/USD • 🟢 BUY
• Akun Eksekusi:
  1. Match-Trader (#1089632)
  2. IC Markets cTrader (#10131354)
• Strategi: ILA (H4 ➡️ M15) | Tiket: Match-Trader (#W...) & cTrader (#692...)
• Entry: 4105.00 | SL: 4098.22 | TP: 4147.00
• R:R: 1:4.72 | Risiko: 1.0% per Akun
⚡ Posisi aktif serentak di Match-Trader & cTrader dengan SL/TP otomatis!
```

### D. Kartu Laporan Closed Position
Ketika posisi selesai ditutup oleh broker:

```text
🎯 [TRADE CLOSED] HIT TAKE PROFIT!
• Tiket: #693069935 (🟡 XAU/USD 🔴 SELL)
• Entry: 4061.97 ➡️ Exit: 4035.44
• Hasil: +2.00 R (+$2.493,82 / +2.4%)
• Saldo Akun: $103.000,80
🧠 Evaluasi & Pembelajaran AI:
• Faktor Kunci: EKSEKUSI_ILA_PRESISI
• Analisa Teknis: Siklus ILA valid sempurna...
• Pelajaran: Pertahankan kedisiplinan konfirmasi candle closed murni di Candle 5.
```
