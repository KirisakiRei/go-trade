package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ila-engine/internal/market"
	"ila-engine/internal/signal"
)

// Publisher menerbitkan sinyal untuk satu strategi.
//
// Ia memegang satu slot per (symbol, htf, ltf): satu setup hanya boleh punya
// satu sinyal aktif. Slot inilah yang membuat engine bisa mengirim CANCELLED,
// EXPIRED, dan SUPERSEDED tanpa menyimpan posisi atau hasil trading.
type Publisher struct {
	eval    Evaluator
	reg     *Registry
	version Version
	log     *slog.Logger
	now     func() time.Time

	mu    sync.Mutex
	seq   int64
	slots map[string]*slotState

	// abaikanJamPasar hanya untuk pengujian: memaksa engine mengevaluasi
	// walau pasar tutup. Tidak pernah aktif di produksi.
	abaikanJamPasar bool
}

// slotState mencatat slot sinyal beserta identitas simbolnya, supaya sinyal
// kedaluwarsa bisa dikirim lengkap ke Runner.
type slotState struct {
	slot signal.Slot
	sym  string
	htf  string
	ltf  string
}

// NewPublisher merangkai evaluator dengan registry pelanggan.
func NewPublisher(eval Evaluator, reg *Registry, version Version, log *slog.Logger) *Publisher {
	return &Publisher{
		eval:    eval,
		reg:     reg,
		version: version,
		log:     log,
		now:     func() time.Time { return time.Now().UTC() },
		slots:   make(map[string]*slotState),
	}
}

// AbaikanJamPasar mematikan pemeriksaan jam pasar. Hanya untuk pengujian
// alur sinyal di luar jam pasar; tidak boleh dipakai di produksi.
func (p *Publisher) AbaikanJamPasar() *Publisher {
	p.abaikanJamPasar = true
	return p
}

// WithClock memakai jam buatan pada pengujian.
func (p *Publisher) WithClock(now func() time.Time) *Publisher {
	p.now = now
	return p
}

// Scan mengevaluasi semua simbol yang diminta pelanggan lalu menerbitkan
// perubahan sinyal. Aman dipanggil berulang.
func (p *Publisher) Scan(ctx context.Context) {
	now := p.now()

	symbols := p.reg.Symbols()
	if len(symbols) == 0 {
		// Tidak ada permintaan berarti tidak ada yang perlu dievaluasi.
		return
	}

	if !p.abaikanJamPasar && market.StateOf(now.Weekday(), now.Hour()) != market.Open {
		// Pasar tutup: tidak ada evaluasi baru. Sinyal yang sudah terbit
		// dibiarkan sampai TTL-nya habis.
		p.expireAll(now)
		return
	}

	for _, symbol := range symbols {
		for _, pair := range p.eval.Pairs() {
			p.evaluateSlot(ctx, symbol, pair, now)
		}
	}

	p.expireAll(now)
}

// evaluateSlot mengevaluasi satu slot lalu menerbitkan hasilnya.
func (p *Publisher) evaluateSlot(ctx context.Context, symbol string, pair Pair, now time.Time) {
	res, err := p.eval.Evaluate(symbol, pair.HTF, pair.LTF)
	if err != nil {
		// Gagal mengambil data bukan sinyal; cukup dicatat.
		p.log.Warn("evaluasi gagal",
			"symbol", symbol, "htf", pair.HTF, "ltf", pair.LTF, "error", err.Error())
		return
	}
	if res == nil {
		return
	}

	status := mapStatus(p.eval.StrategyVersion(), res.Status)
	key := signal.Key(symbol, pair.HTF, pair.LTF)

	p.mu.Lock()
	st := p.slots[key]
	if st == nil {
		st = &slotState{sym: symbol, htf: pair.HTF, ltf: pair.LTF}
		p.slots[key] = st
	}

	// Struktur batal: tarik sinyal yang sudah terbit.
	if status == signal.Invalidated {
		if id, pulled := st.slot.Invalidate(); pulled {
			p.mu.Unlock()
			p.broadcast(cancelled(id, st, now, "struktur setup batal"))
			return
		}
		p.mu.Unlock()
		return
	}

	// Status menunggu hanya untuk tampilan: dikirim sebagai status agar
	// dashboard bisa menunjukkan kemajuan, tetap tanpa harga dan tanpa izin
	// eksekusi (plans/04 §5).
	if status != signal.Ready {
		if !p.reg.WantsStatus(symbol) {
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		p.broadcastStatus(symbol, pair, status, now)
		return
	}

	// Arah wajib jelas: tanpa BUY/SELL, Runner menolak sinyal dan menyisakan
	// baris mati di database.
	if res.Direction != "BUY" && res.Direction != "SELL" {
		p.log.Warn("setup siap tanpa arah jelas, dilewati",
			"symbol", symbol, "ltf", pair.LTF, "bias", res.Direction)
		p.mu.Unlock()
		return
	}

	if res.Entry <= 0 || res.StopLoss <= 0 || res.TakeProfit <= 0 {
		// Angka tidak lengkap tidak boleh terbit: Runner akan menolaknya.
		p.log.Warn("setup siap tanpa harga lengkap, dilewati",
			"symbol", symbol, "ltf", pair.LTF)
		p.mu.Unlock()
		return
	}

	sig := signal.Signal{
		ID:          p.nextID(symbol, pair, now),
		Symbol:      symbol,
		HTF:         pair.HTF,
		LTF:         pair.LTF,
		Status:      signal.Ready,
		Direction:   res.Direction,
		Entry:       res.Entry,
		StopLoss:    res.StopLoss,
		TakeProfit:  res.TakeProfit,
		RiskReward:  res.RiskReward,
		Summary:     res.Summary,
		NextAction:  res.NextAction,
		PublishedAt: now,
		ExpiresAt:   now.Add(signal.TTLUntuk(pair.LTF)),
	}

	action, previous := st.slot.Publish(sig)
	p.mu.Unlock()

	if previous != "" {
		p.broadcast(superseded(previous, st, now))
	}
	p.broadcast(p.toOut(sig, action, res))
}

// expireAll menarik sinyal yang lewat masa berlaku.
func (p *Publisher) expireAll(now time.Time) {
	p.mu.Lock()
	type expired struct {
		id string
		st *slotState
	}
	var list []expired
	for _, st := range p.slots {
		if id, ok := st.slot.Expire(now); ok {
			list = append(list, expired{id: id, st: st})
		}
	}
	p.mu.Unlock()

	for _, e := range list {
		p.broadcast(withAction(cancelled(e.id, e.st, now, "masa berlaku habis"), signal.ActionExpire))
	}
}

// Snapshot mengembalikan sinyal yang masih berlaku untuk simbol tertentu.
// Dipakai Runner yang baru terhubung agar tidak buta (plans/04 §8).
func (p *Publisher) Snapshot(symbols []string) []Signal {
	now := p.now()
	wanted := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		wanted[s] = struct{}{}
	}
	all := len(wanted) == 0

	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]Signal, 0, len(p.slots))
	for _, st := range p.slots {
		active := st.slot.Active()
		if active == nil || !st.slot.Ready(now) {
			continue
		}
		if !all {
			if _, ok := wanted[active.Symbol]; !ok {
				continue
			}
		}
		out = append(out, p.toOut(*active, signal.ActionPublish, nil))
	}
	return out
}

