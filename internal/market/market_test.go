package market

import (
	"testing"
	"time"
)

// Tabel ini mengunci aturan yang sudah dipakai engine sebelum dijadikan paket.
// Perubahan satu sel akan langsung ketahuan.
func TestIsClosed(t *testing.T) {
	cases := []struct {
		day  time.Weekday
		hour int
		want bool
	}{
		{time.Monday, 0, true},    // rollover
		{time.Monday, 1, false},   // buka
		{time.Monday, 10, false},  // sesi London
		{time.Monday, 21, false},  // sesi NY
		{time.Monday, 22, true},   // rollover
		{time.Monday, 23, true},   // rollover
		{time.Friday, 20, false},  // sesi NY masih jalan
		{time.Friday, 21, true},   // margin sebelum tutup
		{time.Friday, 23, true},   // tutup
		{time.Saturday, 10, true}, // libur
		{time.Sunday, 10, true},   // libur
	}
	for _, c := range cases {
		if got := IsClosed(c.day, c.hour); got != c.want {
			t.Errorf("IsClosed(%s, %d) = %v, ingin %v", c.day, c.hour, got, c.want)
		}
	}
}

// Sesi tidak boleh lagi menjadi sesi aktif saat pasar tutup.
func TestSessionSaatTutup(t *testing.T) {
	if got := Session(time.Friday, 21); got != SessionClosed {
		t.Fatalf("Session(Friday,21) = %q, ingin %q", got, SessionClosed)
	}
	if IsActiveSession(SessionClosed) {
		t.Fatal("sesi libur tidak boleh dianggap aktif")
	}
}

func TestSessionNama(t *testing.T) {
	cases := []struct {
		hour int
		want string
	}{
		{2, SessionAsian},
		{9, SessionLondon},
		{13, SessionOverlap},
		{18, SessionNewYork},
	}
	for _, c := range cases {
		if got := Session(time.Wednesday, c.hour); got != c.want {
			t.Errorf("Session(Wednesday, %d) = %q, ingin %q", c.hour, got, c.want)
		}
	}
}

func TestStateOf(t *testing.T) {
	cases := []struct {
		day  time.Weekday
		hour int
		want State
	}{
		{time.Wednesday, 10, Open},
		{time.Wednesday, 23, RolloverBlackout},
		{time.Saturday, 10, Closed},
		{time.Friday, 22, Closed}, // akhir pekan menang atas rollover
	}
	for _, c := range cases {
		if got := StateOf(c.day, c.hour); got != c.want {
			t.Errorf("StateOf(%s, %d) = %q, ingin %q", c.day, c.hour, got, c.want)
		}
	}
}
