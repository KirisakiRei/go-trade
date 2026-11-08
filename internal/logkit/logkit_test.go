package logkit

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// jamTetap mengunci waktu agar folder tanggal bisa diperiksa.
func jamTetap(t time.Time) func() time.Time { return func() time.Time { return t } }

func urai(t *testing.T, jalur string) []map[string]any {
	t.Helper()
	f, err := os.Open(jalur)
	if err != nil {
		t.Fatalf("buka %s: %v", jalur, err)
	}
	defer f.Close()

	var hasil []map[string]any
	s := bufio.NewScanner(f)
	for s.Scan() {
		var m map[string]any
		if err := json.Unmarshal(s.Bytes(), &m); err != nil {
			t.Fatalf("baris bukan JSON: %q: %v", s.Text(), err)
		}
		hasil = append(hasil, m)
	}
	return hasil
}

// Berkas harus berada di logs/YYYY/MM/DD/ dan setiap barisnya JSON dengan
// kunci wajib yang konsisten.
func TestStrukturFolderDanKunci(t *testing.T) {
	dir := t.TempDir()
	waktu := time.Date(2026, 11, 8, 10, 15, 3, 0, time.UTC)

	h := Buat(Opsi{Dir: dir, Layanan: "engine", Level: slog.LevelDebug, Jam: jamTetap(waktu)})
	h.Logger.Info("sinyal terbit", "evt", "signal.publish", "symbol", "XAUUSD")
	h.Logger.Debug("detail", "evt", "eval.detail")
	if err := h.Tutup(); err != nil {
		t.Fatal(err)
	}

	jalur := filepath.Join(dir, "2026", "11", "08", "engine-2026-11-08.log")
	baris := urai(t, jalur)
	if len(baris) != 2 {
		t.Fatalf("baris = %d, ingin 2", len(baris))
	}

	b := baris[0]
	if b[KunciLevel] != "INFO" || b[KunciLayan] != "engine" {
		t.Fatalf("level/svc salah: %v", b)
	}
	if b[KunciTrace] != nil {
		t.Fatal("trace tidak boleh ada bila tidak diisi")
	}
	if b[KunciEvent] != "signal.publish" {
		t.Fatalf("evt = %v", b[KunciEvent])
	}
	if _, ok := b[KunciWaktu].(string); !ok {
		t.Fatal("waktu harus string RFC3339")
	}
	if baris[1][KunciLevel] != "DEBUG" {
		t.Fatalf("baris debug hilang: %v", baris[1])
	}
}

// Level di bawah batas tidak boleh ditulis.
func TestLevelDibawahBatasDibuang(t *testing.T) {
	dir := t.TempDir()
	waktu := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

	h := Buat(Opsi{Dir: dir, Layanan: "engine", Level: slog.LevelInfo, Jam: jamTetap(waktu)})
	h.Logger.Debug("tidak boleh muncul")
	h.Logger.Warn("harus muncul")
	_ = h.Tutup()

	baris := urai(t, filepath.Join(dir, "2026", "11", "08", "engine-2026-11-08.log"))
	if len(baris) != 1 || baris[0][KunciLevel] != "WARN" {
		t.Fatalf("baris = %v, ingin satu WARN", baris)
	}
}

// Ganti hari harus membuat berkas baru, bukan menumpuk di berkas lama.
func TestGantiHariBuatBerkasBaru(t *testing.T) {
	dir := t.TempDir()
	kini := time.Date(2026, 11, 8, 23, 59, 0, 0, time.UTC)

	h := Buat(Opsi{Dir: dir, Layanan: "runner", Level: slog.LevelInfo,
		Jam: func() time.Time { return kini }})
	h.Logger.Info("hari pertama")

	kini = kini.Add(24 * time.Hour) // 2026-11-09
	h.Logger.Info("hari kedua")
	_ = h.Tutup()

	satu := urai(t, filepath.Join(dir, "2026", "11", "08", "runner-2026-11-08.log"))
	dua := urai(t, filepath.Join(dir, "2026", "11", "09", "runner-2026-11-09.log"))
	if len(satu) != 1 || len(dua) != 1 {
		t.Fatalf("berkas per hari salah: %d dan %d", len(satu), len(dua))
	}
	if satu[0][KunciPesan] != "hari pertama" || dua[0][KunciPesan] != "hari kedua" {
		t.Fatal("isi berkas tertukar")
	}
}

