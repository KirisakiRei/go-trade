// Package logsync mengirim baris log Engine ke platform agar bisa dibaca dari
// satu tempat.
//
// Pengiriman bersifat pelengkap: berkas lokal tetap ditulis lebih dulu. Bila
// platform tidak tersedia, trading tidak boleh terganggu sama sekali.
package logsync

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	logkitv1 "ila-engine/gen/logkit/v1"
)

// Opsi mengatur koneksi ke platform.
type Opsi struct {
	// Addr kosong berarti pengiriman dimatikan.
	Addr     string
	Layanan  string
	CertFile string
	KeyFile  string
	CAFile   string
	// Antrean adalah jumlah baris yang boleh menunggu. Bila penuh, baris
	// dibuang: log tidak boleh memperlambat trading.
	Antrean int
	Log     *slog.Logger
}

// Pengirim menampung antrean log dan mengalirkannya ke platform.
type Pengirim struct {
	opsi  Opsi
	log   *slog.Logger
	antri chan *logkitv1.LogEntry

	mu       sync.Mutex
	tumpah   int64
	terkirim int64
}

// Baru membuat pengirim. Tanpa alamat, pengirim tidak aktif dan Kirim menjadi
// tanpa efek.
func Baru(o Opsi) *Pengirim {
	if o.Antrean <= 0 {
		o.Antrean = 2048
	}
	if o.Layanan == "" {
		o.Layanan = "engine"
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Pengirim{
		opsi:  o,
		log:   o.Log,
		antri: make(chan *logkitv1.LogEntry, o.Antrean),
	}
}

// Aktif menyatakan apakah log akan dikirim ke platform.
func (p *Pengirim) Aktif() bool { return p.opsi.Addr != "" }

// SetLog mengganti logger setelah logger utama terbentuk.
//
// Pemisahan ini perlu karena logger membutuhkan hook pengirim ini, sementara
// pengirim ingin mencatat kegagalannya ke logger yang sama.
func (p *Pengirim) SetLog(log *slog.Logger) {
	if log != nil {
		p.log = log
	}
}

// Kirim menaruh satu baris ke antrean tanpa memblokir.
//
// Bila antrean penuh, baris dibuang dan dihitung. Ini disengaja: kehilangan
// beberapa baris log jauh lebih baik daripada Engine berhenti mengevaluasi.
func (p *Pengirim) Kirim(waktu time.Time, baris []byte) {
	if !p.Aktif() {
		return
	}

	entri := &logkitv1.LogEntry{
		Baris:   append([]byte(nil), baris...),
		Layanan: p.opsi.Layanan,
		Waktu:   waktu.UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	select {
	case p.antri <- entri:
	default:
		p.mu.Lock()
		p.tumpah++
		p.mu.Unlock()
	}
}

// Tumpah mengembalikan jumlah baris yang dibuang karena antrean penuh.
func (p *Pengirim) Tumpah() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tumpah
}

// Jalankan menyambung ke platform dan mengalirkan antrean sampai ctx berakhir.
func (p *Pengirim) Jalankan(ctx context.Context) {
	if !p.Aktif() {
		return
	}

	jeda := time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		mulai := time.Now()
		err := p.sesi(ctx)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			p.log.Warn("pengiriman log terputus",
				"evt", "log.kirim.terputus",
				"addr", p.opsi.Addr, "error", err.Error())

			// Sesi yang bertahan lama dianggap sehat; jeda kembali ke awal.
			if time.Since(mulai) > 30*time.Second {
				jeda = time.Second
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(jeda):
		}
		if jeda < 30*time.Second {
			jeda *= 2
		}
	}
}

// sesi membuka satu aliran dan mengirim sampai gagal atau ctx berakhir.
func (p *Pengirim) sesi(ctx context.Context) error {
	kredensial, err := p.kredensial()
	if err != nil {
		return err
	}

	conn, err := grpc.NewClient(p.opsi.Addr,
		grpc.WithTransportCredentials(kredensial),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("membuat koneksi: %w", err)
	}
	defer func() { _ = conn.Close() }()

	stream, err := logkitv1.NewLogServiceClient(conn).KirimLog(ctx)
	if err != nil {
		return fmt.Errorf("membuka aliran log: %w", err)
	}

	p.log.Info("pengiriman log aktif", "evt", "log.kirim.aktif", "addr", p.opsi.Addr)

	for {
		select {
		case <-ctx.Done():
			_, _ = stream.CloseAndRecv()
			return nil
		case entri := <-p.antri:
			if err := stream.Send(entri); err != nil {
				return fmt.Errorf("mengirim log: %w", err)
			}
			p.mu.Lock()
			p.terkirim++
			p.mu.Unlock()
		}
	}
}

// kredensial memakai mTLS bila sertifikat tersedia, TLS biasa bila hanya CA.
func (p *Pengirim) kredensial() (credentials.TransportCredentials, error) {
	if p.opsi.CertFile == "" || p.opsi.KeyFile == "" {
		// Tanpa sertifikat: hanya untuk pengembangan lokal.
		p.log.Warn("pengiriman log tanpa mTLS", "addr", p.opsi.Addr)
		return insecure.NewCredentials(), nil
	}

	cert, err := tls.LoadX509KeyPair(p.opsi.CertFile, p.opsi.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("memuat sertifikat klien: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if p.opsi.CAFile != "" {
		caPEM, readErr := os.ReadFile(p.opsi.CAFile)
		if readErr != nil {
			return nil, fmt.Errorf("membaca CA: %w", readErr)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("CA tidak bisa dibaca")
		}
		cfg.RootCAs = pool
	}

	return credentials.NewTLS(cfg), nil
}
