package engine

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// Pengujian ini mengunci satu janji penting: permintaan pasangan timeframe dari
// Runner HANYA menyaring pasangan yang sudah sah menurut aturan trading engine.
// Runner tidak boleh bisa meminta pasangan karangan, dan engine yang belum
// menerima daftar pasangan harus tetap berperilaku seperti sebelum fitur ini ada.

// evaluatorEmpatPasangan meniru ILA 2.0: empat pasangan tetap.
func evaluatorEmpatPasangan() *fakeEvaluator {
	return &fakeEvaluator{
		strategy: "ila-2.0",
		pairs: []Pair{
			{HTF: "W", LTF: "H4"},
			{HTF: "D1", LTF: "H1"},
			{HTF: "H4", LTF: "M15"},
			{HTF: "H1", LTF: "M5"},
		},
		results: map[string]*Result{
			"W|H4":   readyResult(),
			"D1|H1":  readyResult(),
			"H4|M15": readyResult(),
			"H1|M5":  readyResult(),
			"H2|M30": readyResult(),
		},
	}
}

func TestPasanganKosongMenghitungSemua(t *testing.T) {
	// Perilaku lama: tanpa daftar pasangan, engine menghitung seluruh pasangan.
	reg := NewRegistry()
	reg.Add(Subscriber{ID: "runner", Symbols: map[string]struct{}{"XAUUSD": {}}})

	pub := NewPublisher(evaluatorEmpatPasangan(), reg, Version{Strategy: "ila-2.0"}, discards())
	dipanggil := &[]string{}
	pub.eval = &pencatatEvaluator{inner: evaluatorEmpatPasangan(), log: dipanggil}

	pub.Scan(context.Background())

	if len(*dipanggil) != 4 {
		t.Fatalf("pasangan kosong harus menghitung 4 pasangan, dapat %d: %v", len(*dipanggil), *dipanggil)
	}
}

func TestPasanganDimintaMenyaringTanpaMenambah(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Subscriber{
		ID:      "runner",
		Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs:   []Pair{{HTF: "H4", LTF: "M15"}},
	})

	dipanggil := &[]string{}
	pub := NewPublisher(evaluatorEmpatPasangan(), reg, Version{Strategy: "ila-2.0"}, discards())
	pub.eval = &pencatatEvaluator{inner: evaluatorEmpatPasangan(), log: dipanggil}

	pub.Scan(context.Background())

	if len(*dipanggil) != 1 || (*dipanggil)[0] != "H4|M15" {
		t.Fatalf("hanya H4|M15 yang boleh dihitung, dapat %v", *dipanggil)
	}
}

func TestPasanganDiLuarAturanTidakDitambahkan(t *testing.T) {
	// Runner meminta pasangan yang tidak ada di aturan trading engine.
	// Engine tidak boleh mengarangnya, dan juga tidak boleh diam-diam
	// kembali ke empat pasangan bawaan.
	reg := NewRegistry()
	reg.Add(Subscriber{
		ID:      "runner",
		Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs:   []Pair{{HTF: "H2", LTF: "M30"}},
	})

	dipanggil := &[]string{}
	pub := NewPublisher(evaluatorEmpatPasangan(), reg, Version{Strategy: "ila-2.0"}, discards())
	pub.eval = &pencatatEvaluator{inner: evaluatorEmpatPasangan(), log: dipanggil}

	pub.Scan(context.Background())

	if len(*dipanggil) != 0 {
		t.Fatalf("pasangan di luar aturan tidak boleh dievaluasi, dapat %v", *dipanggil)
	}
}

func TestGabunganPasanganBeberapaPelanggan(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Subscriber{
		ID: "runner-a", Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs: []Pair{{HTF: "H4", LTF: "M15"}},
	})
	reg.Add(Subscriber{
		ID: "runner-b", Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs: []Pair{{HTF: "W", LTF: "H4"}},
	})

	dipanggil := &[]string{}
	pub := NewPublisher(evaluatorEmpatPasangan(), reg, Version{Strategy: "ila-2.0"}, discards())
	pub.eval = &pencatatEvaluator{inner: evaluatorEmpatPasangan(), log: dipanggil}

	pub.Scan(context.Background())

	if len(*dipanggil) != 2 {
		t.Fatalf("gabungan dua pelanggan harus 2 pasangan, dapat %v", *dipanggil)
	}
}

func TestSatuPelangganMemintaSemuaMembatalkanPenyaringan(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Subscriber{
		ID: "sempit", Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs: []Pair{{HTF: "H4", LTF: "M15"}},
	})
	reg.Add(Subscriber{
		ID: "luas", Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs: nil, // semua pasangan
	})

	dipanggil := &[]string{}
	pub := NewPublisher(evaluatorEmpatPasangan(), reg, Version{Strategy: "ila-2.0"}, discards())
	pub.eval = &pencatatEvaluator{inner: evaluatorEmpatPasangan(), log: dipanggil}

	pub.Scan(context.Background())

	if len(*dipanggil) != 4 {
		t.Fatalf("permintaan semua pasangan harus menghitung 4, dapat %v", *dipanggil)
	}
}

