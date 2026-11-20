package ila

import (
	"testing"
	"time"
)

// Rujukan: New ILA 2.0/key-level-irl.md dan key-level-erl.md.

var sibiBase = []C{{110, 112, 108, 109}, {101, 109, 100, 108}, {101, 104, 99, 100}} // c2 bullish: warna tidak disyaratkan

func TestFindIRL_PembentukanSIBIdanBISI(t *testing.T) {
	cs := series(testBase, time.Hour, sibiBase...)
	f := FindIRL(cs)
	if len(f) != 1 || f[0].Type != FVGSIBI || f[0].Top != 108 || f[0].Bottom != 104 || f[0].InitialReference() != 104 {
		t.Fatalf("SIBI salah: %+v", f)
	}
	f = FindIRL(mirror200(cs))
	if len(f) != 1 || f[0].Type != FVGBISI || f[0].Top != 96 || f[0].Bottom != 92 || f[0].InitialReference() != 96 {
		t.Fatalf("BISI salah: %+v", f)
	}
}

func TestFindIRL_CandleBelumCloseTidakMembentuk(t *testing.T) {
	cs := series(testBase, time.Hour, sibiBase...)
	cs[2].IsClosed = false
	if len(FindIRL(cs)) != 0 {
		t.Fatal("FVG tidak boleh terbentuk sebelum candle 3 close")
	}
}

func TestFindIRL_InvalidHanyaDariCloseBody(t *testing.T) {
	wick := FindIRL(series(testBase, time.Hour, append(sibiBase, C{103, 110, 102, 105})...))
	if wick[0].Status != StatusValid {
		t.Fatalf("wick menembus batas atas tidak boleh invalid: %v", wick[0].Status)
	}
	closeBody := FindIRL(series(testBase, time.Hour, append(sibiBase, C{103, 110, 102, 109})...))
	if closeBody[0].Status != StatusIFVG || closeBody[0].InvalidIndex != 3 {
		t.Fatalf("close di atas batas atas harus iFVG: %+v", closeBody[0])
	}
}

func TestFindIRL_DigantikanSejenisBukaniFVG_Permanen(t *testing.T) {
	// SIBI baru (candle 1-3 = idx3-5) langsung menggantikan SIBI lama walau tanpa close menembus.
	cs := series(testBase, time.Hour, append(sibiBase,
		C{103, 104, 102.5, 103}, C{102, 102.5, 95, 96}, C{96, 98, 94, 95})...)
	f := FindIRL(cs)
	if len(f) != 2 || f[0].Status != StatusReplaced || f[0].InvalidIndex != 5 {
		t.Fatalf("SIBI lama harus REPLACED di idx 5: %+v", f)
	}
	// Pengganti jadi iFVG pun, yang lama tetap REPLACED (permanen).
	cs = append(cs, series(testBase.Add(6*time.Hour), time.Hour, C{95, 104, 94, 103})...)
	f = FindIRL(cs)
	if f[0].Status != StatusReplaced || f[1].Status != StatusIFVG {
		t.Fatalf("invalid harus permanen: %v %v", f[0].Status, f[1].Status)
	}
	if LatestIRL(f, FVGSIBI).Candle3Index != 5 {
		t.Fatal("LatestIRL harus mengembalikan FVG sejenis paling baru")
	}
}

var swingLowBase = []C{{11, 12, 10, 11}, {10, 11, 8, 9}, {10, 12, 9, 11}} // level 8

func TestFindERL_PembentukanDanStatus(t *testing.T) {
	cs := series(testBase, time.Hour, swingLowBase...)
	s := FindERL(cs)
	if len(s) != 1 || s[0].Type != ERLSwingLow || s[0].Level != 8 || s[0].MidIndex != 1 {
		t.Fatalf("Swing Low salah: %+v", s)
	}
	s = FindERL(mirror200(cs))
	if len(s) != 1 || s[0].Type != ERLSwingHigh || s[0].Level != 192 {
		t.Fatalf("Swing High salah: %+v", s)
	}
	cs[2].IsClosed = false
	if len(FindERL(cs)) != 0 {
		t.Fatal("swing tidak boleh terbentuk sebelum candle kanan close")
	}
}

func TestFindERL_FTMHanyaDariCloseBody(t *testing.T) {
	if s := FindERL(series(testBase, time.Hour, append(swingLowBase, C{9, 10, 6, 8.5})...)); s[0].Status != StatusValid {
		t.Fatalf("wick menembus level tidak boleh FTM: %v", s[0].Status)
	}
	if s := FindERL(series(testBase, time.Hour, append(swingLowBase, C{9, 10, 6, 7.5})...)); s[0].Status != StatusFTM || s[0].InvalidIndex != 3 {
		t.Fatalf("close di bawah level harus FTM: %+v", s[0])
	}
}

func TestFindERL_DigantikanSejenisBukanFTM_Permanen(t *testing.T) {
	cs := series(testBase, time.Hour, append(swingLowBase,
		C{10, 11, 9.5, 10}, C{9.5, 10, 8.5, 9}, C{9.5, 11, 9.2, 10})...)
	s := FindERL(cs)
	var lows []SwingERL
	for _, x := range s {
		if x.Type == ERLSwingLow {
			lows = append(lows, x)
		}
	}
	if len(lows) != 2 || lows[0].Status != StatusReplaced || lows[0].InvalidIndex != 5 {
		t.Fatalf("swing low lama harus REPLACED di idx 5: %+v", lows)
	}
	if LatestERL(s, ERLSwingLow).Level != 8.5 {
		t.Fatal("LatestERL harus swing sejenis paling baru")
	}
}
