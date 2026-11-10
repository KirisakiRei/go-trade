package grpcserver

import (
	"testing"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/signal"
)

// TestStatusOfILA3 memastikan setiap status ILA 3.0 sampai ke kontrak sebagai
// enum yang benar. Sebelumnya semua status ILA 3.0 menjadi
// SIGNAL_STATUS_UNSPECIFIED dan dibuang oleh Runner.
func TestStatusOfILA3(t *testing.T) {
	cases := map[signal.Status]enginev1.SignalStatus{
		signal.WaitILA3HTFKeyLevel:  enginev1.SignalStatus_WAIT_ILA3_HTF_KEY_LEVEL,
		signal.WaitILA3MTFReversal:  enginev1.SignalStatus_WAIT_ILA3_MTF_REVERSAL,
		signal.WaitILA3LTFCISD1:     enginev1.SignalStatus_WAIT_ILA3_LTF_CISD_1,
		signal.ILA3Expansion1Active: enginev1.SignalStatus_ILA3_EXPANSION_1_ACTIVE,
		signal.WaitILA3MTFFVGRetr:   enginev1.SignalStatus_WAIT_ILA3_MTF_FVG_RETR,
		signal.WaitILA3LTFCISD2:     enginev1.SignalStatus_WAIT_ILA3_LTF_CISD_2,
		signal.ILA3Expansion2Active: enginev1.SignalStatus_ILA3_EXPANSION_2_ACTIVE,
		signal.ILA3TargetReached:    enginev1.SignalStatus_ILA3_TARGET_REACHED,
	}

	for in, want := range cases {
		if got := statusOf(in); got != want {
			t.Errorf("statusOf(%q) = %v, ingin %v", in, got, want)
		}
		if got := statusOf(in); got == enginev1.SignalStatus_SIGNAL_STATUS_UNSPECIFIED {
			t.Errorf("statusOf(%q) tidak terpetakan", in)
		}
	}
}

// TestStatusOfILA2Tetap memastikan kontrak lama tidak berubah.
func TestStatusOfILA2Tetap(t *testing.T) {
	cases := map[signal.Status]enginev1.SignalStatus{
		signal.Ready:              enginev1.SignalStatus_READY_TO_ENTRY,
		signal.WaitHTFSweep:       enginev1.SignalStatus_WAIT_HTF_SWEEP,
		signal.WaitHTFCandleClose: enginev1.SignalStatus_WAIT_HTF_CANDLE_CLOSE,
		signal.WaitLTFCISD:        enginev1.SignalStatus_WAIT_LTF_CISD,
		signal.Invalidated:        enginev1.SignalStatus_INVALIDATED,
	}

	for in, want := range cases {
		if got := statusOf(in); got != want {
			t.Errorf("statusOf(%q) = %v, ingin %v", in, got, want)
		}
	}
}

func TestStatusOfTakDikenal(t *testing.T) {
	if got := statusOf(signal.Status("SESUATU")); got != enginev1.SignalStatus_SIGNAL_STATUS_UNSPECIFIED {
		t.Errorf("statusOf tak dikenal = %v, ingin UNSPECIFIED", got)
	}
}
