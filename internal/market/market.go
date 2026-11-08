// Package market menyatukan jam pasar yang dipakai seluruh engine.
//
// Sebelumnya aturan yang sama disalin di cmd/scanner dan cmd/ila3-scanner.
// Salinan ganda membuat ILA 2.0 dan ILA 3.0 bisa berperilaku berbeda bila
// salah satunya diubah. Di sini hanya ada SATU sumber kebenaran.
package market

import "time"

// State adalah status pasar yang dikirim ke Runner lewat kontrak gRPC.
type State string

const (
	Open             State = "OPEN"
	Closed           State = "CLOSED"
	RolloverBlackout State = "ROLLOVER_BLACKOUT"
)

// Nama sesi. Nilai ini dipakai di notifikasi dan dashboard.
const (
	SessionAsian    = "ASIAN_SESSION"
	SessionLondon   = "LONDON_SESSION"
	SessionOverlap  = "LONDON_NEWYORK_OVERLAP"
	SessionNewYork  = "NEWYORK_SESSION"
	SessionRollover = "ROLLOVER_BLACKOUT"
	SessionClosed   = "WEEKEND_CLOSED"
)

// IsClosed mengikuti aturan yang sudah dipakai engine sejak awal:
// Sabtu/Minggu penuh, Jumat >= 21:00 UTC, dan rollover 22:00-01:00 UTC.
//
// Jam sengaja dalam UTC karena pasar berjalan dengan jam New York; WIB hanya
// untuk menampilkan ke pengguna.
func IsClosed(utcDay time.Weekday, utcHour int) bool {
	if utcDay == time.Saturday || utcDay == time.Sunday {
		return true
	}
	// Jumat >= 21:00 UTC: margin 1 jam sebelum penutupan akhir pekan,
	// menghindari spread melebar tanpa memotong sesi New York.
	if utcDay == time.Friday && utcHour >= 21 {
		return true
	}
	// Rollover subuh: 22:00 - 01:00 UTC (05:00 - 08:00 WIB).
	if utcHour >= 22 || utcHour < 1 {
		return true
	}
	return false
}

// IsRollover menandai jam rollover tanpa menyatakan pasar tutup penuh.
func IsRollover(utcHour int) bool {
	return utcHour >= 22 || utcHour < 1
}

// IsWeekend menandai Sabtu/Minggu penuh.
func IsWeekend(utcDay time.Weekday) bool {
	return utcDay == time.Saturday || utcDay == time.Sunday
}

// Session menentukan nama sesi. Bila pasar tutup, sesi selalu libur agar
// briefing sesi tidak terkirim saat seharusnya diam.
func Session(utcDay time.Weekday, utcHour int) string {
	switch {
	case IsClosed(utcDay, utcHour):
		return SessionClosed
	case IsRollover(utcHour):
		return SessionRollover
	case utcHour >= 12 && utcHour < 17:
		return SessionOverlap
	case utcHour >= 7 && utcHour < 12:
		return SessionLondon
	case utcHour >= 17 && utcHour < 22:
		return SessionNewYork
	default:
		return SessionAsian
	}
}

// StateOf menerjemahkan jam UTC menjadi status pasar untuk kontrak sinyal.
// Rollover dibedakan dari tutup akhir pekan agar Runner bisa membedakan
// "pasar ada tapi jeda" dari "pasar libur".
func StateOf(utcDay time.Weekday, utcHour int) State {
	if IsWeekend(utcDay) || (utcDay == time.Friday && utcHour >= 21) {
		return Closed
	}
	if IsRollover(utcHour) {
		return RolloverBlackout
	}
	return Open
}

// IsActiveSession menandai sesi yang boleh mengirim briefing.
func IsActiveSession(session string) bool {
	switch session {
	case SessionAsian, SessionLondon, SessionOverlap, SessionNewYork:
		return true
	default:
		return false
	}
}
