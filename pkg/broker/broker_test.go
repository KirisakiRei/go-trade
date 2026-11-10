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

func TestExtractTicketIDs(t *testing.T) {
	// 1. Direct Match-Trader ticket with '#'
	cID, mID := ExtractTicketIDs("#W9945097837248137")
	if mID != "W9945097837248137" {
		t.Errorf("expected mtrID=W9945097837248137, got %s", mID)
	}

	// 2. Direct Match-Trader ticket without '#'
	_, mID2 := ExtractTicketIDs("W9945097837248137")
	if mID2 != "W9945097837248137" {
		t.Errorf("expected mtrID=W9945097837248137, got %s", mID2)
	}

	// 3. Composite ticket
	composite := "Match-Trader (Trader API REST)(#W525654572928872, 0.09 lot) & IC Markets cTrader (Native Go)(#693069935, 0.94 lot)"
	cID3, mID3 := ExtractTicketIDs(composite)
	if cID3 != 693069935 {
		t.Errorf("expected cPosID=693069935, got %d", cID3)
	}
	if mID3 != "W525654572928872" {
		t.Errorf("expected mtrID=W525654572928872, got %s", mID3)
	}

	_ = cID
}
