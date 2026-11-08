package signal

import (
	"testing"
	"time"
)

func readySignal(id string, published time.Time) Signal {
	return Signal{
		ID: id, Symbol: "XAUUSD", HTF: "H4", LTF: "M15",
		Status: Ready, Direction: "BUY",
		Entry: 4120.5, StopLoss: 4112.3, TakeProfit: 4149.2, RiskReward: 3.5,
		PublishedAt: published, ExpiresAt: published.Add(TTLUntuk("M15")),
	}
}

func TestTTLPerTimeframe(t *testing.T) {
	cases := map[string]time.Duration{
		"H4":  4 * time.Hour,
		"H1":  time.Hour,
		"M15": 15 * time.Minute,
		"M5":  5 * time.Minute,
	}
	for ltf, want := range cases {
		if got := TTLUntuk(ltf); got != want {
			t.Errorf("TTLUntuk(%q) = %v, ingin %v", ltf, got, want)
		}
	}
	if got := TTLUntuk("M1"); got != 15*time.Minute {
		t.Fatalf("LTF tak dikenal harus aman 15 menit, dapat %v", got)
	}
}

func TestPublishTanpaSinyalSebelumnya(t *testing.T) {
	var s Slot
	now := time.Now()
	action, prev := s.Publish(readySignal("sig-1", now))
	if action != ActionPublish || prev != "" {
		t.Fatalf("action=%q prev=%q, ingin PUBLISH tanpa pendahulu", action, prev)
	}
	if !s.Ready(now) {
		t.Fatal("sinyal baru seharusnya siap dieksekusi")
	}
}

// Sinyal baru pada slot sama harus menggantikan yang lama, bukan menumpuk.
func TestPublishMenggantikanSinyalLama(t *testing.T) {
	var s Slot
	now := time.Now()
	s.Publish(readySignal("sig-1", now))

	action, prev := s.Publish(readySignal("sig-2", now))
	if action != ActionSupersede || prev != "sig-1" {
		t.Fatalf("action=%q prev=%q, ingin SUPERSEDE terhadap sig-1", action, prev)
	}
	if s.Active().ID != "sig-2" {
		t.Fatal("sinyal aktif seharusnya sig-2")
	}
}

func TestExpireSetelahTTL(t *testing.T) {
	var s Slot
	now := time.Now()
	s.Publish(readySignal("sig-1", now))

	if _, ok := s.Expire(now.Add(time.Minute)); ok {
		t.Fatal("sinyal belum boleh kedaluwarsa dalam TTL")
	}
	id, ok := s.Expire(now.Add(16 * time.Minute))
	if !ok || id != "sig-1" {
		t.Fatalf("sinyal harus kedaluwarsa, dapat id=%q ok=%v", id, ok)
	}
	if s.Active() != nil {
		t.Fatal("slot harus kosong setelah kedaluwarsa")
	}
}

// Sinyal yang kedaluwarsa tidak boleh dieksekusi walau tidak ditarik eksplisit.
func TestReadyPalsuSetelahTTL(t *testing.T) {
	var s Slot
	now := time.Now()
	s.Publish(readySignal("sig-1", now))
	if s.Ready(now.Add(20 * time.Minute)) {
		t.Fatal("Ready harus false setelah TTL lewat")
	}
}

func TestInvalidate(t *testing.T) {
	var s Slot
	now := time.Now()
	s.Publish(readySignal("sig-1", now))

	id, ok := s.Invalidate()
	if !ok || id != "sig-1" {
		t.Fatalf("Invalidate harus mengembalikan sig-1, dapat id=%q ok=%v", id, ok)
	}
	if s.Active() != nil {
		t.Fatal("slot harus kosong setelah invalidasi")
	}
}

// Status wait bukan sinyal: tidak boleh ditarik maupun dieksekusi.
func TestSinyalWaitTidakDieksekusi(t *testing.T) {
	var s Slot
	sig := readySignal("sig-w", time.Now())
	sig.Status = WaitLTFCISD
	s.Publish(sig)

	if s.Ready(time.Now()) {
		t.Fatal("sinyal WAIT_LTF_CISD tidak boleh siap dieksekusi")
	}
	if _, ok := s.Invalidate(); ok {
		t.Fatal("sinyal wait belum pernah terbit, tidak perlu ditarik")
	}
}
