# 04. KEBIJAKAN MANAJEMEN RISIKO & PROTOKOL EKSEKUSI (POLICIES)

## 1. Dynamic 1.0% Live Equity Lot Sizing

Ukuran lot tidak pernah ditebak atau menggunakan angka tetap, melainkan dihitung secara matematis tepat **1.0% dari live equity** akun broker saat sinyal tervalidasi:

$$\text{Risk Dollars} = \text{Live Equity} \times 0.01$$

$$\text{Lot Size} = \frac{\text{Risk Dollars}}{\text{Jarak SL} \times \text{Contract Size}}$$

### Parameter Spesifikasi Kontrak Simbol:
1. **XAU/USD (Gold):**
   * Contract Size: `100` troy oz per lot.
   * Nilai Tick: $0.01 = $1.00 per 1.00 lot.
   * Jarak SL $1.00 pada 1.00 lot = risiko $100.
2. **Forex Majors (EUR/USD, GBP/USD, dll):**
   * Contract Size: `100.000` unit base currency.
   * Nilai Pip: 1 pip (0.00010) pada 1.00 lot = $10.00.
3. **Indeks NAS100 / USTEC / US100:**
   * Contract Size: `1` unit per poin.
   * **Volume Step Wajib: `0.10 Lot`** (Minimum 0.10 lot).
   * Rumus Pembulatan: `math.Floor(lot * 10) / 10.0` (mencegah error broker `TRADING_BAD_VOLUME`).

### Batasan Clamp Lot:
* Batas minimal (*floor*): `0.01 Lot` (atau `0.10 Lot` untuk Indeks).
* Batas maksimal (*ceiling*): `20.00 Lot`.

---

## 2. Quality Gate Rasio Risk-to-Reward (Minimal 1:2.0)

* Sistem menolak secara otomatis (*Quality Gate Refusal*) setiap setup yang memiliki rasio Risk to Reward di bawah **1:2.00**.
* Jika jarak TP ke entry lebih kecil dari $2 \times$ jarak SL, order dilarang dibuka.
* **Kecualian DOL Breached:** Jika target eksternal likuiditas (DOL) sudah tertembus lebih dulu oleh lonjakan candle sweep HTF, titik Take Profit otomatis diproyeksikan mengunci rasio **tepat 1:2.00 R:R**.

---

## 3. Proteksi Drawdown Harian (2 Strike & Out Rule)

Demi mengamankan batas maksimal kerugian harian prop firm (**5.0% Max Daily Loss**):
* Jika akun mengalami **2 kali Stop Loss dalam 1 hari kalender UTC**, sistem mengaktifkan status **`DRAWDOWN_LOCK`**.
* Seluruh auto-eksekusi order baru hari itu seketika dikunci rapat.
* Sistem dilarang diam saat penguncian aktif: bot **wajib mengirimkan notifikasi transparansi ke Telegram** bahwa eksekusi tertahan oleh batas proteksi harian.

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
