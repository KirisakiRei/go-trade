package broker

import (
	"context"
	"testing"
)

func TestFallbackSymbolSpec(t *testing.T) {
	// XAUUSD
	xau := FallbackSymbolSpec("XAUUSD")
	if xau.PointValue != 100.0 || xau.VolumeStep != 0.01 {
		t.Errorf("expected XAUUSD PointValue=100.0, VolumeStep=0.01, got %+v", xau)
	}

	// EURUSD
	eur := FallbackSymbolSpec("EURUSD")
	if eur.PointValue != 100000.0 || eur.PricePrecision != 5 {
		t.Errorf("expected EURUSD PointValue=100000.0, got %+v", eur)
	}

	// US100
	us100 := FallbackSymbolSpec("US100")
	if us100.PointValue != 20.0 || us100.VolumeStep != 0.01 {
		t.Errorf("expected US100 PointValue=20.0, VolumeStep=0.01, got %+v", us100)
	}
}

func TestCTraderGetSymbolSpec(t *testing.T) {
	cb := &CTraderBroker{}
	ctx := context.Background()

	spec, err := cb.GetSymbolSpec(ctx, "USTEC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.PointValue != 1.0 || spec.VolumeStep != 0.10 || spec.VolumeMin != 0.10 {
		t.Errorf("expected USTEC PointValue=1.0, VolumeStep=0.10, got %+v", spec)
	}

	xauSpec, err := cb.GetSymbolSpec(ctx, "XAUUSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if xauSpec.PointValue != 100.0 || xauSpec.VolumeStep != 0.01 {
		t.Errorf("expected XAUUSD PointValue=100.0, VolumeStep=0.01, got %+v", xauSpec)
	}
}
