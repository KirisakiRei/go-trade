package logsync

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/grpc"

	logkitv1 "ila-engine/gen/logkit/v1"
)

// Kirim harus tidak memblokir walau belum ada sesi berjalan, dan antrean yang
// penuh membuang baris alih-alih memperlambat engine.
func TestKirimTidakMemblokir(t *testing.T) {
	p := Baru(Opsi{Addr: "127.0.0.1:1", Antrean: 2, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})

	selesai := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			p.Kirim(time.Now(), []byte(`{"msg":"uji"}`))
		}
		close(selesai)
	}()

	select {
	case <-selesai:
	case <-time.After(2 * time.Second):
		t.Fatal("Kirim memblokir; log tidak boleh memperlambat engine")
	}

	if p.Tumpah() == 0 {
		t.Fatal("antrean penuh seharusnya mencatat baris yang dibuang")
	}
}

// Tanpa alamat, pengirim tidak aktif dan tidak menyimpan apa pun.
func TestTanpaAlamatTidakAktif(t *testing.T) {
	p := Baru(Opsi{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if p.Aktif() {
		t.Fatal("tanpa alamat pengirim harus mati")
	}
	p.Kirim(time.Now(), []byte(`{"msg":"x"}`))
	if p.Tumpah() != 0 {
		t.Fatal("baris tidak boleh dihitung saat pengirim mati")
	}
}

// Baris yang terkirim harus memakai waktu UTC format milidetik.
func TestFormatWaktu(t *testing.T) {
	p := Baru(Opsi{Addr: "127.0.0.1:1", Antrean: 4,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})

	waktu := time.Date(2026, 11, 8, 10, 15, 3, 0, time.FixedZone("WIB", 7*3600))
	p.Kirim(waktu, []byte(`{"msg":"x"}`))

	select {
	case e := <-p.antri:
		if e.GetWaktu() != "2026-11-08T03:15:03.000Z" {
			t.Fatalf("waktu = %q, ingin 2026-11-08T03:15:03.000Z", e.GetWaktu())
		}
		if e.GetLayanan() != "engine" {
			t.Fatalf("layanan = %q", e.GetLayanan())
		}
	case <-time.After(time.Second):
		t.Fatal("entri tidak masuk antrean")
	}
}

var _ = context.Background
var _ grpc.ClientConnInterface = nil
var _ = logkitv1.LogEntry{}
