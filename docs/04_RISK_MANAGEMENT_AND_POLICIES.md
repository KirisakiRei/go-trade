# 04. KEBIJAKAN MANAJEMEN RISIKO & PROTOKOL EKSEKUSI (POLICIES)

## 1. Dynamic 1.0% Live Equity Lot Sizing & Dynamic Symbol Spec

Ukuran lot tidak pernah ditebak atau menggunakan angka tetap, melainkan dihitung secara matematis tepat **1.0% dari live equity** akun broker saat sinyal tervalidasi dengan membaca spesifikasi kontrak broker secara dinamis:

$$\text{Risk Dollars} = \text{Live Equity} \times 0.01$$

$$\text{Dollar Loss Per Lot} = |\text{Entry} - \text{Stop Loss}| \times \text{PointValue}$$

$$\text{Lot Size} = \text{math.Floor}\left(\frac{\text{Risk Dollars}}{\text{Dollar Loss Per Lot} \times \text{VolumeStep}}\right) \times \text{VolumeStep}$$

### Mengapa Perlu Pembacaan Spesifikasi Dinamis (`GetSymbolSpec`)?
Setiap broker memiliki spesifikasi kontrak instrumen yang berbeda-beda, terutama pada instrumen Indeks:
* **Match-Trader (Prop Firm):** Mengadopsi spesifikasi CME E-mini futures. Simbol `US100` memiliki `multiplier: 20`, sehingga **1.00 lot = $20.00 per poin indeks**.
* **cTrader (IC Markets):** Mengadopsi spesifikasi retail mini-CFD. Simbol `USTEC` bernilai **$1.00 per poin indeks**.
* **TradeLocker & Broker Lain:** Multiplier bisa bervariasi ($1, $5, $10, $20, $50, $100 per poin).

Engine mengimplementasikan method `GetSymbolSpec` pada interface `Broker` yang otomatis meng-query endpoint broker (`GET /symbols` di Match-Trader / Protobuf di cTrader) saat startup, sehingga:
* Pada Match-Trader (SL 78 poin, equity $13k, risk $131), lot dihitung presisi **0.08 lot** ($124.80 risk).
* Pada cTrader (SL 78 poin, equity $98k, risk $985), lot dihitung presisi **12.60 lot** ($982.80 risk).

### Parameter Standar Fallback:
1. **XAU/USD (Gold):**
   * PointValue: `$100.0` ($10/pip atau $100 per $1.00 move per lot).
   * VolumeStep: `0.01 Lot`.
2. **Forex Majors (EUR/USD, GBP/USD, dll):**
   * PointValue: `$100,000.0` (1 pip 0.00010 pada 1.00 lot = $10.00).
   * VolumeStep: `0.01 Lot`.
3. **Indeks NAS100 / USTEC / US100:**
   * Match-Trader: `US100` PointValue = `$20.0`, VolumeStep = `0.01 Lot`.
   * cTrader: `USTEC` PointValue = `$1.0`, VolumeStep = `0.10 Lot`.
4. **Max Risk Assertion Gate:**
   * Jika pembulatan lot membuat estimasi loss melebihi `Target Risk * 1.10` (toleransi 10%), lot otomatis diturunkan 1 step ke bawah demi menjamin risiko tidak pernah bengkak.

### Batasan Clamp Lot:
* Batas minimal (*floor*): `0.01 Lot` (atau `0.10 Lot` untuk Indeks).
* Batas maksimal (*ceiling*): `20.00 Lot`.

---

## 2. Quality Gate Rentang Rasio Risk-to-Reward (1:2.0 s.d. 1:2.3 — Opsi A)

Sistem menggunakan **Opsi A: DOL-Bounded Clamping** untuk mengunci titik Take Profit di rentang optimal **1:2.00 s.d. 1:2.30 R:R**:

