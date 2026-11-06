# SKILL.md - Imaginary Liquidity Analysis (ILA) Complete Skillset

## 1. Matriks 4 Pasangan Timeframe ILA
1. **Weekly (W) ➡️ H4:** Macro Bias & Swing Draw on Liquidity (DOL)
2. **Daily (D1) ➡️ H1:** Daily Trend & Retracement Sweep
3. **H4 ➡️ M15:** Intraday Primary Setup
4. **H1 ➡️ M5:** Scalping / Precision Trigger

---

## 2. Aturan Baku HTF Sweep
1. **Strict Latest Unmitigated FVG:**
   - Hanya menggunakan Fair Value Gap (FVG) yang paling terakhir terbentuk dari 3 closed candle resmi.
   - FVG Bearish (Low C1 > High C3) ➡️ Fokus cari peluang SELL.
   - FVG Bullish (High C1 < Low C3) ➡️ Fokus cari peluang BUY.
   - **Definisi FVG Termitigasi Penuh (Opsi C):** Jika ada ekor atau body candle yang menusuk hingga melewati batas luar FVG ($\ge$ Top FVG Bearish atau $\le$ Bottom FVG Bullish), maka rongga FVG sudah 100% terisi penuh dan dinyatakan **INVALID / HANGUS**.
2. **Adjacent Candle Sweep Only:**
   - Candle sweep WAJIB hanya menyapu candle persis di sebelahnya (`Candle K` menyapu `Candle K-1`). Dilarang menyapu candle yang melompat.
   - Ekor candle sebelumnya sudah berada di dalam FVG, dan candle sweep menyapu ekor tersebut di dalam FVG tanpa ditutup menembus batas luar FVG.
3. **Penanganan Sweep Beruntun (Consecutive Sweeps):**
   - Jika candle setelah sweep (Candle 5) di HTF malah terus menembus membuat titik Low/High baru dan me-sweep candle sweep sebelumnya, status Candle Sweep otomatis di-reset ke candle terbaru tersebut!
   - Rencana entry di LTF seketika dibatalkan untuk siklus lama.
   - Wajib tunggu candle HTF baru tersebut **100% CLOSED resmi**.

---

## 3. Urutan Validasi LTF & Konfirmasi CISD di Candle Entry
1. **Batas Waktu Murni Candle Entry:**
   - Seluruh anatomi setup eksekusi (titik batas invalidasi Low/High dan garis origin CISD) **WAJIB lahir murni di dalam rentang waktu candle entry saat ini**. Dilarang menyeberang ke candle siklus sebelumnya.
2. **Urutan Langkah di LTF:**
   - **Langkah 1:** Tunggu pembentukan titik batas invalidasi (Low untuk Buy / High untuk Sell) murni di dalam jendela waktu candle entry.
   - **Langkah 2:** Telusuri mundur garis CISD (*Change In State of Delivery*) ke harga **OPEN dari candle lawan PERTAMA pada deretan opposing leg**. Garis ini juga wajib berada di dalam jendela waktu candle entry.
   - **Langkah 3:** Tunggu closed candle LTF menembus garis CISD secara **body-break-body**.
   - **Langkah 4:** Entry dieksekusi di harga pembuka (*open*) candle berikutnya!
3. **Estafet Candle Entry (Candle 5 ➡️ Candle 6):**
   - Jika Candle 5 tidak sweep dan tidak membuat CISD, pencarian berlanjut ke Candle 6.
   - Titik acuan Low/High dan garis CISD di-reset murni mengikuti pergerakan Candle 6.
4. **Batas Invalidation Mutlak:**
   - Jika harga di candle entry menembus/me-sweep ekor candle sweep HTF (Candle 4), setup seketika **BATAL (INVALID)**.

---

## 4. Parameter Eksekusi & Manajemen Risiko
- **Stop Loss:** Diletakkan tepat di titik Low/High candle entry (Low untuk Buy / High untuk Sell).
- **Take Profit:** Target DOL (Swing High/Low HTF).
- **Kunci 1:2.0 R:R Saat DOL Jebol:** Jika target DOL sudah tertembus lebih dulu oleh lonjakan candle sweep HTF, titik TP otomatis dialihkan mengunci rasio **1:2.0 R:R**.
- **Risk Management:** Tepat 1.0% Live Equity per transaksi per broker.
