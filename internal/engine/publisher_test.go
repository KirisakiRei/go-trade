package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ila-engine/internal/signal"
)

// fakeEvaluator mengembalikan hasil yang bisa diatur per pengujian.
type fakeEvaluator struct {
	strategy string
	pairs    []Pair
	results  map[string]*Result // kunci: htf|ltf
	err      error
}

func (f *fakeEvaluator) StrategyVersion() string { return f.strategy }
func (f *fakeEvaluator) Pairs() []Pair           { return f.pairs }
func (f *fakeEvaluator) Evaluate(symbol, htf, ltf string) (*Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results[htf+"|"+ltf], nil
}

func discards() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func readyResult() *Result {
	return &Result{
		Status: "READY_TO_ENTRY", Direction: "BUY",
		Entry: 4120.5, StopLoss: 4112.3, TakeProfit: 4149.2, RiskReward: 3.5,
		PassedMinRR: true, Summary: "siap",
	}
}

// jam pasar buka: Rabu 10:00 UTC.
var jamBuka = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T, results map[string]*Result, now time.Time) (*Publisher, *Registry, *[]Signal) {
	t.Helper()
	reg := NewRegistry()
	received := &[]Signal{}
	reg.Add(Subscriber{
		ID:      "runner-ila-2.0",
		Symbols: map[string]struct{}{"XAUUSD": {}},
		Send:    func(s Signal) { *received = append(*received, s) },
	})
	eval := &fakeEvaluator{
		strategy: "ila-2.0",
		pairs:    []Pair{{HTF: "H4", LTF: "M15"}},
		results:  results,
	}
	pub := NewPublisher(eval, reg, Version{Engine: "test", Strategy: "ila-2.0"}, discards()).
		WithClock(func() time.Time { return now })
	return pub, reg, received
}

func TestScanMenerbitkanSinyalReady(t *testing.T) {
	pub, _, got := setup(t, map[string]*Result{"H4|M15": readyResult()}, jamBuka)
	pub.Scan(context.Background())

	if len(*got) != 1 {
		t.Fatalf("sinyal terkirim = %d, ingin 1", len(*got))
	}
	s := (*got)[0]
	if s.Action != signal.ActionPublish || s.Status != signal.Ready {
		t.Fatalf("action=%q status=%q, ingin PUBLISH/READY", s.Action, s.Status)
	}
	if s.ExpiresAt.Sub(s.PublishedAt) != 15*time.Minute {
		t.Fatalf("TTL M15 salah: %v", s.ExpiresAt.Sub(s.PublishedAt))
	}
	if s.Entry != 4120.5 || s.StopLoss != 4112.3 {
		t.Fatal("harga dari evaluator harus diteruskan apa adanya")
	}
}

// Sinyal baru pada slot yang sama harus menyatakan sinyal lama SUPERSEDED.
func TestScanKeduaMenggantikanSinyalLama(t *testing.T) {
	pub, _, got := setup(t, map[string]*Result{"H4|M15": readyResult()}, jamBuka)
	pub.Scan(context.Background())
	pub.Scan(context.Background())

	if len(*got) != 3 {
		t.Fatalf("terkirim = %d, ingin 3 (publish, supersede, publish)", len(*got))
	}
	if (*got)[1].Action != signal.ActionSupersede {
		t.Fatalf("aksi kedua = %q, ingin SUPERSEDE", (*got)[1].Action)
	}
	if (*got)[1].ID != (*got)[0].ID {
		t.Fatal("SUPERSEDE harus menunjuk sinyal sebelumnya")
	}
	if (*got)[2].ID == (*got)[0].ID {
		t.Fatal("sinyal baru harus punya ID berbeda")
	}
}

// Setup yang batal setelah terbit harus mengirim CANCEL.
func TestScanMengirimCancelSaatStrukturBatal(t *testing.T) {
	results := map[string]*Result{"H4|M15": readyResult()}
	pub, _, got := setup(t, results, jamBuka)
	pub.Scan(context.Background())

	results["H4|M15"] = &Result{Status: "INVALIDATED"}
	pub.Scan(context.Background())

	last := (*got)[len(*got)-1]
	if last.Action != signal.ActionCancel {
		t.Fatalf("aksi terakhir = %q, ingin CANCEL", last.Action)
	}
	if pub.ActiveCount() != 0 {
		t.Fatal("sinyal harus hilang setelah dibatalkan")
	}
}