* **Skenario 1 (Jarak DOL Jauh / R:R $\ge$ 2.30):**
  * Titik Take Profit **di-CAP (dibatasi) tepat pada 1:2.30 R:R** demi mencegah *greed penalty* (harga berbalik setelah terbang +2R).
* **Skenario 2 (Jarak DOL di Rentang 2.00 s.d. 2.30):**
  * Titik Take Profit diletakkan **tepat pada garis DOL** (misal 1:2.15 R:R), menyelaraskan target dengan tarikan likuiditas riil pasar.
* **Skenario 3 (Jarak DOL Sempit / DOL Breached Duluan):**
  * Jika target DOL sudah tertembus saat sweep HTF atau jaraknya $< 2.00$, titik Take Profit otomatis **dikunci di batas dasar 1:2.00 R:R**.

---

## 3. Trade Management Harian (WIB / Jam Jakarta): 1 Win = Stop, Max 2 Trades

Untuk menegakkan kedisiplinan dan menjaga konsistensi prop firm:
* **Zona Waktu:** Dihitung berdasarkan kalender **WIB (Asia/Jakarta / UTC+7)** dari 00:00:00 hingga 23:59:59 WIB.
* **Maksimal Trade:** **2 kali per hari**.
* **Aturan Keputusan State Machine:**
  1. **Trade 1 WIN (+2.0R s/d +2.3R = +2.0% s/d +2.3%):**
     * ➡️ **SELESAI (STOP TRADING)** untuk hari itu. Amankan profit harian!
  2. **Trade 1 LOSS (-1.0R = -1.0%):**
     * ➡️ Sistem membuka kesempatan untuk **Trade ke-2** di hari yang sama.
     * Jika Trade 2 **WIN (+2.0R = +2.0%)** ➡️ Hari berakhir dengan net profit +1.0%. Eksekusi ditutup.
     * Jika Trade 2 **LOSS (-1.0R = -1.0%)** ➡️ Kuota 2 trade habis (**STOP TRADING**). Hari berakhir dengan risiko terkontrol maksimal -2.0%.
* **Transparansi Notifikasi:**
  * Scanner Telegram **tetap memantau pasar** dan mengirim update radar ILA, tetapi auto-eksekusi order dikunci dengan keterangan transparan:
    * `🎯 [EKSEKUSI DIKUNCI] Target harian tercapai (1x WIN). Profit diamankan!`
    * `🛑 [EKSEKUSI DIKUNCI] Jatah trade harian habis (Max 2 Trades). Kuota selesai!`

---

## 4. Aturan Anti-SL Tipis (Structural SL Floor)

Untuk mencegah posisi tersapu oleh *spread widening* atau volatilitas wajar:
* **Gold (XAU/USD):** Jarak Stop Loss minimal wajib **$4.50 (45 pips)**.
* **Forex (EUR/USD):** Jarak Stop Loss minimal wajib **20 pips (0.00200)**.
* Jika jarak SL struktural di bawah batas minimal ini, eksekusi ditolak dengan alasan SL terlalu tipis.

---

## 5. Kebijakan Eksekusi Pasif Server Broker (Broker-Managed SL/TP)

* **Prinsip Utama:** Begitu market order dibuka, harga Stop Loss dan Take Profit langsung dipasang di server broker cTrader dan Match-Trader.
* **Pantangan Mutlak:** Bot **DILARANG KERAS** melikuidasi / menutup posisi secara sepihak (*market close*) berdasarkan estimasi polling harga lokal atau feed luar (misal lonjakan futures Yahoo `GC=F`).
* Server broker yang mengeksekusi penutupan posisi secara presisi di pasar asli, lalu sistem lokal menyinkronkan statusnya secara pasif.

---

## 6. Standar Presisi Angka Desimal

* **Forex Majors:** Wajib menggunakan format string **5 angka desimal (`%.5f`)** (contoh: `1.12030`, bukan `1.12`).
* **Gold & Indeks:** Wajib menggunakan format string **2 angka desimal (`%.2f`)** (contoh: `4061.97`).
