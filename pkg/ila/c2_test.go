package ila

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// Rujukan: New ILA 2.0/c2-closure-irl.md (Kasus A-I) dan c2-closure-erl.md (Kasus A-H).
// Gambar IRL: img/Desktop - 1..7.png dipetakan di komentar tiap kasus.

type c2Case struct {
	name    string
	extra   []C
	status  C2Status
	c2Index int
	invalid int // -1 bila tidak invalid
}

// SIBI: Top U=108, Bottom/acuan awal L=104; candle 3 = idx2, extra mulai idx3.
var irlCases = []c2Case{
	{"A C2 langsung (Desktop 2 kiri)", []C{{102, 106, 101, 103}}, C2Active, 3, -1},
	{"B diabaikan lalu C2 (Desktop 7)", []C{{101, 103, 100.5, 102}, {102, 106, 101, 103}}, C2Active, 4, -1},
	{"C acuan bergeser lalu C2 (Desktop 1, 2 kanan)", []C{{103, 106, 102.5, 105}, {105, 107, 104.5, 105.5}}, C2Active, 4, -1},
	{"D close di atas batas atas = iFVG (Desktop 6 kiri)", []C{{103, 110, 102, 109}}, C2Invalid, -1, 3},
	{"E invalid lalu turun kembali tetap bukan C2 (Desktop 6 kanan)", []C{{103, 110, 102, 109}, {109, 109, 102, 103}}, C2Invalid, -1, 3},
	{"F wick tembus batas atas tetap C2", []C{{102, 110, 101, 103}}, C2Active, 3, -1},
	{"G C2 berpindah (Desktop 3)", []C{{102, 106, 101, 103}, {103, 107, 102, 104}}, C2Active, 4, -1},
	{"H C2 gugur lalu C2 baru (Desktop 5)", []C{{102, 106, 101, 103}, {103, 107, 102.5, 106.5}, {106, 108, 105, 106}}, C2Active, 5, -1},
	{"close tepat di acuan bukan C2", []C{{102, 106, 101, 104}}, C2None, -1, -1},
}

func TestC2IRL_KasusAsampaiH_SIBIdanBISI(t *testing.T) {
	for _, tc := range irlCases {
		base := series(testBase, time.Hour, append(append([]C{}, sibiBase...), tc.extra...)...)
		for _, cs := range [][]indicators.Candle{base, mirror200(base)} {
			f := FindIRL(cs)[0]
			r := EvaluateC2IRL(f, cs)
			if r.Status != tc.status || r.C2Index != tc.c2Index || r.InvalidIndex != tc.invalid {
				t.Errorf("%s (%s): got %+v", tc.name, f.Type, r)
			}
		}
	}
}

func TestC2IRL_KasusI_DigantikanFVGBaru_Desktop4(t *testing.T) {
	cs := series(testBase, time.Hour, append(append([]C{}, sibiBase...),
		C{103, 104, 102.5, 103}, C{102, 102.5, 95, 96}, C{96, 98, 94, 95})...)
	fvgs := FindIRL(cs)
	r := EvaluateC2IRL(fvgs[0], cs)
	if r.Status != C2Invalid || r.InvalidIndex != 5 {
		t.Fatalf("SIBI lama harus invalid saat SIBI baru terbentuk: %+v", r)
	}
	// Urutan 1 mendahului 2: close menembus pada candle penggantian tetap dihitung digantikan.
	if fvgs[0].Status != StatusReplaced {
		t.Fatalf("status harus REPLACED: %v", fvgs[0].Status)
	}
}

func TestC2IRL_CandleBerjalanBelumFinal(t *testing.T) {
	cs := series(testBase, time.Hour, append(append([]C{}, sibiBase...), C{102, 106, 101, 103})...)
	cs[3].IsClosed = false
	r := EvaluateC2IRL(FindIRL(cs)[0], cs)
	if r.Status != C2None || !r.RunningTouch {
		t.Fatalf("candle berjalan menyentuh acuan harus ditandai belum final: %+v", r)
	}
}

// Swing Low S=8, candle kanan = idx2, extra mulai idx3.
var erlCases = []c2Case{
	{"A C2 langsung", []C{{10, 11, 7.5, 9.5}}, C2Active, 3, -1},
	{"B diabaikan lalu C2", []C{{10, 11, 8.5, 10}, {10, 11, 7.5, 9.5}}, C2Active, 4, -1},
	{"C invalid FTM", []C{{9, 10, 7.5, 7.8}}, C2Invalid, -1, 3},
	{"D invalid lalu naik kembali bukan C2", []C{{9, 10, 7.5, 7.8}, {9, 10, 8, 9}}, C2Invalid, -1, 3},
	{"E C2 berpindah", []C{{10, 11, 7.5, 9.5}, {10, 11, 7, 9}}, C2Active, 4, -1},
	// F: C2 diikuti candle yang tidak menyentuh acuan. Low C2 < low candle kiri & kanan, jadi C2 sendiri
	// membentuk swing low baru dan swing lama REPLACED saat candle sesudah C2 (C3) close.
	{"F C2 diikuti candle tak menyentuh acuan: digantikan swing baru", []C{{10, 11, 7.5, 9.5}, {10, 11, 8.5, 10}, {10, 11, 9, 10}}, C2Invalid, -1, 4},
	{"G C2 gugur karena FTM", []C{{10, 11, 7.5, 9.5}, {9, 10, 7, 7.5}}, C2Invalid, -1, 4},
	{"H digantikan swing baru", []C{{10, 11, 9.5, 10}, {9.5, 10, 8.5, 9}, {9.5, 11, 9.2, 10}}, C2Invalid, -1, 5},
}

func TestC2ERL_KasusAsampaiH_SwingLowdanSwingHigh(t *testing.T) {
	for _, tc := range erlCases {
		base := series(testBase, time.Hour, append(append([]C{}, swingLowBase...), tc.extra...)...)
		for _, cs := range [][]indicators.Candle{base, mirror200(base)} {
			sw := FindERL(cs)[0]
			r := EvaluateC2ERL(sw, cs)
			if r.Status != tc.status || r.C2Index != tc.c2Index || r.InvalidIndex != tc.invalid {
				t.Errorf("%s (%s): got %+v", tc.name, sw.Type, r)
			}
		}
	}
}
