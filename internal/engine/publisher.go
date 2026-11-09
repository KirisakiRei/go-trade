package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ila-engine/internal/market"
	"ila-engine/internal/signal"
	"ila-engine/pkg/ila"
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
	// gagalPada mencatat kapan slot terakhir gagal, untuk membatasi log.
	gagalPada map[string]time.Time

	// abaikanJamPasar hanya untuk pengujian: memaksa engine mengevaluasi
	// walau pasar tutup. Tidak pernah aktif di produksi.
	abaikanJamPasar  bool
	pakaiJadwalLilin bool
	bufferLilin      time.Duration
}

// slotState mencatat slot sinyal beserta identitas simbolnya, supaya sinyal
// kedaluwarsa bisa dikirim lengkap ke Runner.
type slotState struct {
	slot            signal.Slot
	sym             string
	htf             string
	ltf             string
	lastStatus      signal.Status
	lastEvaluatedAt time.Time
}

// NewPublisher merangkai evaluator dengan registry pelanggan.
func NewPublisher(eval Evaluator, reg *Registry, version Version, log *slog.Logger) *Publisher {
	return &Publisher{
		eval:      eval,
		reg:       reg,
		version:   version,
		log:       log,
		now:       func() time.Time { return time.Now().UTC() },
		slots:     make(map[string]*slotState),
		gagalPada: make(map[string]time.Time),
	}
}

// AbaikanJamPasar mematikan pemeriksaan jam pasar. Hanya untuk pengujian
// alur sinyal di luar jam pasar; tidak boleh dipakai di produksi.
func (p *Publisher) AbaikanJamPasar() *Publisher {
	p.abaikanJamPasar = true
	return p
}

// DenganJadwalLilin menyalakan evaluasi berbasis penutupan lilin.
func (p *Publisher) DenganJadwalLilin(buffer time.Duration) *Publisher {
	p.pakaiJadwalLilin = true
	p.bufferLilin = buffer
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
		for _, pair := range p.pairsFor(symbol) {
			key := signal.Key(symbol, pair.HTF, pair.LTF)
			p.mu.Lock()
			st := p.slots[key]
			due := p.slotDue(st, pair, now)
			p.mu.Unlock()

			if due {
				p.evaluateSlot(ctx, symbol, pair, now)
			}
		}
	}

	p.expireAll(now)
}

// pairsFor menentukan pasangan timeframe yang dihitung untuk satu simbol.
//
// Aturan trading tetap pakai: pasangan yang boleh dievaluasi hanya yang ada di
// daftar evaluator. Permintaan pelanggan HANYA menyaring, tidak pernah
// menambah. Runner yang meminta pasangan di luar aturan ILA tidak akan membuat
// engine mengarang setup yang tidak sah.
func (p *Publisher) pairsFor(symbol string) []Pair {
	semua := p.eval.Pairs()
	diminta, dibatasi := p.reg.WantedPairs(symbol)
	if !dibatasi {
		return semua
	}

	sah := make(map[Pair]struct{}, len(semua))
	for _, pair := range semua {
		sah[pair] = struct{}{}
	}

	out := make([]Pair, 0, len(diminta))
	for _, pair := range diminta {
		if _, ok := sah[pair]; ok {
			out = append(out, pair)
		}
	}
	if len(out) == 0 {
		// Semua permintaan di luar aturan: jangan mengevaluasi apa pun,
		// tetapi juga jangan diam-diam kembali ke empat pasangan — itu
		// membuat pemfilteran tampak tidak berfungsi.
		return nil
	}
	return out
}

// evaluateSlot mengevaluasi satu slot lalu menerbitkan hasilnya.
func (p *Publisher) evaluateSlot(ctx context.Context, symbol string, pair Pair, now time.Time) {
	res, err := p.eval.Evaluate(symbol, pair.HTF, pair.LTF)
	if err != nil {
		// Gagal mengambil data bukan sinyal. Dicatat paling sering sekali per
		// menit per slot: simbol yang salah ketik akan gagal terus dan tanpa
		// batas ini log engine tenggelam.
		p.logGagal(symbol, pair, err)
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
	st.lastEvaluatedAt = now
	st.lastStatus = status

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
		p.broadcastStatus(symbol, pair, status, now, res)
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

// logGagal mencatat kegagalan evaluasi dengan jeda agar tidak membanjiri log.
func (p *Publisher) logGagal(symbol string, pair Pair, err error) {
	key := signal.Key(symbol, pair.HTF, pair.LTF)
	now := p.now()

	p.mu.Lock()
	terakhir, ada := p.gagalPada[key]
	if ada && now.Sub(terakhir) < time.Minute {
		p.mu.Unlock()
		return
	}
	p.gagalPada[key] = now
	p.mu.Unlock()

	p.log.Warn("evaluasi gagal",
		"symbol", symbol, "htf", pair.HTF, "ltf", pair.LTF, "error", err.Error())
}

// broadcastStatus mengirim status menunggu dengan data struktur pendukung.
func (p *Publisher) broadcastStatus(symbol string, pair Pair, status signal.Status, now time.Time, res *Result) {
	dir := "NEUTRAL"
	var summary, nextAction string
	var htfSweep *ila.HTFSweep
	var cisd *ila.CISD

	if res != nil {
		if res.Direction != "" {
			dir = res.Direction
		}
		summary = res.Summary
		nextAction = res.NextAction
		htfSweep = res.HTFSweep
		cisd = res.CISD
	}

	p.reg.Broadcast(Signal{
		ID:          statusID(symbol, pair),
		Symbol:      symbol,
		HTF:         pair.HTF,
		LTF:         pair.LTF,
		Status:      status,
		Action:      signal.ActionPublish,
		Direction:   dir,
		Summary:     summary,
		NextAction:  nextAction,
		HTFSweep:    htfSweep,
		CISD:        cisd,
		PublishedAt: now,
		ExpiresAt:   now.Add(signal.TTLUntuk(pair.LTF)),
	})
}

func (p *Publisher) slotDue(st *slotState, pair Pair, now time.Time) bool {
	if !p.pakaiJadwalLilin {
		return true
	}
	if st == nil || st.lastEvaluatedAt.IsZero() {
		return true
	}

	tf := pair.HTF
	if st.lastStatus == signal.WaitLTFCISD {
		tf = pair.LTF
	}

	return isCandleClosedSince(tf, st.lastEvaluatedAt, now, p.bufferLilin)
}

func isCandleClosedSince(tf string, lastEval, now time.Time, buffer time.Duration) bool {
	tClose := latestCandleClose(tf, now.Add(-buffer))
	if tClose.IsZero() {
		return true
	}
	tReady := tClose.Add(buffer)
	if now.Before(tReady) {
		return false
	}
	return lastEval.Before(tReady)
}

func latestCandleClose(tf string, t time.Time) time.Time {
	t = t.UTC()
	switch tf {
	case "M5", "5m":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), (t.Minute()/5)*5, 0, 0, time.UTC)
	case "M15", "15m":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), (t.Minute()/15)*15, 0, 0, time.UTC)
	case "H1", "1H", "1h":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case "H4", "4H", "4h":
		return time.Date(t.Year(), t.Month(), t.Day(), (t.Hour()/4)*4, 0, 0, 0, time.UTC)
	case "D", "D1", "1D", "1d":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case "W", "W1", "1W", "1w":
		days := (int(t.Weekday()) + 6) % 7
		return time.Date(t.Year(), t.Month(), t.Day()-days, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
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
		out.HTFSweep = res.HTFSweep
		out.CISD = res.CISD
	}
	return out
}
