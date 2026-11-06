package stdev

import (
	"testing"
)

func TestBullishProjection(t *testing.T) {
	proj := NewProjector()
	// Point 1 (Low) = 100, Point 0 (MSS) = 110 -> dist = 10
	res := proj.CalculateProjection(DirectionBullish, 100, 110)

	if res.LevelMinus1 != 120 {
		t.Errorf("expected Level -1 = 120, got %.2f", res.LevelMinus1)
	}
	if res.LevelMinus2 != 130 {
		t.Errorf("expected Level -2 = 130, got %.2f", res.LevelMinus2)
	}
	if res.LevelMinus2_5 != 135 {
		t.Errorf("expected Level -2.5 = 135, got %.2f", res.LevelMinus2_5)
	}
	if res.LevelMinus4 != 150 {
		t.Errorf("expected Level -4 = 150, got %.2f", res.LevelMinus4)
	}

	// Zone status test
	status := proj.CheckZoneStatus(res, 132)
	if !status.InCriticalZone {
		t.Errorf("expected 132 to be inside critical zone [-2, -2.5]")
	}
}

func TestBearishProjection(t *testing.T) {
	proj := NewProjector()
	// Point 1 (High) = 200, Point 0 (MSS) = 190 -> dist = 10
	res := proj.CalculateProjection(DirectionBearish, 200, 190)

	if res.LevelMinus1 != 180 {
		t.Errorf("expected Level -1 = 180, got %.2f", res.LevelMinus1)
	}
	if res.LevelMinus2 != 170 {
		t.Errorf("expected Level -2 = 170, got %.2f", res.LevelMinus2)
	}
	if res.LevelMinus2_5 != 165 {
		t.Errorf("expected Level -2.5 = 165, got %.2f", res.LevelMinus2_5)
	}
	if res.LevelMinus4 != 150 {
		t.Errorf("expected Level -4 = 150, got %.2f", res.LevelMinus4)
	}

	status := proj.CheckZoneStatus(res, 168)
	if !status.InCriticalZone {
		t.Errorf("expected 168 to be inside critical zone [-2, -2.5]")
	}
}
