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
	default:
		// State menunggu: tidak diterbitkan sebagai sinyal utuh.
		return ""
	}
}
