package ila

// Time Cycle. Sumber aturan: "New ILA 2.0/time-cycle.md".
//   - Semua putaran mulai 18:00 NY (America/New_York, DST otomatis).
//   - Week 1: tgl 1 Minggu -> minggu itu; selain itu Minggu pertama setelah tgl 1.
//   - Minggu ke-5 di luar Monthly/Quarterly/Yearly, tetap di dalam Weekly ke bawah.
//   - C2 closure dan C3 wajib di dalam cycle yang sama.

import (
	"strings"
	"time"
)

// TimeCycle adalah nama cycle baku.
type TimeCycle string

const (
	CycleYearly    TimeCycle = "YEARLY"
	CycleQuarterly TimeCycle = "QUARTERLY"
	CycleMonthly   TimeCycle = "MONTHLY"
	CycleWeekly    TimeCycle = "WEEKLY"
	CycleDaily     TimeCycle = "DAILY"
	Cycle90M       TimeCycle = "90M"   // durasi 6 jam
	CycleMicro     TimeCycle = "MICRO" // durasi 90 menit
)

// Timeframe kanonik.
const (
	TFQuarterly = "Q"
	TFMonthly   = "MN"
	TFWeekly    = "W"
	TFDaily     = "D"
	TFH4        = "H4"
	TFH1        = "H1"
	TFM15       = "M15"
	TFM5        = "M5"
	TFM1        = "M1"
)

// CycleSpec memetakan cycle ke pasangan HTF -> LTF (time-cycle.md, Tabel Time Cycle).
var CycleSpec = map[TimeCycle][2]string{
	CycleYearly:    {TFQuarterly, TFWeekly},
	CycleQuarterly: {TFMonthly, TFDaily},
	CycleMonthly:   {TFWeekly, TFH4},
	CycleWeekly:    {TFDaily, TFH1},
	CycleDaily:     {TFH4, TFM15},
	Cycle90M:       {TFH1, TFM5},
	CycleMicro:     {TFM15, TFM1},
}

var nyLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// shortcut: tanpa tzdata DST hilang (EST tetap); sertakan time/tzdata bila build di mesin tanpa zoneinfo.
		return time.FixedZone("EST", -5*3600)
	}
	return loc
}()

// ParseTimeframe menormalkan nama timeframe ke bentuk kanonik.
// "1M"/"3M" sengaja dibaca menit agar konsisten dengan pkg/marketfeed;
// Monthly/Quarterly wajib ditulis "MN"/"MN1"/"MONTHLY" dan "Q"/"Q1"/"QUARTERLY".
func ParseTimeframe(tf string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tf)) {
	case "Q", "Q1", "QUARTERLY", "3MO":
		return TFQuarterly, true
	case "MN", "MN1", "MONTHLY", "1MO":
		return TFMonthly, true
	case "W", "W1", "1W", "WEEKLY":
		return TFWeekly, true
	case "D", "D1", "1D", "DAILY":
		return TFDaily, true
	case "H4", "4H", "240", "240M":
		return TFH4, true
	case "H1", "1H", "60", "60M":
		return TFH1, true
	case "M15", "15M", "15":
		return TFM15, true
	case "M5", "5M", "5":
		return TFM5, true
	case "M1", "1M", "1":
		return TFM1, true
	}
	return "", false
}

// CycleForHTF mengembalikan cycle yang HTF-nya = tf.
func CycleForHTF(tf string) (TimeCycle, bool) {
	k, ok := ParseTimeframe(tf)
	if !ok {
		return "", false
	}
	for c, pair := range CycleSpec {
		if pair[0] == k {
			return c, true
		}
	}
	return "", false
}

// candleEnd menghitung waktu tutup candle dari waktu buka dan timeframe kanonik.
func candleEnd(open time.Time, tf string) time.Time {
	switch tf {
	case TFQuarterly:
		return open.AddDate(0, 3, 0)
	case TFMonthly:
		return open.AddDate(0, 1, 0)
	case TFWeekly:
		return open.AddDate(0, 0, 7)
	case TFDaily:
		return open.AddDate(0, 0, 1)
	case TFH4:
		return open.Add(4 * time.Hour)
	case TFH1:
		return open.Add(time.Hour)
	case TFM15:
		return open.Add(15 * time.Minute)
	case TFM5:
		return open.Add(5 * time.Minute)
	default:
		return open.Add(time.Minute)
	}
}

// at18 membuat waktu 18:00 NY pada tanggal (y, m, d); time.Date menormalkan overflow.
func at18(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 18, 0, 0, 0, nyLocation)
}

// tradingDayStart = 18:00 NY terakhir yang <= t (hari trading mulai 18:00 NY).
func tradingDayStart(t time.Time) time.Time {
	n := t.In(nyLocation)
	s := at18(n.Year(), n.Month(), n.Day())
	if n.Before(s) {
		s = at18(n.Year(), n.Month(), n.Day()-1)
	}
	return s
}

