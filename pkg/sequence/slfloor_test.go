package sequence

import "testing"

// SL floor harus mengikuti desimal instrumen, bukan satu angka untuk semua Forex.
func TestMinSLFloorFor(t *testing.T) {
	cases := []struct {
		symbol string
		want   float64
	}{
		{"EURUSD", 0.00150},
		{"GBPUSD", 0.00150},
		{"XAUUSD", 4.00},
		{"GOLD", 4.00},
		{"USTEC", 15.0},
		{"NAS100", 15.0},
		{"USDJPY", 0.150},
		{"GBPJPY", 0.150},
	}

	for _, c := range cases {
		if got := minSLFloorFor(c.symbol); got != c.want {
			t.Errorf("minSLFloorFor(%s) = %v, want %v", c.symbol, got, c.want)
		}
	}
}
