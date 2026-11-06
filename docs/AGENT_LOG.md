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

> **Commit terakhir saat log ini ditulis:** `4074754`
> **Tag titik sinkron:** `sync/2026-11-06`

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

*(Belum ada entri. Hermes: silakan tambahkan balasan/konfirmasi di sini.)*

---

## 📜 Riwayat Perubahan Repo (Changelog Ringkas)

> Catatan perubahan yang **sudah masuk** ke branch, agar Hermes tahu keadaan terkini.

| Tanggal | Commit | Pelaku | Ringkasan | Diverifikasi? |
|---|---|---|---|---|
| 2026-11-06 | *(pending)* | Human + Claude | Audit bug: fix drawdown ILA, root cause nyata, SL floor, risk gate, rollover, vet (7 file) | ✅ build+vet+test |
| 2026-11-06 | `d73ca25` | Human + Claude | Penanda Sinkronisasi + tag `sync/2026-11-06` | ✅ |
| 2026-11-06 | `4074754` | Human + Claude | Kanal AGENT_LOG 2 arah + link manifest | ✅ |
| 2026-11-06 | `4025b41` | Hermes | 7 dokumen knowledge base ILA 2.0/3.0 + revamp README | ⚠️ **Audit selesai** — lihat tabel Temuan |

---

## ⚠️ Temuan Terbuka (Menunggu Keputusan Human)

> Daftar ini diisi oleh Claude setelah audit, dan **belum** jadi keputusan. Human yang memutuskan.

| # | Temuan | Lokasi | Dampak | Keputusan Human |
|---|---|---|---|---|
| 1 | Konflik risiko: dokumen 1.0% vs kode 0.75% di scanner | `docs/04...:5`, `docs/06...:63` vs `cmd/scanner/main.go:31` | Konstanta scanner **tidak dipakai** untuk sizing (lot dari equity riil = 1.0%). Tinggal rapikan dokumen/konstanta. | ❓ **BELUM** (cosmetic) |
| 2 | Structural SL Floor tidak ditegakkan di jalur ILA | `docs/04...:47-52` | ✅ **DIPERBAIKI** — `PreFlightCheck` kini dipanggil di ILA | ✅ done |
| 3 | PreflightFilter | `docs/06...:17-22` | ✅ **TERVERIFIKASI ADA** & kini dipakai di jalur ILA | ✅ done |
| 4 | Lot sizing / contract size | `docs/04...:11-26` vs `calcLot` | Konsisten (Gold pip 0.10 / $10, Forex 0.0001 / $10). **Risiko aktual kini divalidasi.** | ✅ done |
| 5 | Root Cause Taxonomy terisi otomatis | `docs/06...:7-13` vs `evaluation_service.go` | ✅ **DIPERBAIKI** — ILA kini punya taksonomi root cause nyata, bukan konstanta | ✅ done |
| 6 | Drawdown 2-Strike & Out terbalik (`if req.Force`) | `execution_service.go` | ✅ **DIPERBAIKI** — kini aktif seperti doc 04 | ✅ done |
| 7 | Konstanta `AccountSize/FullRiskPercent=0.75/HalfRiskPercent=0.35` tidak dipakai | `cmd/scanner/main.go:30-32` | Cosmetic — tidak mempengaruhi sizing. Perlu keputusan dihapus/dipakai. | ❓ **BELUM** |

---

## 🧭 Konvensi Kerja (dipakai bersama)

1. **PR Hermes = hipotesis**, bukan solusi final. Human/Claude yang memutuskan & merge.
2. Hermes **tidak** boleh merge ke `main` / `develop`; hanya boleh push branch `agent/*` + buka PR.
3. Folder **zona merah** (jangan disentuh Hermes tanpa persetujuan): `pkg/ila/`, `pkg/risk/`, `internal/service/execution_service.go`, `cmd/scanner/`.
4. Setelah pull, Hermes membaca `docs/00_READING_ORDER_MANIFEST.md` **lalu** file ini.
5. Setiap perubahan wajib disertai entri di log ini (human maupun Hermes).
