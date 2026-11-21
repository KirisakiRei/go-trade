package ila

import (
	"testing"
	"time"
)

// Rujukan: New ILA 2.0/time-cycle.md.

func ny(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, nyLocation)
}

func TestCycleSpec_TujuhCycleDanPasangan(t *testing.T) {
	want := map[TimeCycle][2]string{
		CycleYearly: {"Q", "W"}, CycleQuarterly: {"MN", "D"}, CycleMonthly: {"W", "H4"},
		CycleWeekly: {"D", "H1"}, CycleDaily: {"H4", "M15"}, Cycle90M: {"H1", "M5"}, CycleMicro: {"M15", "M1"},
	}
	if len(CycleSpec) != 7 {
		t.Fatalf("harus 7 cycle, got %d", len(CycleSpec))
	}
	for c, p := range want {
		if CycleSpec[c] != p {
			t.Errorf("%s: got %v want %v", c, CycleSpec[c], p)
		}
		if got, ok := CycleForHTF(p[0]); !ok || got != c {
			t.Errorf("CycleForHTF(%s) = %v, want %v", p[0], got, c)
		}
	}
}

func TestParseTimeframe_1MAdalahMenit(t *testing.T) {
	for in, want := range map[string]string{"1M": "M1", "M1": "M1", "MN1": "MN", "monthly": "MN", "Q1": "Q", "4H": "H4", "d1": "D"} {
		if got, ok := ParseTimeframe(in); !ok || got != want {
			t.Errorf("ParseTimeframe(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}

func TestWeek1Start_Tgl1MingguVsBukan(t *testing.T) {
	if got := Week1Start(2025, time.June); !got.Equal(ny(2025, time.June, 1, 18, 0)) { // 1 Juni 2025 = Minggu
		t.Errorf("tgl 1 Minggu: %v", got)
	}
	if got := Week1Start(2025, time.July); !got.Equal(ny(2025, time.July, 6, 18, 0)) { // 1 Juli 2025 = Selasa
		t.Errorf("tgl 1 bukan Minggu: %v", got)
	}
	if got := Week4End(2025, time.June); !got.Equal(ny(2025, time.June, 27, 18, 0)) || got.Weekday() != time.Friday {
		t.Errorf("week 4 berakhir Jumat 18:00: %v", got)
	}
}

func TestMonthly_Minggu5DiLuarCycleTapiDiDalamWeekly(t *testing.T) {
	w5 := ny(2025, time.June, 28, 12, 0) // Sabtu setelah Jumat week 4
	if _, _, in := CycleWindowAt(CycleMonthly, w5); in {
		t.Error("minggu ke-5 harus di luar monthly")
	}
	if _, _, in := CycleWindowAt(CycleQuarterly, ny(2025, time.June, 30, 12, 0)); in {
		t.Error("minggu ke-5 harus di luar quarterly")
	}
	if _, _, in := CycleWindowAt(CycleYearly, ny(2024, time.December, 30, 12, 0)); in {
		t.Error("minggu ke-5 harus di luar yearly")
	}
	if _, _, in := CycleWindowAt(CycleDaily, ny(2025, time.June, 30, 12, 0)); !in {
		t.Error("minggu ke-5 tetap di dalam daily")
	}
	if _, _, in := CycleWindowAt(CycleMonthly, ny(2025, time.July, 3, 12, 0)); in {
		t.Error("sebelum week 1 (bukan Minggu tgl 1) di luar monthly")
	}
	if s, e, in := CycleWindowAt(CycleMonthly, ny(2025, time.June, 10, 12, 0)); !in || !s.Equal(ny(2025, time.June, 1, 18, 0)) || !e.Equal(ny(2025, time.June, 27, 18, 0)) {
		t.Errorf("window monthly Juni: %v %v %v", s, e, in)
	}
}

func TestWeekly_MingguSampaiJumat1800NY(t *testing.T) {
	if s, e, in := CycleWindowAt(CycleWeekly, ny(2025, time.June, 11, 12, 0)); !in || !s.Equal(ny(2025, time.June, 8, 18, 0)) || !e.Equal(ny(2025, time.June, 13, 18, 0)) {
		t.Errorf("Rabu: %v %v %v", s, e, in)
	}
	if _, _, in := CycleWindowAt(CycleWeekly, ny(2025, time.June, 14, 12, 0)); in {
		t.Error("Sabtu di luar weekly")
	}
	if _, _, in := CycleWindowAt(CycleWeekly, ny(2025, time.June, 15, 17, 59)); in {
		t.Error("Minggu 17:59 di luar weekly")
	}
	if s, _, in := CycleWindowAt(CycleWeekly, ny(2025, time.June, 15, 18, 0)); !in || !s.Equal(ny(2025, time.June, 15, 18, 0)) {
		t.Error("Minggu 18:00 memulai weekly baru")
	}
}

func TestDaily_Anchor1800NY_DanDST(t *testing.T) {
	s, e, _ := CycleWindowAt(CycleDaily, ny(2025, time.June, 10, 17, 59))
	if !s.Equal(ny(2025, time.June, 9, 18, 0)) || !e.Equal(ny(2025, time.June, 10, 18, 0)) {
		t.Errorf("17:59 masih hari trading sebelumnya: %v %v", s, e)
	}
	s, _, _ = CycleWindowAt(CycleDaily, ny(2025, time.June, 10, 18, 0))
	if !s.Equal(ny(2025, time.June, 10, 18, 0)) {
		t.Errorf("18:00 memulai hari trading baru: %v", s)
	}
	s, e, _ = CycleWindowAt(CycleDaily, ny(2025, time.March, 9, 10, 0)) // DST maju 9 Maret 2025
	if e.Sub(s) != 23*time.Hour {
		t.Errorf("hari DST maju 23 jam, got %v", e.Sub(s))
	}
	s, e, _ = CycleWindowAt(CycleDaily, ny(2025, time.November, 2, 10, 0)) // DST mundur 2 Nov 2025
	if e.Sub(s) != 25*time.Hour {
		t.Errorf("hari DST mundur 25 jam, got %v", e.Sub(s))
	}
}

func TestCycle90MdanMicro(t *testing.T) {
	at := ny(2025, time.June, 10, 3, 10)
	s, e, _ := CycleWindowAt(Cycle90M, at)
	if !s.Equal(ny(2025, time.June, 10, 0, 0)) || !e.Equal(ny(2025, time.June, 10, 6, 0)) {
		t.Errorf("90M blok 6 jam 00-06: %v %v", s, e)
	}
	s, e, _ = CycleWindowAt(CycleMicro, at)
	if !s.Equal(ny(2025, time.June, 10, 3, 0)) || !e.Equal(ny(2025, time.June, 10, 4, 30)) {
		t.Errorf("Micro putaran 03:00-04:30: %v %v", s, e)
	}
	s, _, _ = CycleWindowAt(Cycle90M, ny(2025, time.June, 10, 19, 0))
	if !s.Equal(ny(2025, time.June, 10, 18, 0)) {
		t.Errorf("blok 18-00: %v", s)
	}
	s, _, _ = CycleWindowAt(CycleMicro, ny(2025, time.June, 10, 5, 59))
	if !s.Equal(ny(2025, time.June, 10, 4, 30)) {
		t.Errorf("putaran micro terakhir 04:30: %v", s)
	}
}

func TestValidateC2AndC3InCycle(t *testing.T) {
	cases := []struct {
		name   string
		cycle  TimeCycle
		htf    string
		c2, c3 time.Time
		want   bool
	}{
		{"weekly: Selasa->Rabu", CycleWeekly, TFDaily, ny(2025, time.June, 10, 18, 0), ny(2025, time.June, 11, 18, 0), true},
		{"weekly: C3 melewati Jumat 18:00", CycleWeekly, TFDaily, ny(2025, time.June, 12, 18, 0), ny(2025, time.June, 13, 18, 0), false},
		{"daily: 4H dalam hari yang sama", CycleDaily, TFH4, ny(2025, time.June, 10, 6, 0), ny(2025, time.June, 10, 10, 0), true},
		{"daily: C3 pindah hari trading", CycleDaily, TFH4, ny(2025, time.June, 10, 14, 0), ny(2025, time.June, 10, 18, 0), false},
		{"monthly: C2 akhir bulan, C3 bulan baru", CycleMonthly, TFWeekly, ny(2026, time.February, 15, 18, 0), ny(2026, time.February, 22, 18, 0), true},
		{"monthly: lintas bulan ditolak", CycleMonthly, TFWeekly, ny(2026, time.February, 22, 18, 0), ny(2026, time.March, 1, 18, 0), false},
		{"90M: lintas blok 6 jam ditolak", Cycle90M, TFH1, ny(2025, time.June, 10, 5, 0), ny(2025, time.June, 10, 6, 0), false},
		{"micro: dalam 90 menit", CycleMicro, TFM15, ny(2025, time.June, 10, 3, 0), ny(2025, time.June, 10, 3, 15), true},
	}
	for _, tc := range cases {
		if got := ValidateC2AndC3InCycle(tc.cycle, tc.htf, tc.c2, tc.c3); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestWeek4SpilloverKeBulanBerikutnya(t *testing.T) {
	// Des 2025 mulai Senin -> Week 1 = 7 Des, Week 4 berakhir Jumat 2 Jan 2026 18:00 NY.
	jan1 := ny(2026, time.January, 1, 12, 0)
	if s, e, in := CycleWindowAt(CycleMonthly, jan1); !in || !s.Equal(ny(2025, time.December, 7, 18, 0)) || !e.Equal(ny(2026, time.January, 2, 18, 0)) {
		t.Errorf("1 Jan 2026 masih Week 4 Des 2025: %v %v %v", s, e, in)
	}
	if s, _, in := CycleWindowAt(CycleQuarterly, jan1); !in || !s.Equal(Week1Start(2025, time.October)) {
		t.Errorf("1 Jan 2026 masih Q4 2025: %v %v", s, in)
	}
	if s, _, in := CycleWindowAt(CycleYearly, jan1); !in || !s.Equal(Week1Start(2025, time.January)) {
		t.Errorf("1 Jan 2026 masih Yearly 2025: %v %v", s, in)
	}
	if _, _, in := CycleWindowAt(CycleMonthly, ny(2026, time.January, 2, 18, 0)); in {
		t.Error("2 Jan 2026 18:00 sudah di luar Week 4 Des 2025")
	}
	// Feb 2027 mulai Senin -> Week 1 = 7 Feb, Week 4 berakhir 5 Mar 2027 18:00 NY.
	if s, _, in := CycleWindowAt(CycleMonthly, ny(2027, time.March, 3, 12, 0)); !in || !s.Equal(ny(2027, time.February, 7, 18, 0)) {
		t.Errorf("3 Mar 2027 masih Week 4 Feb 2027: %v %v", s, in)
	}
}
