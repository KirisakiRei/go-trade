package main

import (
	"testing"
	"time"

	"ila-engine/internal/market"
)

// Market tutup: Sabtu/Minggu penuh, Jumat >= 21:00 UTC, dan rollover 22:00-01:00 UTC.
func TestIsMarketClosed(t *testing.T) {
	cases := []struct {
		name string
		day  time.Weekday
		hour int
		want bool
	}{
		{"Senin pagi", time.Monday, 7, false},
		{"Rabu siang", time.Wednesday, 12, false},
		{"Jumat sesi New York 19:59 UTC", time.Friday, 19, false},
		{"Jumat sesi New York 20:00 UTC", time.Friday, 20, false},
		{"Jumat tutup 21:00 UTC", time.Friday, 21, true},
		{"Jumat tutup 23:00 UTC", time.Friday, 23, true},
		{"Sabtu penuh", time.Saturday, 10, true},
		{"Minggu penuh", time.Sunday, 10, true},
		{"Rollover 22:00 UTC", time.Tuesday, 22, true},
		{"Rollover 00:30 UTC", time.Tuesday, 0, true},
		{"Rollover selesai 01:00 UTC", time.Tuesday, 1, false},
	}

	for _, c := range cases {
		if got := market.IsClosed(c.day, c.hour); got != c.want {
			t.Errorf("%s: market.IsClosed(%v, %d) = %v, want %v", c.name, c.day, c.hour, got, c.want)
		}
	}
}

// Jumat 20:00 UTC = Sabtu 03:00 WIB, belum penutupan pasar. Gate tidak boleh menutup di sini.
func TestFridayGateNotTooEarly(t *testing.T) {
	if market.IsClosed(time.Friday, 20) {
		t.Fatal("Jumat 20:00 UTC masih sesi New York; gate tidak boleh menutup pasar")
	}
}
