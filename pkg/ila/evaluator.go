package ila

// Orkestrator alur entry New ILA 2.0. Sumber aturan: "New ILA 2.0/alur-entry.md" dan "time-cycle.md".
//
//	1. HTF: key level ERL/IRL terbaru dapat C2 closure aktif (di dalam cycle)
//	2. C3 : candle HTF tepat setelah C2 aktif, di dalam cycle; entry hanya dicari selama C3 berjalan
//	3. LTF: -CISD / +CISD dengan key level wajib ERL (cisd.go)
//	4. Entry = close candle konfirmasi, SL = puncak/dasar rangkaian, Target = RR 1:2

import (
	"fmt"
	"math"
	"time"

	"ila-engine/pkg/indicators"
)

// Evaluator mengevaluasi satu pasangan HTF -> LTF. Tidak menyimpan state.
type Evaluator struct{}

func NewEvaluator() *Evaluator { return &Evaluator{} }

func formatCandleDateTime(t time.Time) string {
	return t.In(nyLocation).Format("02 Jan 15:04 NY")
}

func formatILAVal(v float64) string {
	if v < 50 {
		return fmt.Sprintf("%.5f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// htfCand adalah satu key level HTF terbaru beserta hasil C2 closure-nya.
type htfCand struct {
	kind KeyLevelKind
	fvg  *FVG
	sw   *SwingERL
	res  C2Result
	dir  string
	name string
}

// outcome adalah hasil evaluasi satu kandidat; rank terbesar yang dilaporkan.
type outcome struct {
	rank           int
	status         string
	note           string
	c3Start, c3End time.Time
	cisd           *CISD
}

const (
	rankNone = iota
	rankOffCycleOrStale
	rankC3Closed
	rankC3NotStarted
	rankC2Running
	rankLiveWaiting
	rankLiveReady
)

// EvaluateILA menjalankan alur lengkap New ILA 2.0. Cycle ditentukan dari HTF
// (pasangan HTF -> LTF di time-cycle.md bersifat satu-satu).
func (e *Evaluator) EvaluateILA(htf, ltf []indicators.Candle, htfName, ltfName, symbol string) *ILASetup {
	s := &ILASetup{Symbol: symbol, HTF: htfName, LTF: ltfName, Bias: DirNeutral,
		Status: StatusWaitC2, EvaluatedAt: time.Now().UTC()}
	wait := func(msg string) *ILASetup {
		s.NextAction, s.Summary = "⏳ "+msg, msg
		return s
	}

	cycle, ok := CycleForHTF(htfName)
	if !ok {
		return wait(fmt.Sprintf("HTF %s tidak ada di tabel time cycle New ILA 2.0, LTF tidak boleh ditebak.", htfName))
	}
	s.Cycle = string(cycle)
	htfK, _ := ParseTimeframe(htfName)
	ltfK, ok := ParseTimeframe(ltfName)
	if !ok || ltfK != CycleSpec[cycle][1] {
		return wait(fmt.Sprintf("LTF %s tidak sesuai pasangan %s cycle %s (seharusnya %s).", ltfName, htfName, cycle, CycleSpec[cycle][1]))
	}

	var best outcome
	var bestCand htfCand
	liveDirByC2 := map[int]string{}
	conflictC2 := -1
	for _, c := range htfCandidates(htf) {
		o := evalCand(c, htf, ltf, cycle, htfK, ltfK)
		if o.rank >= rankLiveWaiting {
			if prev, exists := liveDirByC2[c.res.C2Index]; exists && prev != c.dir {
				conflictC2 = c.res.C2Index
			}
			liveDirByC2[c.res.C2Index] = c.dir
		}
		if o.rank > best.rank {
			best, bestCand = o, c
		}
	}
	if best.rank == rankNone {
		return wait(fmt.Sprintf("Belum ada C2 closure aktif pada key level (ERL/IRL) di %s.", htfName))
	}
	if best.rank >= rankLiveWaiting && bestCand.res.C2Index == conflictC2 {
		return wait(fmt.Sprintf("Konflik arah HTF %s: candle C2 membentuk C2 closure bearish dan bullish sekaligus, tidak memaksa entry.", htfName))
	}

	s.Bias, s.Status, s.CISD = bestCand.dir, best.status, best.cisd
	s.HTFSweep = buildSweep(bestCand, best, htf, cycle)
	s.HTFSweep.SweepInProgress = best.rank == rankC2Running
	s.HTFSweep.Detected = best.rank >= rankC3Closed && best.rank != rankC2Running
	s.HTFSweep.Details = fmt.Sprintf("%s %s: %s", bestCand.name, htfName, bestCand.res.Detail)

	switch best.status {
	case StatusReady:
		planPosition(s)
		s.NextAction = "🎯 SIAP EKSEKUSI: " + best.cisd.Details
		s.Summary = fmt.Sprintf("💎 SETUP NEW ILA 2.0 (%s ➡️ %s, cycle %s): %s | Entry=%s SL=%s TP=%s (RR 1:%.0f)",
			htfName, ltfName, cycle, best.cisd.Details, formatILAVal(s.EntryPrice), formatILAVal(s.StopLoss),
			formatILAVal(s.TakeProfit), s.RiskReward)
	case StatusWaitCISD:
		s.NextAction = fmt.Sprintf("⏳ C3 %s berjalan di dalam cycle %s, menunggu CISD di %s.", htfName, cycle, ltfName)
		if best.cisd != nil {
			s.NextAction += " " + best.cisd.Details
		}
		s.Summary = s.NextAction
	case StatusInvalid:
		s.NextAction = fmt.Sprintf("🛑 C3 %s sudah close tanpa setup, setup invalid.", htfName)
		s.Summary = s.NextAction
	default:
		s.NextAction = "⏳ " + best.note
		s.Summary = s.HTFSweep.Details + ". " + best.note
	}
	return s
}

// htfCandidates mengambil key level sejenis TERBARU (ERL dan IRL) yang masih
// punya peluang C2: C2 aktif, atau candle berjalan sedang menyentuh acuan.
func htfCandidates(htf []indicators.Candle) []htfCand {
	var out []htfCand
	add := func(c htfCand) {
		if c.res.Status == C2Active || c.res.RunningTouch {
			out = append(out, c)
		}
	}
	fvgs, swings := FindIRL(htf), FindERL(htf)
	for _, t := range []FVGType{FVGSIBI, FVGBISI} {
		if f := LatestIRL(fvgs, t); f != nil {
			dir := DirBullish
			if t == FVGSIBI {
				dir = DirBearish
			}
			add(htfCand{kind: KeyLevelIRL, fvg: f, res: EvaluateC2IRL(*f, htf), dir: dir, name: string(t)})
		}
	}
	for _, t := range []ERLType{ERLSwingHigh, ERLSwingLow} {
		if sw := LatestERL(swings, t); sw != nil {
			dir, name := DirBullish, "Swing Low"
			if t == ERLSwingHigh {
				dir, name = DirBearish, "Swing High"
			}
			add(htfCand{kind: KeyLevelERL, sw: sw, res: EvaluateC2ERL(*sw, htf), dir: dir, name: name})
		}
	}
	return out
}

func evalCand(c htfCand, htf, ltf []indicators.Candle, cycle TimeCycle, htfK, ltfK string) outcome {
	n, r := len(htf), c.res
	if r.Status != C2Active {
		return outcome{rank: rankC2Running, status: StatusC2Running,
			note: "Candle HTF berjalan menyentuh acuan, C2 belum final sampai candle close."}
	}
	c2 := r.C2Index
	if c2+1 >= n {
		return outcome{rank: rankC3NotStarted, status: StatusWaitC2, note: "C2 aktif, C3 belum dimulai."}
	}
	c3 := htf[c2+1]
	o := outcome{c3Start: c3.Timestamp, c3End: candleEnd(c3.Timestamp, htfK)}
	if !ValidateC2AndC3InCycle(cycle, htfK, htf[c2].Timestamp, c3.Timestamp) {
		o.rank, o.status, o.note = rankOffCycleOrStale, StatusWaitC2, fmt.Sprintf("C2/C3 di luar cycle %s, tidak berlaku.", cycle)
		return o
	}
	if c2+1 < n-2 {
		o.rank, o.status, o.note = rankOffCycleOrStale, StatusWaitC2, "C3 sudah lewat (C4 dst.), menunggu C2 baru."
		return o
	}
	cisd := DetectCISD(ltf, c.dir, o.c3Start, o.c3End)
	if cisd != nil && cisd.Detected && !validPlan(cisd, c.dir) {
		cisd.Detected = false
	}
	o.cisd = cisd
	detected := cisd != nil && cisd.Detected
	if c2+1 == n-1 && !c3.IsClosed { // C3 sedang berjalan
		if detected {
			if isLatestClosedBar(cisd.BreakTime, ltf) {
				o.rank, o.status = rankLiveReady, StatusReady
			} else {
				// Setup sudah terkonfirmasi pada candle LTF sebelumnya di C3; dibiarkan berjalan, tidak diterbitkan ulang.
				o.rank, o.status, o.note = rankC3Closed, StatusWaitC2, "Setup di C3 sudah terkonfirmasi sebelumnya; tidak diterbitkan ulang."
			}
		} else {
			o.rank, o.status = rankLiveWaiting, StatusWaitCISD
		}
		return o
	}
	// CISD yang terkonfirmasi tepat pada candle LTF penutup C3 (open..close C3) tetap sah
	// sesaat ketika C3 baru selesai ditutup (live maupun backtest).
	if detected && isClosingBarOfC3(cisd.BreakTime, ltf, ltfK, o.c3End) {
		o.rank, o.status = rankLiveReady, StatusReady
		return o
	}
	o.rank = rankC3Closed
	if detected {
		// Setup sudah terjadi di C3 dan dibiarkan berjalan; tidak diterbitkan ulang.
		o.status, o.note = StatusWaitC2, "C3 sudah close; setup yang terkonfirmasi di C3 tidak diterbitkan ulang."
	} else {
		o.status = StatusInvalid
	}
	return o
}

func isLatestClosedBar(breakTime time.Time, ltf []indicators.Candle) bool {
	for i := len(ltf) - 1; i >= 0; i-- {
		if ltf[i].IsClosed {
			return ltf[i].Timestamp.Equal(breakTime)
		}
	}
	return false
}

func isClosingBarOfC3(breakTime time.Time, ltf []indicators.Candle, ltfK string, c3End time.Time) bool {
	if candleEnd(breakTime, ltfK).Before(c3End) {
		return false
	}
	return isLatestClosedBar(breakTime, ltf)
}

func validPlan(c *CISD, dir string) bool {
	if dir == DirBearish {
		return c.ExtremeSL > c.EntryPrice
	}
	return c.ExtremeSL < c.EntryPrice
}

func buildSweep(c htfCand, o outcome, htf []indicators.Candle, cycle TimeCycle) *HTFSweep {
	h := &HTFSweep{Direction: c.dir, KeyLevelKind: c.kind, FVG: c.fvg, Swing: c.sw, Cycle: string(cycle),
		SweepCandleIndex: -1, C3Start: o.c3Start, C3End: o.c3End,
		C2Status: c.res.Status, C2Detail: c.res.Detail}
	if c.kind == KeyLevelIRL {
		h.SweptCandle = htf[c.fvg.Candle3Index]
	} else {
		h.SweptCandle = htf[c.sw.MidIndex]
	}
	if c.res.C2Index >= 0 {
		c2 := htf[c.res.C2Index]
		h.SweepCandle, h.SweepCandleIndex, h.SweepTime = c2, c.res.C2Index, c2.Timestamp
		h.StopLoss = extremeOf(c2, c.dir) // ekstrem C2: high (bearish) / low (bullish)
	}
	return h
}

// planPosition mengisi Entry, SL, dan Target RR 1:2 (alur-entry.md §5).
func planPosition(s *ILASetup) {
	c := s.CISD
	s.EntryPrice, s.StopLoss = c.EntryPrice, c.ExtremeSL
	risk := math.Abs(s.EntryPrice - s.StopLoss)
	tp := s.EntryPrice + 2*risk
	if s.Bias == DirBearish {
		tp = s.EntryPrice - 2*risk
	}
	digits := 2
	switch {
	case s.EntryPrice < 50:
		digits = 5
	case s.EntryPrice < 500:
		digits = 3
	}
	pow := math.Pow10(digits)
	s.TakeProfit = math.Round(tp*pow) / pow
	s.RiskReward, s.PassedMinRR = 2, true
}
