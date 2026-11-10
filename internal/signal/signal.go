// Package signal mengurus masa hidup sinyal yang sudah diterbitkan engine.
//
// Ini BUKAN aturan trading. Isinya hanya tiga hal: berapa lama sinyal berlaku
// (TTL), kapan sinyal ditarik, dan sinyal mana yang digantikan sinyal baru.
// Penentuan entry/SL/TP tetap milik pkg/ila dan pkg/sequence.
//
// Semua status di sini sengaja sama dengan kontrak di plans/07 sehingga Runner
// bisa memakainya apa adanya.
package signal

import (
	"strings"
	"time"
)

// Status adalah status sinyal pada kontrak keluar.
type Status string

const (
	// Status menunggu. Hanya untuk tampilan, tidak boleh dieksekusi.
	WaitHTFSweep       Status = "WAIT_HTF_SWEEP"
	WaitHTFCandleClose Status = "WAIT_HTF_CANDLE_CLOSE"
	WaitLTFCISD        Status = "WAIT_LTF_CISD"

	// Ready adalah satu-satunya status yang boleh menghasilkan order.
	Ready Status = "READY_TO_ENTRY"

	// Invalidated adalah status internal: strukturnya batal sehingga sinyal
	// yang sudah terbit harus ditarik. Tidak pernah dikirim ke Runner sebagai
	// status sinyal, melainkan sebagai aksi CANCEL.
	Invalidated Status = "INVALIDATED"

	// Status ILA 3.0. Namanya sengaja sama dengan enum di proto agar tidak ada
	// penerjemahan tambahan di Runner. Semuanya hanya untuk tampilan kemajuan;
	// satu-satunya status yang boleh dieksekusi tetap Ready (READY_ENTRY_1 dan
	// READY_ENTRY_2 milik ILA 3.0 dipetakan ke Ready karena keduanya adalah
	// titik entry yang sah).
	WaitILA3HTFKeyLevel  Status = "WAIT_ILA3_HTF_KEY_LEVEL"
	WaitILA3MTFReversal  Status = "WAIT_ILA3_MTF_REVERSAL"
	WaitILA3LTFCISD1     Status = "WAIT_ILA3_LTF_CISD_1"
	ILA3Expansion1Active Status = "ILA3_EXPANSION_1_ACTIVE"
	WaitILA3MTFFVGRetr   Status = "WAIT_ILA3_MTF_FVG_RETR"
	WaitILA3LTFCISD2     Status = "WAIT_ILA3_LTF_CISD_2"
	ILA3Expansion2Active Status = "ILA3_EXPANSION_2_ACTIVE"
	ILA3TargetReached    Status = "ILA3_TARGET_REACHED"
)

// Action adalah jenis perubahan yang dikirim ke Runner.
type Action string

const (
	ActionPublish   Action = "PUBLISH"
	ActionCancel    Action = "CANCEL"
	ActionExpire    Action = "EXPIRE"
	ActionSupersede Action = "SUPERSEDE"
)

// TTLUntuk mengembalikan masa berlaku satu sinyal.
//
// Satu candle LTF (plans/04 §6): bila entry tidak terjadi dalam satu candle
// LTF, setup sudah tidak relevan. Penulisan timeframe dinormalkan karena ILA
// 2.0 memakai bentuk H4/M15 sedangkan ILA 3.0 memakai 4H/15m.
//
// Nilai tak dikenal memakai 15 menit: lebih baik sinyal mati cepat daripada
// hidup terlalu lama lalu dieksekusi pada harga yang sudah jauh.
func TTLUntuk(ltf string) time.Duration {
	switch strings.ToUpper(strings.TrimSpace(ltf)) {
	case "M5", "5M":
		return 5 * time.Minute
	case "M15", "15M":
		return 15 * time.Minute
	case "M30", "30M":
		return 30 * time.Minute
	case "H1", "1H":
		return time.Hour
	case "H4", "4H":
		return 4 * time.Hour
	case "D", "D1", "1D":
		return 24 * time.Hour
	case "W", "W1", "1W":
		return 7 * 24 * time.Hour
	default:
		return 15 * time.Minute
	}
}

// Signal adalah sinyal yang sedang dipegang engine.
type Signal struct {
	ID          string
	Symbol      string
	HTF         string
	LTF         string
	Status      Status
	Direction   string
	Entry       float64
	StopLoss    float64
	TakeProfit  float64
	RiskReward  float64
	Summary     string
	NextAction  string
	PublishedAt time.Time
	ExpiresAt   time.Time
}

// Key mengidentifikasi slot satu setup. Satu slot hanya boleh punya satu sinyal
// aktif: sinyal baru pada slot yang sama menggantikan yang lama.
func Key(symbol, htf, ltf string) string {
	return symbol + "|" + htf + "|" + ltf
}

// Slot menerbitkan dan menutup sinyal untuk satu (symbol, htf, ltf).
type Slot struct {
	active *Signal
}

// Active mengembalikan sinyal yang sedang berlaku, atau nil.
func (s *Slot) Active() *Signal {
	return s.active
}

// Publish menerbitkan sinyal baru.
//
// Bila masih ada sinyal ready pada slot yang sama, sinyal lama dinyatakan
// SUPERSEDED: harga sudah bergerak, jadi entry lama tidak boleh dipakai lagi.
func (s *Slot) Publish(sig Signal) (Action, string) {
	previous := ""
	if s.active != nil && s.active.Status == Ready {
		previous = s.active.ID
	}
	s.active = &sig
	if previous != "" {
		return ActionSupersede, previous
	}
	return ActionPublish, ""
}

// Invalidate menarik sinyal aktif karena strukturnya batal.
// Sinyal wait tidak pernah ditarik: belum pernah diterbitkan ke Runner.
func (s *Slot) Invalidate() (id string, ok bool) {
	if s.active == nil || s.active.Status != Ready {
		return "", false
	}
	id = s.active.ID
	s.active = nil
	return id, true
}

// Expire menarik sinyal aktif bila masa berlakunya sudah lewat.
func (s *Slot) Expire(now time.Time) (id string, ok bool) {
	if s.active == nil || s.active.Status != Ready {
		return "", false
	}
	if now.Before(s.active.ExpiresAt) {
		return "", false
	}
	id = s.active.ID
	s.active = nil
	return id, true
}

// Ready berarti sinyal aktif masih boleh dieksekusi.
func (s *Slot) Ready(now time.Time) bool {
	if s.active == nil || s.active.Status != Ready {
		return false
	}
	return now.Before(s.active.ExpiresAt)
}
