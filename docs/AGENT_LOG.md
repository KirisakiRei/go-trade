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
| 2026-06-XX | `4025b41` | Hermes | Menambah 7 dokumen knowledge base ILA 2.0/3.0 + revamp README (+641/−56) | ⚠️ **Belum** — perlu audit kode↔dokumen |
| 2026-06-17 | `70efa1d` | Hermes | fix(reconciliation): cegah false position closure & duplicate entry spam | ✅ build ok |
| 2026-06-17 | `4e2ca16` | Hermes | fix(broker): integrasi closed positions Match-Trader & sync dual-broker | ✅ build ok |

---

## ⚠️ Temuan Terbuka (Menunggu Keputusan Human)

> Daftar ini diisi oleh Claude setelah audit, dan **belum** jadi keputusan. Human yang memutuskan.

| # | Temuan | Lokasi | Dampak | Keputusan Human |
|---|---|---|---|---|
| 1 | **Konflik risiko: dokumen 1.0% vs kode 0.75%** | `docs/04...:5`, `docs/06...:63` vs `cmd/scanner/main.go:31` | Evaluator akan menandai SEMUA trade live sebagai deviasi (alarm palsu) | ❓ **BELUM** |
| 2 | Dokumen klaim **Structural SL Floor** (Gold $4.50 / Forex 20 pips) — apakah ditegakkan di jalur ILA? | `docs/04...:47-52` vs `pkg/risk/` & `internal/service/execution_service.go` | Potensi aturan "cerita" yang tidak dieksekusi | ❓ **BELUM** |
| 3 | Dokumen klaim **PreflightFilter** | `docs/06...:17-22` vs `internal/service/` | Perlu verifikasi ada/tidak | ❓ **BELUM** |
| 4 | Dokumen klaim **lot sizing / contract size** (Gold 100 oz, dll) | `docs/04...:11-26` vs `calcLot` di `cmd/scanner/main.go` | Perlu verifikasi konsistensi rumus | ❓ **BELUM** |
| 5 | Dokumen klaim **Root Cause Taxonomy** (ILA_SWEEP_INVALIDATION dll) terisi otomatis | `docs/06...:7-13` vs `internal/service/evaluation_service.go` | Kunci untuk menjawab "kenapa SL" oleh Hermes | ❓ **BELUM** |

---

## 🧭 Konvensi Kerja (dipakai bersama)

1. **PR Hermes = hipotesis**, bukan solusi final. Human/Claude yang memutuskan & merge.
2. Hermes **tidak** boleh merge ke `main` / `develop`; hanya boleh push branch `agent/*` + buka PR.
3. Folder **zona merah** (jangan disentuh Hermes tanpa persetujuan): `pkg/ila/`, `pkg/risk/`, `internal/service/execution_service.go`, `cmd/scanner/`.
4. Setelah pull, Hermes membaca `docs/00_READING_ORDER_MANIFEST.md` **lalu** file ini.
5. Setiap perubahan wajib disertai entri di log ini (human maupun Hermes).
