package logkit

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// hasil adalah pegangan atas logger beserta perawat berkasnya.
type Hasil struct {
	Logger *slog.Logger
	// Tutup harus dipanggil saat proses berhenti agar berkas ter-flush.
	Tutup func() error
	// Rawat menghapus log kedaluwarsa lalu mengompresi hari yang sudah lewat.
	Rawat func() error
}

var hasilMu sync.Mutex

// Buat merangkai logger JSONL harian.
//
// Bila Dir kosong, logger tetap dibuat dan hanya menulis ke stdout, sehingga
// pemanggil tidak perlu bercabang.
func Buat(o Opsi) *Hasil {
	if o.Lokasi == nil {
		o.Lokasi = time.UTC
	}
	if o.Jam == nil {
		o.Jam = time.Now
	}
	if o.HariSimpan <= 0 {
		o.HariSimpan = 7
	}
	if o.Layanan == "" {
		o.Layanan = "app"
	}

	penulis := &tulisBerkas{opts: o}
	var out io.Writer = penulis
	if o.JugaStdout || o.Dir == "" {
		out = io.MultiWriter(os.Stdout, penulis)
	}

	logger := slog.New(&handler{
		out:     out,
		level:   o.Level,
		layanan: o.Layanan,
		salin:   o.Salin,
	})

	return &Hasil{
		Logger: logger,
		Tutup:  penulis.Close,
		Rawat:  func() error { return rawat(o) },
	}
}

// rawat menjalankan retensi: hapus hari terlama, lalu kompres hari yang lewat.
func rawat(o Opsi) error {
	if o.Dir == "" {
		return nil
	}

	hari, err := daftarHari(o.Dir)
	if err != nil {
		return err
	}
	if len(hari) == 0 {
		return nil
	}

	sekarang := o.Jam().In(o.Lokasi)
	hariIni := sekarang.Format("2006/01/02")
	batas := sekarang.AddDate(0, 0, -o.HariSimpan).Format("2006/01/02")

	for _, h := range hari {
		if h >= hariIni {
			continue
		}

		// Kompres hari yang sudah lewat agar hemat ruang.
		if err := kompresHari(o.Dir, h); err != nil {
			return err
		}
	}

	// Buang yang melewati masa simpan, terlama lebih dulu.
	for _, h := range hari {
		if h >= hariIni || h >= batas {
			continue
		}
		if err := os.RemoveAll(filepath.Join(o.Dir, filepath.FromSlash(h))); err != nil {
			return err
		}
	}

	// Batas ukuran berlaku mutlak: hapus hari terlama sampai di bawah batas.
	if o.BatasBytes > 0 {
		for {
			total, err := ukuranTotal(o.Dir)
			if err != nil {
				return err
			}
			if total <= o.BatasBytes {
				break
			}

			hari, err = daftarHari(o.Dir)
			if err != nil {
				return err
			}

			// Cari hari tertua yang bukan hari ini.
			tertua := ""
			for _, h := range hari {
				if h < hariIni {
					tertua = h
					break
				}
			}
			if tertua == "" {
				break // hanya tersisa hari ini; tidak ada lagi yang bisa dibuang
			}

			if err := os.RemoveAll(filepath.Join(o.Dir, filepath.FromSlash(tertua))); err != nil {
				return err
			}
		}
	}

	return nil
}
