// Package engine menyatukan pelanggan (Runner) dengan evaluator ILA.
//
// Engine tidak mengenal user, akun, atau lot. Ia hanya tahu simbol apa yang
// diminta pelanggan, lalu menjalankan evaluator yang sudah ada dan menerbitkan
// hasilnya. Seluruh keputusan uang ada di Runner.
package engine

import (
	"sort"
	"sync"
)

// Subscriber adalah satu Runner yang terhubung.
type Subscriber struct {
	ID         string
	Symbols    map[string]struct{}
	WantStatus bool
	// Pairs adalah pasangan timeframe yang ingin dihitung pelanggan ini.
	//
	// Nil atau kosong berarti "semua pasangan" — perilaku lama dipertahankan
	// agar klien yang belum mengirim pairs tidak berubah perilakunya.
	Pairs []Pair
	// Send dipanggil untuk setiap sinyal. Harus tidak memblokir lama.
	Send func(Signal)
}

// Registry menyimpan pelanggan dan simbol yang mereka minta.
//
// Simbol yang diminta digabung: bila dua Runner meminta XAUUSD, engine hanya
// mengevaluasi XAUUSD sekali lalu menyiarkannya ke keduanya.
type Registry struct {
	mu   sync.RWMutex
	subs map[string]*Subscriber
}

// NewRegistry membuat registry kosong.
func NewRegistry() *Registry {
	return &Registry{subs: make(map[string]*Subscriber)}
}

// Add mendaftarkan pelanggan. Pelanggan dengan ID sama akan diganti (reconnect).
func (r *Registry) Add(s Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Symbols == nil {
		s.Symbols = make(map[string]struct{})
	}
	copy := s
	r.subs[s.ID] = &copy
}

// SetPairs mengganti pasangan timeframe satu pelanggan.
//
// Daftar kosong berarti "semua pasangan" dan bukan "tidak ada", supaya Runner
// yang belum mengatur saklar timeframe tetap menerima sinyal seperti sebelum
// fitur ini ada.
func (r *Registry) SetPairs(id string, pairs []Pair) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[id]
	if !ok {
		return false
	}
	s.Pairs = normalizePairs(pairs)
	return true
}

// WantedPairs mengembalikan pasangan timeframe yang perlu dihitung untuk satu
// simbol, hasil gabungan seluruh pelanggan yang memantau simbol itu.
//
// Hasil `false` berarti "tidak ada yang membatasi": engine memakai seluruh
// pasangan evaluator. Ini terjadi bila belum ada pelanggan yang mengirim pairs,
// yang memang perilaku awal engine.
func (r *Registry) WantedPairs(symbol string) ([]Pair, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adaPermintaan := false
	seen := make(map[Pair]struct{})
	for _, s := range r.subs {
		if _, ok := s.Symbols[symbol]; !ok {
			continue
		}
		if len(s.Pairs) == 0 {
			// Satu pelanggan saja yang meminta semua pasangan membuat
			// pembatasan tidak berlaku: sinyalnya harus tetap dihitung.
			return nil, false
		}
		adaPermintaan = true
		for _, p := range s.Pairs {
			seen[p] = struct{}{}
		}
	}
	if !adaPermintaan {
		return nil, false
	}

	out := make([]Pair, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	// Urutan stabil: evaluasi yang sama tidak boleh menghasilkan susunan
	// berbeda tiap menit, agar log dan perilaku mudah dibaca.
	sort.Slice(out, func(i, j int) bool {
		if out[i].HTF == out[j].HTF {
			return out[i].LTF < out[j].LTF
		}
		return out[i].HTF < out[j].HTF
	})
	return out, true
}

// normalizePairs membuang pasangan yang tidak lengkap dan duplikat.
func normalizePairs(pairs []Pair) []Pair {
	if len(pairs) == 0 {
		return nil
	}
	seen := make(map[Pair]struct{}, len(pairs))
	out := make([]Pair, 0, len(pairs))
	for _, p := range pairs {
		// HTF dan LTF wajib terisi: pasangan separuh hanya akan gagal
		// dievaluasi berulang tanpa hasil.
		if p.HTF == "" || p.LTF == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Remove mencabut pelanggan.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subs, id)
}

// SetSymbols mengganti daftar simbol satu pelanggan.
func (r *Registry) SetSymbols(id string, symbols []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[id]
	if !ok {
		return false
	}
	s.Symbols = toSet(symbols)
	return true
}

// AddSymbols menambah simbol tanpa menghapus yang lama.
func (r *Registry) AddSymbols(id string, symbols []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[id]
	if !ok {
		return false
	}
	if s.Symbols == nil {
		s.Symbols = make(map[string]struct{})
	}
	for _, sym := range symbols {
		s.Symbols[sym] = struct{}{}
	}
	return true
}

// RemoveSymbols mencabut sebagian simbol.
func (r *Registry) RemoveSymbols(id string, symbols []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[id]
	if !ok {
		return false
	}
	for _, sym := range symbols {
		delete(s.Symbols, sym)
	}
	return true
}

// Symbols mengembalikan seluruh simbol yang diminta, terurut dan tanpa duplikat.
// Inilah daftar kerja engine: tidak ada simbol di sini berarti tidak ada scan.
func (r *Registry) Symbols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, s := range r.subs {
		for sym := range s.Symbols {
			seen[sym] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for sym := range seen {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

// Broadcast mengirim sinyal ke pelanggan yang memantau simbol tersebut.
func (r *Registry) Broadcast(sig Signal) int {
	r.mu.RLock()
	targets := make([]*Subscriber, 0, len(r.subs))
	for _, s := range r.subs {
		if _, ok := s.Symbols[sig.Symbol]; ok {
			targets = append(targets, s)
		}
	}
	r.mu.RUnlock()

	for _, s := range targets {
		if s.Send != nil {
			s.Send(sig)
		}
	}
	return len(targets)
}

// WantsStatus menyatakan apakah ada pelanggan yang ingin menerima status
// menunggu untuk simbol ini. Bila tidak ada, engine tidak mengirim status
// agar tidak membanjiri jaringan dengan data yang tidak dipakai.
func (r *Registry) WantsStatus(symbol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.subs {
		if !s.WantStatus {
			continue
		}
		if _, ok := s.Symbols[symbol]; ok {
			return true
		}
	}
	return false
}

// Count mengembalikan jumlah pelanggan terhubung.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subs)
}

// HasSymbol menyatakan apakah ada pelanggan yang memantau simbol ini.
func (r *Registry) HasSymbol(symbol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.subs {
		if _, ok := s.Symbols[symbol]; ok {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it != "" {
			out[it] = struct{}{}
		}
	}
	return out
}
