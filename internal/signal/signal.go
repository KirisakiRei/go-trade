// Package signal mengurus masa hidup sinyal yang sudah diterbitkan engine.
//
// Ini BUKAN aturan trading. Isinya hanya tiga hal: berapa lama sinyal berlaku
// (TTL), kapan sinyal ditarik, dan sinyal mana yang digantikan sinyal baru.
// Penentuan entry/SL/TP tetap milik pkg/ila dan pkg/sequence.
//
// Semua status di sini sengaja sama dengan kontrak di plans/07 sehingga Runner
// bisa memakainya apa adanya.
package signal

import "time"

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
)

// Action adalah jenis perubahan yang dikirim ke Runner.
type Action string

const (
	ActionPublish   Action = "PUBLISH"
	ActionCancel    Action = "CANCEL"
	ActionExpire    Action = "EXPIRE"
	ActionSupersede Action = "SUPERSEDE"
)

// ttlByLTF: satu candle LTF (plans/04 §6). Bila entry tidak terjadi dalam satu
// candle LTF, setup sudah tidak relevan.
var ttlByLTF = map[string]time.Duration{
	"H4":  4 * time.Hour,
	"H1":  time.Hour,
	"M15": 15 * time.Minute,
	"M5":  5 * time.Minute,
}

// TTLUntuk mengembalikan masa berlaku satu sinyal.
//
// Nilai tak dikenal memakai 15 menit: lebih baik sinyal mati cepat daripada
// hidup terlalu lama lalu dieksekusi pada harga yang sudah jauh.
func TTLUntuk(ltf string) time.Duration {
	if d, ok := ttlByLTF[ltf]; ok {
		return d
	}
	return 15 * time.Minute
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