// ActiveCount menghitung sinyal yang masih hidup.
func (p *Publisher) ActiveCount() int {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, st := range p.slots {
		if st.slot.Ready(now) {
			n++
		}
	}
	return n
}

// broadcastStatus mengirim status menunggu. Tidak pernah membawa harga dan
// tidak boleh dieksekusi Runner.
//
// ID status sengaja stabil per slot, bukan unik tiap scan: Runner menyimpan
// setiap sinyal yang diterimanya, jadi ID baru tiap menit akan membuat tabel
// sinyal membengkak tanpa batas.
func (p *Publisher) broadcastStatus(symbol string, pair Pair, status signal.Status, now time.Time) {
	p.reg.Broadcast(Signal{
		ID:          statusID(symbol, pair),
		Symbol:      symbol,
		HTF:         pair.HTF,
		LTF:         pair.LTF,
		Status:      status,
		Action:      signal.ActionPublish,
		PublishedAt: now,
	})
}

// statusID membentuk identitas tetap untuk status menunggu satu slot.
func statusID(symbol string, pair Pair) string {
	return "status-" + symbol + "-" + pair.HTF + "-" + pair.LTF
}

func (p *Publisher) broadcast(sig Signal) {
	if sig.ID == "" {
		return
	}
	p.reg.Broadcast(sig)
}

// nextID menambahkan nomor urut karena dua scan bisa terjadi pada nanodetik
// yang sama saat jam dipalsukan atau sistem sangat cepat.
func (p *Publisher) nextID(symbol string, pair Pair, now time.Time) string {
	p.seq++
	return fmt.Sprintf("sig-%s-%s-%s-%d-%d", symbol, pair.HTF, pair.LTF, now.UnixNano(), p.seq)
}

func withAction(sig Signal, action signal.Action) Signal {
	sig.Action = action
	return sig
}

func cancelled(id string, st *slotState, now time.Time, reason string) Signal {
	return Signal{
		ID: id, Symbol: st.sym, HTF: st.htf, LTF: st.ltf,
		Action: signal.ActionCancel, Reason: reason, PublishedAt: now,
	}
}

func superseded(id string, st *slotState, now time.Time) Signal {
	return Signal{
		ID: id, Symbol: st.sym, HTF: st.htf, LTF: st.ltf,
		Action: signal.ActionSupersede, Reason: "digantikan sinyal baru", PublishedAt: now,
	}
}

func (p *Publisher) toOut(sig signal.Signal, action signal.Action, res *Result) Signal {
	out := Signal{
		ID: sig.ID, Symbol: sig.Symbol, HTF: sig.HTF, LTF: sig.LTF,
		Status: sig.Status, Action: action, Direction: sig.Direction,
		Entry: sig.Entry, StopLoss: sig.StopLoss, TakeProfit: sig.TakeProfit,
		RiskReward: sig.RiskReward, Summary: sig.Summary, NextAction: sig.NextAction,
		PublishedAt: sig.PublishedAt, ExpiresAt: sig.ExpiresAt,
	}
	if res != nil {
		out.PassedMinRR = res.PassedMinRR
		out.SetupType = res.SetupType
		out.SequenceState = res.SequenceState
		out.CriticalZoneTP = res.CriticalZoneTP
		out.FinalZoneTP = res.FinalZoneTP
		out.QuarterInfo = res.QuarterInfo
		out.SSMTResult = res.SSMTResult
	}
	return out
}
