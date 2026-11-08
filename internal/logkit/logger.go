// Package logkit menulis log JSONL ke berkas harian di dalam struktur
// logs/YYYY/MM/DD/ lalu merawat retensinya sendiri.
//
// Satu baris = satu JSON utuh dengan kunci pendek dan tetap, sehingga mudah
// dibaca manusia sekaligus bisa di-query dengan jq.
package logkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Kunci JSON. Pendek dan tetap agar baris log ringkas dan konsisten.
const (
	KunciWaktu = "t"
	KunciLevel = "lvl"
	KunciLayan = "svc"
	KunciTrace = "trace"
	KunciEvent = "evt"
	KunciPesan = "msg"
)

// Opsi mengatur perilaku penulis berkas log.
type Opsi struct {
	// Dir adalah akar direktori log, misal "/opt/data/engine/logs".
	Dir string
	// Layanan adalah nama yang mengisi kunci "svc", misal "engine".
	Layanan string
	// Level minimum yang ditulis.
	Level slog.Level
	// JugaStdout menggandakan keluaran ke stdout (dipakai Docker).
	JugaStdout bool
	// HariSimpan adalah usia berkas harian sebelum dihapus.
	HariSimpan int
	// BatasBytes adalah batas total direktori log. Bila terlampaui, hari
	// terlama dihapus lebih dulu meski belum mencapai HariSimpan.
	BatasBytes int64
	// Lokasi waktu untuk menentukan folder tanggal.
	Lokasi *time.Location
	// Jam adalah sumber waktu, bisa diganti saat pengujian.
	Jam func() time.Time
	// Salin, bila diisi, menerima setiap baris yang juga ditulis ke berkas.
	// Dipakai untuk mengirim log ke platform tanpa mengubah cara pemakaian.
	Salin func(waktu time.Time, baris []byte)
}

// tulisBerkas adalah io.Writer yang menulis ke berkas harian dan berganti
// berkas sendiri saat tanggal berganti.
type tulisBerkas struct {
	mu     sync.Mutex
	opts   Opsi
	hari   string
	berkas *os.File
}

func (w *tulisBerkas) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.pastikanBerkas(); err != nil {
		return 0, err
	}
	return w.berkas.Write(p)
}

// pastikanBerkas membuka berkas hari ini, menggantinya bila tanggal berubah.
func (w *tulisBerkas) pastikanBerkas() error {
	sekarang := w.opts.Jam().In(w.opts.Lokasi)
	hari := sekarang.Format("2006/01/02")
	if w.berkas != nil && w.hari == hari {
		return nil
	}

	if w.berkas != nil {
		_ = w.berkas.Close()
		w.berkas = nil
		w.hari = ""
	}

	dir := filepath.Join(w.opts.Dir, filepath.FromSlash(hari))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("buat folder log %s: %w", dir, err)
	}

	nama := fmt.Sprintf("%s-%s.log", w.opts.Layanan, sekarang.Format("2006-01-02"))
	f, err := os.OpenFile(filepath.Join(dir, nama), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("buka berkas log: %w", err)
	}

	w.berkas = f
	w.hari = hari
	return nil
}

// Close menutup berkas yang sedang terbuka.
func (w *tulisBerkas) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.berkas == nil {
		return nil
	}
	err := w.berkas.Close()
	w.berkas = nil
	return err
}

// Handler membungkus slog.Handler agar setiap baris membawa "svc" tetap dan
// waktu UTC berpresisi milidetik.
type handler struct {
	mu      sync.Mutex
	out     io.Writer
	level   slog.Level
	layanan string
	attrs   []slog.Attr
	groups  []string
	salin   func(time.Time, []byte)
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	baris := make(map[string]any, 8+r.NumAttrs())
	baris[KunciWaktu] = r.Time.UTC().Format("2006-01-02T15:04:05.000Z")
	baris[KunciLevel] = r.Level.String()
	baris[KunciLayan] = h.layanan
	baris[KunciPesan] = r.Message

	for _, a := range h.attrs {
		sisipkan(baris, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		sisipkan(baris, a)
		return true
	})

	data, err := json.Marshal(baris)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	h.mu.Lock()
	_, err = h.out.Write(data)
	h.mu.Unlock()

	// Salinan dikirim setelah berkas ditulis: berkas lokal adalah sumber utama.
	if h.salin != nil {
		h.salin(r.Time, data)
	}
	return err
}

// sisipkan menaruh satu attr ke peta; grup ditulis sebagai "grup.kunci".
func sisipkan(baris map[string]any, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	kunci := a.Key
	if kunci == "" {
		kunci = KunciEvent
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, sub := range a.Value.Group() {
			sisipkan(baris, slog.Attr{Key: kunci + "." + sub.Key, Value: sub.Value})
		}
		return
	}
	baris[kunci] = a.Value.Any()
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &handler{
		out:     h.out,
		level:   h.level,
		layanan: h.layanan,
		attrs:   append(append([]slog.Attr{}, h.attrs...), as...),
		groups:  h.groups,
		salin:   h.salin,
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{
		out:     h.out,
		level:   h.level,
		layanan: h.layanan,
		attrs:   h.attrs,
		groups:  append(append([]string{}, h.groups...), name),
		salin:   h.salin,
	}
}

// Trace membuat pengenal pendek untuk merangkai satu alur lintas layanan.
func Trace() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
