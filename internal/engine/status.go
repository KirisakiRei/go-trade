package engine

import "ila-engine/internal/signal"

// mapStatus menerjemahkan status internal evaluator menjadi status sinyal.
//
// ILA 2.0 sudah memakai nama yang sama dengan kontrak. ILA 3.0 memakai state
// sequence yang berbeda nama, jadi dipetakan di sini — bukan di pkg/sequence,
// karena paket itu adalah aturan trading dan tidak boleh disentuh.
func mapStatus(strategy, internal string) signal.Status {
	switch strategy {
	case "ila-3.0":
		return mapILA3Status(internal)
	default:
		return mapILA2Status(internal)
	}
}

func mapILA2Status(s string) signal.Status {
	switch s {
	case "READY_TO_ENTRY":
		return signal.Ready
	case "WAIT_HTF_SWEEP":
		return signal.WaitHTFSweep
	case "WAIT_HTF_CANDLE_CLOSE":
		return signal.WaitHTFCandleClose
	case "WAIT_LTF_CISD":
		return signal.WaitLTFCISD
	case "INVALIDATED":
		return signal.Invalidated
	default:
		return ""
	}
}

func mapILA3Status(s string) signal.Status {
	switch s {
	// Dua titik entry ILA 3.0 sama-sama boleh dieksekusi.
	case "READY_ENTRY_1", "READY_ENTRY_2":
		return signal.Ready
	case "INVALIDATED":
		return signal.Invalidated
	// State menunggu dan perluasan diteruskan sebagai status tampilan.
	// Sebelumnya semuanya dibuang (""), sehingga Radar tidak pernah
	// menunjukkan kemajuan ILA 3.0 sama sekali.
	case "WAIT_HTF_KEY_LEVEL":
		return signal.WaitILA3HTFKeyLevel
	case "WAIT_MTF_REVERSAL":
		return signal.WaitILA3MTFReversal
	case "WAIT_LTF_CISD_1":
		return signal.WaitILA3LTFCISD1
	case "EXPANSION_1_ACTIVE":
		return signal.ILA3Expansion1Active
	case "WAIT_MTF_FVG_RETRACE":
		return signal.WaitILA3MTFFVGRetr
	case "WAIT_LTF_CISD_2":
		return signal.WaitILA3LTFCISD2
	case "EXPANSION_2_ACTIVE":
		return signal.ILA3Expansion2Active
	case "TARGET_REACHED":
		return signal.ILA3TargetReached
	default:
		// EXPIRED dan state tak dikenal tidak diterbitkan sebagai sinyal.
		return ""
	}
}

// menungguLTF menandai status yang hanya bisa berubah setelah candle LTF
// ditutup. Dipakai penjadwal evaluasi agar tidak memindai lebih sering dari
// yang diperlukan.
func menungguLTF(s signal.Status) bool {
	switch s {
	case signal.WaitHTFCandleClose, signal.WaitLTFCISD,
		signal.WaitILA3MTFReversal, signal.WaitILA3LTFCISD1,
		signal.WaitILA3MTFFVGRetr, signal.WaitILA3LTFCISD2:
		return true
	default:
		return false
	}
}
