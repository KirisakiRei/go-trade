// Command ila-engine menjalankan engine sebagai server gRPC yang hidup terus.
//
// Engine hanya mencari setup dan menerbitkannya. Ia tidak mengeksekusi order,
// tidak menghitung lot, dan tidak menyimpan posisi. Semua keputusan uang ada
// di Runner.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log/slog"
	"net"
	"os"
	osignal "os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/engine"
	"ila-engine/internal/grpcserver"
	"ila-engine/internal/logkit"
	"ila-engine/internal/logsync"
	"ila-engine/internal/market"
	"ila-engine/internal/service"
	"ila-engine/pkg/marketfeed"
)

func main() {
	var (
		addr     = flag.String("addr", env("ENGINE_GRPC_ADDR", ":9090"), "alamat dengar gRPC")
		strategy = flag.String("strategy", env("ENGINE_STRATEGY", "ila-2.0"), "strategi: ila-2.0 | ila-3.0")
		symbols  = flag.String("symbols", env("ENGINE_SYMBOLS", ""), "simbol awal, pisahkan koma; kosong berarti menunggu langganan Runner")
		interval = flag.Duration("interval", envDuration("ENGINE_EVAL_INTERVAL", time.Minute), "jeda antar evaluasi")
		certFile = flag.String("tls-cert", env("ENGINE_MTLS_CERT", ""), "sertifikat server (kosong = tanpa TLS)")
		keyFile  = flag.String("tls-key", env("ENGINE_MTLS_KEY", ""), "kunci server")
		caFile   = flag.String("tls-ca", env("ENGINE_MTLS_CA", ""), "CA klien untuk mTLS")
		version  = flag.String("version", env("ENGINE_VERSION", "dev"), "versi engine untuk log & sinyal")
		verbose  = flag.Bool("v", false, "log debug")
		demo     = flag.Bool("demo", false, "evaluator tiruan untuk uji alur sinyal tanpa data pasar")
		logDir   = flag.String("log-dir", env("ENGINE_LOG_DIR", "/opt/data/ila-engine/logs"), "folder log harian; kosong = hanya stdout")
		logHari  = flag.Int("log-hari", envInt("ENGINE_LOG_HARI", 7), "hari penyimpanan log sebelum dihapus")
		logBatas = flag.Int64("log-batas-mb", envInt64("ENGINE_LOG_BATAS_MB", 150), "batas total ukuran log dalam MB")
		logKirim = flag.String("log-kirim", env("ENGINE_LOG_GRPC_ADDR", ""), "alamat platform penerima log; kosong = hanya lokal")
		logCert  = flag.String("log-cert", env("ENGINE_LOG_MTLS_CERT", ""), "sertifikat klien untuk mengirim log")
		logKey   = flag.String("log-key", env("ENGINE_LOG_MTLS_KEY", ""), "kunci klien untuk mengirim log")
		logCA    = flag.String("log-ca", env("ENGINE_LOG_MTLS_CA", ""), "CA platform untuk verifikasi")
	)
	flag.Parse()

	// Pengirim disiapkan lebih dulu karena logger membutuhkan hook salinannya.
	pengirim := logsync.Baru(logsync.Opsi{
		Addr:     *logKirim,
		Layanan:  "engine",
		CertFile: *logCert,
		KeyFile:  *logKey,
		CAFile:   *logCA,
	})

	hasilLog := logkit.Buat(logkit.Opsi{
		Dir:        *logDir,
		Layanan:    "engine",
		Level:      pilihLevel(*verbose),
		JugaStdout: true,
		HariSimpan: *logHari,
		BatasBytes: *logBatas * 1024 * 1024,
		Salin:      pengirim.Kirim,
	})
	defer hasilLog.Tutup()

	logger := hasilLog.Logger
	slog.SetDefault(logger)
	pengirim.SetLog(logger)

	reg := engine.NewRegistry()

	var evaluator engine.Evaluator
	if *demo {
		// Evaluator tiruan: menguji alur engine -> Runner tanpa data pasar.
		pairs := engine.ILA2Pairs
		hasil := map[string]*engine.Result{"H4|M15": engine.DemoResult()}
		if *strategy == "ila-3.0" {
			pairs = []engine.Pair{{HTF: "4H", LTF: "5m"}, {HTF: "D", LTF: "15m"}}
			hasil = map[string]*engine.Result{
				"4H|5m": engine.DemoResult(),
				"D|15m": engine.DemoResult(),
			}
		}
		evaluator = engine.NewUjiEvaluator(*strategy, pairs, hasil)
		logger.Warn("mode demo: sinyal berasal dari evaluator tiruan")
	}

	switch {
	case evaluator != nil:
		// mode demo sudah menentukan evaluator
	case *strategy == "ila-3.0":
		feed := marketfeed.NewMultiFeedProvider()
		evaluator = engine.NewILA3Evaluator(feed)
	default:
		if *strategy != "ila-2.0" {
			logger.Warn("strategi tidak dikenal, memakai ila-2.0", "diminta", *strategy)
		}
		feed := marketfeed.NewMultiFeedProvider()
		evaluator = engine.NewILA2Evaluator(service.NewILA2Service(feed))
	}

	ver := engine.Version{Engine: *version, Strategy: evaluator.StrategyVersion()}
	pub := engine.NewPublisher(evaluator, reg, ver, logger)
	if *demo {
		// Mode demo sengaja melewati jam pasar agar alur bisa diuji kapan saja.
		pub = pub.AbaikanJamPasar()
	}

	// Simbol dari konfigurasi dijadikan pelanggan tetap agar engine bisa
	// dipakai tanpa Runner selama pengujian.
	if awal := splitCSV(*symbols); len(awal) > 0 {
		reg.Add(engine.Subscriber{
			ID: "bootstrap", Symbols: toSet(awal), WantStatus: true,
			Send: func(engine.Signal) {},
		})
		logger.Info("simbol awal dipantau", "symbols", awal)
	}

	srv := grpcserver.New(grpcserver.Deps{
		Registry:  reg,
		Publisher: pub,
		Version:   ver,
		MarketState: func() market.State {
			now := time.Now().UTC()
			return market.StateOf(now.Weekday(), now.Hour())
		},
		Log: logger,
	})

	ctx, stop := osignal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go scanLoop(ctx, pub, *interval, logger)
	go rawatLoop(ctx, hasilLog, logger)
	go pengirim.Jalankan(ctx)

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Error("gagal mendengar", "addr", *addr, "error", err.Error())
		os.Exit(1)
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 10 * time.Minute,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
		}),
	}
	if *certFile != "" && *keyFile != "" {
		creds, err := serverCreds(*certFile, *keyFile, *caFile)
		if err != nil {
			logger.Error("TLS gagal disiapkan", "error", err.Error())
			os.Exit(1)
		}
		opts = append(opts, grpc.Creds(creds))
		logger.Info("mTLS aktif", "ca", *caFile != "")
	} else {
		logger.Warn("TLS tidak aktif: hanya untuk pengembangan lokal")
	}

	g := grpc.NewServer(opts...)
	enginev1.RegisterEngineServiceServer(g, srv)

	go func() {
		<-ctx.Done()
		logger.Info("menghentikan engine")
		g.GracefulStop()
	}()

	logger.Info("engine siap", "addr", *addr, "strategy", ver.Strategy,
		"engine_version", ver.Engine, "interval", interval.String())

	if err := g.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		logger.Error("server berhenti", "error", err.Error())
		os.Exit(1)
	}
}

