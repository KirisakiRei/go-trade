package datadir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAkarDariEnv(t *testing.T) {
	t.Setenv("ILA_DATA_DIR", "/data/uji")
	if got := Akar(); got != "/data/uji" {
		t.Fatalf("Akar() = %q", got)
	}
	if got := Berkas("ila.db"); got != filepath.Join("/data/uji", "ila.db") {
		t.Fatalf("Berkas() = %q", got)
	}
}

func TestAkarDefault(t *testing.T) {
	t.Setenv("ILA_DATA_DIR", "")
	if got := Akar(); got != Default {
		t.Fatalf("Akar() = %q, ingin %q", got, Default)
	}
}

// TulisJSON harus menghasilkan isi yang benar dan tidak meninggalkan berkas
// sementara di belakang.
func TestTulisJSONAtomik(t *testing.T) {
	dir := t.TempDir()
	jalur := filepath.Join(dir, "state.json")

	awal := map[string]any{"posisi": 3, "simbol": "XAUUSD"}
	if err := TulisJSON(jalur, awal); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(jalur)
	if err != nil {
		t.Fatal(err)
	}
	var hasil map[string]any
	if err := json.Unmarshal(data, &hasil); err != nil {
		t.Fatalf("isi bukan JSON: %v", err)
	}
	if hasil["simbol"] != "XAUUSD" {
		t.Fatalf("isi salah: %v", hasil)
	}

	// Tidak boleh ada .tmp tertinggal.
	isi, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range isi {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Fatalf("berkas sementara tertinggal: %s", f.Name())
		}
	}
}

// Menulis ulang mengganti isi lama sepenuhnya, bukan menambah.
func TestTulisJSONMengganti(t *testing.T) {
	dir := t.TempDir()
	jalur := filepath.Join(dir, "state.json")

	_ = TulisJSON(jalur, map[string]any{"a": 1, "b": 2})
	_ = TulisJSON(jalur, map[string]any{"a": 9})

	data, _ := os.ReadFile(jalur)
	var hasil map[string]any
	_ = json.Unmarshal(data, &hasil)
	if hasil["a"] != float64(9) {
		t.Fatalf("isi tidak terganti: %v", hasil)
	}
	if _, ada := hasil["b"]; ada {
		t.Fatal("kunci lama harus hilang")
	}
}
