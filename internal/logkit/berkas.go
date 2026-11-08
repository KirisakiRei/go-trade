package logkit

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// daftarHari mengembalikan folder hari (format 2006/01/02) yang ada, terurut
// menaik sehingga elemen pertama adalah yang tertua.
func daftarHari(dir string) ([]string, error) {
	tahun, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var hari []string
	for _, t := range tahun {
		if !t.IsDir() || len(t.Name()) != 4 {
			continue
		}
		bulan, err := os.ReadDir(filepath.Join(dir, t.Name()))
		if err != nil {
			return nil, err
		}
		for _, b := range bulan {
			if !b.IsDir() || len(b.Name()) != 2 {
				continue
			}
			tanggal, err := os.ReadDir(filepath.Join(dir, t.Name(), b.Name()))
			if err != nil {
				return nil, err
			}
			for _, tg := range tanggal {
				if !tg.IsDir() || len(tg.Name()) != 2 {
					continue
				}
				hari = append(hari, t.Name()+"/"+b.Name()+"/"+tg.Name())
			}
		}
	}

	sort.Strings(hari)
	return hari, nil
}

// ukuranHari menghitung total byte seluruh berkas dalam satu folder hari.
func ukuranHari(dir, hari string) (int64, error) {
	akar := filepath.Join(dir, filepath.FromSlash(hari))
	var total int64
	err := filepath.Walk(akar, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ukuranTotal menghitung ukuran seluruh isi folder log.
func ukuranTotal(dir string) (int64, error) {
	hari, err := daftarHari(dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, h := range hari {
		u, err := ukuranHari(dir, h)
		if err != nil {
			return 0, err
		}
		total += u
	}
	return total, nil
}

// kompresHari meng-gzip setiap berkas .log di folder hari lalu menghapus aslinya.
// Berkas yang sudah .gz dilewati, sehingga aman dipanggil berulang.
func kompresHari(dir, hari string) error {
	akar := filepath.Join(dir, filepath.FromSlash(hari))
	isi, err := os.ReadDir(akar)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, f := range isi {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".log") {
			continue
		}
		if err := gzipBerkas(filepath.Join(akar, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

// gzipBerkas menulis <nama>.gz dari <nama>, lalu menghapus <nama>.
func gzipBerkas(jalur string) error {
	if _, err := os.Stat(jalur + ".gz"); err == nil {
		return nil // sudah pernah dikompres
	}

	sumber, err := os.Open(jalur)
	if err != nil {
		return err
	}
	defer sumber.Close()

	tujuan, err := os.Create(jalur + ".gz")
	if err != nil {
		return err
	}

	penulis := gzip.NewWriter(tujuan)
	if _, err := io.Copy(penulis, sumber); err != nil {
		_ = penulis.Close()
		_ = tujuan.Close()
		return err
	}
	if err := penulis.Close(); err != nil {
		_ = tujuan.Close()
		return err
	}
	if err := tujuan.Close(); err != nil {
		return err
	}

	_ = sumber.Close()
	return os.Remove(jalur)
}