func TestSetPairsMenggantiDaftar(t *testing.T) {
	// Perubahan saklar di platform tiba sebagai Subscribe ulang.
	reg := NewRegistry()
	reg.Add(Subscriber{ID: "runner", Symbols: map[string]struct{}{"XAUUSD": {}}})

	if !reg.SetPairs("runner", []Pair{{HTF: "W", LTF: "H4"}}) {
		t.Fatal("SetPairs harus menemukan pelanggan yang terdaftar")
	}

	pairs, dibatasi := reg.WantedPairs("XAUUSD")
	if !dibatasi || len(pairs) != 1 || pairs[0] != (Pair{HTF: "W", LTF: "H4"}) {
		t.Fatalf("pasangan tidak terganti, dapat %v (dibatasi=%v)", pairs, dibatasi)
	}

	// Daftar kosong kembali berarti semua pasangan.
	reg.SetPairs("runner", nil)
	if _, dibatasi := reg.WantedPairs("XAUUSD"); dibatasi {
		t.Fatal("daftar kosong harus berarti semua pasangan")
	}
}

func TestILA3CyclePairsAndNormalization(t *testing.T) {
	eval := NewILA3Evaluator(nil)
	if eval.StrategyVersion() != "ila-3.0" {
		t.Errorf("expected strategy ila-3.0, got %s", eval.StrategyVersion())
	}

	pairs := eval.Pairs()
	if len(pairs) != 4 {
		t.Fatalf("expected 4 cycle pairs for ILA 3.0, got %d", len(pairs))
	}

	// Pastikan ke-4 siklus terdaftar
	expectedCycles := map[string]bool{
		"4H|5m": true, // Daily
		"D|15m": true, // Weekly
		"1H|5m": true, // 90M
		"W|1H":  true, // Monthly
	}
	for _, p := range pairs {
		k := p.HTF + "|" + p.LTF
		if !expectedCycles[k] {
			t.Errorf("unexpected pair %s", k)
		}
	}

	// Uji toleransi normalisasi timeframe
	normCases := []struct {
		inputHTF string
		inputLTF string
		wantMode string
	}{
		{"4H", "5m", "Daily Cycle (New Day)"},
		{"H4", "M5", "Daily Cycle (New Day)"},
		{"D", "15m", "Weekly Cycle (New Week)"},
		{"D1", "M15", "Weekly Cycle (New Week)"},
		{"1H", "5m", "90M Cycle (Intraday)"},
		{"H1", "M5", "90M Cycle (Intraday)"},
		{"W", "1H", "Monthly Cycle (New Month)"},
		{"W1", "H1", "Monthly Cycle (New Month)"},
	}

	ila3Eval := eval.(*ila3Evaluator)
	for _, tc := range normCases {
		p, ok := ila3Eval.presetOf[tc.inputHTF+"|"+tc.inputLTF]
		if !ok {
			p, ok = ila3Eval.presetOf[normTF(tc.inputHTF)+"|"+normTF(tc.inputLTF)]
		}
		if !ok {
			t.Errorf("pair %s|%s not found in presetOf", tc.inputHTF, tc.inputLTF)
		} else if p.Mode != tc.wantMode {
			t.Errorf("pair %s|%s expected mode %s, got %s", tc.inputHTF, tc.inputLTF, tc.wantMode, p.Mode)
		}
	}
}

func TestPasanganTidakLengkapDibuang(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Subscriber{ID: "runner", Symbols: map[string]struct{}{"XAUUSD": {}}})
	reg.SetPairs("runner", []Pair{
		{HTF: "", LTF: "M15"},
		{HTF: "H4", LTF: ""},
		{HTF: "H4", LTF: "M15"},
		{HTF: "H4", LTF: "M15"}, // duplikat
	})

	pairs, _ := reg.WantedPairs("XAUUSD")
	if len(pairs) != 1 {
		t.Fatalf("pasangan tidak lengkap dan duplikat harus dibuang, dapat %v", pairs)
	}
}

// pencatatEvaluator merekam pasangan mana yang benar-benar dievaluasi.
type pencatatEvaluator struct {
	inner *fakeEvaluator
	log   *[]string
}

func (p *pencatatEvaluator) StrategyVersion() string { return p.inner.StrategyVersion() }
func (p *pencatatEvaluator) Pairs() []Pair           { return p.inner.Pairs() }
func (p *pencatatEvaluator) Evaluate(symbol, htf, ltf string) (*Result, error) {
	*p.log = append(*p.log, htf+"|"+ltf)
	return p.inner.Evaluate(symbol, htf, ltf)
}

var _ = time.Now

func TestPairsForDenganNormalisasiBerbeda(t *testing.T) {
	eval := NewILA3Evaluator(nil)
	reg := NewRegistry()
	pub := NewPublisher(eval, reg, Version{Strategy: "ila-3.0"}, slog.Default())

	// Runner meminta pasangan dalam format huruf besar (mis. 4H-5M dari NormalizeTimeframe),
	// sedangkan preset evaluator mendefinisikan 4H-5m.
	reg.Add(Subscriber{
		ID:      "runner-ila3",
		Symbols: map[string]struct{}{"XAUUSD": {}},
		Pairs: []Pair{
			{HTF: "4H", LTF: "5M"},
			{HTF: "H4", LTF: "M5"}, // variasi lain yang sama
			{HTF: "D1", LTF: "15M"},
		},
	})

	pairs := pub.pairsFor("XAUUSD")
	if len(pairs) != 2 {
		t.Fatalf("pairsFor harus menghasilkan 2 pasangan kanonikal unik, dapat %d: %v", len(pairs), pairs)
	}

	// Pastikan pasangan kanonikal evaluator yang dikembalikan
	expected := map[Pair]bool{
		{HTF: "4H", LTF: "5m"}: true,
		{HTF: "D", LTF: "15m"}: true,
	}
	for _, p := range pairs {
		if !expected[p] {
			t.Errorf("pasangan kanonikal tidak terduga: %v", p)
		}
	}
}