// Sinyal yang lewat TTL harus ditarik walau evaluator masih menganggap ready.
func TestSinyalKedaluwarsaDitarik(t *testing.T) {
	now := jamBuka
	pub, _, got := setup(t, map[string]*Result{"H4|M15": readyResult()}, now)
	pub.Scan(context.Background())

	// Maju 16 menit melewati TTL. Pasar tetap buka, tapi setup ditahan.
	results := map[string]*Result{"H4|M15": &Result{Status: "WAIT_LTF_CISD"}}
	pub.eval.(*fakeEvaluator).results = results
	pub.WithClock(func() time.Time { return now.Add(16 * time.Minute) })
	pub.Scan(context.Background())

	last := (*got)[len(*got)-1]
	if last.Action != signal.ActionExpire {
		t.Fatalf("aksi terakhir = %q, ingin EXPIRE", last.Action)
	}
}

// Pasar tutup: tidak ada evaluasi baru, sehingga tidak ada sinyal baru.
func TestPasarTutupTidakMemindai(t *testing.T) {
	sabtu := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	pub, _, got := setup(t, map[string]*Result{"H4|M15": readyResult()}, sabtu)
	pub.Scan(context.Background())

	if len(*got) != 0 {
		t.Fatalf("sinyal terkirim = %d, ingin 0 saat pasar tutup", len(*got))
	}
}

// Tanpa pelanggan tidak ada yang dievaluasi: engine menganggur.
func TestTanpaSubscriberTidakMemindai(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	eval := &fakeEvaluator{
		strategy: "ila-2.0",
		pairs:    []Pair{{HTF: "H4", LTF: "M15"}},
		results:  map[string]*Result{"H4|M15": readyResult()},
	}
	pub := NewPublisher(eval, reg, Version{}, discards())
	reg.Add(Subscriber{ID: "x", Symbols: map[string]struct{}{}})
	_ = calls
	pub.Scan(context.Background())

	if pub.ActiveCount() != 0 {
		t.Fatal("tanpa simbol tidak boleh ada sinyal")
	}
}

// Hanya pelanggan yang memantau simbol itu yang menerima sinyal.
func TestBroadcastHanyaKePemantau(t *testing.T) {
	reg := NewRegistry()
	var keXAU, keEUR int
	reg.Add(Subscriber{ID: "a", Symbols: map[string]struct{}{"XAUUSD": {}},
		Send: func(Signal) { keXAU++ }})
	reg.Add(Subscriber{ID: "b", Symbols: map[string]struct{}{"EURUSD": {}},
		Send: func(Signal) { keEUR++ }})

	// Semua simbol yang diminta memang dievaluasi; yang diuji di sini adalah
	// penyiaran: sinyal satu simbol tidak boleh sampai ke pemantau simbol lain.
	reg.Broadcast(Signal{ID: "sig-1", Symbol: "XAUUSD"})

	if keXAU != 1 || keEUR != 0 {
		t.Fatalf("XAU=%d EUR=%d, ingin 1 dan 0", keXAU, keEUR)
	}
}

// Symbo1 yang diminta dua pelanggan hanya dievaluasi sekali.
func TestSimbolDigabung(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Subscriber{ID: "a", Symbols: map[string]struct{}{"XAUUSD": {}}})
	reg.Add(Subscriber{ID: "b", Symbols: map[string]struct{}{"XAUUSD": {}}})

	syms := reg.Symbols()
	if len(syms) != 1 || syms[0] != "XAUUSD" {
		t.Fatalf("simbol = %v, ingin satu XAUUSD", syms)
	}
}

func TestSnapshotHanyaYangAktif(t *testing.T) {
	pub, _, _ := setup(t, map[string]*Result{"H4|M15": readyResult()}, jamBuka)
	pub.Scan(context.Background())

	snap := pub.Snapshot([]string{"XAUUSD"})
	if len(snap) != 1 || snap[0].Symbol != "XAUUSD" {
		t.Fatalf("snapshot = %v, ingin satu XAUUSD", snap)
	}
	if got := pub.Snapshot([]string{"EURUSD"}); len(got) != 0 {
		t.Fatalf("simbol lain tidak boleh ikut ter-snapshot, dapat %d", len(got))
	}
}