// scanLoop mengevaluasi secara berkala. Engine sengaja tidak memakai cron:
// proses harus hidup agar bisa mengirim stream ke Runner.
func scanLoop(ctx context.Context, pub *engine.Publisher, interval time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = time.Minute
	}

	pub.Scan(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pub.Scan(ctx)
		}
	}
}

// rawatLoop menjalankan retensi log di awal dan tiap tengah malam.
// Retensi berjalan tanpa memblokir evaluasi sinyal.
func rawatLoop(ctx context.Context, hasil *logkit.Hasil, log *slog.Logger) {
	if hasil.Rawat == nil {
		return
	}
	if err := hasil.Rawat(); err != nil {
		log.Warn("perawatan log gagal", "evt", "log.rawat.gagal", "error", err.Error())
	}

	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := hasil.Rawat(); err != nil {
				log.Warn("perawatan log gagal", "evt", "log.rawat.gagal", "error", err.Error())
			}
		}
	}
}

func serverCreds(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}

	// Tanpa CA, TLS tetap jalan tetapi klien tidak diautentikasi.
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA klien tidak bisa dibaca")
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return credentials.NewTLS(cfg), nil
}

func pilihLevel(verbose bool) slog.Level {
	if verbose {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, it := range items {
		out[it] = struct{}{}
	}
	return out
}
