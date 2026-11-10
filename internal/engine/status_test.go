package engine

import (
	"testing"

	"ila-engine/internal/signal"
)

// TestMapILA3StatusMempertahankanKemajuan memastikan status menunggu ILA 3.0
// tidak lagi dibuang. Sebelum perbaikan ini semua WAIT_* menjadi "" sehingga
// Runner tidak pernah menerima kemajuan radar ILA 3.0.
func TestMapILA3StatusMempertahankanKemajuan(t *testing.T) {
	cases := map[string]signal.Status{
		"WAIT_HTF_KEY_LEVEL":   signal.WaitILA3HTFKeyLevel,
		"WAIT_MTF_REVERSAL":    signal.WaitILA3MTFReversal,
		"WAIT_LTF_CISD_1":      signal.WaitILA3LTFCISD1,
		"EXPANSION_1_ACTIVE":   signal.ILA3Expansion1Active,
		"WAIT_MTF_FVG_RETRACE": signal.WaitILA3MTFFVGRetr,
		"WAIT_LTF_CISD_2":      signal.WaitILA3LTFCISD2,
		"EXPANSION_2_ACTIVE":   signal.ILA3Expansion2Active,
		"TARGET_REACHED":       signal.ILA3TargetReached,
	}

	for internal, want := range cases {
		got := mapStatus("ila-3.0", internal)
		if got != want {
			t.Errorf("mapStatus(ila-3.0, %q) = %q, ingin %q", internal, got, want)
		}
		if got == "" {
			t.Errorf("status %q dibuang; kemajuan ILA 3.0 tidak akan terlihat", internal)
		}
	}
}

// TestMapILA3EntryTetapReady memastikan kedua titik entry tetap boleh
// dieksekusi lewat status Ready. Status tampilan tidak boleh mengubah izin
// eksekusi.
func TestMapILA3EntryTetapReady(t *testing.T) {
	for _, internal := range []string{"READY_ENTRY_1", "READY_ENTRY_2"} {
		if got := mapStatus("ila-3.0", internal); got != signal.Ready {
			t.Errorf("mapStatus(ila-3.0, %q) = %q, ingin %q", internal, got, signal.Ready)
		}
	}
}

// TestMapILA3MengabaikanStatusTakDikenal menjaga perilaku lama: status yang
// tidak dikenal tidak boleh diterbitkan sebagai sinyal.
func TestMapILA3MengabaikanStatusTakDikenal(t *testing.T) {
	for _, internal := range []string{"", "EXPIRED", "SESUATU_BARU"} {
		if got := mapStatus("ila-3.0", internal); got != "" {
			t.Errorf("mapStatus(ila-3.0, %q) = %q, ingin kosong", internal, got)
		}
	}
}

func TestMenungguLTF(t *testing.T) {
	ya := []signal.Status{
		signal.WaitLTFCISD,
		signal.WaitILA3LTFCISD1,
		signal.WaitILA3LTFCISD2,
	}
	for _, s := range ya {
		if !menungguLTF(s) {
			t.Errorf("menungguLTF(%q) = false, ingin true", s)
		}
	}

	tidak := []signal.Status{
		signal.WaitHTFSweep,
		signal.WaitILA3HTFKeyLevel,
		signal.Ready,
	}
	for _, s := range tidak {
		if menungguLTF(s) {
			t.Errorf("menungguLTF(%q) = true, ingin false", s)
		}
	}
}