// Evaluator gagal bukan sinyal: tidak boleh terbit apa pun.
func TestEvaluatorGagalTidakMenerbitkan(t *testing.T) {
	now := jamBuka
	pub, _, got := setup(t, nil, now)
	pub.eval.(*fakeEvaluator).err = errBoom
	pub.Scan(context.Background())

	if len(*got) != 0 {
		t.Fatalf("sinyal = %d, ingin 0 saat evaluasi gagal", len(*got))
	}
}

// Kegagalan berulang pada slot yang sama hanya dicatat sekali per menit, agar
// simbol yang salah ketik tidak menenggelamkan log engine.
func TestLogGagalDibatasiPerMenit(t *testing.T) {
	now := jamBuka
	var buf bytes.Buffer
	reg := NewRegistry()
	reg.Add(Subscriber{ID: "a", Symbols: map[string]struct{}{"XAUUSD": {}}})
	eval := &fakeEvaluator{strategy: "ila-2.0", pairs: []Pair{{HTF: "H4", LTF: "M15"}}, err: errBoom}
	pub := NewPublisher(eval, reg, Version{}, slog.New(slog.NewTextHandler(&buf, nil))).
		WithClock(func() time.Time { return now })

	pub.Scan(context.Background())
	pub.Scan(context.Background())
	if n := strings.Count(buf.String(), "evaluasi gagal"); n != 1 {
		t.Fatalf("catatan dalam menit sama = %d, ingin 1", n)
	}

	// Setelah lewat satu menit kegagalan yang masih berlangsung dicatat lagi.
	pub.WithClock(func() time.Time { return now.Add(61 * time.Second) })
	pub.Scan(context.Background())
	if n := strings.Count(buf.String(), "evaluasi gagal"); n != 2 {
		t.Fatalf("catatan setelah lewat menit = %d, ingin 2", n)
	}
}

// Setup ready tanpa harga lengkap tidak boleh terbit.
func TestHargaTidakLengkapDilewati(t *testing.T) {
	r := readyResult()
	r.TakeProfit = 0
	pub, _, got := setup(t, map[string]*Result{"H4|M15": r}, jamBuka)
	pub.Scan(context.Background())

	if len(*got) != 0 {
		t.Fatalf("sinyal = %d, ingin 0 karena harga tidak lengkap", len(*got))
	}
}

var errBoom = errors.New("data pasar gagal diambil")

// Setup ready tanpa arah (bias netral) tidak boleh terbit: Runner akan
// menolaknya dan menyisakan baris sinyal mati.
func TestTanpaArahTidakTerbit(t *testing.T) {
	r := readyResult()
	r.Direction = ""
	pub, _, got := setup(t, map[string]*Result{"H4|M15": r}, jamBuka)
	pub.Scan(context.Background())

	if len(*got) != 0 {
		t.Fatalf("sinyal = %d, ingin 0 karena arah tidak jelas", len(*got))
	}
}

// Status menunggu dikirim untuk dashboard, tanpa harga dan tanpa izin eksekusi.
func TestStatusMenungguDikirimBilaDiminta(t *testing.T) {
	reg := NewRegistry()
	var diterima []Signal
	reg.Add(Subscriber{
		ID: "dash", Symbols: map[string]struct{}{"XAUUSD": {}}, WantStatus: true,
		Send: func(s Signal) { diterima = append(diterima, s) },
	})
	eval := &fakeEvaluator{strategy: "ila-2.0", pairs: []Pair{{HTF: "H4", LTF: "M15"}},
		results: map[string]*Result{"H4|M15": {Status: "WAIT_LTF_CISD"}}}
	pub := NewPublisher(eval, reg, Version{}, discards()).
		WithClock(func() time.Time { return jamBuka })
	pub.Scan(context.Background())

	if len(diterima) != 1 {
		t.Fatalf("status = %d, ingin 1", len(diterima))
	}
	if diterima[0].Status != signal.WaitLTFCISD {
		t.Fatalf("status = %q, ingin WAIT_LTF_CISD", diterima[0].Status)
	}
	if diterima[0].Entry != 0 || diterima[0].StopLoss != 0 {
		t.Fatal("status menunggu tidak boleh membawa harga")
	}
}

