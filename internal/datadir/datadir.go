// Package datadir menyatukan lokasi berkas data engine agar bisa dipindah
// tanpa mengubah kode, dan menulis state secara atomik.
package datadir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Default adalah lokasi lama, dipakai bila ILA_DATA_DIR tidak diset.
const Default = "/opt/data/ila-engine"

// Akar mengembalikan folder data engine.
//
// Di Docker folder ini dipasang sebagai volume, sehingga path di dalam
// container tidak harus sama dengan path di host.
func Akar() string {
	if v := strings.TrimSpace(os.Getenv("ILA_DATA_DIR")); v != "" {
		return v
	}
	return Default
}

// Berkas menggabungkan nama berkas ke dalam folder data.
func Berkas(nama ...string) string {
	return filepath.Join(append([]string{Akar()}, nama...)...)
}

// TulisJSON menulis data secara atomik: tulis ke berkas sementara di folder
// yang sama, fsync, lalu rename.
//
// Rename dalam satu filesystem bersifat atomik, sehingga proses yang mati di
// tengah penulisan tidak meninggalkan berkas state rusak. State rusak membuat
// engine gagal memuat posisi saat start, jadi ini bukan kerapian belaka.
func TulisJSON(jalur string, v any) error {
	dir := filepath.Dir(jalur)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("buat folder %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("kodekan state: %w", err)
	}

	sementara, err := os.CreateTemp(dir, filepath.Base(jalur)+".*.tmp")
	if err != nil {
		return fmt.Errorf("buat berkas sementara: %w", err)
	}
	namaSementara := sementara.Name()

	// Bersihkan bila gagal di tengah jalan.
	sukses := false
	defer func() {
		if !sukses {
			_ = os.Remove(namaSementara)
		}
	}()

	if _, err := sementara.Write(data); err != nil {
		_ = sementara.Close()
		return fmt.Errorf("tulis state: %w", err)
	}
	if err := sementara.Sync(); err != nil {
		_ = sementara.Close()
		return fmt.Errorf("sinkronkan state: %w", err)
	}
	if err := sementara.Close(); err != nil {
		return fmt.Errorf("tutup berkas sementara: %w", err)
	}
	if err := os.Chmod(namaSementara, 0o644); err != nil {
		return fmt.Errorf("atur izin state: %w", err)
	}
	if err := os.Rename(namaSementara, jalur); err != nil {
		return fmt.Errorf("pasang state: %w", err)
	}

	sukses = true
	return nil
}
