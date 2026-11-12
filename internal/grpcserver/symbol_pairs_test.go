package grpcserver

import (
	"io"
	"log/slog"
	"testing"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/engine"
)

// Pengujian ini mengunci konversi kebijakan pasangan timeframe dari kontrak
// gRPC ke bentuk Registry. Kesalahan di sini membuat switch per simbol tidak
// pernah sampai ke engine, dan gejalanya sulit dilacak karena tidak ada error.

func TestKebijakanDariMempertahankanRestrictedKosong(t *testing.T) {
	// Simbol dengan seluruh switch dimatikan (Opsi A) harus tetap dikirim,
	// bukan dibuang: "kosong" di sini punya arti, bukan "tidak ada data".
	got := kebijakanDari(map[string]*enginev1.SymbolPairs{
		"XAUUSD": {Restricted: true},
	})

	policy, ok := got["XAUUSD"]
	if !ok {
		t.Fatalf("XAUUSD harus tetap ada walau daftarnya kosong")
	}
	if !policy.Restricted {
		t.Fatalf("Restricted harus diteruskan")
	}
	if len(policy.Pairs) != 0 {
		t.Fatalf("daftar harus kosong, dapat %v", policy.Pairs)
	}
}

func TestKebijakanDariMemetakanPasangan(t *testing.T) {
	got := kebijakanDari(map[string]*enginev1.SymbolPairs{
		"EURUSD": {
			Restricted: true,
			Pairs: []*enginev1.Pair{
				{Htf: "H4", Ltf: "M15"},
				{Htf: "H1", Ltf: "M5"},
			},
		},
	})

	policy := got["EURUSD"]
	if len(policy.Pairs) != 2 {
		t.Fatalf("harus 2 pasangan, dapat %d: %v", len(policy.Pairs), policy.Pairs)
	}
	if !policy.Restricted {
		t.Fatalf("Restricted harus diteruskan")
	}
}

func TestKebijakanDariMembuangSimbolKosong(t *testing.T) {
	got := kebijakanDari(map[string]*enginev1.SymbolPairs{
		"":       {Restricted: true},
		"XAUUSD": {Restricted: true},
	})
	if _, ada := got[""]; ada {
		t.Fatalf("simbol kosong harus dibuang")
	}
	if len(got) != 1 {
		t.Fatalf("harus tersisa 1 simbol, dapat %d", len(got))
	}
}

func TestKebijakanDariPetaKosong(t *testing.T) {
	if got := kebijakanDari(nil); got != nil {
		t.Fatalf("peta kosong harus menghasilkan nil, dapat %v", got)
	}
}

func TestSubscribeMeneruskanKebijakanPerSimbol(t *testing.T) {
	// Jalur penuh: Subscribe dengan symbol_pairs harus sampai ke Registry.
	reg := engine.NewRegistry()
	srv := New(Deps{Registry: reg, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	_, err := srv.Subscribe(t.Context(), &enginev1.SubscribeRequest{
		SubscriberId: "runner-ila-2.0",
		Symbols:      []string{"XAUUSD", "EURUSD"},
		SymbolPairs: map[string]*enginev1.SymbolPairs{
			"XAUUSD": {Restricted: true, Pairs: []*enginev1.Pair{{Htf: "H1", Ltf: "M5"}}},
			"EURUSD": {
				Restricted: true,
				Pairs: []*enginev1.Pair{
					{Htf: "H4", Ltf: "M15"},
					{Htf: "H1", Ltf: "M5"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	xau, ok := reg.WantedPairs("XAUUSD")
	if !ok || len(xau) != 1 {
		t.Fatalf("XAUUSD harus 1 pasangan, dapat ok=%v pairs=%v", ok, xau)
	}
	eur, ok := reg.WantedPairs("EURUSD")
	if !ok || len(eur) != 2 {
		t.Fatalf("EURUSD harus 2 pasangan, dapat ok=%v pairs=%v", ok, eur)
	}
}