// Week1Start = Minggu 18:00 NY awal week 1 pada bulan tersebut.
func Week1Start(year int, month time.Month) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, nyLocation)
	offset := (7 - int(first.Weekday())) % 7
	return at18(year, month, 1+offset)
}

// Week4End = Jumat 18:00 NY di week 4 (3 minggu + 5 hari setelah Week1Start).
func Week4End(year int, month time.Month) time.Time {
	w1 := Week1Start(year, month)
	return at18(w1.Year(), w1.Month(), w1.Day()+26)
}

// monthWindow mengembalikan window monthly cycle yang memuat t.
// Week 4 bulan sebelumnya bisa melebar sampai awal bulan ini (mis. Des 2025 -> 2 Jan 2026),
// jadi cek bulan sebelumnya dulu baru bulan berjalan.
func monthWindow(t time.Time) (time.Time, time.Time, bool) {
	n := t.In(nyLocation)
	prev := time.Date(n.Year(), n.Month()-1, 1, 0, 0, 0, 0, nyLocation)
	if ps, pe := Week1Start(prev.Year(), prev.Month()), Week4End(prev.Year(), prev.Month()); !t.Before(ps) && t.Before(pe) {
		return ps, pe, true
	}
	s, e := Week1Start(n.Year(), n.Month()), Week4End(n.Year(), n.Month())
	return s, e, !t.Before(s) && t.Before(e)
}

// CycleWindowAt mengembalikan window [start, end) cycle yang memuat t.
// inside=false bila t berada di luar putaran (akhir pekan untuk Weekly,
// minggu ke-5 / sebelum week 1 untuk Monthly/Quarterly/Yearly).
func CycleWindowAt(c TimeCycle, t time.Time) (start, end time.Time, inside bool) {
	day := tradingDayStart(t)
	switch c {
	case CycleDaily:
		return day, at18(day.Year(), day.Month(), day.Day()+1), true
	case Cycle90M, CycleMicro:
		// Blok 6 jam: 18-00, 00-06, 06-12, 12-18 NY (jam dinding NY).
		start = day
		for h := 6; h <= 24; h += 6 {
			next := time.Date(day.Year(), day.Month(), day.Day(), 18+h, 0, 0, 0, nyLocation)
			if t.Before(next) {
				end = next
				break
			}
			start = next
		}
		if c == Cycle90M {
			return start, end, true
		}
		// 4 putaran 90 menit per blok; putaran terakhir menyerap selisih hari DST.
		r := int(t.Sub(start) / (90 * time.Minute))
		if r > 3 {
			r = 3
		}
		s := start.Add(time.Duration(r) * 90 * time.Minute)
		e := s.Add(90 * time.Minute)
		if r == 3 {
			e = end
		}
		return s, e, true
	case CycleWeekly:
		sun := at18(day.Year(), day.Month(), day.Day()-int(day.Weekday()))
		fri := at18(sun.Year(), sun.Month(), sun.Day()+5)
		return sun, fri, t.Before(fri)
	case CycleMonthly:
		return monthWindow(t)
	case CycleQuarterly, CycleYearly:
		ms, _, inMonth := monthWindow(t)
		// Pakai tahun/bulan pemilik week 1..4 agar spillover awal Januari tetap masuk Q4/Yearly tahun sebelumnya.
		ref := t.In(nyLocation)
		if inMonth {
			ref = ms
		}
		firstMonth, months := time.Month(1), 12
		if c == CycleQuarterly {
			firstMonth, months = time.Month((int(ref.Month())-1)/3*3+1), 3
		}
		start = Week1Start(ref.Year(), firstMonth)
		lastMonth := firstMonth + time.Month(months-1)
		end = Week4End(ref.Year(), lastMonth)
		return start, end, inMonth && !t.Before(start) && t.Before(end)
	}
	return time.Time{}, time.Time{}, false
}

// candleMid dipakai untuk menentukan posisi candle di cycle; timestamp buka
// broker bisa meleset dari 18:00 NY (mis. Daily FX dibuka 17:00 NY).
func candleMid(open time.Time, tf string) time.Time {
	return open.Add(candleEnd(open, tf).Sub(open) / 2)
}

// ValidateC2AndC3InCycle: C2 dan C3 (waktu buka, timeframe HTF kanonik) wajib
// berada di dalam satu putaran cycle yang sama.
func ValidateC2AndC3InCycle(c TimeCycle, htf string, c2Open, c3Open time.Time) bool {
	s2, _, in2 := CycleWindowAt(c, candleMid(c2Open, htf))
	s3, _, in3 := CycleWindowAt(c, candleMid(c3Open, htf))
	return in2 && in3 && s2.Equal(s3)
}