// Retensi: hari yang lewat masa simpan dihapus, hari lain tetap.
func TestRawatHapusHariKedaluwarsa(t *testing.T) {
	dir := t.TempDir()
	for _, hari := range []string{"2026/10/31", "2026/11/06", "2026/11/07", "2026/11/08"} {
		p := filepath.Join(dir, filepath.FromSlash(hari))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "x.log"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sekarang := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	h := Buat(Opsi{Dir: dir, Layanan: "engine", HariSimpan: 7, Jam: jamTetap(sekarang)})
	if err := h.Rawat(); err != nil {
		t.Fatal(err)
	}

	ada := func(hari string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(hari)))
		return err == nil
	}
	if ada("2026/10/31") {
		t.Fatal("31 Okt (di luar 7 hari) harus terhapus")
	}
	for _, h := range []string{"2026/11/06", "2026/11/07", "2026/11/08"} {
		if !ada(h) {
			t.Fatalf("%s tidak boleh dihapus", h)
		}
	}
}

// Hari yang sudah lewat dikompres dan berkas aslinya hilang.
func TestRawatKompresHariLalu(t *testing.T) {
	dir := t.TempDir()
	lama := filepath.Join(dir, "2026", "11", "07")
	if err := os.MkdirAll(lama, 0o755); err != nil {
		t.Fatal(err)
	}
	jalur := filepath.Join(lama, "engine-2026-11-07.log")
	if err := os.WriteFile(jalur, []byte(`{"msg":"lama"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sekarang := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	h := Buat(Opsi{Dir: dir, Layanan: "engine", HariSimpan: 7, Jam: jamTetap(sekarang)})
	if err := h.Rawat(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(jalur); !os.IsNotExist(err) {
		t.Fatal("berkas asli harus dihapus setelah dikompres")
	}
	if _, err := os.Stat(jalur + ".gz"); err != nil {
		t.Fatalf("berkas .gz harus ada: %v", err)
	}

	// Rawat kedua kali tidak boleh merusak apa pun.
	if err := h.Rawat(); err != nil {
		t.Fatalf("rawat kedua gagal: %v", err)
	}
}

// Batas ukuran mutlak: hari tertua dibuang walau belum melewati masa simpan.
// Berkas .gz tidak dikompres ulang, jadi ukurannya tetap dan jalur penghapusan
// benar-benar teruji.
func TestRawatBatasUkuran(t *testing.T) {
	dir := t.TempDir()
	buat := func(hari, nama string, ukuran int) {
		p := filepath.Join(dir, filepath.FromSlash(hari))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		isi := strings.Repeat("x", ukuran)
		if err := os.WriteFile(filepath.Join(p, nama), []byte(isi), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	buat("2026/11/06", "engine-2026-11-06.log.gz", 400)
	buat("2026/11/07", "engine-2026-11-07.log.gz", 400)
	buat("2026/11/08", "engine-2026-11-08.log.gz", 400)

	sekarang := time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC)
	h := Buat(Opsi{Dir: dir, Layanan: "engine", HariSimpan: 30, BatasBytes: 500,
		Jam: jamTetap(sekarang)})
	if err := h.Rawat(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "2026", "11", "08")); err != nil {
		t.Fatal("hari ini tidak boleh dihapus")
	}
	if _, err := os.Stat(filepath.Join(dir, "2026", "11", "06")); !os.IsNotExist(err) {
		t.Fatal("hari tertua harus dibuang karena batas ukuran")
	}
}