// Pelanggan yang tidak minta status tidak dibanjiri status menunggu.
func TestStatusMenungguTidakDikirimBilaTidakDiminta(t *testing.T) {
	reg := NewRegistry()
	var diterima []Signal
	reg.Add(Subscriber{
		ID: "diam", Symbols: map[string]struct{}{"XAUUSD": {}}, WantStatus: false,
		Send: func(s Signal) { diterima = append(diterima, s) },
	})
	eval := &fakeEvaluator{strategy: "ila-2.0", pairs: []Pair{{HTF: "H4", LTF: "M15"}},
		results: map[string]*Result{"H4|M15": {Status: "WAIT_LTF_CISD"}}}
	pub := NewPublisher(eval, reg, Version{}, discards()).
		WithClock(func() time.Time { return jamBuka })
	pub.Scan(context.Background())

	if len(diterima) != 0 {
		t.Fatalf("status = %d, ingin 0", len(diterima))
	}
}

// ID status menunggu harus tetap sama antar scan agar Runner tidak menyimpan
// baris baru setiap menit.
func TestIDStatusStabil(t *testing.T) {
	reg := NewRegistry()
	var id []string
	reg.Add(Subscriber{
		ID: "dash", Symbols: map[string]struct{}{"XAUUSD": {}}, WantStatus: true,
		Send: func(s Signal) { id = append(id, s.ID) },
	})
	eval := &fakeEvaluator{strategy: "ila-2.0", pairs: []Pair{{HTF: "H4", LTF: "M15"}},
		results: map[string]*Result{"H4|M15": {Status: "WAIT_LTF_CISD"}}}
	pub := NewPublisher(eval, reg, Version{}, discards()).
		WithClock(func() time.Time { return jamBuka })

	pub.Scan(context.Background())
	pub.WithClock(func() time.Time { return jamBuka.Add(time.Minute) })
	pub.Scan(context.Background())

	if len(id) != 2 {
		t.Fatalf("status diterima = %d, ingin 2", len(id))
	}
	if id[0] != id[1] {
		t.Fatalf("ID status berubah antar scan: %q vs %q", id[0], id[1])
	}
}

func TestJadwalLilinCloseDriven(t *testing.T) {
	reg := NewRegistry()
	var count int
	reg.Add(Subscriber{
		ID: "test", Symbols: map[string]struct{}{"XAUUSD": {}}, WantStatus: true,
		Send: func(s Signal) { count++ },
	})
	eval := &fakeEvaluator{
		strategy: "ila-2.0",
		pairs:    []Pair{{HTF: "H4", LTF: "M15"}},
		results:  map[string]*Result{"H4|M15": {Status: "WAIT_HTF_SWEEP"}},
	}

	// Mulai pada jam 10:05 UTC (di tengah candle H4)
	jamAwal := time.Date(2026, 11, 2, 10, 5, 0, 0, time.UTC)
	pub := NewPublisher(eval, reg, Version{}, discards()).
		DenganJadwalLilin(3 * time.Second).
		WithClock(func() time.Time { return jamAwal })

	// Scan 1: Evaluasi awal wajib jalan
	pub.Scan(context.Background())
	if count != 1 {
		t.Fatalf("evaluasi awal harus jalan, got count=%d", count)
	}

	// Scan 2: 1 menit kemudian (10:06 UTC), candle H4 belum tutup -> lewati
	pub.WithClock(func() time.Time { return jamAwal.Add(time.Minute) })
	pub.Scan(context.Background())
	if count != 1 {
		t.Fatalf("sebelum candle H4 tutup evaluasi harus dilewati, got count=%d", count)
	}

	// Scan 3: Saat candle H4 tutup (14:00:04 UTC / 09:00 NY) -> evaluasi jalan
	jamH4Tutup := time.Date(2026, 11, 2, 14, 0, 4, 0, time.UTC)
	pub.WithClock(func() time.Time { return jamH4Tutup })
	pub.Scan(context.Background())
	if count != 2 {
		t.Fatalf("saat candle H4 tutup evaluasi harus jalan, got count=%d", count)
	}
}
